package orders

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

var (
	errNoOpenShift      = apperr.Conflict("NO_OPEN_SHIFT", "Chưa mở ca")
	errTableOccupied    = apperr.Conflict("TABLE_OCCUPIED", "Bàn đang có khách")
	errOrderNotOpen     = apperr.Conflict("ORDER_NOT_OPEN", "Đơn không còn mở")
	errOrderPaid        = apperr.Conflict("ORDER_PAID", "Đơn đã thanh toán")
	errSoldOut          = apperr.Conflict("PRODUCT_SOLD_OUT", "Món tạm hết")
	errInvalidModifier  = apperr.BadRequest("INVALID_MODIFIER", "Tuỳ chọn không hợp lệ cho món này")
	errInvalidSize      = apperr.BadRequest("INVALID_SIZE", "Vui lòng chọn size hợp lệ")
	errApprovalRequired = apperr.Forbidden("APPROVAL_REQUIRED", "Giảm giá vượt mức cho phép, cần quản lý duyệt")
	errInvalidMgrPIN    = apperr.Forbidden("INVALID_MANAGER_PIN", "PIN quản lý không đúng")
	errInvalidTaxCode   = apperr.BadRequest("INVALID_TAX_CODE", "Mã số thuế không hợp lệ")
)

func validation(reason string) error {
	return apperr.ErrValidation.WithDetails(map[string]any{"reason": reason})
}

// lockOrder reads an order FOR UPDATE and hides other branches' orders (404).
func lockOrder(ctx context.Context, q *store.Queries, p *auth.Principal, id int64) (store.Order, error) {
	o, err := q.OrdGetForUpdate(ctx, id)
	if err != nil {
		return o, err
	}
	if err := auth.CheckBranch(p, o.BranchID); err != nil {
		return o, err
	}
	return o, nil
}

// lockOpenOrder is lockOrder plus "must be open" (409 ORDER_NOT_OPEN).
func lockOpenOrder(ctx context.Context, q *store.Queries, p *auth.Principal, id int64) (store.Order, error) {
	o, err := lockOrder(ctx, q, p, id)
	if err != nil {
		return o, err
	}
	if o.Status != "open" {
		return o, errOrderNotOpen
	}
	return o, nil
}

// discountAmount applies the stored discount to a subtotal. Percent discounts
// stay a percent; amount discounts are clamped to the subtotal so the total is
// never negative.
func discountAmount(subtotal int64, typ *string, value int64) int64 {
	if typ == nil {
		return 0
	}
	if *typ == string(domain.DiscountPercent) {
		a, err := domain.ComputeDiscount(subtotal, domain.DiscountPercent, value)
		if err != nil {
			return 0
		}
		return a
	}
	return min(value, subtotal)
}

// recompute sets subtotal, discount amount and total from the order's items.
func recompute(ctx context.Context, q *store.Queries, id int64, typ *string, value int64, now time.Time) error {
	sub, err := q.OrdItemsSubtotal(ctx, id)
	if err != nil {
		return err
	}
	amt := discountAmount(sub, typ, value)
	return q.OrdUpdateTotals(ctx, store.OrdUpdateTotalsParams{ID: id, Subtotal: sub, DiscountAmount: amt, Total: sub - amt, UpdatedAt: now})
}

// newOrder allocates the next code of the branch for the VN business date of now
// (order_counters upsert: the row lock serialises concurrent creations) and inserts the order.
func newOrder(ctx context.Context, q *store.Queries, branchID int64, typ string, tableID *int64, shiftID, createdBy int64, splitFrom *int64, now time.Time) (int64, error) {
	code, err := q.OrdBranchCode(ctx, branchID)
	if err != nil {
		return 0, err
	}
	bd := domain.BusinessDate(now)
	seq, err := q.OrdNextSeq(ctx, store.OrdNextSeqParams{BranchID: branchID, BusinessDate: bd})
	if err != nil {
		return 0, err
	}
	return q.OrdCreate(ctx, store.OrdCreateParams{
		BranchID: branchID, Code: domain.OrderCode(code, bd, int(seq)), BusinessDate: bd, Seq: seq,
		OrderType: typ, TableID: tableID, ShiftID: shiftID, SplitFromOrderID: splitFrom, CreatedBy: createdBy, CreatedAt: now,
	})
}

// lockFreeTable locks a table of branchID and checks no open order sits on it.
func lockFreeTable(ctx context.Context, q *store.Queries, branchID, tableID int64) error {
	t, err := q.OrdLockTable(ctx, tableID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (t.BranchID != branchID || !t.IsActive)) {
		return apperr.ErrNotFound
	}
	if err != nil {
		return err
	}
	n, err := q.OrdCountOpenOrdersOnTable(ctx, &tableID)
	if err != nil {
		return err
	}
	if n > 0 {
		return errTableOccupied
	}
	return nil
}

// checkApprover verifies managerId + managerPin: an active branch manager of
// the order's branch, or the owner, whose PIN matches.
func checkApprover(ctx context.Context, q *store.Queries, branchID, managerID int64, pin string) error {
	u, err := q.OrdGetUser(ctx, managerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errInvalidMgrPIN
	}
	if err != nil {
		return err
	}
	role := domain.Role(u.Role)
	okRole := role == domain.RoleOwner || (role == domain.RoleBranchManager && u.BranchID != nil && *u.BranchID == branchID)
	if !okRole || u.DisabledAt != nil || u.PinHash == nil {
		return errInvalidMgrPIN
	}
	if bcrypt.CompareHashAndPassword([]byte(*u.PinHash), []byte(pin)) != nil {
		return errInvalidMgrPIN
	}
	return nil
}
