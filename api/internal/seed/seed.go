// Package seed loads demo data used by tests and local development:
// 5 branches (CN1..CN5), staff, a POS device for CN1 and CN2, the full menu,
// tables and invoice series. It expects an empty, migrated database.
//
// Test fixtures from docs/test-cases/phase-1-pos.md:
//   - owner@moc.vn (owner), kho@moc.vn (warehouse manager)
//   - ql1@moc.vn  branch manager CN1, PIN 9999
//   - tn1         cashier CN1, PIN 1234
//   - pc1         barista CN1, PIN 4321
//   - ql2@moc.vn  branch manager CN2, PIN 8888
//   - tn2         cashier CN2, PIN 5678
//
// Password for every email account: matkhau123
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const DefaultPassword = "matkhau123"

type Options struct {
	BcryptCost int // 0 = bcrypt.DefaultCost; tests pass bcrypt.MinCost for speed
}

type Users struct {
	Owner, Warehouse, Ql1, Tn1, Pc1, Ql2, Tn2 int64
}

type Products struct {
	CaPheSua, CaPheDen, BacXiu, ColdBrew, TraDao, Croissant int64
}

type Modifiers struct {
	ThemShot, ItDa, KhongDa, ItNgot int64 // modifiers of Cà phê sữa
}

// Fixtures holds ids of seeded rows so tests can refer to them.
type Fixtures struct {
	Branches  [6]int64 // index 1..5 = CN1..CN5
	Users     Users
	Products  Products
	Modifiers Modifiers
	// Tables of CN1: index 1..12 = "Bàn 1".."Bàn 12" (Tầng 1). CN2 tables in TablesCN2[1..10].
	TablesCN1 [13]int64
	TablesCN2 [11]int64
	// POS devices with their raw cookie tokens.
	DeviceCN1, DeviceCN2           int64
	DeviceCN1Token, DeviceCN2Token string
	PIN                            map[string]string // user key -> PIN
}

// Device tokens are fixed in seed data so local dev and tests can use them.
const (
	DeviceCN1Token = "dev-device-cn1-token-0000000000000000000000"
	DeviceCN2Token = "dev-device-cn2-token-0000000000000000000000"
)

type product struct {
	cat, name string
	price     int64
	sizes     bool
	mods      string // "cf", "tra", "dx" or ""
	featured  bool
}

var categories = []string{"Cà phê", "Trà", "Đá xay", "Bánh"}

var menu = []product{
	{"Cà phê", "Cà phê đen", 25000, true, "cf", false},
	{"Cà phê", "Cà phê sữa", 29000, true, "cf", true},
	{"Cà phê", "Bạc xỉu", 32000, true, "cf", true},
	{"Cà phê", "Cà phê muối", 35000, true, "cf", true},
	{"Cà phê", "Americano", 35000, true, "cf", false},
	{"Cà phê", "Latte", 45000, true, "cf", false},
	{"Cà phê", "Cappuccino", 45000, true, "cf", false},
	{"Cà phê", "Cold brew", 42000, true, "cf", false},
	{"Trà", "Trà đào cam sả", 42000, true, "tra", true},
	{"Trà", "Trà vải", 39000, true, "tra", false},
	{"Trà", "Trà sen vàng", 45000, true, "tra", false},
	{"Trà", "Trà ô long sữa", 39000, true, "tra", false},
	{"Đá xay", "Cà phê đá xay", 49000, true, "dx", false},
	{"Đá xay", "Matcha đá xay", 52000, true, "dx", false},
	{"Đá xay", "Cookie đá xay", 52000, true, "dx", false},
	{"Bánh", "Croissant bơ", 35000, false, "", false},
	{"Bánh", "Bánh chuối", 29000, false, "", false},
	{"Bánh", "Tiramisu", 45000, false, "", false},
}

var modSets = map[string][]struct {
	name  string
	price int64
}{
	"cf":  {{"Thêm shot", 8000}, {"Ít đá", 0}, {"Không đá", 0}, {"Ít ngọt", 0}},
	"tra": {{"Trân châu trắng", 8000}, {"Ít đá", 0}, {"Ít ngọt", 0}},
	"dx":  {{"Kem cheese", 10000}, {"Ít ngọt", 0}},
}

// Seed inserts demo data inside one transaction and returns the fixture ids.
func Seed(ctx context.Context, pool *pgxpool.Pool, opt Options) (*Fixtures, error) {
	cost := opt.BcryptCost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	hash := func(s string) string {
		h, err := bcrypt.GenerateFromPassword([]byte(s), cost)
		if err != nil {
			panic(err)
		}
		return string(h)
	}
	fx := &Fixtures{PIN: map[string]string{"ql1": "9999", "tn1": "1234", "pc1": "4321", "ql2": "8888", "tn2": "5678"}}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	one := func(dst *int64, sql string, args ...any) {
		if err == nil {
			err = tx.QueryRow(ctx, sql, args...).Scan(dst)
		}
	}

	_, err = tx.Exec(ctx, `UPDATE app_settings SET company_name = 'Chuỗi cà phê Mộc', company_tax_code = '0100000000', tax_gateway = 'off'`)

	for i := 1; i <= 5; i++ {
		one(&fx.Branches[i], `INSERT INTO branches (code, name, address, opening_hours, vat_rate_bp, vietqr_bank_bin, vietqr_account_no, vietqr_account_name)
			VALUES ($1, $2, $3, '06:30 – 22:00', 800, '970436', '0123456789', 'CONG TY MOC') RETURNING id`,
			fmt.Sprintf("CN%d", i), fmt.Sprintf("Mộc – CN%d", i), fmt.Sprintf("Địa chỉ CN%d", i))
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO invoice_series (branch_id, symbol) VALUES ($1, $2)`, fx.Branches[i], fmt.Sprintf("1C26MA%c", 'A'+i-1))
		}
	}
	cn1, cn2 := fx.Branches[1], fx.Branches[2]
	pw := hash(DefaultPassword)

	one(&fx.Users.Owner, `INSERT INTO users (name, email, password_hash, role) VALUES ('Chủ chuỗi', 'owner@moc.vn', $1, 'owner') RETURNING id`, pw)
	one(&fx.Users.Warehouse, `INSERT INTO users (name, email, password_hash, role) VALUES ('Quản lý kho tổng', 'kho@moc.vn', $1, 'warehouse_manager') RETURNING id`, pw)
	one(&fx.Users.Ql1, `INSERT INTO users (name, email, password_hash, pin_hash, role, branch_id) VALUES ('Tuấn', 'ql1@moc.vn', $1, $2, 'branch_manager', $3) RETURNING id`, pw, hash("9999"), cn1)
	one(&fx.Users.Tn1, `INSERT INTO users (name, pin_hash, role, branch_id) VALUES ('Lan', $1, 'cashier', $2) RETURNING id`, hash("1234"), cn1)
	one(&fx.Users.Pc1, `INSERT INTO users (name, pin_hash, role, branch_id) VALUES ('Minh', $1, 'barista', $2) RETURNING id`, hash("4321"), cn1)
	one(&fx.Users.Ql2, `INSERT INTO users (name, email, password_hash, pin_hash, role, branch_id) VALUES ('Quản lý CN2', 'ql2@moc.vn', $1, $2, 'branch_manager', $3) RETURNING id`, pw, hash("8888"), cn2)
	one(&fx.Users.Tn2, `INSERT INTO users (name, pin_hash, role, branch_id) VALUES ('Nam', $1, 'cashier', $2) RETURNING id`, hash("5678"), cn2)

	fx.DeviceCN1Token, fx.DeviceCN2Token = DeviceCN1Token, DeviceCN2Token
	one(&fx.DeviceCN1, `INSERT INTO devices (branch_id, name, token_hash, activated_by) VALUES ($1, 'iPad quầy 1', $2, $3) RETURNING id`, cn1, sha(DeviceCN1Token), fx.Users.Ql1)
	one(&fx.DeviceCN2, `INSERT INTO devices (branch_id, name, token_hash, activated_by) VALUES ($1, 'iPad quầy 1', $2, $3) RETURNING id`, cn2, sha(DeviceCN2Token), fx.Users.Ql2)

	catID := map[string]int64{}
	for i, c := range categories {
		var id int64
		one(&id, `INSERT INTO categories (name, sort_order) VALUES ($1, $2) RETURNING id`, c, i)
		catID[c] = id
	}
	for i, p := range menu {
		var pid int64
		one(&pid, `INSERT INTO products (category_id, name, base_price, is_featured, sort_order) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			catID[p.cat], p.name, p.price, p.featured, i)
		if err != nil {
			break
		}
		if p.sizes {
			_, err = tx.Exec(ctx, `INSERT INTO product_sizes (product_id, code, name, price_delta, sort_order) VALUES ($1, 'M', 'M', 0, 0), ($1, 'L', 'L', 6000, 1)`, pid)
		}
		modIDs := map[string]int64{}
		for j, m := range modSets[p.mods] {
			var mid int64
			one(&mid, `INSERT INTO modifiers (product_id, name, price, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`, pid, m.name, m.price, j)
			modIDs[m.name] = mid
		}
		switch p.name {
		case "Cà phê sữa":
			fx.Products.CaPheSua = pid
			fx.Modifiers = Modifiers{ThemShot: modIDs["Thêm shot"], ItDa: modIDs["Ít đá"], KhongDa: modIDs["Không đá"], ItNgot: modIDs["Ít ngọt"]}
		case "Cà phê đen":
			fx.Products.CaPheDen = pid
		case "Bạc xỉu":
			fx.Products.BacXiu = pid
		case "Cold brew":
			fx.Products.ColdBrew = pid
		case "Trà đào cam sả":
			fx.Products.TraDao = pid
		case "Croissant bơ":
			fx.Products.Croissant = pid
		}
	}

	for i := 1; i <= 5; i++ {
		b := fx.Branches[i]
		areas := [][2]any{{"Tầng 1", 12}, {"Tầng 2", 8}, {"Sân vườn", 6}}
		if i > 1 {
			areas = [][2]any{{"Tầng 1", 10}}
		}
		n := 0
		for ai, a := range areas {
			var areaID int64
			one(&areaID, `INSERT INTO areas (branch_id, name, sort_order) VALUES ($1, $2, $3) RETURNING id`, b, a[0], ai)
			for k := 0; k < a[1].(int); k++ {
				n++
				var tid int64
				one(&tid, `INSERT INTO dining_tables (branch_id, area_id, name, sort_order) VALUES ($1, $2, $3, $4) RETURNING id`, b, areaID, fmt.Sprintf("Bàn %d", n), n)
				if i == 1 && n <= 12 {
					fx.TablesCN1[n] = tid
				}
				if i == 2 && n <= 10 {
					fx.TablesCN2[n] = tid
				}
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return fx, nil
}

var _ pgx.Tx // keep import for readers looking for the tx type
