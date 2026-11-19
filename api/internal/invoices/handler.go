package invoices

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// RequireAny allows callers whose role may perform at least one of actions.
func RequireAny(actions ...domain.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := auth.From(c)
		if p == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": apperr.ErrUnauthenticated})
			return
		}
		for _, a := range actions {
			if domain.Can(p.Role, a) != domain.ScopeNone {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": apperr.ErrForbidden})
	}
}

type handler struct{ d *platform.Deps }

// Register adds the invoice routes. The public lookup needs no session (E-11).
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.GET("/public/invoices/:code", h.public)
	g.GET("/invoices", auth.RequireAction(domain.ActViewInvoices), h.list)
	// The POS (cashier) may open and email the invoice it just issued.
	g.GET("/invoices/:id", RequireAny(domain.ActViewInvoices, domain.ActSell), h.get)
	g.POST("/invoices/:id/email", RequireAny(domain.ActViewInvoices, domain.ActSell), h.email)
	g.POST("/invoices/:id/resend", auth.RequireAction(domain.ActViewInvoices), h.resend)
}

var invoiceStatuses = map[string]bool{"not_sent": true, "sending": true, "sent": true, "failed": true}

type listResponse struct {
	Items  []*Invoice `json:"items"`
	Total  int32      `json:"total"`
	Counts counts     `json:"counts"`
}

type counts struct {
	IssuedToday int32 `json:"issuedToday"`
	NotSent     int32 `json:"notSent"`
	Failed      int32 `json:"failed"`
	Cancelled   int32 `json:"cancelled"`
}

func (h *handler) list(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	var requested *int64
	if s := c.Query("branchId"); s != "" {
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			httpx.Fail(c, apperr.ErrValidation)
			return
		}
		requested = &v
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var status *string
	if s := c.Query("status"); s != "" {
		if !invoiceStatuses[s] {
			httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "status"}))
			return
		}
		status = &s
	}
	var from, to *time.Time
	if s := c.Query("date"); s != "" {
		day, err := time.ParseInLocation("2006-01-02", s, domain.VN)
		if err != nil {
			httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "date"}))
			return
		}
		end := day.AddDate(0, 0, 1)
		from, to = &day, &end
	}
	pg := httpx.GetPage(c)
	q := store.New(h.d.Pool)
	ids, err := q.InvListInvoiceIDs(ctx, store.InvListInvoiceIDsParams{
		BranchID: branch, Status: status, FromAt: from, ToAt: to, Lim: pg.Limit(), Off: pg.Offset(),
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := q.InvCountInvoices(ctx, store.InvCountInvoicesParams{BranchID: branch, Status: status, FromAt: from, ToAt: to})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	now := h.d.Clock.Now().In(domain.VN)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, domain.VN)
	cn, err := q.InvCounts(ctx, store.InvCountsParams{DayStart: dayStart, DayEnd: dayStart.AddDate(0, 0, 1), BranchID: branch})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	res := listResponse{Items: make([]*Invoice, 0, len(ids)), Total: total,
		Counts: counts{IssuedToday: cn.IssuedToday, NotSent: cn.NotSent, Failed: cn.Failed, Cancelled: cn.Cancelled}}
	for _, id := range ids {
		inv, err := Load(ctx, q, h.d.Cfg.PublicBaseURL, id)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		res.Items = append(res.Items, inv)
	}
	httpx.OK(c, res)
}

// loadScoped loads an invoice and hides other branches' invoices (404, A-10).
func (h *handler) loadScoped(c *gin.Context) (*Invoice, error) {
	id, err := httpx.PathID(c, "id")
	if err != nil {
		return nil, err
	}
	inv, err := Load(c.Request.Context(), store.New(h.d.Pool), h.d.Cfg.PublicBaseURL, id)
	if err != nil {
		return nil, err
	}
	if err := auth.CheckBranch(auth.From(c), inv.BranchID); err != nil {
		return nil, err
	}
	return inv, nil
}

func (h *handler) get(c *gin.Context) {
	inv, err := h.loadScoped(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, inv)
}

func (h *handler) public(c *gin.Context) {
	ctx := c.Request.Context()
	q := store.New(h.d.Pool)
	id, err := q.InvGetInvoiceIDByLookup(ctx, NormalizeLookupCode(c.Param("code")))
	if err != nil {
		httpx.Fail(c, apperr.NotFound("INVOICE_NOT_FOUND", "Không tìm thấy hoá đơn với mã tra cứu này"))
		return
	}
	inv, err := Load(ctx, q, h.d.Cfg.PublicBaseURL, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, inv)
}

// resend puts a not_sent or failed invoice back in the submit queue (E-09).
func (h *handler) resend(c *gin.Context) {
	inv, err := h.loadScoped(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		cur, err := q.InvLockInvoice(ctx, inv.ID)
		if err != nil {
			return err
		}
		if cur.Status != "failed" && cur.Status != "not_sent" {
			return apperr.Conflict("INVOICE_NOT_RESENDABLE", "Chỉ gửi lại được hoá đơn chưa gửi hoặc gửi thất bại")
		}
		if cur.Status == "failed" {
			if err := q.InvSetInvoiceStatus(ctx, store.InvSetInvoiceStatusParams{ID: cur.ID, Status: "not_sent"}); err != nil {
				return err
			}
		}
		key := SubmitDedupeKey(cur.ID)
		return q.InvRequeueJob(ctx, store.InvRequeueJobParams{
			Kind: JobSubmit, Payload: mustJSON(SubmitPayload{InvoiceID: cur.ID}), DedupeKey: &key, RunAt: h.d.Clock.Now(),
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}

func (h *handler) email(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required"`
	}
	if err := httpx.Bind(c, &body); err != nil {
		httpx.Fail(c, err)
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(body.Email))
	if err != nil || addr.Address != strings.TrimSpace(body.Email) {
		httpx.Fail(c, apperr.BadRequest("INVALID_EMAIL", "Email không hợp lệ"))
		return
	}
	inv, err := h.loadScoped(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	_, err = platform.EnqueueJob(ctx, store.New(h.d.Pool), JobEmail, EmailPayload{InvoiceID: inv.ID, Email: addr.Address}, "", h.d.Clock.Now())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued"})
}
