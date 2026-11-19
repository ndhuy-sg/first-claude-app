package menu_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type size struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	PriceDelta int64  `json:"priceDelta"`
}

type modifier struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

type menuProduct struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	BasePrice   int64      `json:"basePrice"`
	IsFeatured  bool       `json:"isFeatured"`
	SoldOut     bool       `json:"soldOut"`
	Sizes       []size     `json:"sizes"`
	Modifiers   []modifier `json:"modifiers"`
}

type menu struct {
	Categories []struct {
		ID       int64         `json:"id"`
		Name     string        `json:"name"`
		Products []menuProduct `json:"products"`
	} `json:"categories"`
}

type category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sortOrder"`
}

type product struct {
	ID               int64      `json:"id"`
	CategoryID       int64      `json:"categoryId"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	BasePrice        int64      `json:"basePrice"`
	IsActive         bool       `json:"isActive"`
	IsFeatured       bool       `json:"isFeatured"`
	SortOrder        int        `json:"sortOrder"`
	Sizes            []size     `json:"sizes"`
	Modifiers        []modifier `json:"modifiers"`
	SoldOutBranchIDs []int64    `json:"soldOutBranchIds"`
}

func getMenu(t *testing.T, c *testutil.Client, path string) menu {
	t.Helper()
	var m menu
	c.Get(path).MustStatus(t, 200).JSON(t, &m)
	return m
}

func findProduct(m menu, id int64) *menuProduct {
	for _, c := range m.Categories {
		for i := range c.Products {
			if c.Products[i].ID == id {
				return &c.Products[i]
			}
		}
	}
	return nil
}

func listProducts(t *testing.T, c *testutil.Client) []product {
	t.Helper()
	var ps []product
	c.Get("/api/v1/products").MustStatus(t, 200).JSON(t, &ps)
	return ps
}

func findAdminProduct(ps []product, id int64) *product {
	for i := range ps {
		if ps[i].ID == id {
			return &ps[i]
		}
	}
	return nil
}

func countAudit(t *testing.T, env *testutil.Env, action, entityID string) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_id = $2`, action, entityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func categoryID(t *testing.T, env *testutil.Env, name string) int64 {
	t.Helper()
	var id int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT id FROM categories WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("category %s: %v", name, err)
	}
	return id
}

func soldOutPath(branchID, productID int64) string {
	return fmt.Sprintf("/api/v1/branches/%d/sold-out/%d", branchID, productID)
}

// B-06 (now: GET /menu is for the POS, requires a session) + menu shape and order.
func TestB06_MenuRequiresAuthAndShape(t *testing.T) {
	env := testutil.NewEnv(t)
	if r := env.Anon().Get("/api/v1/menu"); r.Status != 401 || r.ErrCode() != "UNAUTHENTICATED" {
		t.Fatalf("anon: status %d code %s", r.Status, r.ErrCode())
	}

	m := getMenu(t, env.Tn1(), "/api/v1/menu")
	var names []string
	for _, c := range m.Categories {
		names = append(names, c.Name)
	}
	if want := []string{"Cà phê", "Trà", "Đá xay", "Bánh"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("categories = %v, want %v", names, want)
	}
	if got := m.Categories[0].Products[0].Name; got != "Cà phê đen" {
		t.Fatalf("first product = %q, want Cà phê đen (sort order)", got)
	}
	if n := len(m.Categories[0].Products); n != 8 {
		t.Fatalf("Cà phê has %d products, want 8", n)
	}

	p := findProduct(m, env.Fx.Products.CaPheSua)
	if p == nil {
		t.Fatal("Cà phê sữa missing")
	}
	if p.BasePrice != 29_000 || !p.IsFeatured || p.SoldOut {
		t.Fatalf("Cà phê sữa = %+v", p)
	}
	if want := []size{{"M", "M", 0}, {"L", "L", 6_000}}; !reflect.DeepEqual(p.Sizes, want) {
		t.Fatalf("sizes = %+v", p.Sizes)
	}
	wantMods := []modifier{
		{env.Fx.Modifiers.ThemShot, "Thêm shot", 8_000},
		{env.Fx.Modifiers.ItDa, "Ít đá", 0},
		{env.Fx.Modifiers.KhongDa, "Không đá", 0},
		{env.Fx.Modifiers.ItNgot, "Ít ngọt", 0},
	}
	if !reflect.DeepEqual(p.Modifiers, wantMods) {
		t.Fatalf("modifiers = %+v", p.Modifiers)
	}
	// Croissant has no sizes and no modifiers: lists must be [] not null.
	r := env.Tn1().Get("/api/v1/menu")
	var raw struct {
		Categories []struct {
			Products []map[string]any `json:"products"`
		} `json:"categories"`
	}
	r.JSON(t, &raw)
	for _, c := range raw.Categories {
		for _, pr := range c.Products {
			if pr["sizes"] == nil || pr["modifiers"] == nil {
				t.Fatalf("null list in %v", pr)
			}
		}
	}
}

// B-03: one price list for the chain.
func TestB03_SamePriceAllBranches(t *testing.T) {
	env := testutil.NewEnv(t)
	cn1 := getMenu(t, env.Tn1(), "/api/v1/menu")
	cn2 := getMenu(t, env.Tn2(), "/api/v1/menu")
	if !reflect.DeepEqual(cn1, cn2) {
		t.Fatalf("CN1 and CN2 menus differ")
	}
	owner := getMenu(t, env.Owner(), fmt.Sprintf("/api/v1/menu?branchId=%d", env.Fx.Branches[2]))
	if !reflect.DeepEqual(cn1, owner) {
		t.Fatalf("owner view of CN2 differs from CN1")
	}
}

// B-04 (sold-out half): ql1 marks Cà phê sữa sold out at CN1; CN2 still has it.
func TestB04_SoldOutPerBranch(t *testing.T) {
	env := testutil.NewEnv(t)
	cn1, cn2, pid := env.Fx.Branches[1], env.Fx.Branches[2], env.Fx.Products.CaPheSua

	env.Ql1().Put(soldOutPath(cn1, pid), map[string]any{"soldOut": true}).MustStatus(t, 204)

	if p := findProduct(getMenu(t, env.Tn1(), "/api/v1/menu"), pid); p == nil || !p.SoldOut {
		t.Fatalf("CN1: %+v, want soldOut", p)
	}
	if p := findProduct(getMenu(t, env.Tn2(), "/api/v1/menu"), pid); p == nil || p.SoldOut {
		t.Fatalf("CN2: %+v, want available", p)
	}
	// a branch-bound user cannot peek at another branch with ?branchId
	if p := findProduct(getMenu(t, env.Tn2(), fmt.Sprintf("/api/v1/menu?branchId=%d", cn1)), pid); p.SoldOut {
		t.Fatal("tn2 with ?branchId=CN1 must still see CN2")
	}
	owner := env.Owner()
	if p := findProduct(getMenu(t, owner, fmt.Sprintf("/api/v1/menu?branchId=%d", cn1)), pid); !p.SoldOut {
		t.Fatal("owner ?branchId=CN1: want soldOut")
	}
	if p := findProduct(getMenu(t, owner, fmt.Sprintf("/api/v1/menu?branchId=%d", cn2)), pid); p.SoldOut {
		t.Fatal("owner ?branchId=CN2: want available")
	}
	if p := findProduct(getMenu(t, owner, "/api/v1/menu"), pid); p.SoldOut {
		t.Fatal("owner without branchId: soldOut must be false")
	}
	if p := findAdminProduct(listProducts(t, owner), pid); !reflect.DeepEqual(p.SoldOutBranchIDs, []int64{cn1}) {
		t.Fatalf("soldOutBranchIds = %v", p.SoldOutBranchIDs)
	}

	// audit + event
	if n := countAudit(t, env, "product.sold_out", fmt.Sprint(pid)); n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
	var auditBranch int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT branch_id FROM audit_logs WHERE action = 'product.sold_out'`).Scan(&auditBranch); err != nil || auditBranch != cn1 {
		t.Fatalf("audit branch = %d (%v)", auditBranch, err)
	}
	found := false
	for _, e := range env.Events.Events {
		if e.Type == "product.sold_out" && e.BranchID == cn1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no product.sold_out event for CN1: %+v", env.Events.Events)
	}

	// setting the same value again changes nothing
	env.Ql1().Put(soldOutPath(cn1, pid), map[string]any{"soldOut": true}).MustStatus(t, 204)
	if n := countAudit(t, env, "product.sold_out", fmt.Sprint(pid)); n != 1 {
		t.Fatalf("repeat toggle: audit rows = %d, want 1", n)
	}

	// back in stock
	env.Ql1().Put(soldOutPath(cn1, pid), map[string]any{"soldOut": false}).MustStatus(t, 204)
	if p := findProduct(getMenu(t, env.Tn1(), "/api/v1/menu"), pid); p.SoldOut {
		t.Fatal("CN1 after un-toggle: want available")
	}
	if n := countAudit(t, env, "product.sold_out", fmt.Sprint(pid)); n != 2 {
		t.Fatalf("audit rows = %d, want 2", n)
	}

	// owner can toggle any branch
	owner.Put(soldOutPath(cn2, pid), map[string]any{"soldOut": true}).MustStatus(t, 204)
	if p := findProduct(getMenu(t, env.Tn2(), "/api/v1/menu"), pid); !p.SoldOut {
		t.Fatal("CN2 after owner toggle: want soldOut")
	}
	// barista may toggle in own branch
	pc1 := env.AsOnDevice(env.Fx.Users.Pc1, env.Fx.DeviceCN1, env.Fx.DeviceCN1Token)
	pc1.Put(soldOutPath(cn1, env.Fx.Products.TraDao), map[string]any{"soldOut": true}).MustStatus(t, 204)
}

func TestB04_SoldOutPermissions(t *testing.T) {
	env := testutil.NewEnv(t)
	cn1, cn2, pid := env.Fx.Branches[1], env.Fx.Branches[2], env.Fx.Products.CaPheSua

	// ql1 cannot touch CN2: 404 (do not reveal other branches)
	if r := env.Ql1().Put(soldOutPath(cn2, pid), map[string]any{"soldOut": true}); r.Status != 404 {
		t.Fatalf("ql1 → CN2: %d %s", r.Status, r.Body)
	}
	// cashier has no sold-out permission
	if r := env.Tn1().Put(soldOutPath(cn1, pid), map[string]any{"soldOut": true}); r.Status != 403 || r.ErrCode() != "FORBIDDEN" {
		t.Fatalf("tn1: %d %s", r.Status, r.Body)
	}
	if r := env.Anon().Put(soldOutPath(cn1, pid), map[string]any{"soldOut": true}); r.Status != 401 {
		t.Fatalf("anon: %d", r.Status)
	}
	// unknown product / branch → 404
	env.Ql1().Put(soldOutPath(cn1, 999999), map[string]any{"soldOut": true}).MustStatus(t, 404)
	env.Owner().Put(soldOutPath(999999, pid), map[string]any{"soldOut": true}).MustStatus(t, 404)
	// missing body field → 400
	if r := env.Ql1().Put(soldOutPath(cn1, pid), map[string]any{}); r.Status != 400 || r.ErrCode() != "VALIDATION_ERROR" {
		t.Fatalf("missing soldOut: %d %s", r.Status, r.Body)
	}
	var n int
	_ = env.Pool.QueryRow(t.Context(), `SELECT count(*) FROM branch_sold_out`).Scan(&n)
	if n != 0 {
		t.Fatalf("branch_sold_out rows = %d, want 0", n)
	}
}

// B-02: owner creates and edits categories, products, sizes and modifiers.
func TestB02_OwnerEditsMenu(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()

	// category
	var cat category
	owner.Post("/api/v1/categories", map[string]any{"name": "Nước ép", "sortOrder": 5}).MustStatus(t, 201).JSON(t, &cat)
	if cat.ID == 0 || cat.Name != "Nước ép" || cat.SortOrder != 5 {
		t.Fatalf("category = %+v", cat)
	}
	owner.Put(fmt.Sprintf("/api/v1/categories/%d", cat.ID), map[string]any{"name": "Nước ép tươi", "sortOrder": 4}).MustStatus(t, 200).JSON(t, &cat)
	if cat.Name != "Nước ép tươi" || cat.SortOrder != 4 {
		t.Fatalf("updated category = %+v", cat)
	}
	if r := owner.Post("/api/v1/categories", map[string]any{"name": ""}); r.Status != 400 || r.ErrCode() != "VALIDATION_ERROR" {
		t.Fatalf("empty category name: %d %s", r.Status, r.Body)
	}
	owner.Put("/api/v1/categories/999999", map[string]any{"name": "X"}).MustStatus(t, 404)

	// product with sizes and modifiers
	in := map[string]any{
		"categoryId": cat.ID, "name": "Nước cam", "description": "Cam vắt", "basePrice": 30_000, "isFeatured": true,
		"sizes":     []map[string]any{{"code": "M", "name": "Vừa", "priceDelta": 0}, {"code": "L", "name": "Lớn", "priceDelta": 5_000}},
		"modifiers": []map[string]any{{"name": "Thêm mật ong", "price": 5_000}, {"name": "Ít đá", "price": 0}},
	}
	var p product
	owner.Post("/api/v1/products", in).MustStatus(t, 201).JSON(t, &p)
	if p.ID == 0 || p.CategoryID != cat.ID || p.Name != "Nước cam" || p.Description != "Cam vắt" || p.BasePrice != 30_000 ||
		!p.IsActive || !p.IsFeatured || len(p.Sizes) != 2 || len(p.Modifiers) != 2 || p.SoldOutBranchIDs == nil {
		t.Fatalf("created product = %+v", p)
	}
	if p.Sizes[1] != (size{"L", "Lớn", 5_000}) || p.Modifiers[0].Name != "Thêm mật ong" || p.Modifiers[0].ID == 0 {
		t.Fatalf("created sizes/modifiers = %+v %+v", p.Sizes, p.Modifiers)
	}
	honey, itDa := p.Modifiers[0].ID, p.Modifiers[1].ID

	// it appears on every branch's POS menu
	for _, c := range []*testutil.Client{env.Tn1(), env.Tn2()} {
		if mp := findProduct(getMenu(t, c, "/api/v1/menu"), p.ID); mp == nil || mp.BasePrice != 30_000 {
			t.Fatalf("new product not on menu: %+v", mp)
		}
	}

	// duplicate name in the same category → 409; in another category → ok
	if r := owner.Post("/api/v1/products", map[string]any{"categoryId": cat.ID, "name": "Nước cam", "basePrice": 1}); r.Status != 409 || r.ErrCode() != "PRODUCT_NAME_TAKEN" {
		t.Fatalf("duplicate: %d %s", r.Status, r.Body)
	}
	var other product
	owner.Post("/api/v1/products", map[string]any{"categoryId": categoryID(t, env, "Trà"), "name": "Nước cam", "basePrice": 0}).MustStatus(t, 201).JSON(t, &other)
	if other.BasePrice != 0 || other.Sizes == nil || other.Modifiers == nil {
		t.Fatalf("free product = %+v", other)
	}

	// validation: price must be an integer ≥ 0
	bad := []map[string]any{
		{"categoryId": cat.ID, "name": "Âm", "basePrice": -1},
		{"categoryId": cat.ID, "name": "Lẻ", "basePrice": 1.5},
		{"categoryId": cat.ID, "name": "", "basePrice": 1},
		{"categoryId": cat.ID, "name": "Thiếu giá"},
		{"categoryId": 999999, "name": "Không nhóm", "basePrice": 1},
		{"categoryId": cat.ID, "name": "Size âm", "basePrice": 1, "sizes": []map[string]any{{"code": "L", "name": "L", "priceDelta": -1}}},
		{"categoryId": cat.ID, "name": "Size trùng", "basePrice": 1, "sizes": []map[string]any{{"code": "L", "name": "L"}, {"code": "L", "name": "L2"}}},
		{"categoryId": cat.ID, "name": "Tuỳ chọn âm", "basePrice": 1, "modifiers": []map[string]any{{"name": "X", "price": -5}}},
		{"categoryId": cat.ID, "name": "Tuỳ chọn trùng", "basePrice": 1, "modifiers": []map[string]any{{"name": "X", "price": 0}, {"name": "X", "price": 1}}},
	}
	for _, b := range bad {
		if r := owner.Post("/api/v1/products", b); r.Status != 400 || r.ErrCode() != "VALIDATION_ERROR" {
			t.Errorf("%v: %d %s", b, r.Status, r.Body)
		}
	}

	// update: rename, new price, change L delta, keep honey (new price), drop ít đá, add a new modifier
	up := map[string]any{
		"categoryId": cat.ID, "name": "Nước cam vắt", "basePrice": 32_000,
		"sizes":     []map[string]any{{"code": "M", "name": "Vừa", "priceDelta": 0}, {"code": "L", "name": "Lớn", "priceDelta": 7_000}},
		"modifiers": []map[string]any{{"id": honey, "name": "Thêm mật ong", "price": 6_000}, {"name": "Thêm chanh", "price": 0}},
	}
	owner.Put(fmt.Sprintf("/api/v1/products/%d", p.ID), up).MustStatus(t, 200).JSON(t, &p)
	if p.Name != "Nước cam vắt" || p.BasePrice != 32_000 || p.Sizes[1].PriceDelta != 7_000 || len(p.Modifiers) != 2 {
		t.Fatalf("updated = %+v", p)
	}
	if p.Modifiers[0].ID != honey || p.Modifiers[0].Price != 6_000 || p.Modifiers[1].Name != "Thêm chanh" || p.Modifiers[1].ID == itDa {
		t.Fatalf("modifiers after update = %+v", p.Modifiers)
	}
	// removed modifier is deactivated, not deleted (old order snapshots stay valid)
	var active bool
	if err := env.Pool.QueryRow(t.Context(), `SELECT is_active FROM modifiers WHERE id = $1`, itDa).Scan(&active); err != nil || active {
		t.Fatalf("ít đá: active=%v err=%v", active, err)
	}
	// price change is audited with before/after
	if n := countAudit(t, env, "product.price", fmt.Sprint(p.ID)); n != 1 {
		t.Fatalf("product.price audit rows = %d, want 1", n)
	}
	var before, after map[string]any
	var actor int64
	if err := env.Pool.QueryRow(t.Context(), `SELECT actor_id, before, after FROM audit_logs WHERE action = 'product.price'`).Scan(&actor, &before, &after); err != nil {
		t.Fatal(err)
	}
	if actor != env.Fx.Users.Owner || before["basePrice"] != float64(30_000) || after["basePrice"] != float64(32_000) {
		t.Fatalf("audit actor=%d before=%v after=%v", actor, before, after)
	}

	// a rename without price change is not a price audit
	up["name"] = "Nước cam ép"
	up["modifiers"] = []map[string]any{{"id": honey, "name": "Mật ong", "price": 6_000}, {"id": p.Modifiers[1].ID, "name": "Thêm chanh", "price": 0}}
	owner.Put(fmt.Sprintf("/api/v1/products/%d", p.ID), up).MustStatus(t, 200).JSON(t, &p)
	if n := countAudit(t, env, "product.price", fmt.Sprint(p.ID)); n != 1 {
		t.Fatalf("rename audited as price change: %d rows", n)
	}
	// re-adding a removed modifier by name reactivates it
	up["modifiers"] = []map[string]any{{"name": "Ít đá", "price": 0}}
	owner.Put(fmt.Sprintf("/api/v1/products/%d", p.ID), up).MustStatus(t, 200).JSON(t, &p)
	if len(p.Modifiers) != 1 || p.Modifiers[0].ID != itDa {
		t.Fatalf("re-added modifier = %+v (want id %d)", p.Modifiers, itDa)
	}
	// modifier id of another product → 400
	up["modifiers"] = []map[string]any{{"id": env.Fx.Modifiers.ThemShot, "name": "Thêm shot", "price": 0}}
	if r := owner.Put(fmt.Sprintf("/api/v1/products/%d", p.ID), up); r.Status != 400 {
		t.Fatalf("foreign modifier: %d %s", r.Status, r.Body)
	}
	// rename to a taken name in the same category → 409
	owner.Post("/api/v1/products", map[string]any{"categoryId": cat.ID, "name": "Nước dứa", "basePrice": 30_000}).MustStatus(t, 201)
	up["modifiers"] = []map[string]any{}
	up["name"] = "Nước dứa"
	if r := owner.Put(fmt.Sprintf("/api/v1/products/%d", p.ID), up); r.Status != 409 || r.ErrCode() != "PRODUCT_NAME_TAKEN" {
		t.Fatalf("rename to taken: %d %s", r.Status, r.Body)
	}
	owner.Put("/api/v1/products/999999", up).MustStatus(t, 404)
}

// B-02: changing the price of Cà phê sữa (sizes and modifiers too) is audited.
func TestB02_PriceChangeAudited(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	pid := env.Fx.Products.CaPheSua
	cat := categoryID(t, env, "Cà phê")
	mods := []map[string]any{
		{"id": env.Fx.Modifiers.ThemShot, "name": "Thêm shot", "price": 10_000},
		{"id": env.Fx.Modifiers.ItDa, "name": "Ít đá", "price": 0},
		{"id": env.Fx.Modifiers.KhongDa, "name": "Không đá", "price": 0},
		{"id": env.Fx.Modifiers.ItNgot, "name": "Ít ngọt", "price": 0},
	}
	owner.Put(fmt.Sprintf("/api/v1/products/%d", pid), map[string]any{
		"categoryId": cat, "name": "Cà phê sữa", "basePrice": 29_000, "isFeatured": true,
		"sizes":     []map[string]any{{"code": "M", "name": "M", "priceDelta": 0}, {"code": "L", "name": "L", "priceDelta": 6_000}},
		"modifiers": mods,
	}).MustStatus(t, 200)
	if n := countAudit(t, env, "product.price", fmt.Sprint(pid)); n != 1 {
		t.Fatalf("modifier price change: audit rows = %d, want 1", n)
	}
	m := getMenu(t, env.Tn1(), "/api/v1/menu")
	if p := findProduct(m, pid); p.Modifiers[0].Price != 10_000 || p.Modifiers[0].ID != env.Fx.Modifiers.ThemShot {
		t.Fatalf("menu after change: %+v", p.Modifiers)
	}
}

// B-05: hidden products are not on the POS menu but stay in the admin list.
func TestB05_HiddenProductNotInMenu(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	pid := env.Fx.Products.Croissant
	owner.Put(fmt.Sprintf("/api/v1/products/%d", pid), map[string]any{
		"categoryId": categoryID(t, env, "Bánh"), "name": "Croissant bơ", "basePrice": 35_000, "isActive": false,
	}).MustStatus(t, 200)

	for _, c := range []*testutil.Client{env.Tn1(), env.Tn2(), owner} {
		if p := findProduct(getMenu(t, c, "/api/v1/menu"), pid); p != nil {
			t.Fatalf("hidden product on menu: %+v", p)
		}
	}
	ps := listProducts(t, owner)
	if len(ps) != 18 {
		t.Fatalf("admin list has %d products, want 18", len(ps))
	}
	if p := findAdminProduct(ps, pid); p == nil || p.IsActive {
		t.Fatalf("admin list: %+v", p)
	}
}

// A-09 / B-02: only the owner edits the menu.
func TestB02_MenuEditPermissions(t *testing.T) {
	env := testutil.NewEnv(t)
	cat := categoryID(t, env, "Cà phê")
	body := map[string]any{"categoryId": cat, "name": "Món lạ", "basePrice": 10_000}
	path := fmt.Sprintf("/api/v1/products/%d", env.Fx.Products.CaPheSua)
	for name, c := range map[string]*testutil.Client{"tn1": env.Tn1(), "ql1": env.Ql1(), "kho": env.As(env.Fx.Users.Warehouse)} {
		if r := c.Post("/api/v1/products", body); r.Status != 403 || r.ErrCode() != "FORBIDDEN" {
			t.Errorf("%s create product: %d %s", name, r.Status, r.Body)
		}
		if r := c.Put(path, body); r.Status != 403 {
			t.Errorf("%s update product: %d", name, r.Status)
		}
		if r := c.Post("/api/v1/categories", map[string]any{"name": "X"}); r.Status != 403 {
			t.Errorf("%s create category: %d", name, r.Status)
		}
		if r := c.Put(fmt.Sprintf("/api/v1/categories/%d", cat), map[string]any{"name": "X"}); r.Status != 403 {
			t.Errorf("%s update category: %d", name, r.Status)
		}
	}
	if r := env.Anon().Post("/api/v1/products", body); r.Status != 401 {
		t.Errorf("anon: %d", r.Status)
	}
	// the cashier cannot read the admin list; ql1 can and only sees CN1 sold-out flags
	env.Tn1().Get("/api/v1/products").MustStatus(t, 403)
	env.Owner().Put(soldOutPath(env.Fx.Branches[2], env.Fx.Products.CaPheSua), map[string]any{"soldOut": true}).MustStatus(t, 204)
	if p := findAdminProduct(listProducts(t, env.Ql1()), env.Fx.Products.CaPheSua); len(p.SoldOutBranchIDs) != 0 {
		t.Fatalf("ql1 sees other branch sold-out flags: %v", p.SoldOutBranchIDs)
	}
}
