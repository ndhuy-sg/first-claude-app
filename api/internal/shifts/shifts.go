// Package shifts handles cashier shifts on a POS device: open with opening
// cash, summary, and close with cash reconciliation (tests S-01..S-05).
//
// Money is attributed to the shift that is open on the device at payment time:
// payments.shift_id (set when the payment is confirmed) and orders.paid_shift_id.
package shifts

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

var (
	ErrDeviceRequired = apperr.New(http.StatusUnauthorized, "DEVICE_REQUIRED", "Thao tác này cần máy POS đã kích hoạt")
	ErrNoOpenShift    = apperr.Conflict("NO_OPEN_SHIFT", "Chưa mở ca")
	ErrAlreadyOpen    = apperr.Conflict("SHIFT_ALREADY_OPEN", "Máy này đang có ca mở")
	ErrShiftClosed    = apperr.Conflict("SHIFT_CLOSED", "Ca đã chốt")
	ErrNoteRequired   = apperr.BadRequest("NOTE_REQUIRED", "Chênh lệch tiền lớn, vui lòng nhập ghi chú")
)

// Shift is the "Shift" schema.
type Shift struct {
	ID          int64                 `json:"id"`
	BranchID    int64                 `json:"branchId"`
	DeviceID    int64                 `json:"deviceId"`
	Status      string                `json:"status"`
	OpenedBy    orderview.UserSummary `json:"openedBy"`
	OpenedAt    time.Time             `json:"openedAt"`
	OpeningCash int64                 `json:"openingCash"`
	ClosedAt    *time.Time            `json:"closedAt"`
}

// Summary is the "ShiftSummary" schema.
type Summary struct {
	Shift             Shift  `json:"shift"`
	OrderCount        int32  `json:"orderCount"`
	Revenue           int64  `json:"revenue"`
	CashSales         int64  `json:"cashSales"`
	QrSales           int64  `json:"qrSales"`
	CashRefunds       int64  `json:"cashRefunds"`
	ExpectedCash      int64  `json:"expectedCash"`
	CountedCash       *int64 `json:"countedCash"`
	CashDiff          *int64 `json:"cashDiff"`
	Note              string `json:"note"`
	OpenOrders        int32  `json:"openOrders"`
	PendingQrPayments int32  `json:"pendingQrPayments"`
	InvoicesNotSent   int32  `json:"invoicesNotSent"`
}

func shiftFrom(r store.ShiftGetShiftWithUserRow) Shift {
	return Shift{
		ID: r.ID, BranchID: r.BranchID, DeviceID: r.DeviceID, Status: r.Status,
		OpenedBy: orderview.UserSummary{ID: r.OpenedBy, Name: r.OpenedByName},
		OpenedAt: r.OpenedAt, OpeningCash: r.OpeningCash, ClosedAt: r.ClosedAt,
	}
}

// OpenShiftID returns the id of the shift open on the caller's device, or
// NO_OPEN_SHIFT (409) when the request has no device or the device has no open shift.
// Used by the payments module to attribute payments (S-05).
func OpenShiftID(ctx context.Context, q *store.Queries, dev *auth.Device) (int64, error) {
	if dev == nil {
		return 0, ErrNoOpenShift
	}
	s, err := q.ShiftGetOpenShiftByDevice(ctx, dev.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoOpenShift
	}
	if err != nil {
		return 0, err
	}
	return s.ID, nil
}

// LoadSummary computes the summary of a shift. It does not check branch access.
func LoadSummary(ctx context.Context, q *store.Queries, id int64) (*Summary, error) {
	r, err := q.ShiftGetShiftWithUser(ctx, id)
	if err != nil {
		return nil, err
	}
	t, err := q.ShiftTotals(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Summary{
		Shift: shiftFrom(r), OrderCount: t.OrderCount, Revenue: t.CashSales + t.QrSales,
		CashSales: t.CashSales, QrSales: t.QrSales, CashRefunds: t.CashRefunds,
		ExpectedCash: domain.ExpectedCash(r.OpeningCash, t.CashSales, t.CashRefunds),
		CountedCash:  r.CountedCash, CashDiff: r.CashDiff, Note: r.CloseNote,
		OpenOrders: t.OpenOrders, PendingQrPayments: t.PendingQr, InvoicesNotSent: t.InvoicesNotSent,
	}, nil
}

type handler struct{ d *platform.Deps }

func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.GET("/shifts/current", auth.RequireAuth(), h.current)
	g.POST("/shifts/open", auth.RequireAction(domain.ActShift), h.open)
	g.GET("/shifts/:id/summary", auth.RequireAction(domain.ActShift), h.summary)
	g.POST("/shifts/:id/close", auth.RequireAction(domain.ActShift), h.close)
}

func (h *handler) current(c *gin.Context) {
	dev := auth.DeviceFrom(c)
	if dev == nil {
		c.JSON(http.StatusOK, nil)
		return
	}
	r, err := store.New(h.d.Pool).ShiftGetOpenShiftByDevice(c.Request.Context(), dev.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusOK, nil)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, shiftFrom(store.ShiftGetShiftWithUserRow(r)))
}

func (h *handler) open(c *gin.Context) {
	ctx := c.Request.Context()
	p, dev := auth.From(c), auth.DeviceFrom(c)
	if dev == nil {
		httpx.Fail(c, ErrDeviceRequired)
		return
	}
	if err := auth.CheckBranch(p, dev.BranchID); err != nil {
		httpx.Fail(c, apperr.ErrForbidden)
		return
	}
	var body struct {
		OpeningCash *int64 `json:"openingCash" binding:"required"`
	}
	if err := httpx.Bind(c, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	if *body.OpeningCash < 0 {
		httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "openingCash"}))
		return
	}
	q := store.New(h.d.Pool)
	id, err := q.ShiftInsertShift(ctx, store.ShiftInsertShiftParams{
		BranchID: dev.BranchID, DeviceID: dev.ID, OpenedBy: p.UserID, OpenedAt: h.d.Clock.Now(), OpeningCash: *body.OpeningCash,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httpx.Fail(c, ErrAlreadyOpen)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	r, err := q.ShiftGetShiftWithUser(ctx, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	s := shiftFrom(r)
	h.d.Events.Publish(platform.Event{Type: "shift.opened", BranchID: s.BranchID, Data: s})
	httpx.Created(c, s)
}

func (h *handler) summary(c *gin.Context) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	s, err := LoadSummary(c.Request.Context(), store.New(h.d.Pool), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := auth.CheckBranch(auth.From(c), s.Shift.BranchID); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, s)
}

type openOrder struct {
	ID        int64   `json:"id"`
	Code      string  `json:"code"`
	TableName *string `json:"tableName"`
	Total     int64   `json:"total"`
}

func (h *handler) close(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var body struct {
		CountedCash *int64 `json:"countedCash" binding:"required"`
		Note        string `json:"note"`
	}
	if err := httpx.Bind(c, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	if *body.CountedCash < 0 {
		httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "countedCash"}))
		return
	}
	note := strings.TrimSpace(body.Note)
	var sum *Summary
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		sh, err := q.ShiftLockShift(ctx, id)
		if err != nil {
			return err
		}
		if err := auth.CheckBranch(p, sh.BranchID); err != nil {
			return err
		}
		if sh.Status != "open" {
			return ErrShiftClosed
		}
		open, err := q.ShiftListOpenOrdersOfShift(ctx, id)
		if err != nil {
			return err
		}
		if len(open) > 0 {
			list := make([]openOrder, 0, len(open))
			for _, o := range open {
				list = append(list, openOrder{ID: o.ID, Code: o.Code, TableName: o.TableName, Total: o.Total})
			}
			return apperr.Conflict("OPEN_ORDERS", "Còn đơn chưa thanh toán trong ca").WithDetails(map[string]any{"orders": list})
		}
		s, err := LoadSummary(ctx, q, id)
		if err != nil {
			return err
		}
		diff := domain.CashDiff(*body.CountedCash, s.ExpectedCash)
		st, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		if domain.ShiftNoteRequired(diff, st.ShiftNoteThreshold) && note == "" {
			return ErrNoteRequired.WithDetails(map[string]any{"cashDiff": diff, "threshold": st.ShiftNoteThreshold})
		}
		now := h.d.Clock.Now()
		if err := q.ShiftCloseShift(ctx, store.ShiftCloseShiftParams{
			ID: id, ClosedBy: &p.UserID, ClosedAt: &now, ExpectedCash: &s.ExpectedCash,
			CountedCash: body.CountedCash, CashDiff: &diff, CloseNote: note,
		}); err != nil {
			return err
		}
		if err := platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &sh.BranchID, Action: "shift.close", EntityType: "shift", EntityID: itoa(id),
			After: map[string]any{"openingCash": sh.OpeningCash, "expectedCash": s.ExpectedCash, "countedCash": *body.CountedCash, "cashDiff": diff},
			Note:  note,
		}); err != nil {
			return err
		}
		sum, err = LoadSummary(ctx, q, id)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.d.Events.Publish(platform.Event{Type: "shift.closed", BranchID: sum.Shift.BranchID, Data: sum})
	httpx.OK(c, sum)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
