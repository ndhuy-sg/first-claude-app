package testutil

import (
	"context"
	"fmt"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
)

// Direct-SQL builders so each module can test without the other modules' endpoints.

// OpenShift inserts an open shift for a device and returns its id.
func (e *Env) OpenShift(branchID, deviceID, userID, openingCash int64) int64 {
	e.T.Helper()
	var id int64
	err := e.Pool.QueryRow(context.Background(),
		`INSERT INTO shifts (branch_id, device_id, opened_by, opened_at, opening_cash) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		branchID, deviceID, userID, e.Clock.Now(), openingCash).Scan(&id)
	if err != nil {
		e.T.Fatalf("open shift: %v", err)
	}
	return id
}

// OpenShiftCN1 opens a shift on the CN1 iPad for tn1 with 1.000.000đ.
func (e *Env) OpenShiftCN1() int64 {
	return e.OpenShift(e.Fx.Branches[1], e.Fx.DeviceCN1, e.Fx.Users.Tn1, 1_000_000)
}

// ItemSpec is one order line for InsertOrder. UnitPrice is taken as-is.
type ItemSpec struct {
	ProductID int64
	Name      string
	SizeCode  string
	UnitPrice int64
	Qty       int
}

// OrderSpec describes an order to insert directly.
type OrderSpec struct {
	BranchID  int64
	ShiftID   int64
	TableID   *int64 // nil = takeaway
	CreatedBy int64
	Items     []ItemSpec
	CreatedAt time.Time // zero = env clock
}

// InsertOrder inserts an open order with items and correct subtotal/total, returning its id.
func (e *Env) InsertOrder(s OrderSpec) int64 {
	e.T.Helper()
	ctx := context.Background()
	at := s.CreatedAt
	if at.IsZero() {
		at = e.Clock.Now()
	}
	var code string
	if err := e.Pool.QueryRow(ctx, `SELECT code FROM branches WHERE id = $1`, s.BranchID).Scan(&code); err != nil {
		e.T.Fatalf("branch: %v", err)
	}
	bd := domain.BusinessDate(at)
	var seq int
	if err := e.Pool.QueryRow(ctx,
		`INSERT INTO order_counters (branch_id, business_date, last_seq) VALUES ($1, $2, 1)
		 ON CONFLICT (branch_id, business_date) DO UPDATE SET last_seq = order_counters.last_seq + 1 RETURNING last_seq`,
		s.BranchID, bd).Scan(&seq); err != nil {
		e.T.Fatalf("counter: %v", err)
	}
	typ := "takeaway"
	if s.TableID != nil {
		typ = "dine_in"
	}
	var subtotal int64
	for _, it := range s.Items {
		subtotal += it.UnitPrice * int64(it.Qty)
	}
	var id int64
	err := e.Pool.QueryRow(ctx,
		`INSERT INTO orders (branch_id, code, business_date, seq, order_type, table_id, shift_id, subtotal, total, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, $10, $10) RETURNING id`,
		s.BranchID, fmt.Sprintf("%s-%s-%04d", code, bd.Format("20060102"), seq), bd, seq, typ, s.TableID, s.ShiftID, subtotal, s.CreatedBy, at).Scan(&id)
	if err != nil {
		e.T.Fatalf("insert order: %v", err)
	}
	for _, it := range s.Items {
		name := it.Name
		if name == "" {
			_ = e.Pool.QueryRow(ctx, `SELECT name FROM products WHERE id = $1`, it.ProductID).Scan(&name)
		}
		if _, err := e.Pool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, size_code, size_name, unit_price, qty, line_total, created_by, created_at)
			 VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8, $9)`,
			id, it.ProductID, name, it.SizeCode, it.UnitPrice, it.Qty, it.UnitPrice*int64(it.Qty), s.CreatedBy, at); err != nil {
			e.T.Fatalf("insert item: %v", err)
		}
	}
	return id
}

// InsertBan5Order inserts the reference order used across the design and test cases:
// CN1, Bàn 5, 2 × Cà phê sữa L + thêm shot (43.000đ) and 1 × Trà đào cam sả M (42.000đ) = 128.000đ.
func (e *Env) InsertBan5Order(shiftID int64) int64 {
	return e.InsertOrder(OrderSpec{
		BranchID: e.Fx.Branches[1], ShiftID: shiftID, TableID: &e.Fx.TablesCN1[5], CreatedBy: e.Fx.Users.Tn1,
		Items: []ItemSpec{
			{ProductID: e.Fx.Products.CaPheSua, SizeCode: "L", UnitPrice: 43_000, Qty: 2},
			{ProductID: e.Fx.Products.TraDao, SizeCode: "M", UnitPrice: 42_000, Qty: 1},
		},
	})
}
