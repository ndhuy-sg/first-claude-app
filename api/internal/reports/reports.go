// Package reports serves the sales reports (GET /reports/sales) and the
// management dashboard (GET /reports/dashboard).
//
// Rules (test cases R-01..R-06):
//   - Revenue = sum of orders.total of orders with status 'paid'. Cancelled,
//     merged and refunded orders are excluded. Refunds are reported separately
//     in totals.refunds (by refund date).
//   - An order belongs to the Vietnam calendar day (Asia/Ho_Chi_Minh) of its paid_at.
//     from/to are inclusive VN dates.
//   - Branch-bound users (ql1) always see their own branch; branchId is ignored.
//     Chain users see all branches when branchId is omitted.
//   - groupBy=product: qty and revenue come from order_items. Product revenue is
//     the sum of line totals BEFORE order-level discounts; the discount is shown
//     once in totals.discount, so sum(product rows) = totals.revenue + totals.discount.
//   - groupBy=payment_method: confirmed payments of paid orders (split payments
//     count towards each method; orderCount = distinct orders using the method).
//   - groupBy=staff: the user who created the order (orders.created_by).
//   - groupBy=shift: the shift that took the payment (paid_shift_id, else shift_id).
package reports

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

// MaxRangeDays limits one sales report to about a year.
const MaxRangeDays = 366

// Register mounts the report routes.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	r := g.Group("/reports", auth.RequireAction(domain.ActViewSalesReport))
	r.GET("/sales", h.sales)
	r.GET("/dashboard", h.dashboard)
}

type handler struct{ d *platform.Deps }

// ---------- JSON shapes (openapi: SalesReport, Dashboard) ----------

type Totals struct {
	Revenue    int64 `json:"revenue"`
	OrderCount int64 `json:"orderCount"`
	Discount   int64 `json:"discount"`
	Refunds    int64 `json:"refunds"`
}

type Row struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Revenue    int64  `json:"revenue"`
	OrderCount int64  `json:"orderCount"`
	Qty        int64  `json:"qty"`
}

type SalesReport struct {
	From    string `json:"from"`
	To      string `json:"to"`
	GroupBy string `json:"groupBy"`
	Totals  Totals `json:"totals"`
	Rows    []Row  `json:"rows"`
}

type BranchStat struct {
	BranchID   int64  `json:"branchId"`
	BranchCode string `json:"branchCode"`
	Revenue    int64  `json:"revenue"`
	OrderCount int64  `json:"orderCount"`
}

type TopProduct struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	Qty       int64  `json:"qty"`
	Revenue   int64  `json:"revenue"`
}

type Alert struct {
	Kind     string  `json:"kind"`
	Severity string  `json:"severity"` // info | warning | critical
	Message  string  `json:"message"`
	Link     *string `json:"link"`
}

type Dashboard struct {
	Date           string       `json:"date"`
	Revenue        int64        `json:"revenue"`
	OrderCount     int64        `json:"orderCount"`
	AvgOrderValue  int64        `json:"avgOrderValue"`
	InvoicesIssued int64        `json:"invoicesIssued"`
	ByBranch       []BranchStat `json:"byBranch"`
	TopProducts    []TopProduct `json:"topProducts"`
	Alerts         []Alert      `json:"alerts"`
}

// ---------- helpers ----------

func badRequest(reason string) error {
	return apperr.ErrValidation.WithDetails(map[string]any{"reason": reason})
}

// parseDate parses YYYY-MM-DD as a VN calendar date (returned as midnight UTC, like domain.BusinessDate).
func parseDate(s string) (time.Time, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, badRequest("date must be YYYY-MM-DD")
	}
	return t, nil
}

// vnStart returns the instant when VN calendar day d starts.
func vnStart(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, domain.VN)
}

// branchScope resolves the branch filter: own branch for branch-bound users,
// else the optional ?branchId (nil = whole chain).
func branchScope(c *gin.Context) (*int64, error) {
	var requested *int64
	if s := c.Query("branchId"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			return nil, badRequest("branchId must be a positive integer")
		}
		requested = &id
	}
	return auth.ResolveBranch(auth.From(c), requested, false)
}

// FormatVND formats an amount as "1.234.000đ" (negative: "-60.000đ").
func FormatVND(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(ch)
	}
	out := b.String() + "đ"
	if neg {
		out = "-" + out
	}
	return out
}

var methodLabel = map[string]string{"cash": "Tiền mặt", "vietqr": "Chuyển khoản"}

// ---------- GET /reports/sales ----------

func (h *handler) sales(c *gin.Context) {
	rep, err := h.buildSales(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, rep)
}

func (h *handler) buildSales(c *gin.Context) (*SalesReport, error) {
	groupBy := c.Query("groupBy")
	switch groupBy {
	case "day", "product", "hour", "payment_method", "staff", "shift", "branch":
	default:
		return nil, badRequest("groupBy must be one of day, product, hour, payment_method, staff, shift, branch")
	}
	if c.Query("from") == "" || c.Query("to") == "" {
		return nil, badRequest("from and to are required")
	}
	from, err := parseDate(c.Query("from"))
	if err != nil {
		return nil, err
	}
	to, err := parseDate(c.Query("to"))
	if err != nil {
		return nil, err
	}
	if to.Before(from) {
		return nil, badRequest("to must not be before from")
	}
	if to.Sub(from) >= MaxRangeDays*24*time.Hour {
		return nil, badRequest(fmt.Sprintf("range must be at most %d days", MaxRangeDays))
	}
	branch, err := branchScope(c)
	if err != nil {
		return nil, err
	}
	fromTs, toTs := vnStart(from), vnStart(to.AddDate(0, 0, 1))
	ctx := c.Request.Context()
	q := store.New(h.d.Pool)

	tot, err := q.ReportTotals(ctx, store.ReportTotalsParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
	if err != nil {
		return nil, err
	}
	refunds, err := q.ReportRefunds(ctx, store.ReportRefundsParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
	if err != nil {
		return nil, err
	}
	rows, err := h.rows(ctx, q, groupBy, from, to, fromTs, toTs, branch)
	if err != nil {
		return nil, err
	}
	return &SalesReport{
		From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), GroupBy: groupBy,
		Totals: Totals{Revenue: tot.Revenue, OrderCount: tot.OrderCount, Discount: tot.Discount, Refunds: refunds},
		Rows:   rows,
	}, nil
}

func (h *handler) rows(ctx context.Context, q *store.Queries, groupBy string, from, to, fromTs, toTs time.Time, branch *int64) ([]Row, error) {
	out := []Row{}
	switch groupBy {
	case "day":
		rs, err := q.ReportByDay(ctx, store.ReportByDayParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		byDay := map[string]store.ReportByDayRow{}
		for _, r := range rs {
			byDay[r.Day] = r
		}
		// every day of the range is listed, also days without sales (charts)
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			key := d.Format(time.DateOnly)
			r := byDay[key]
			out = append(out, Row{Key: key, Label: d.Format("02/01/2006"), Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	case "hour":
		rs, err := q.ReportByHour(ctx, store.ReportByHourParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			out = append(out, Row{Key: fmt.Sprintf("%02d", r.Hour), Label: fmt.Sprintf("%02d:00–%02d:00", r.Hour, (r.Hour+1)%24), Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	case "product":
		rs, err := q.ReportByProduct(ctx, store.ReportByProductParams{FromTs: fromTs, ToTs: toTs, BranchID: branch, MaxRows: 1000})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			out = append(out, Row{Key: strconv.FormatInt(r.ProductID, 10), Label: r.Name, Revenue: r.Revenue, OrderCount: r.OrderCount, Qty: r.Qty})
		}
	case "payment_method":
		rs, err := q.ReportByPaymentMethod(ctx, store.ReportByPaymentMethodParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			label := methodLabel[r.Method]
			if label == "" {
				label = r.Method
			}
			out = append(out, Row{Key: r.Method, Label: label, Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	case "staff":
		rs, err := q.ReportByStaff(ctx, store.ReportByStaffParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			out = append(out, Row{Key: strconv.FormatInt(r.UserID, 10), Label: r.Name, Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	case "shift":
		rs, err := q.ReportByShift(ctx, store.ReportByShiftParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			label := fmt.Sprintf("Ca #%d · %s · %s · %s %s", r.ShiftID, r.BranchCode, r.DeviceName, r.OpenedByName, r.OpenedAt.In(domain.VN).Format("15:04 02/01"))
			out = append(out, Row{Key: strconv.FormatInt(r.ShiftID, 10), Label: label, Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	case "branch":
		rs, err := q.ReportByBranch(ctx, store.ReportByBranchParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			out = append(out, Row{Key: strconv.FormatInt(r.BranchID, 10), Label: r.BranchCode + " · " + r.BranchName, Revenue: r.Revenue, OrderCount: r.OrderCount})
		}
	}
	return out, nil
}

// ---------- GET /reports/dashboard ----------

func (h *handler) dashboard(c *gin.Context) {
	day := domain.BusinessDate(h.d.Clock.Now())
	if s := c.Query("date"); s != "" {
		var err error
		if day, err = parseDate(s); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	branch, err := branchScope(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	d, err := h.buildDashboard(c.Request.Context(), day, branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *handler) buildDashboard(ctx context.Context, day time.Time, branch *int64) (*Dashboard, error) {
	q := store.New(h.d.Pool)
	fromTs, toTs := vnStart(day), vnStart(day.AddDate(0, 0, 1))

	tot, err := q.ReportTotals(ctx, store.ReportTotalsParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
	if err != nil {
		return nil, err
	}
	issued, err := q.CountInvoicesIssued(ctx, store.CountInvoicesIssuedParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
	if err != nil {
		return nil, err
	}
	d := &Dashboard{
		Date: day.Format(time.DateOnly), Revenue: tot.Revenue, OrderCount: tot.OrderCount, InvoicesIssued: issued,
		ByBranch: []BranchStat{}, TopProducts: []TopProduct{}, Alerts: []Alert{},
	}
	if tot.OrderCount > 0 {
		d.AvgOrderValue = (tot.Revenue + tot.OrderCount/2) / tot.OrderCount // rounded to the nearest đồng
	}

	branches, err := q.ReportByBranch(ctx, store.ReportByBranchParams{FromTs: fromTs, ToTs: toTs, BranchID: branch})
	if err != nil {
		return nil, err
	}
	for _, b := range branches {
		d.ByBranch = append(d.ByBranch, BranchStat{BranchID: b.BranchID, BranchCode: b.BranchCode, Revenue: b.Revenue, OrderCount: b.OrderCount})
	}

	top, err := q.ReportByProduct(ctx, store.ReportByProductParams{FromTs: fromTs, ToTs: toTs, BranchID: branch, MaxRows: 5})
	if err != nil {
		return nil, err
	}
	for _, p := range top {
		d.TopProducts = append(d.TopProducts, TopProduct{ProductID: p.ProductID, Name: p.Name, Qty: p.Qty, Revenue: p.Revenue})
	}

	settings, err := q.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.TaxSubmissionEnabled {
		d.Alerts = append(d.Alerts, Alert{
			Kind: "tax_submission_off", Severity: "critical",
			Message: "Chưa bật gửi hoá đơn điện tử lên cơ quan thuế. Hoá đơn vẫn được xuất nhưng chưa được gửi đi.",
			Link:    platform.Ptr("/admin/hoa-don"),
		})
	}
	failed, err := q.CountInvoicesFailed(ctx, branch)
	if err != nil {
		return nil, err
	}
	if failed > 0 {
		d.Alerts = append(d.Alerts, Alert{
			Kind: "invoice_failed", Severity: "critical",
			Message: fmt.Sprintf("%d hoá đơn gửi lên cơ quan thuế bị lỗi, cần kiểm tra và gửi lại.", failed),
			Link:    platform.Ptr("/admin/hoa-don?status=failed"),
		})
	}
	shifts, err := q.ShiftsWithCashDiff(ctx, store.ShiftsWithCashDiffParams{FromTs: fromTs, ToTs: toTs, Threshold: settings.ShiftNoteThreshold, BranchID: branch})
	if err != nil {
		return nil, err
	}
	for _, s := range shifts {
		var diff int64
		if s.CashDiff != nil {
			diff = *s.CashDiff
		}
		word := "thừa"
		if diff < 0 {
			word = "thiếu"
			diff = -diff
		}
		d.Alerts = append(d.Alerts, Alert{
			Kind: "shift_cash_diff", Severity: "warning",
			Message: fmt.Sprintf("Ca #%d (%s · %s) chốt ca %s %s tiền mặt.", s.ID, s.BranchCode, s.DeviceName, word, FormatVND(diff)),
			Link:    platform.Ptr(fmt.Sprintf("/admin/ca/%d", s.ID)),
		})
	}
	return d, nil
}
