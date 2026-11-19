// Package payments takes payments (cash, VietQR, mixed), confirms or cancels
// pending QR payments, refunds paid orders and suggests cash amounts.
//
// One payment request = one transaction (docs/ARCHITECTURE.md §10): lock the
// order, check it, insert the payment and — when confirmed payments reach the
// stored order total — mark the order paid, issue the invoice (number from the
// locked branch series) and enqueue its submit job (E-01).
package payments

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/invoices"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/shifts"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

var (
	ErrOrderNotOpen      = apperr.Conflict("ORDER_NOT_OPEN", "Đơn không còn mở (đã thanh toán hoặc đã huỷ)")
	ErrEmptyOrder        = apperr.BadRequest("EMPTY_ORDER", "Đơn chưa có món nào")
	ErrInsufficientCash  = apperr.BadRequest("INSUFFICIENT_CASH", "Tiền khách đưa ít hơn số tiền cần trả")
	ErrAmountExceedsDue  = apperr.BadRequest("AMOUNT_EXCEEDS_DUE", "Số tiền lớn hơn số còn phải trả")
	ErrVietQRNotSet      = apperr.BadRequest("VIETQR_NOT_CONFIGURED", "Chi nhánh chưa cài tài khoản nhận chuyển khoản")
	ErrPaymentNotPending = apperr.Conflict("PAYMENT_NOT_PENDING", "Khoản thanh toán không ở trạng thái chờ")
	ErrOrderNotPaid      = apperr.Conflict("ORDER_NOT_PAID", "Chỉ hoàn tiền được đơn đã thanh toán")
	ErrApprovalRequired  = apperr.Forbidden("APPROVAL_REQUIRED", "Cần quản lý duyệt bằng mã PIN")
	ErrInvalidManagerPIN = apperr.Forbidden("INVALID_MANAGER_PIN", "Mã PIN quản lý không đúng")
)

// Result is the "PaymentResult" schema.
type Result struct {
	Payment orderview.Payment `json:"payment"`
	Order   *orderview.Order  `json:"order"`
}

type handler struct{ d *platform.Deps }

func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.POST("/orders/:id/payments", auth.RequireAction(domain.ActSell), h.create)
	g.POST("/payments/:id/confirm", auth.RequireAction(domain.ActSell), h.confirm)
	g.POST("/payments/:id/cancel", auth.RequireAction(domain.ActSell), h.cancel)
	g.POST("/orders/:id/refund", auth.RequireAction(domain.ActSell), h.refund)
	g.GET("/payments/cash-suggestions", auth.RequireAuth(), h.suggestions)
}

func (h *handler) suggestions(c *gin.Context) {
	amount, err := strconv.ParseInt(c.Query("amount"), 10, 64)
	if err != nil || amount <= 0 {
		httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "amount"}))
		return
	}
	httpx.OK(c, domain.CashSuggestions(amount))
}

// lockOrder locks the order row and hides other branches' orders (404).
// When the request comes from a POS device, the order must belong to the device's branch.
func lockOrder(ctx context.Context, q *store.Queries, c *gin.Context, id int64) (store.Order, error) {
	o, err := q.PayLockOrder(ctx, id)
	if err != nil {
		return o, err
	}
	if err := auth.CheckBranch(auth.From(c), o.BranchID); err != nil {
		return o, err
	}
	if dev := auth.DeviceFrom(c); dev != nil && dev.BranchID != o.BranchID {
		return o, apperr.ErrNotFound
	}
	return o, nil
}

// settle marks the order paid and issues its invoice when confirmed payments cover the total.
func (h *handler) settle(ctx context.Context, q *store.Queries, o store.Order, shiftID int64) (bool, error) {
	sums, err := q.PayOrderPaymentSums(ctx, o.ID)
	if err != nil {
		return false, err
	}
	if sums.Confirmed < o.Total {
		return false, nil
	}
	now := h.d.Clock.Now()
	if err := q.PayMarkOrderPaid(ctx, store.PayMarkOrderPaidParams{ID: o.ID, PaidAt: &now, PaidShiftID: &shiftID}); err != nil {
		return false, err
	}
	if _, err := invoices.Issue(ctx, q, h.d, o.ID); err != nil {
		return false, err
	}
	return true, nil
}

func paymentView(p store.Payment) orderview.Payment {
	return orderview.Payment{
		ID: p.ID, Method: p.Method, Status: p.Status, Amount: p.Amount, CashReceived: p.CashReceived,
		ChangeAmount: p.ChangeAmount, QRPayload: p.QrPayload, CreatedAt: p.CreatedAt, ConfirmedAt: p.ConfirmedAt,
	}
}

func (h *handler) finish(c *gin.Context, status int, paymentID int64) {
	ctx := c.Request.Context()
	q := store.New(h.d.Pool)
	p, err := q.PayGetPayment(ctx, paymentID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	o, err := orderview.Load(ctx, q, h.d.Cfg.PublicBaseURL, p.OrderID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.d.Events.Publish(platform.Event{Type: "order.updated", BranchID: o.BranchID, Data: o})
	c.JSON(status, Result{Payment: paymentView(p), Order: o})
}

type paymentRequest struct {
	Method       string `json:"method" binding:"required,oneof=cash vietqr"`
	Amount       int64  `json:"amount" binding:"required,min=1"`
	CashReceived *int64 `json:"cashReceived"`
}

func (h *handler) create(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	orderID, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req paymentRequest
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	if req.Method == "cash" && req.CashReceived == nil {
		httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "cashReceived"}))
		return
	}
	var paymentID int64
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOrder(ctx, q, c, orderID)
		if err != nil {
			return err
		}
		if o.Status != "open" {
			return ErrOrderNotOpen // also stops a concurrent duplicate after the first one paid (P-08)
		}
		n, err := q.PayCountOrderItems(ctx, o.ID)
		if err != nil {
			return err
		}
		if n == 0 || o.Total <= 0 {
			return ErrEmptyOrder
		}
		shiftID, err := shifts.OpenShiftID(ctx, q, auth.DeviceFrom(c))
		if err != nil {
			return err
		}
		sums, err := q.PayOrderPaymentSums(ctx, o.ID)
		if err != nil {
			return err
		}
		// Pending QR payments reserve their amount until confirmed or cancelled.
		due := o.Total - sums.Confirmed - sums.Pending
		if req.Amount > due {
			return ErrAmountExceedsDue.WithDetails(map[string]any{"due": due})
		}
		now := h.d.Clock.Now()
		params := store.PayInsertPaymentParams{
			OrderID: o.ID, BranchID: o.BranchID, ShiftID: shiftID, Method: req.Method, Amount: req.Amount,
			CreatedBy: p.UserID, CreatedAt: now,
		}
		switch req.Method {
		case "cash":
			change, err := domain.Change(req.Amount, *req.CashReceived)
			if err != nil {
				return ErrInsufficientCash
			}
			params.Status, params.CashReceived, params.ChangeAmount = "confirmed", req.CashReceived, &change
			params.ConfirmedBy, params.ConfirmedAt = &p.UserID, &now
		case "vietqr":
			br, err := q.GetBranch(ctx, o.BranchID)
			if err != nil {
				return err
			}
			payload, err := domain.BuildVietQR(domain.VietQRInput{
				BankBIN: br.VietqrBankBin, AccountNo: br.VietqrAccountNo, Amount: req.Amount, Purpose: o.Code,
			})
			if errors.Is(err, domain.ErrVietQRAccount) {
				return ErrVietQRNotSet
			}
			if err != nil {
				return err
			}
			params.Status, params.QrPayload = "pending", payload
		}
		paymentID, err = q.PayInsertPayment(ctx, params)
		if err != nil {
			return err
		}
		if params.Status == "confirmed" {
			_, err = h.settle(ctx, q, o, shiftID)
		}
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.finish(c, http.StatusCreated, paymentID)
}

// lockPending locks the payment's order then the payment (same lock order as create).
func lockPending(ctx context.Context, q *store.Queries, c *gin.Context, paymentID int64) (store.Order, store.Payment, error) {
	pay, err := q.PayGetPayment(ctx, paymentID)
	if err != nil {
		return store.Order{}, pay, err
	}
	o, err := lockOrder(ctx, q, c, pay.OrderID)
	if err != nil {
		return o, pay, err
	}
	pay, err = q.PayLockPayment(ctx, paymentID)
	if err != nil {
		return o, pay, err
	}
	if pay.Status != "pending" {
		return o, pay, ErrPaymentNotPending
	}
	return o, pay, nil
}

// confirm: the cashier saw the transfer arrive ("Đã nhận tiền", P-04).
func (h *handler) confirm(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, _, err := lockPending(ctx, q, c, id)
		if err != nil {
			return err
		}
		if o.Status != "open" {
			return ErrOrderNotOpen
		}
		shiftID, err := shifts.OpenShiftID(ctx, q, auth.DeviceFrom(c))
		if err != nil {
			return err
		}
		now := h.d.Clock.Now()
		if err := q.PayConfirmPayment(ctx, store.PayConfirmPaymentParams{ID: id, ConfirmedBy: &p.UserID, ConfirmedAt: &now, ShiftID: shiftID}); err != nil {
			return err
		}
		_, err = h.settle(ctx, q, o, shiftID)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.finish(c, http.StatusOK, id)
}

// cancel drops a pending QR payment (P-05); the order stays open.
func (h *handler) cancel(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		if _, _, err := lockPending(ctx, q, c, id); err != nil {
			return err
		}
		return q.PayCancelPayment(ctx, id)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.finish(c, http.StatusOK, id)
}

// verifyManager checks a manager's PIN for an approval in branchID.
func verifyManager(ctx context.Context, q *store.Queries, managerID int64, pin string, branchID int64) error {
	u, err := q.GetUser(ctx, managerID)
	if err != nil || u.DisabledAt != nil || u.PinHash == nil ||
		domain.Can(domain.Role(u.Role), domain.ActApprove) == domain.ScopeNone ||
		(u.BranchID != nil && *u.BranchID != branchID) {
		return ErrInvalidManagerPIN
	}
	if bcrypt.CompareHashAndPassword([]byte(*u.PinHash), []byte(pin)) != nil {
		return ErrInvalidManagerPIN
	}
	return nil
}

type refundRequest struct {
	Reason     string `json:"reason"`
	ManagerID  *int64 `json:"managerId"`
	ManagerPIN string `json:"managerPin"`
}

// refund (P-09, E-08): paid orders only; managers directly, cashiers with a manager's PIN.
// The cash part of the order is given back from the drawer of the device's open shift.
func (h *handler) refund(c *gin.Context) {
	ctx := c.Request.Context()
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req refundRequest
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		httpx.Fail(c, apperr.BadRequest("REASON_REQUIRED", "Vui lòng nhập lý do hoàn tiền"))
		return
	}
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		o, err := lockOrder(ctx, q, c, id)
		if err != nil {
			return err
		}
		if o.Status != "paid" {
			return ErrOrderNotPaid
		}
		approver := p.UserID
		if domain.Can(p.Role, domain.ActApprove) == domain.ScopeNone {
			if req.ManagerID == nil || req.ManagerPIN == "" {
				return ErrApprovalRequired
			}
			if err := verifyManager(ctx, q, *req.ManagerID, req.ManagerPIN, o.BranchID); err != nil {
				return err
			}
			approver = *req.ManagerID
		}
		var shiftID *int64
		if sid, err := shifts.OpenShiftID(ctx, q, auth.DeviceFrom(c)); err == nil {
			shiftID = &sid
		} else if apperr.As(err) == nil {
			return err
		}
		sums, err := q.PayOrderPaymentSums(ctx, o.ID)
		if err != nil {
			return err
		}
		cash := min(sums.ConfirmedCash, o.Total)
		now := h.d.Clock.Now()
		if _, err := q.PayInsertRefund(ctx, store.PayInsertRefundParams{
			OrderID: o.ID, BranchID: o.BranchID, ShiftID: shiftID, Amount: o.Total, CashAmount: cash,
			Reason: reason, CreatedBy: p.UserID, ApprovedBy: approver, CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := q.PayMarkOrderRefunded(ctx, store.PayMarkOrderRefundedParams{ID: o.ID, UpdatedAt: now}); err != nil {
			return err
		}
		if err := platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, ApproverID: &approver, BranchID: &o.BranchID,
			Action: "order.refund", EntityType: "order", EntityID: strconv.FormatInt(o.ID, 10),
			Before: map[string]any{"status": o.Status, "total": o.Total},
			After:  map[string]any{"status": "refunded", "amount": o.Total, "cashAmount": cash},
			Note:   reason,
		}); err != nil {
			return err
		}
		_, err = invoices.IssueCancel(ctx, q, h.d, o.ID)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	o, err := orderview.Load(ctx, store.New(h.d.Pool), h.d.Cfg.PublicBaseURL, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	h.d.Events.Publish(platform.Event{Type: "order.updated", BranchID: o.BranchID, Data: o})
	httpx.OK(c, o)
}
