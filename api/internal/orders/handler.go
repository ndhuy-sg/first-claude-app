// Package orders owns the floor plan (GET /tables) and the order lifecycle
// before payment: create, items, discount, buyer, move, merge, split, cancel.
//
// Every mutation runs in one transaction that first locks the order row
// (SELECT ... FOR UPDATE), so concurrent tablets never lose items (O-14).
// Responses use orderview.Load so the Order JSON matches openapi.yaml.
package orders

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

type handler struct{ d *platform.Deps }

// Register adds the orders and floor-plan routes.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	view := auth.RequireAction(domain.ActViewOrders)
	sell := auth.RequireAction(domain.ActSell)

	g.GET("/tables", view, h.listTables)
	g.GET("/orders", view, h.listOrders)
	g.POST("/orders", sell, h.createOrder)
	g.GET("/orders/:id", view, h.getOrder)
	g.POST("/orders/:id/items", sell, h.addItem)
	g.PATCH("/orders/:id/items/:itemId", sell, h.updateItem)
	g.DELETE("/orders/:id/items/:itemId", sell, h.deleteItem)
	g.POST("/orders/:id/discount", auth.RequireAction(domain.ActDiscount), h.setDiscount)
	g.PUT("/orders/:id/buyer", sell, h.setBuyer)
	g.POST("/orders/:id/move", sell, h.move)
	g.POST("/orders/:id/merge", sell, h.merge)
	g.POST("/orders/:id/split", sell, h.split)
	g.POST("/orders/:id/cancel", sell, h.cancel)
}

func (h *handler) now() time.Time { return h.d.Clock.Now() }

func (h *handler) load(ctx context.Context, id int64) (*orderview.Order, error) {
	return orderview.Load(ctx, store.New(h.d.Pool), h.d.Cfg.PublicBaseURL, id)
}

// publish sends order.updated for each order and table.updated for each table (after commit).
func (h *handler) publish(orders []*orderview.Order, tables ...*int64) {
	seen := map[int64]bool{}
	for _, o := range orders {
		h.d.Events.Publish(platform.Event{Type: "order.updated", BranchID: o.BranchID, Data: o})
		if o.TableID != nil {
			tables = append(tables, o.TableID)
		}
	}
	if len(orders) == 0 {
		return
	}
	for _, t := range tables {
		if t != nil && !seen[*t] {
			seen[*t] = true
			h.d.Events.Publish(platform.Event{Type: "table.updated", BranchID: orders[0].BranchID, Data: gin.H{"tableId": *t}})
		}
	}
}

// respond loads the order, publishes events and writes it with status.
func (h *handler) respond(c *gin.Context, status int, id int64, extraTables ...*int64) {
	o, err := h.load(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.publish([]*orderview.Order{o}, extraTables...)
	c.JSON(status, o)
}

// mutate runs fn on the locked, open order inside a transaction, recomputes
// totals and answers with the updated order.
func (h *handler) mutate(c *gin.Context, status int, fn func(ctx context.Context, q *store.Queries, o store.Order) error) {
	h.mutateTables(c, status, nil, fn)
}

// mutateTables is mutate that also announces table.updated for *leftTable
// (the table an order moved away from), when set by fn.
func (h *handler) mutateTables(c *gin.Context, status int, leftTable **int64, fn func(ctx context.Context, q *store.Queries, o store.Order) error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p := auth.From(c)
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOpenOrder(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := fn(ctx, q, o); err != nil {
			return err
		}
		return recompute(ctx, q, o.ID, o.DiscountType, o.DiscountValue, h.now())
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var extra *int64
	if leftTable != nil {
		extra = *leftTable
	}
	h.respond(c, status, id, extra)
}

// ---------- reads ----------

type openOrderSummary struct {
	ID        int64     `json:"id"`
	Code      string    `json:"code"`
	ItemCount int32     `json:"itemCount"`
	Total     int64     `json:"total"`
	CreatedAt time.Time `json:"createdAt"`
}

type tableJSON struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	AreaID    int64             `json:"areaId"`
	Status    string            `json:"status"`
	OpenOrder *openOrderSummary `json:"openOrder"`
}

type areaJSON struct {
	ID     int64       `json:"id"`
	Name   string      `json:"name"`
	Tables []tableJSON `json:"tables"`
}

func queryID(c *gin.Context, name string) (*int64, error) {
	s := c.Query(name)
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v <= 0 {
		return nil, validation(name + " không hợp lệ")
	}
	return &v, nil
}

func (h *handler) listTables(c *gin.Context) {
	ctx := c.Request.Context()
	requested, err := queryID(c, "branchId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	branch, err := auth.ResolveBranch(auth.From(c), requested, true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	q := store.New(h.d.Pool)
	areas, err := q.OrdFloorAreas(ctx, *branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	tables, err := q.OrdFloorTables(ctx, *branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	open, err := q.OrdFloorOpenOrders(ctx, *branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// The table's summary shows its main order (the first one not split off); a
	// pending QR payment on any open order of the table marks it awaiting payment.
	main := map[int64]*openOrderSummary{}
	pendingQR := map[int64]bool{}
	for _, o := range open {
		tid := *o.TableID
		if o.HasPendingQr {
			pendingQR[tid] = true
		}
		if cur, ok := main[tid]; !ok || (o.SplitFromOrderID == nil && cur == nil) {
			main[tid] = &openOrderSummary{ID: o.ID, Code: o.Code, ItemCount: o.ItemCount, Total: o.Total, CreatedAt: o.CreatedAt}
		}
	}
	out := make([]areaJSON, 0, len(areas))
	idx := map[int64]int{}
	for _, a := range areas {
		idx[a.ID] = len(out)
		out = append(out, areaJSON{ID: a.ID, Name: a.Name, Tables: []tableJSON{}})
	}
	for _, t := range tables {
		i, ok := idx[t.AreaID]
		if !ok {
			continue
		}
		tj := tableJSON{ID: t.ID, Name: t.Name, AreaID: t.AreaID, Status: "free", OpenOrder: main[t.ID]}
		if tj.OpenOrder != nil {
			tj.Status = "occupied"
			if pendingQR[t.ID] {
				tj.Status = "awaiting_payment"
			}
		}
		out[i].Tables = append(out[i].Tables, tj)
	}
	httpx.OK(c, out)
}

var orderStatuses = []string{"open", "paid", "cancelled", "merged", "refunded"}

func (h *handler) listOrders(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	requested, err := queryID(c, "branchId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	shiftID, err := queryID(c, "shiftId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var status *string
	if s := c.Query("status"); s != "" {
		if !slices.Contains(orderStatuses, s) {
			httpx.Fail(c, validation("status không hợp lệ"))
			return
		}
		status = &s
	}
	date := domain.BusinessDate(h.now())
	if s := c.Query("date"); s != "" {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			httpx.Fail(c, validation("date phải có dạng YYYY-MM-DD"))
			return
		}
		date = d
	}
	var search *string
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		// "128.000" typed on the POS searches the amount 128000.
		if digits := strings.NewReplacer(".", "", ",", "", "đ", "").Replace(s); digits != "" && strings.Trim(digits, "0123456789") == "" {
			if _, err := strconv.ParseInt(digits, 10, 64); err == nil && digits != s {
				s = digits
			}
		}
		search = &s
	}
	page := httpx.GetPage(c)
	q := store.New(h.d.Pool)
	total, err := q.OrdListCount(ctx, store.OrdListCountParams{BranchID: branch, BusinessDate: date, Status: status, ShiftID: shiftID, Q: search})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ids, err := q.OrdListIDs(ctx, store.OrdListIDsParams{BranchID: branch, BusinessDate: date, Status: status, ShiftID: shiftID, Q: search, Lim: page.Limit(), Off: page.Offset()})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items := make([]orderview.Order, 0, len(ids))
	for _, id := range ids {
		o, err := orderview.Load(ctx, q, h.d.Cfg.PublicBaseURL, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		items = append(items, *o)
	}
	httpx.OK(c, httpx.List[orderview.Order]{Items: items, Total: total})
}

func (h *handler) getOrder(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	o, err := h.load(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := auth.CheckBranch(auth.From(c), o.BranchID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, o)
}

// ---------- create ----------

type createReq struct {
	Type    string `json:"type" binding:"required,oneof=dine_in takeaway"`
	TableID *int64 `json:"tableId"`
	// Any branchId in the body is ignored: the branch comes from the device (A-12).
}

func (h *handler) createOrder(c *gin.Context) {
	var req createReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p := auth.From(c)
	dev := auth.DeviceFrom(c)
	if dev == nil {
		httpx.Fail(c, errNoOpenShift)
		return
	}
	if p.BranchID != nil && *p.BranchID != dev.BranchID {
		httpx.Fail(c, apperr.ErrForbidden)
		return
	}
	var tableID *int64
	if req.Type == "dine_in" {
		if req.TableID == nil {
			httpx.Fail(c, validation("Đơn tại bàn cần chọn bàn"))
			return
		}
		tableID = req.TableID
	}
	ctx := c.Request.Context()
	var id int64
	err := store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		shift, err := q.OrdOpenShiftForDevice(ctx, dev.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errNoOpenShift
		}
		if err != nil {
			return err
		}
		if tableID != nil {
			if err := lockFreeTable(ctx, q, dev.BranchID, *tableID); err != nil {
				return err
			}
		}
		id, err = newOrder(ctx, q, dev.BranchID, req.Type, tableID, shift.ID, p.UserID, nil, h.now())
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respond(c, 201, id)
}

// ---------- items ----------

type addItemReq struct {
	ProductID   int64   `json:"productId" binding:"required"`
	SizeCode    string  `json:"sizeCode"`
	ModifierIDs []int64 `json:"modifierIds"`
	Qty         int     `json:"qty" binding:"required,min=1,max=999"`
	Note        string  `json:"note"`
	// unitPrice from the client is deliberately not read (O-03).
}

func (h *handler) addItem(c *gin.Context) {
	var req addItemReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	p := auth.From(c)
	h.mutate(c, 201, func(ctx context.Context, q *store.Queries, o store.Order) error {
		prod, err := q.OrdGetProduct(ctx, req.ProductID)
		if err != nil {
			return err // ErrNoRows → 404
		}
		if !prod.IsActive {
			return apperr.ErrNotFound
		}
		soldOut, err := q.OrdIsSoldOut(ctx, store.OrdIsSoldOutParams{BranchID: o.BranchID, ProductID: prod.ID})
		if err != nil {
			return err
		}
		if soldOut {
			return errSoldOut
		}
		sizes, err := q.OrdProductSizes(ctx, prod.ID)
		if err != nil {
			return err
		}
		var sizeCode, sizeName string
		var sizeDelta int64
		size := strings.TrimSpace(req.SizeCode)
		if len(sizes) > 0 {
			i := slices.IndexFunc(sizes, func(s store.ProductSize) bool { return s.Code == size })
			if i < 0 {
				return errInvalidSize
			}
			sizeCode, sizeName, sizeDelta = sizes[i].Code, sizes[i].Name, sizes[i].PriceDelta
		} else if size != "" {
			return errInvalidSize
		}
		modIDs := slices.Compact(slices.Sorted(slices.Values(req.ModifierIDs)))
		var mods []store.Modifier
		if len(modIDs) > 0 {
			if mods, err = q.OrdModifiersByIDs(ctx, modIDs); err != nil {
				return err
			}
			if len(mods) != len(modIDs) {
				return errInvalidModifier
			}
			for _, m := range mods {
				if m.ProductID != prod.ID || !m.IsActive {
					return errInvalidModifier
				}
			}
		}
		prices := make([]int64, len(mods))
		for i, m := range mods {
			prices[i] = m.Price
		}
		unit := domain.UnitPrice(prod.BasePrice, sizeDelta, prices)
		itemID, err := q.OrdInsertItem(ctx, store.OrdInsertItemParams{
			OrderID: o.ID, ProductID: prod.ID, ProductName: prod.Name, SizeCode: sizeCode, SizeName: sizeName,
			UnitPrice: unit, Qty: int32(req.Qty), LineTotal: domain.LineTotal(unit, req.Qty), Note: strings.TrimSpace(req.Note),
			CreatedBy: p.UserID, CreatedAt: h.now(),
		})
		if err != nil {
			return err
		}
		for _, m := range mods {
			if err := q.OrdInsertItemModifier(ctx, store.OrdInsertItemModifierParams{OrderItemID: itemID, ModifierID: m.ID, Name: m.Name, Price: m.Price}); err != nil {
				return err
			}
		}
		return nil
	})
}

type updateItemReq struct {
	Qty  *int    `json:"qty"`
	Note *string `json:"note"`
}

func (h *handler) updateItem(c *gin.Context) {
	var req updateItemReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	if req.Qty != nil && (*req.Qty < 1 || *req.Qty > 999) {
		httpx.Fail(c, validation("Số lượng phải từ 1"))
		return
	}
	itemID, err := httpx.PathID(c, "itemId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.mutate(c, 200, func(ctx context.Context, q *store.Queries, o store.Order) error {
		it, err := q.OrdGetItem(ctx, store.OrdGetItemParams{ID: itemID, OrderID: o.ID})
		if err != nil {
			return err
		}
		qty, note := int(it.Qty), it.Note
		if req.Qty != nil {
			qty = *req.Qty
		}
		if req.Note != nil {
			note = strings.TrimSpace(*req.Note)
		}
		return q.OrdUpdateItem(ctx, store.OrdUpdateItemParams{ID: it.ID, Qty: int32(qty), Note: note, LineTotal: domain.LineTotal(it.UnitPrice, qty)})
	})
}

func (h *handler) deleteItem(c *gin.Context) {
	itemID, err := httpx.PathID(c, "itemId")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.mutate(c, 200, func(ctx context.Context, q *store.Queries, o store.Order) error {
		it, err := q.OrdGetItem(ctx, store.OrdGetItemParams{ID: itemID, OrderID: o.ID})
		if err != nil {
			return err
		}
		return q.OrdDeleteItem(ctx, it.ID)
	})
}

// ---------- discount ----------

type discountReq struct {
	Type       string `json:"type" binding:"required,oneof=percent amount"`
	Value      *int64 `json:"value" binding:"required"`
	Reason     string `json:"reason"`
	ManagerID  *int64 `json:"managerId"`
	ManagerPin string `json:"managerPin"`
}

type discountAudit struct {
	Type   *string `json:"type"`
	Value  int64   `json:"value"`
	Amount int64   `json:"amount"`
}

func (h *handler) setDiscount(c *gin.Context) {
	var req discountReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		httpx.Fail(c, validation("Vui lòng nhập lý do giảm giá"))
		return
	}
	value := *req.Value
	if value < 0 {
		httpx.Fail(c, validation("Giá trị giảm không hợp lệ"))
		return
	}
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOpenOrder(ctx, q, p, id)
		if err != nil {
			return err
		}
		sub, err := q.OrdItemsSubtotal(ctx, o.ID)
		if err != nil {
			return err
		}
		before := discountAudit{Type: o.DiscountType, Value: o.DiscountValue, Amount: o.DiscountAmount}
		params := store.OrdSetDiscountParams{ID: o.ID, DiscountBy: &p.UserID, UpdatedAt: h.now()}
		var approver *int64
		if value > 0 {
			kind := domain.DiscountKind(req.Type)
			amt, err := domain.ComputeDiscount(sub, kind, value)
			if err != nil {
				return validation("Giảm giá không hợp lệ hoặc lớn hơn tổng đơn")
			}
			settings, err := q.GetSettings(ctx)
			if err != nil {
				return err
			}
			maxPct := int(settings.DiscountMaxPct)
			needs := domain.DiscountNeedsApproval(sub, amt, maxPct)
			if kind == domain.DiscountPercent {
				needs = value > int64(maxPct)
			}
			if needs {
				switch {
				case domain.Can(p.Role, domain.ActApprove) != domain.ScopeNone:
					approver = &p.UserID // a manager (own branch, checked above) or the owner approves their own discount
				case req.ManagerID == nil || req.ManagerPin == "":
					return errApprovalRequired
				default:
					if err := checkApprover(ctx, q, o.BranchID, *req.ManagerID, req.ManagerPin); err != nil {
						return err
					}
					approver = req.ManagerID
				}
			}
			params.DiscountType, params.DiscountValue, params.DiscountReason, params.DiscountApprovedBy = &req.Type, value, reason, approver
		} else {
			params.DiscountBy = nil // value 0 removes the discount
		}
		if err := q.OrdSetDiscount(ctx, params); err != nil {
			return err
		}
		if err := recompute(ctx, q, o.ID, params.DiscountType, params.DiscountValue, h.now()); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, ApproverID: approver, BranchID: &o.BranchID,
			Action: "order.discount", EntityType: "order", EntityID: fmt.Sprint(o.ID),
			Before: before, After: discountAudit{Type: params.DiscountType, Value: params.DiscountValue, Amount: discountAmount(sub, params.DiscountType, params.DiscountValue)},
			Note: reason,
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respond(c, 200, id)
}

// ---------- buyer ----------

type buyerReq struct {
	Name    string `json:"name"`
	TaxCode string `json:"taxCode"`
	Address string `json:"address"`
	Email   string `json:"email"`
}

func (h *handler) setBuyer(c *gin.Context) {
	var req buyerReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	b := buyerReq{Name: strings.TrimSpace(req.Name), TaxCode: strings.TrimSpace(req.TaxCode), Address: strings.TrimSpace(req.Address), Email: strings.TrimSpace(req.Email)}
	if b.TaxCode != "" && !domain.ValidateTaxCode(b.TaxCode) {
		httpx.Fail(c, errInvalidTaxCode)
		return
	}
	if b.Email != "" {
		if a, err := mail.ParseAddress(b.Email); err != nil || a.Address != b.Email {
			httpx.Fail(c, validation("Email không hợp lệ"))
			return
		}
	}
	h.mutate(c, 200, func(ctx context.Context, q *store.Queries, o store.Order) error {
		return q.OrdSetBuyer(ctx, store.OrdSetBuyerParams{ID: o.ID, BuyerName: b.Name, BuyerTaxCode: b.TaxCode, BuyerAddress: b.Address, BuyerEmail: b.Email, UpdatedAt: h.now()})
	})
}

// ---------- move / merge / split / cancel ----------

type moveReq struct {
	TableID int64 `json:"tableId" binding:"required"`
}

func (h *handler) move(c *gin.Context) {
	var req moveReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	var from *int64
	h.mutateTables(c, 200, &from, func(ctx context.Context, q *store.Queries, o store.Order) error {
		from = o.TableID
		if o.TableID != nil && *o.TableID == req.TableID {
			return nil
		}
		if err := lockFreeTable(ctx, q, o.BranchID, req.TableID); err != nil {
			return err
		}
		return q.OrdSetTable(ctx, store.OrdSetTableParams{ID: o.ID, TableID: &req.TableID, UpdatedAt: h.now()})
	})
}

type mergeReq struct {
	TargetOrderID int64 `json:"targetOrderId" binding:"required"`
}

func (h *handler) merge(c *gin.Context) {
	var req mergeReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if req.TargetOrderID == id {
		httpx.Fail(c, validation("Không thể gộp đơn vào chính nó"))
		return
	}
	p := auth.From(c)
	ctx := c.Request.Context()
	var srcTable *int64
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		// Lock both orders in id order so two opposite merges cannot deadlock.
		locked := map[int64]store.Order{}
		for _, oid := range []int64{min(id, req.TargetOrderID), max(id, req.TargetOrderID)} {
			o, err := lockOrder(ctx, q, p, oid)
			if err != nil {
				return err
			}
			locked[oid] = o
		}
		src, dst := locked[id], locked[req.TargetOrderID]
		if src.Status != "open" || dst.Status != "open" {
			return errOrderNotOpen
		}
		if src.BranchID != dst.BranchID {
			return apperr.ErrNotFound
		}
		srcTable = src.TableID
		if err := q.OrdMoveAllItems(ctx, store.OrdMoveAllItemsParams{ToOrderID: dst.ID, FromOrderID: src.ID}); err != nil {
			return err
		}
		if err := q.OrdMarkMerged(ctx, store.OrdMarkMergedParams{ID: src.ID, MergedIntoOrderID: &dst.ID, UpdatedAt: h.now()}); err != nil {
			return err
		}
		return recompute(ctx, q, dst.ID, dst.DiscountType, dst.DiscountValue, h.now())
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	src, err := h.load(ctx, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	dst, err := h.load(ctx, req.TargetOrderID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.publish([]*orderview.Order{src, dst}, srcTable)
	httpx.OK(c, dst)
}

type splitReq struct {
	ItemIDs []int64 `json:"itemIds"`
}

func (h *handler) split(c *gin.Context) {
	var req splitReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	itemIDs := slices.Compact(slices.Sorted(slices.Values(req.ItemIDs)))
	if len(itemIDs) == 0 {
		httpx.Fail(c, validation("Chọn ít nhất một món để tách"))
		return
	}
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p := auth.From(c)
	ctx := c.Request.Context()
	var newID int64
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOpenOrder(ctx, q, p, id)
		if err != nil {
			return err
		}
		n, err := q.OrdCountItems(ctx, o.ID)
		if err != nil {
			return err
		}
		if len(itemIDs) >= int(n) {
			return validation("Phải giữ lại ít nhất một món ở đơn gốc")
		}
		// The new order keeps the original's table and shift; it carries no discount.
		newID, err = newOrder(ctx, q, o.BranchID, o.OrderType, o.TableID, o.ShiftID, p.UserID, &o.ID, h.now())
		if err != nil {
			return err
		}
		moved, err := q.OrdMoveItems(ctx, store.OrdMoveItemsParams{ToOrderID: newID, FromOrderID: o.ID, ItemIds: itemIDs})
		if err != nil {
			return err
		}
		if moved != int64(len(itemIDs)) {
			return validation("Có món không thuộc đơn này")
		}
		if err := recompute(ctx, q, newID, nil, 0, h.now()); err != nil {
			return err
		}
		return recompute(ctx, q, o.ID, o.DiscountType, o.DiscountValue, h.now())
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	orig, err := h.load(ctx, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	created, err := h.load(ctx, newID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.publish([]*orderview.Order{orig, created})
	httpx.OK(c, gin.H{"original": orig, "created": created})
}

type cancelReq struct {
	Reason string `json:"reason"`
}

func (h *handler) cancel(c *gin.Context) {
	var req cancelReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		httpx.Fail(c, validation("Vui lòng nhập lý do huỷ đơn"))
		return
	}
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p := auth.From(c)
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOrder(ctx, q, p, id)
		if err != nil {
			return err
		}
		switch o.Status {
		case "open":
		case "paid", "refunded":
			return errOrderPaid
		default:
			return errOrderNotOpen
		}
		paid, err := q.OrdConfirmedPaid(ctx, o.ID)
		if err != nil {
			return err
		}
		if paid > 0 {
			return errOrderPaid // part of a combined payment is already confirmed
		}
		if err := q.OrdCancelPendingPayments(ctx, o.ID); err != nil {
			return err
		}
		if err := q.OrdCancel(ctx, store.OrdCancelParams{ID: o.ID, CancelReason: reason, UpdatedAt: h.now()}); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &o.BranchID, Action: "order.cancel", EntityType: "order", EntityID: fmt.Sprint(o.ID),
			Before: gin.H{"status": o.Status, "total": o.Total}, After: gin.H{"status": "cancelled"}, Note: reason,
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.respond(c, 200, id)
}
