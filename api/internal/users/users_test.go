package users_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/seed"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type user struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Email        *string `json:"email"`
	Role         string  `json:"role"`
	BranchID     *int64  `json:"branchId"`
	Status       string  `json:"status"`
	HasPin       bool    `json:"hasPin"`
	LastActiveAt *string `json:"lastActiveAt"`
}

func path(id int64, suffix string) string {
	return "/api/v1/users/" + strconv.FormatInt(id, 10) + suffix
}

func countAudit(t *testing.T, env *testutil.Env, action string) int {
	var n int
	if err := env.Pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_logs WHERE action = $1", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestA06_LockUserRevokesSessionsImmediately(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	tn1.Get("/api/v1/auth/me").MustStatus(t, 200)
	other := env.Tn1() // a second session of the same user
	ql1 := env.Ql1()

	ql1.Post(path(env.Fx.Users.Tn1, "/lock"), nil).MustStatus(t, 204)
	tn1.Get("/api/v1/auth/me").MustStatus(t, 401)
	other.Get("/api/v1/auth/me").MustStatus(t, 401)

	// Cannot log in again with PIN.
	tablet := env.Anon()
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 401)
	// Locked user is no longer offered on the device.
	var di struct {
		Staff []struct{ ID int64 } `json:"staff"`
	}
	tablet.Get("/api/v1/device").MustStatus(t, 200).JSON(t, &di)
	for _, s := range di.Staff {
		if s.ID == env.Fx.Users.Tn1 {
			t.Fatal("locked user still in device staff list")
		}
	}

	var found bool
	for _, e := range env.Events.Events {
		if e.Type == "user.locked" && e.BranchID == env.Fx.Branches[1] {
			found = true
		}
	}
	if !found {
		t.Fatalf("no user.locked event: %+v", env.Events.Events)
	}
	if countAudit(t, env, "user.lock") != 1 {
		t.Fatal("no user.lock audit")
	}
	var list []user
	ql1.Get("/api/v1/users").MustStatus(t, 200).JSON(t, &list)
	for _, u := range list {
		if u.ID == env.Fx.Users.Tn1 && u.Status != "disabled" {
			t.Fatalf("status = %s", u.Status)
		}
	}

	// Unlock: PIN login works again.
	ql1.Post(path(env.Fx.Users.Tn1, "/unlock"), nil).MustStatus(t, 204)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)

	// Locking a password user also blocks login.
	env.Owner().Post(path(env.Fx.Users.Ql2, "/lock"), nil).MustStatus(t, 204)
	r := env.Anon().Post("/api/v1/auth/login", map[string]any{"email": "ql2@moc.vn", "password": seed.DefaultPassword})
	if r.Status == 200 {
		t.Fatal("locked user could log in")
	}
	// Nobody can lock themselves.
	ql1.Post(path(env.Fx.Users.Ql1, "/lock"), nil).MustStatus(t, 400)
}

func TestA09_CashierCannotCallAdminAPIs(t *testing.T) {
	env := testutil.NewEnv(t)
	tn1 := env.Tn1()
	tn1.Get("/api/v1/users").MustStatus(t, 403)
	tn1.Post("/api/v1/users", map[string]any{"name": "X", "role": "cashier", "pin": "1357"}).MustStatus(t, 403)
	tn1.Post(path(env.Fx.Users.Pc1, "/lock"), nil).MustStatus(t, 403)
	tn1.Post(path(env.Fx.Users.Pc1, "/reset-pin"), map[string]any{"pin": "1357"}).MustStatus(t, 403)
	tn1.Put("/api/v1/settings", map[string]any{}).MustStatus(t, 403)
	tn1.Post("/api/v1/branches", map[string]any{"code": "CN9", "name": "x"}).MustStatus(t, 403)
	tn1.Post("/api/v1/areas", map[string]any{"name": "Tầng 3"}).MustStatus(t, 403)
	tn1.Get("/api/v1/devices").MustStatus(t, 403)
}

func TestA11_ManagerCannotCreateStaffForOtherBranch(t *testing.T) {
	env := testutil.NewEnv(t)
	ql1 := env.As(env.Fx.Users.Ql1)
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]

	r := ql1.Post("/api/v1/users", map[string]any{"name": "Gián điệp", "role": "cashier", "branchId": cn2, "pin": "1357"})
	r.MustStatus(t, 403)
	var n int
	_ = env.Pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE name = 'Gián điệp'").Scan(&n)
	if n != 0 {
		t.Fatal("user created")
	}
	// Cannot create chain-level roles either.
	ql1.Post("/api/v1/users", map[string]any{"name": "X", "role": "owner", "email": "x@moc.vn", "password": "matkhau123"}).MustStatus(t, 403)
	ql1.Post("/api/v1/users", map[string]any{"name": "X", "role": "warehouse_manager", "email": "x@moc.vn", "password": "matkhau123"}).MustStatus(t, 403)
	// Cannot see or change CN2 staff (404).
	ql1.Put(path(env.Fx.Users.Tn2, ""), map[string]any{"name": "Đổi tên", "role": "cashier"}).MustStatus(t, 404)
	ql1.Post(path(env.Fx.Users.Tn2, "/lock"), nil).MustStatus(t, 404)
	ql1.Post(path(env.Fx.Users.Tn2, "/reset-pin"), map[string]any{"pin": "1357"}).MustStatus(t, 404)
	ql1.Post(path(env.Fx.Users.Owner, "/lock"), nil).MustStatus(t, 404)
	// Cannot move own staff to CN2.
	ql1.Put(path(env.Fx.Users.Tn1, ""), map[string]any{"name": "Lan", "role": "cashier", "branchId": cn2}).MustStatus(t, 403)

	// Own branch works; branchId may be omitted.
	var u user
	ql1.Post("/api/v1/users", map[string]any{"name": "Hoa", "role": "barista", "pin": "1357"}).MustStatus(t, 201).JSON(t, &u)
	if u.BranchID == nil || *u.BranchID != cn1 || u.Role != "barista" || !u.HasPin || u.Status != "active" || u.Email != nil {
		t.Fatalf("user = %+v", u)
	}
	ql1.Post("/api/v1/users", map[string]any{"name": "Hà", "role": "cashier", "branchId": cn1, "pin": "2468"}).MustStatus(t, 201)
	var list []user
	ql1.Get("/api/v1/users?branchId="+strconv.FormatInt(cn2, 10)).MustStatus(t, 200).JSON(t, &list)
	for _, x := range list {
		if x.BranchID == nil || *x.BranchID != cn1 {
			t.Fatalf("ql1 sees user outside CN1: %+v", x)
		}
	}
	if countAudit(t, env, "user.create") != 2 {
		t.Fatal("user.create audit missing")
	}
}

// A-10 for the modules owned here: another branch's users, devices and areas answer 404, not 403.
func TestA10_OtherBranchDataIs404(t *testing.T) {
	env := testutil.NewEnv(t)
	ql1 := env.As(env.Fx.Users.Ql1)
	for _, r := range []*testutil.Resp{
		ql1.Put(path(env.Fx.Users.Tn2, ""), map[string]any{"name": "X", "role": "cashier"}),
		ql1.Post(path(env.Fx.Users.Tn2, "/unlock"), nil),
		ql1.Post(path(env.Fx.Users.Ql2, "/lock"), nil),
		ql1.Post("/api/v1/devices/"+strconv.FormatInt(env.Fx.DeviceCN2, 10)+"/revoke", nil),
		ql1.Post(path(999999, "/lock"), nil),
	} {
		if r.Status != 404 || r.ErrCode() != "NOT_FOUND" {
			t.Errorf("status %d %s, want 404", r.Status, r.Body)
		}
	}
	env.Tn2().Get("/api/v1/auth/me").MustStatus(t, 200)
}

func TestA13_OwnerSeesAllBranches(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	var list []user
	owner.Get("/api/v1/users").MustStatus(t, 200).JSON(t, &list)
	seen := map[int64]bool{}
	for _, u := range list {
		seen[u.ID] = true
	}
	if !seen[env.Fx.Users.Tn1] || !seen[env.Fx.Users.Tn2] || !seen[env.Fx.Users.Owner] {
		t.Fatalf("owner users = %+v", list)
	}
	owner.Get("/api/v1/users?branchId="+strconv.FormatInt(env.Fx.Branches[2], 10)).MustStatus(t, 200).JSON(t, &list)
	for _, u := range list {
		if u.BranchID == nil || *u.BranchID != env.Fx.Branches[2] {
			t.Fatalf("filter: %+v", u)
		}
	}
	if len(list) != 2 {
		t.Fatalf("CN2 users = %+v", list)
	}
	var branches []struct{ ID int64 }
	owner.Get("/api/v1/branches").MustStatus(t, 200).JSON(t, &branches)
	if len(branches) != 5 {
		t.Fatalf("owner branches = %d", len(branches))
	}
	env.As(env.Fx.Users.Ql1).Get("/api/v1/branches").MustStatus(t, 200).JSON(t, &branches)
	if len(branches) != 1 || branches[0].ID != env.Fx.Branches[1] {
		t.Fatalf("ql1 branches = %+v", branches)
	}
	// Owner manages staff of any branch.
	owner.Post(path(env.Fx.Users.Tn2, "/reset-pin"), map[string]any{"pin": "1357"}).MustStatus(t, 204)
}

func TestD13_PINRulesOnCreateAndReset(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]
	for _, pin := range []string{"123", "1234567", "12a4", ""} {
		r := owner.Post("/api/v1/users", map[string]any{"name": "X", "role": "cashier", "branchId": cn1, "pin": pin})
		if r.Status != 400 {
			t.Errorf("pin %q: status %d %s", pin, r.Status, r.Body)
		}
	}
	// 1234 belongs to tn1 in CN1 → taken in CN1, fine in CN2.
	r := owner.Post("/api/v1/users", map[string]any{"name": "X", "role": "cashier", "branchId": cn1, "pin": "1234"})
	if r.Status != 409 || r.ErrCode() != "PIN_TAKEN" {
		t.Fatalf("dup pin: %d %s", r.Status, r.Body)
	}
	owner.Post("/api/v1/users", map[string]any{"name": "Y", "role": "cashier", "branchId": cn2, "pin": "1234"}).MustStatus(t, 201)
	// 6 digits OK.
	owner.Post("/api/v1/users", map[string]any{"name": "Z", "role": "barista", "branchId": cn1, "pin": "135790"}).MustStatus(t, 201)

	// Reset: taken by another user → 409; own PIN or a new one → 204 and must change.
	ql1 := env.As(env.Fx.Users.Ql1)
	if r := ql1.Post(path(env.Fx.Users.Tn1, "/reset-pin"), map[string]any{"pin": "4321"}); r.Status != 409 || r.ErrCode() != "PIN_TAKEN" {
		t.Fatalf("reset to pc1 pin: %d %s", r.Status, r.Body)
	}
	ql1.Post(path(env.Fx.Users.Tn1, "/reset-pin"), map[string]any{"pin": "12"}).MustStatus(t, 400)
	ql1.Post(path(env.Fx.Users.Tn1, "/reset-pin"), map[string]any{"pin": "7777"}).MustStatus(t, 204)
	tablet := env.Anon()
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 401)
	var m struct {
		User struct {
			PinMustChange bool `json:"pinMustChange"`
		} `json:"user"`
	}
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "7777"}).MustStatus(t, 200).JSON(t, &m)
	if !m.User.PinMustChange {
		t.Fatal("pinMustChange not set after reset")
	}
	// A disabled user's PIN does not block reuse.
	ql1.Post(path(env.Fx.Users.Pc1, "/lock"), nil).MustStatus(t, 204)
	ql1.Post("/api/v1/users", map[string]any{"name": "Mới", "role": "barista", "pin": "4321"}).MustStatus(t, 201)
}

func TestUsers_CreateValidation(t *testing.T) {
	env := testutil.NewEnv(t)
	owner := env.Owner()
	cn1 := env.Fx.Branches[1]
	// Manager needs email + password (≥ 8 chars).
	owner.Post("/api/v1/users", map[string]any{"name": "QL", "role": "branch_manager", "branchId": cn1}).MustStatus(t, 400)
	owner.Post("/api/v1/users", map[string]any{"name": "QL", "role": "branch_manager", "branchId": cn1, "email": "m@moc.vn", "password": "short"}).MustStatus(t, 400)
	// Cashier needs a PIN and a branch.
	owner.Post("/api/v1/users", map[string]any{"name": "TN", "role": "cashier", "branchId": cn1}).MustStatus(t, 400)
	if r := owner.Post("/api/v1/users", map[string]any{"name": "TN", "role": "cashier", "pin": "1357"}); r.Status != 400 {
		t.Fatalf("no branch: %d", r.Status)
	}
	owner.Post("/api/v1/users", map[string]any{"name": "", "role": "cashier", "branchId": cn1, "pin": "1357"}).MustStatus(t, 400)
	owner.Post("/api/v1/users", map[string]any{"name": "X", "role": "boss", "branchId": cn1, "pin": "1357"}).MustStatus(t, 400)
	// Duplicate email (case-insensitive).
	if r := owner.Post("/api/v1/users", map[string]any{"name": "QL", "role": "branch_manager", "branchId": cn1, "email": "QL1@moc.vn", "password": "matkhau123"}); r.Status != 409 || r.ErrCode() != "EMAIL_TAKEN" {
		t.Fatalf("dup email: %d %s", r.Status, r.Body)
	}
	// Owner creates a manager who can then log in.
	var u user
	owner.Post("/api/v1/users", map[string]any{"name": "QL3", "role": "branch_manager", "branchId": env.Fx.Branches[3], "email": "QL3@moc.vn", "password": "matkhau123"}).MustStatus(t, 201).JSON(t, &u)
	if u.Email == nil || *u.Email != "ql3@moc.vn" || u.HasPin {
		t.Fatalf("user = %+v", u)
	}
	env.Anon().Post("/api/v1/auth/login", map[string]any{"email": "ql3@moc.vn", "password": "matkhau123"}).MustStatus(t, 200)
	// Owner-level users have no branch.
	owner.Post("/api/v1/users", map[string]any{"name": "Kho 2", "role": "warehouse_manager", "email": "kho2@moc.vn", "password": "matkhau123"}).MustStatus(t, 201).JSON(t, &u)
	if u.BranchID != nil {
		t.Fatalf("warehouse manager has branch %v", *u.BranchID)
	}

	// Update keeps hashes when password/pin are omitted.
	var up user
	owner.Put(path(env.Fx.Users.Tn1, ""), map[string]any{"name": "Lan Anh", "role": "cashier", "branchId": cn1}).MustStatus(t, 200).JSON(t, &up)
	if up.Name != "Lan Anh" || !up.HasPin {
		t.Fatalf("updated = %+v", up)
	}
	tablet := env.Anon()
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)
	if countAudit(t, env, "user.update") != 1 {
		t.Fatal("user.update audit missing")
	}
}
