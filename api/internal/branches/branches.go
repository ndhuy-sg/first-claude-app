// Package branches serves system settings, branches (with tax info, invoice
// symbol and VietQR account) and the creation of areas and tables.
// GET /tables (live floor plan) belongs to the orders module.
package branches

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

var (
	errCodeTaken  = apperr.Conflict("BRANCH_CODE_TAKEN", "Mã chi nhánh đã tồn tại")
	errAreaTaken  = apperr.Conflict("AREA_NAME_TAKEN", "Tên khu vực đã có trong chi nhánh")
	errTableTaken = apperr.Conflict("TABLE_NAME_TAKEN", "Tên bàn đã có trong chi nhánh")

	// Invoice symbol per Circular 78/2021: 1|2, C|K, 2-digit year, invoice type letter, 2 letters (e.g. 1C26MAA).
	reSymbol  = regexp.MustCompile(`^[12][CK][0-9]{2}[TDLMNBGH][A-Z]{2}$`)
	reTaxCode = regexp.MustCompile(`^[0-9]{10}(-[0-9]{3})?$`)
	reBankBin = regexp.MustCompile(`^[0-9]{6}$`)
	reDigits  = regexp.MustCompile(`^[0-9]{1,19}$`)
)

type handler struct{ d *platform.Deps }

// Register adds settings, branches, areas and table-creation routes.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.GET("/settings", auth.RequireAuth(), h.getSettings)
	g.PUT("/settings", auth.RequireAction(domain.ActSettings), h.putSettings)
	g.GET("/branches", auth.RequireAuth(), h.listBranches)
	g.POST("/branches", auth.RequireAction(domain.ActManageBranches), h.createBranch)
	g.PUT("/branches/:id", auth.RequireAction(domain.ActManageBranches), h.updateBranch)
	g.POST("/areas", auth.RequireAction(domain.ActManageTables), h.createArea)
	g.POST("/tables", auth.RequireAction(domain.ActManageTables), h.createTable)
}

func invalid(field string) *apperr.Error {
	return apperr.ErrValidation.WithDetails(map[string]any{"field": field})
}

// ---------- settings ----------

// Settings is the JSON shape of app_settings. Pointers make every field required on PUT.
type Settings struct {
	CompanyName          *string `json:"companyName"`
	CompanyTaxCode       *string `json:"companyTaxCode"`
	InvoiceEmailFrom     *string `json:"invoiceEmailFrom"`
	TaxSubmissionEnabled *bool   `json:"taxSubmissionEnabled"`
	TaxGateway           *string `json:"taxGateway"`
	DiscountMaxPct       *int32  `json:"discountMaxPct"`
	PosIdleLockMinutes   *int32  `json:"posIdleLockMinutes"`
	LoginMaxFailures     *int32  `json:"loginMaxFailures"`
	LoginLockMinutes     *int32  `json:"loginLockMinutes"`
	ShiftNoteThreshold   *int64  `json:"shiftNoteThreshold"`
	PricesIncludeVat     *bool   `json:"pricesIncludeVat"`
}

func settingsJSON(s store.AppSetting) Settings {
	return Settings{
		CompanyName: &s.CompanyName, CompanyTaxCode: &s.CompanyTaxCode, InvoiceEmailFrom: &s.InvoiceEmailFrom,
		TaxSubmissionEnabled: &s.TaxSubmissionEnabled, TaxGateway: &s.TaxGateway, DiscountMaxPct: &s.DiscountMaxPct,
		PosIdleLockMinutes: &s.PosIdleLockMinutes, LoginMaxFailures: &s.LoginMaxFailures, LoginLockMinutes: &s.LoginLockMinutes,
		ShiftNoteThreshold: &s.ShiftNoteThreshold, PricesIncludeVat: &s.PricesIncludeVat,
	}
}

func (s Settings) validate() error {
	switch {
	case s.CompanyName == nil:
		return invalid("companyName")
	case s.CompanyTaxCode == nil || (*s.CompanyTaxCode != "" && !reTaxCode.MatchString(*s.CompanyTaxCode)):
		return invalid("companyTaxCode")
	case s.InvoiceEmailFrom == nil:
		return invalid("invoiceEmailFrom")
	case s.TaxSubmissionEnabled == nil:
		return invalid("taxSubmissionEnabled")
	case s.TaxGateway == nil || !oneOf(*s.TaxGateway, "off", "direct", "tvan", "fake"):
		return invalid("taxGateway")
	case s.DiscountMaxPct == nil || *s.DiscountMaxPct < 0 || *s.DiscountMaxPct > 100:
		return invalid("discountMaxPct")
	case s.PosIdleLockMinutes == nil || *s.PosIdleLockMinutes < 1 || *s.PosIdleLockMinutes > 24*60:
		return invalid("posIdleLockMinutes")
	case s.LoginMaxFailures == nil || *s.LoginMaxFailures < 1 || *s.LoginMaxFailures > 100:
		return invalid("loginMaxFailures")
	case s.LoginLockMinutes == nil || *s.LoginLockMinutes < 1 || *s.LoginLockMinutes > 24*60:
		return invalid("loginLockMinutes")
	case s.ShiftNoteThreshold == nil || *s.ShiftNoteThreshold < 0:
		return invalid("shiftNoteThreshold")
	case s.PricesIncludeVat == nil:
		return invalid("pricesIncludeVat")
	}
	return nil
}

func oneOf(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}

func (h *handler) getSettings(c *gin.Context) {
	s, err := store.New(h.d.Pool).GetSettings(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, settingsJSON(s))
}

func (h *handler) putSettings(c *gin.Context) {
	p := auth.From(c)
	var in Settings
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := in.validate(); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out store.AppSetting
	err := store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		before, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		out, err = q.BranchesUpdateSettings(ctx, store.BranchesUpdateSettingsParams{
			CompanyName: strings.TrimSpace(*in.CompanyName), CompanyTaxCode: *in.CompanyTaxCode,
			InvoiceEmailFrom: strings.TrimSpace(*in.InvoiceEmailFrom), TaxSubmissionEnabled: *in.TaxSubmissionEnabled,
			TaxGateway: *in.TaxGateway, DiscountMaxPct: *in.DiscountMaxPct, PosIdleLockMinutes: *in.PosIdleLockMinutes,
			LoginMaxFailures: *in.LoginMaxFailures, LoginLockMinutes: *in.LoginLockMinutes,
			ShiftNoteThreshold: *in.ShiftNoteThreshold, PricesIncludeVat: *in.PricesIncludeVat, Now: h.d.Clock.Now(),
		})
		if err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, Action: "settings.update", EntityType: "settings", EntityID: "1",
			Before: settingsJSON(before), After: settingsJSON(out),
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, settingsJSON(out))
}

// ---------- branches ----------

type vietQR struct {
	BankBin     string `json:"bankBin"`
	AccountNo   string `json:"accountNo"`
	AccountName string `json:"accountName"`
}

// Branch is the JSON shape of a branch.
type Branch struct {
	ID                int64  `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Address           string `json:"address"`
	OpeningHours      string `json:"openingHours"`
	TaxCode           string `json:"taxCode"`
	VatRateBp         int32  `json:"vatRateBp"`
	InvoiceSymbol     string `json:"invoiceSymbol"`
	NextInvoiceNumber int64  `json:"nextInvoiceNumber"`
	Vietqr            vietQR `json:"vietqr"`
	IsActive          bool   `json:"isActive"`
}

func branchJSON(r store.BranchesListRow) Branch {
	return Branch{
		ID: r.ID, Code: r.Code, Name: r.Name, Address: r.Address, OpeningHours: r.OpeningHours, TaxCode: r.TaxCode,
		VatRateBp: r.VatRateBp, InvoiceSymbol: r.InvoiceSymbol, NextInvoiceNumber: r.NextInvoiceNumber,
		Vietqr:   vietQR{BankBin: r.VietqrBankBin, AccountNo: r.VietqrAccountNo, AccountName: r.VietqrAccountName},
		IsActive: r.IsActive,
	}
}

func (h *handler) listBranches(c *gin.Context) {
	p := auth.From(c)
	rows, err := store.New(h.d.Pool).BranchesList(c.Request.Context(), p.BranchID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]Branch, 0, len(rows))
	for _, r := range rows {
		out = append(out, branchJSON(r))
	}
	httpx.OK(c, out)
}

// branchInput is BranchInput. Optional fields are pointers: absent keeps the current value on update.
type branchInput struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Address       *string `json:"address"`
	OpeningHours  *string `json:"openingHours"`
	TaxCode       *string `json:"taxCode"`
	VatRateBp     *int32  `json:"vatRateBp"`
	InvoiceSymbol *string `json:"invoiceSymbol"`
	Vietqr        *vietQR `json:"vietqr"`
}

// apply validates in and merges it over cur (the zero branch with defaults on create).
func (in branchInput) apply(cur store.Branch, curSymbol string) (store.Branch, string, error) {
	b := cur
	b.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	b.Name = strings.TrimSpace(in.Name)
	if b.Code == "" || len(b.Code) > 20 {
		return b, "", invalid("code")
	}
	if b.Name == "" {
		return b, "", invalid("name")
	}
	if in.Address != nil {
		b.Address = strings.TrimSpace(*in.Address)
	}
	if in.OpeningHours != nil {
		b.OpeningHours = strings.TrimSpace(*in.OpeningHours)
	}
	if in.TaxCode != nil {
		t := strings.TrimSpace(*in.TaxCode)
		if t != "" && !reTaxCode.MatchString(t) {
			return b, "", invalid("taxCode")
		}
		b.TaxCode = t
	}
	if in.VatRateBp != nil {
		if *in.VatRateBp < 0 || *in.VatRateBp > 10000 {
			return b, "", invalid("vatRateBp")
		}
		b.VatRateBp = *in.VatRateBp
	}
	symbol := curSymbol
	if in.InvoiceSymbol != nil {
		symbol = strings.ToUpper(strings.TrimSpace(*in.InvoiceSymbol))
	}
	if !reSymbol.MatchString(symbol) {
		return b, "", invalid("invoiceSymbol")
	}
	if in.Vietqr != nil {
		v := *in.Vietqr
		v.BankBin, v.AccountNo, v.AccountName = strings.TrimSpace(v.BankBin), strings.TrimSpace(v.AccountNo), strings.TrimSpace(v.AccountName)
		if (v.BankBin != "" && !reBankBin.MatchString(v.BankBin)) || (v.AccountNo != "" && !reDigits.MatchString(v.AccountNo)) {
			return b, "", invalid("vietqr")
		}
		b.VietqrBankBin, b.VietqrAccountNo, b.VietqrAccountName = v.BankBin, v.AccountNo, v.AccountName
	}
	return b, symbol, nil
}

func (h *handler) loadBranch(c *gin.Context, q *store.Queries, id int64) (Branch, error) {
	rows, err := q.BranchesList(c.Request.Context(), &id)
	if err != nil {
		return Branch{}, err
	}
	if len(rows) == 0 {
		return Branch{}, apperr.ErrNotFound
	}
	return branchJSON(rows[0]), nil
}

func (h *handler) createBranch(c *gin.Context) {
	p := auth.From(c)
	var in branchInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	b, symbol, err := in.apply(store.Branch{VatRateBp: 800, IsActive: true}, "")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out Branch
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		taken, err := q.BranchesCodeTaken(ctx, store.BranchesCodeTakenParams{Code: b.Code, ExceptID: 0})
		if err != nil {
			return err
		}
		if taken {
			return errCodeTaken
		}
		id, err := q.BranchesCreate(ctx, store.BranchesCreateParams{
			Code: b.Code, Name: b.Name, Address: b.Address, OpeningHours: b.OpeningHours, TaxCode: b.TaxCode, VatRateBp: b.VatRateBp,
			VietqrBankBin: b.VietqrBankBin, VietqrAccountNo: b.VietqrAccountNo, VietqrAccountName: b.VietqrAccountName,
		})
		if err != nil {
			return err
		}
		if err := q.BranchesUpsertSeries(ctx, store.BranchesUpsertSeriesParams{BranchID: id, Symbol: symbol}); err != nil {
			return err
		}
		if out, err = h.loadBranch(c, q, id); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &id, Action: "branch.create", EntityType: "branch", EntityID: strconv.FormatInt(id, 10), After: out,
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *handler) updateBranch(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in branchInput
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out Branch
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		cur, err := q.BranchesGetForUpdate(ctx, id)
		if err != nil {
			return err
		}
		// Lock the series so a concurrent invoice cannot take a number while the symbol changes.
		var curSymbol string
		series, err := q.BranchesGetSeriesForUpdate(ctx, id)
		switch {
		case err == nil:
			curSymbol = series.Symbol
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		before, err := h.loadBranch(c, q, id)
		if err != nil {
			return err
		}
		b, symbol, err := in.apply(cur, curSymbol)
		if err != nil {
			return err
		}
		taken, err := q.BranchesCodeTaken(ctx, store.BranchesCodeTakenParams{Code: b.Code, ExceptID: id})
		if err != nil {
			return err
		}
		if taken {
			return errCodeTaken
		}
		if err := q.BranchesUpdate(ctx, store.BranchesUpdateParams{
			ID: id, Code: b.Code, Name: b.Name, Address: b.Address, OpeningHours: b.OpeningHours, TaxCode: b.TaxCode, VatRateBp: b.VatRateBp,
			VietqrBankBin: b.VietqrBankBin, VietqrAccountNo: b.VietqrAccountNo, VietqrAccountName: b.VietqrAccountName,
		}); err != nil {
			return err
		}
		if symbol != curSymbol {
			if err := q.BranchesUpsertSeries(ctx, store.BranchesUpsertSeriesParams{BranchID: id, Symbol: symbol}); err != nil {
				return err
			}
		}
		if out, err = h.loadBranch(c, q, id); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &id, Action: "branch.update", EntityType: "branch", EntityID: strconv.FormatInt(id, 10),
			Before: before, After: out,
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

// ---------- areas and tables ----------

// Table is the JSON shape of a newly created table (always free, no open order).
type Table struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AreaID    int64  `json:"areaId"`
	Status    string `json:"status"`
	OpenOrder any    `json:"openOrder"`
}

// Area is the JSON shape of an area.
type Area struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Tables []Table `json:"tables"`
}

type areaReq struct {
	BranchID *int64 `json:"branchId"`
	Name     string `json:"name"`
}

func (h *handler) createArea(c *gin.Context) {
	p := auth.From(c)
	var req areaReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	// Branch managers act on their own branch only; naming another branch is refused (B-07).
	if p.BranchID != nil && req.BranchID != nil && *req.BranchID != *p.BranchID {
		httpx.Fail(c, apperr.ErrForbidden)
		return
	}
	branchID, err := auth.ResolveBranch(p, req.BranchID, true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.Fail(c, invalid("name"))
		return
	}
	ctx := c.Request.Context()
	var a store.Area
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		if _, err := q.GetBranch(ctx, *branchID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return invalid("branchId")
			}
			return err
		}
		a, err = q.BranchesCreateArea(ctx, store.BranchesCreateAreaParams{BranchID: *branchID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return errAreaTaken
		}
		if err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: branchID, Action: "area.create", EntityType: "area", EntityID: strconv.FormatInt(a.ID, 10),
			After: map[string]any{"name": a.Name},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, Area{ID: a.ID, Name: a.Name, Tables: []Table{}})
}

type tableReq struct {
	AreaID int64  `json:"areaId" binding:"required"`
	Name   string `json:"name"`
}

func (h *handler) createTable(c *gin.Context) {
	p := auth.From(c)
	var req tableReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.Fail(c, invalid("name"))
		return
	}
	ctx := c.Request.Context()
	var t store.DiningTable
	err := store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		a, err := q.BranchesGetArea(ctx, req.AreaID)
		if err != nil {
			return err
		}
		if err := auth.CheckBranch(p, a.BranchID); err != nil {
			return err
		}
		t, err = q.BranchesCreateTable(ctx, store.BranchesCreateTableParams{BranchID: a.BranchID, AreaID: a.ID, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return errTableTaken
		}
		if err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &a.BranchID, Action: "table.create", EntityType: "table", EntityID: strconv.FormatInt(t.ID, 10),
			After: map[string]any{"name": t.Name, "areaId": a.ID},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, Table{ID: t.ID, Name: t.Name, AreaID: t.AreaID, Status: "free", OpenOrder: nil})
}
