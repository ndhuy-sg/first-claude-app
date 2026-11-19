package branches_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type branch struct {
	ID                int64  `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Address           string `json:"address"`
	OpeningHours      string `json:"openingHours"`
	TaxCode           string `json:"taxCode"`
	VatRateBp         int    `json:"vatRateBp"`
	InvoiceSymbol     string `json:"invoiceSymbol"`
	NextInvoiceNumber int64  `json:"nextInvoiceNumber"`
	Vietqr            struct {
		BankBin     string `json:"bankBin"`
		AccountNo   string `json:"accountNo"`
		AccountName string `json:"accountName"`
	} `json:"vietqr"`
	IsActive bool `json:"isActive"`
}

func id(n int64) string { return strconv.FormatInt(n, 10) }

func TestB01_OwnerCreatesBranchWithTaxInfo(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	in := map[string]any{
		"code": "CN6", "name": "Mộc – CN6", "address": "1 Lê Lợi", "openingHours": "07:00 – 22:00",
		"taxCode": "0101234567-001", "vatRateBp": 1000, "invoiceSymbol": "1C26MAF",
		"vietqr": map[string]any{"bankBin": "970436", "accountNo": "999888777", "accountName": "CONG TY MOC"},
	}
	var b branch
	env.As(env.Fx.Users.Ql1).Post("/api/v1/branches", in).MustStatus(t, 403)
	env.As(env.Fx.Users.Warehouse).Post("/api/v1/branches", in).MustStatus(t, 403)
	owner.Post("/api/v1/branches", in).MustStatus(t, 201).JSON(t, &b)
	if b.ID == 0 || b.Code != "CN6" || b.TaxCode != "0101234567-001" || b.VatRateBp != 1000 || b.InvoiceSymbol != "1C26MAF" ||
		b.NextInvoiceNumber != 1 || b.Vietqr.AccountNo != "999888777" || !b.IsActive || b.OpeningHours != "07:00 – 22:00" {
		t.Fatalf("branch = %+v", b)
	}
	var symbol string
	if err := env.Pool.QueryRow(context.Background(), "SELECT symbol FROM invoice_series WHERE branch_id = $1", b.ID).Scan(&symbol); err != nil || symbol != "1C26MAF" {
		t.Fatalf("series %q %v", symbol, err)
	}
	// Duplicate code → 409.
	if r := owner.Post("/api/v1/branches", in); r.Status != 409 || r.ErrCode() != "BRANCH_CODE_TAKEN" {
		t.Fatalf("dup: %d %s", r.Status, r.Body)
	}
	// Validation.
	bad := []map[string]any{
		{"code": "CN7", "name": ""},
		{"code": "", "name": "x", "invoiceSymbol": "1C26MAG"},
		{"code": "CN7", "name": "x", "invoiceSymbol": "1C26MAG", "taxCode": "12345"},
		{"code": "CN7", "name": "x", "invoiceSymbol": "1C26MAG", "vatRateBp": 12000},
		{"code": "CN7", "name": "x", "invoiceSymbol": "BAD"},
		{"code": "CN7", "name": "x"}, // invoice symbol is required on create
	}
	for _, x := range bad {
		if r := owner.Post("/api/v1/branches", x); r.Status != 400 {
			t.Errorf("%v: %d %s", x, r.Status, r.Body)
		}
	}

	// Update (owner only).
	upd := map[string]any{"code": "CN6", "name": "Mộc – CN6 mới", "taxCode": "", "vatRateBp": 800, "invoiceSymbol": "1C27MAF",
		"vietqr": map[string]any{"bankBin": "970415", "accountNo": "111", "accountName": "MOC"}}
	env.As(env.Fx.Users.Ql1).Put("/api/v1/branches/"+id(env.Fx.Branches[1]), upd).MustStatus(t, 403)
	owner.Put("/api/v1/branches/"+id(b.ID), upd).MustStatus(t, 200).JSON(t, &b)
	if b.Name != "Mộc – CN6 mới" || b.TaxCode != "" || b.VatRateBp != 800 || b.InvoiceSymbol != "1C27MAF" || b.Vietqr.BankBin != "970415" || b.Address != "1 Lê Lợi" {
		t.Fatalf("updated = %+v", b)
	}
	owner.Put("/api/v1/branches/999999", upd).MustStatus(t, 404)
	// Renaming to another branch's code → 409.
	upd["code"] = "CN1"
	owner.Put("/api/v1/branches/"+id(b.ID), upd).MustStatus(t, 409)

	var n int
	_ = env.Pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_logs WHERE action IN ('branch.create', 'branch.update')").Scan(&n)
	if n != 2 {
		t.Fatalf("audit rows = %d", n)
	}
	// The new branch is listed for the owner.
	var list []branch
	owner.Get("/api/v1/branches").MustStatus(t, 200).JSON(t, &list)
	if len(list) != 6 || list[0].InvoiceSymbol != "1C26MAA" {
		t.Fatalf("list = %+v", list)
	}
}

func TestB07_ManagerCreatesAreasAndTablesOnlyForOwnBranch(t *testing.T) {
	env := testutil.NewEnv(t)
	ql1 := env.As(env.Fx.Users.Ql1)
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]

	var area struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Tables []any  `json:"tables"`
	}
	ql1.Post("/api/v1/areas", map[string]any{"name": "Tầng 3"}).MustStatus(t, 201).JSON(t, &area)
	if area.ID == 0 || area.Name != "Tầng 3" || area.Tables == nil || len(area.Tables) != 0 {
		t.Fatalf("area = %+v", area)
	}
	var branchID int64
	_ = env.Pool.QueryRow(context.Background(), "SELECT branch_id FROM areas WHERE id = $1", area.ID).Scan(&branchID)
	if branchID != cn1 {
		t.Fatalf("area in branch %d", branchID)
	}
	// Explicit CN2 is refused.
	ql1.Post("/api/v1/areas", map[string]any{"name": "Tầng 4", "branchId": cn2}).MustStatus(t, 403)
	// Duplicate name in the same branch → 409.
	if r := ql1.Post("/api/v1/areas", map[string]any{"name": "Tầng 3"}); r.Status != 409 || r.ErrCode() != "AREA_NAME_TAKEN" {
		t.Fatalf("dup area: %d %s", r.Status, r.Body)
	}
	ql1.Post("/api/v1/areas", map[string]any{"name": "  "}).MustStatus(t, 400)

	var table struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		AreaID    int64  `json:"areaId"`
		Status    string `json:"status"`
		OpenOrder any    `json:"openOrder"`
	}
	ql1.Post("/api/v1/tables", map[string]any{"areaId": area.ID, "name": "Bàn 30"}).MustStatus(t, 201).JSON(t, &table)
	if table.ID == 0 || table.AreaID != area.ID || table.Status != "free" || table.OpenOrder != nil || table.Name != "Bàn 30" {
		t.Fatalf("table = %+v", table)
	}
	if r := ql1.Post("/api/v1/tables", map[string]any{"areaId": area.ID, "name": "Bàn 1"}); r.Status != 409 || r.ErrCode() != "TABLE_NAME_TAKEN" {
		t.Fatalf("dup table: %d %s", r.Status, r.Body)
	}
	// A CN2 area is invisible to ql1.
	var cn2Area int64
	_ = env.Pool.QueryRow(context.Background(), "SELECT id FROM areas WHERE branch_id = $1 LIMIT 1", cn2).Scan(&cn2Area)
	ql1.Post("/api/v1/tables", map[string]any{"areaId": cn2Area, "name": "Bàn 99"}).MustStatus(t, 404)

	// Owner must say which branch for an area; may use any branch.
	owner := env.Owner()
	owner.Post("/api/v1/areas", map[string]any{"name": "Sân thượng"}).MustStatus(t, 400)
	owner.Post("/api/v1/areas", map[string]any{"name": "Sân thượng", "branchId": cn2}).MustStatus(t, 201)
	owner.Post("/api/v1/tables", map[string]any{"areaId": cn2Area, "name": "Bàn 99"}).MustStatus(t, 201)
	// Cashier cannot.
	env.Tn1().Post("/api/v1/tables", map[string]any{"areaId": area.ID, "name": "Bàn 31"}).MustStatus(t, 403)
}

func TestSettings_OwnerOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	type settings struct {
		CompanyName          string `json:"companyName"`
		CompanyTaxCode       string `json:"companyTaxCode"`
		InvoiceEmailFrom     string `json:"invoiceEmailFrom"`
		TaxSubmissionEnabled bool   `json:"taxSubmissionEnabled"`
		TaxGateway           string `json:"taxGateway"`
		DiscountMaxPct       int    `json:"discountMaxPct"`
		PosIdleLockMinutes   int    `json:"posIdleLockMinutes"`
		LoginMaxFailures     int    `json:"loginMaxFailures"`
		LoginLockMinutes     int    `json:"loginLockMinutes"`
		ShiftNoteThreshold   int64  `json:"shiftNoteThreshold"`
		PricesIncludeVat     bool   `json:"pricesIncludeVat"`
	}
	var s settings
	env.Tn1().Get("/api/v1/settings").MustStatus(t, 200).JSON(t, &s)
	if s.CompanyName != "Chuỗi cà phê Mộc" || s.DiscountMaxPct != 20 || s.TaxGateway != "off" || !s.PricesIncludeVat {
		t.Fatalf("settings = %+v", s)
	}
	s.DiscountMaxPct = 25
	s.TaxGateway = "fake"
	s.TaxSubmissionEnabled = true
	env.As(env.Fx.Users.Ql1).Put("/api/v1/settings", s).MustStatus(t, 403)
	var got settings
	env.Owner().Put("/api/v1/settings", s).MustStatus(t, 200).JSON(t, &got)
	if got != s {
		t.Fatalf("got %+v want %+v", got, s)
	}
	bad := s
	bad.DiscountMaxPct = 101
	env.Owner().Put("/api/v1/settings", bad).MustStatus(t, 400)
	bad = s
	bad.TaxGateway = "vendor"
	env.Owner().Put("/api/v1/settings", bad).MustStatus(t, 400)
	// Missing fields are rejected rather than reset to zero.
	env.Owner().Put("/api/v1/settings", map[string]any{"companyName": "x"}).MustStatus(t, 400)

	var n int
	_ = env.Pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_logs WHERE action = 'settings.update'").Scan(&n)
	if n != 1 {
		t.Fatalf("audit = %d", n)
	}
}
