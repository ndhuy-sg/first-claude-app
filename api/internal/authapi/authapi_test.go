package authapi_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/seed"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type me struct {
	User struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Role          string `json:"role"`
		BranchID      *int64 `json:"branchId"`
		PinMustChange bool   `json:"pinMustChange"`
	} `json:"user"`
	Device *struct {
		ID       int64  `json:"id"`
		BranchID int64  `json:"branchId"`
		Code     string `json:"branchCode"`
	} `json:"device"`
	Settings struct {
		DiscountMaxPct     int `json:"discountMaxPct"`
		PosIdleLockMinutes int `json:"posIdleLockMinutes"`
	} `json:"settings"`
}

type deviceInfo struct {
	Device struct {
		ID         int64   `json:"id"`
		Name       string  `json:"name"`
		BranchID   int64   `json:"branchId"`
		BranchCode string  `json:"branchCode"`
		LastSeenAt *string `json:"lastSeenAt"`
		Revoked    bool    `json:"revoked"`
	} `json:"device"`
	Staff []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	} `json:"staff"`
}

func login(c *testutil.Client, email, pw string) *testutil.Resp {
	return c.Post("/api/v1/auth/login", map[string]any{"email": email, "password": pw})
}

func TestA01_ManagerLoginSetsSessionCookie(t *testing.T) {
	env := testutil.NewEnv(t)
	c := env.Anon()
	r := login(c, "ql1@moc.vn", seed.DefaultPassword).MustStatus(t, 200)
	var m me
	r.JSON(t, &m)
	if m.User.ID != env.Fx.Users.Ql1 || m.User.Role != "branch_manager" || m.User.BranchID == nil || *m.User.BranchID != env.Fx.Branches[1] {
		t.Fatalf("me = %+v", m)
	}
	if m.Settings.DiscountMaxPct != 20 || m.Settings.PosIdleLockMinutes != 15 {
		t.Fatalf("settings = %+v", m.Settings)
	}
	var ck *http.Cookie
	for _, x := range (&http.Response{Header: r.Header}).Cookies() {
		if x.Name == auth.SessionCookie {
			ck = x
		}
	}
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Fatalf("session cookie = %+v", ck)
	}
	// Secure follows cfg.CookieSecure (false in tests; see auth.TestA01_CookieFlags for true).
	if ck.Secure {
		t.Fatalf("Secure set although CookieSecure=false")
	}
	// The cookie works.
	c.Get("/api/v1/auth/me").MustStatus(t, 200)
	// Email is case-insensitive.
	login(env.Anon(), "QL1@Moc.VN", seed.DefaultPassword).MustStatus(t, 200)
}

func TestA01_SecureCookieWhenConfigured(t *testing.T) {
	env := testutil.NewEnv(t)
	env.Deps.Cfg.CookieSecure = true // handlers read cfg per request
	r := login(env.Anon(), "owner@moc.vn", seed.DefaultPassword).MustStatus(t, 200)
	found := false
	for _, x := range (&http.Response{Header: r.Header}).Cookies() {
		if x.Name == auth.SessionCookie {
			found = true
			if !x.Secure || !x.HttpOnly || x.SameSite != http.SameSiteLaxMode {
				t.Fatalf("cookie = %+v", x)
			}
		}
	}
	if !found {
		t.Fatal("no session cookie")
	}
}

func TestA02_WrongPasswordLocksAfterFive(t *testing.T) {
	env := testutil.NewEnv(t)
	c := env.Anon()

	// Unknown email and wrong password give the same code and message.
	unknown := login(c, "nobody@moc.vn", "saimatkhau").MustStatus(t, 401)
	wrong := login(c, "ql1@moc.vn", "saimatkhau").MustStatus(t, 401)
	if unknown.ErrCode() != "INVALID_CREDENTIALS" || wrong.ErrCode() != "INVALID_CREDENTIALS" {
		t.Fatalf("codes %q %q", unknown.ErrCode(), wrong.ErrCode())
	}
	type errBody struct {
		Error struct {
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	var u, w errBody
	unknown.JSON(t, &u)
	wrong.JSON(t, &w)
	if u.Error.Message != w.Error.Message {
		t.Fatalf("messages differ: %q vs %q", u.Error.Message, w.Error.Message)
	}
	low := strings.ToLower(w.Error.Message)
	if strings.Contains(low, "email") && !strings.Contains(low, "mật khẩu") {
		t.Fatalf("message reveals which field is wrong: %q", w.Error.Message)
	}
	if w.Error.Details["remainingAttempts"] != float64(4) {
		t.Fatalf("remainingAttempts = %v", w.Error.Details["remainingAttempts"])
	}

	for i := 2; i <= 4; i++ {
		login(c, "ql1@moc.vn", "saimatkhau").MustStatus(t, 401)
	}
	fifth := login(c, "ql1@moc.vn", "saimatkhau").MustStatus(t, 423)
	if fifth.ErrCode() != "ACCOUNT_LOCKED" {
		t.Fatalf("5th failure code %q", fifth.ErrCode())
	}
	// Even the right password is refused while locked.
	locked := login(c, "ql1@moc.vn", seed.DefaultPassword).MustStatus(t, 423)
	var lb errBody
	locked.JSON(t, &lb)
	want := env.Clock.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339)
	if lb.Error.Details["lockedUntil"] != want {
		t.Fatalf("lockedUntil = %v, want %s", lb.Error.Details["lockedUntil"], want)
	}
	// Other accounts are unaffected.
	login(env.Anon(), "ql2@moc.vn", seed.DefaultPassword).MustStatus(t, 200)

	env.Clock.T = env.Clock.T.Add(14 * time.Minute)
	login(c, "ql1@moc.vn", seed.DefaultPassword).MustStatus(t, 423)
	env.Clock.T = env.Clock.T.Add(2 * time.Minute)
	login(c, "ql1@moc.vn", seed.DefaultPassword).MustStatus(t, 200)

	// A success resets the counter.
	var n int
	if err := env.Pool.QueryRow(context.Background(), "SELECT failed_logins FROM users WHERE id = $1", env.Fx.Users.Ql1).Scan(&n); err != nil || n != 0 {
		t.Fatalf("failed_logins = %d, %v", n, err)
	}
}

func TestA03_ActivateDevice(t *testing.T) {
	env := testutil.NewEnv(t)
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]
	tablet := env.Anon()
	tablet.Get("/api/v1/device").MustStatus(t, 401)

	// ql1 cannot activate a tablet for CN2.
	r := tablet.Post("/api/v1/devices/activate", map[string]any{"email": "ql1@moc.vn", "password": seed.DefaultPassword, "branchId": cn2, "name": "iPad 2"})
	r.MustStatus(t, 403)
	if tablet.Cookie(auth.DeviceCookie) != "" {
		t.Fatal("device cookie set on refusal")
	}
	// Wrong password.
	tablet.Post("/api/v1/devices/activate", map[string]any{"email": "ql1@moc.vn", "password": "x", "branchId": cn1, "name": "iPad 2"}).MustStatus(t, 401)

	r = tablet.Post("/api/v1/devices/activate", map[string]any{"email": "ql1@moc.vn", "password": seed.DefaultPassword, "branchId": cn1, "name": "iPad quầy 2"})
	r.MustStatus(t, 201)
	var di deviceInfo
	r.JSON(t, &di)
	if di.Device.BranchID != cn1 || di.Device.BranchCode != "CN1" || di.Device.Name != "iPad quầy 2" || di.Device.Revoked {
		t.Fatalf("device = %+v", di.Device)
	}
	var ck *http.Cookie
	for _, x := range (&http.Response{Header: r.Header}).Cookies() {
		if x.Name == auth.DeviceCookie {
			ck = x
		}
	}
	if ck == nil || !ck.HttpOnly || ck.MaxAge != int(auth.DeviceCookieTTL.Seconds()) {
		t.Fatalf("device cookie = %+v", ck)
	}
	// Staff list: CN1 staff with a PIN only.
	names := map[int64]bool{}
	for _, s := range di.Staff {
		names[s.ID] = true
	}
	if !names[env.Fx.Users.Tn1] || !names[env.Fx.Users.Pc1] || !names[env.Fx.Users.Ql1] || names[env.Fx.Users.Tn2] || names[env.Fx.Users.Owner] {
		t.Fatalf("staff = %+v", di.Staff)
	}
	// The tablet now identifies as the CN1 device, and tn1 can log in with PIN on it.
	var got deviceInfo
	tablet.Get("/api/v1/device").MustStatus(t, 200).JSON(t, &got)
	if got.Device.ID != di.Device.ID {
		t.Fatalf("GET /device = %+v", got.Device)
	}
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)

	// Owner may activate for any branch.
	env.Anon().Post("/api/v1/devices/activate", map[string]any{"email": "owner@moc.vn", "password": seed.DefaultPassword, "branchId": cn2, "name": "iPad CN2"}).MustStatus(t, 201)
	// Warehouse manager may not.
	env.Anon().Post("/api/v1/devices/activate", map[string]any{"email": "kho@moc.vn", "password": seed.DefaultPassword, "branchId": cn2, "name": "x"}).MustStatus(t, 403)
}

func TestA03_ListAndRevokeDevices(t *testing.T) {
	env := testutil.NewEnv(t)
	var list []struct {
		ID       int64 `json:"id"`
		BranchID int64 `json:"branchId"`
		Revoked  bool  `json:"revoked"`
	}
	ql1 := env.As(env.Fx.Users.Ql1)
	ql1.Get("/api/v1/devices").MustStatus(t, 200).JSON(t, &list)
	if len(list) != 1 || list[0].ID != env.Fx.DeviceCN1 {
		t.Fatalf("ql1 devices = %+v", list)
	}
	env.Owner().Get("/api/v1/devices").MustStatus(t, 200).JSON(t, &list)
	if len(list) != 2 {
		t.Fatalf("owner devices = %+v", list)
	}
	env.Tn1().Get("/api/v1/devices").MustStatus(t, 403)
	// ql1 cannot revoke the CN2 device (404, not revealed).
	ql1.Post("/api/v1/devices/"+itoa(env.Fx.DeviceCN2)+"/revoke", nil).MustStatus(t, 404)

	tn1 := env.Tn1()
	tn1.Get("/api/v1/auth/me").MustStatus(t, 200)
	ql1.Post("/api/v1/devices/"+itoa(env.Fx.DeviceCN1)+"/revoke", nil).MustStatus(t, 204)
	// Sessions on the revoked device end and the device cookie stops working.
	tn1.Get("/api/v1/auth/me").MustStatus(t, 401)
	tn1.Get("/api/v1/device").MustStatus(t, 401)
}

func TestA04_PinLoginOnlyForDeviceBranch(t *testing.T) {
	env := testutil.NewEnv(t)
	tablet := env.Anon()
	// Without a device: DEVICE_REQUIRED.
	r := tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 401)
	if r.ErrCode() != "DEVICE_REQUIRED" {
		t.Fatalf("code %q", r.ErrCode())
	}
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)

	// tn2's correct PIN on the CN1 device → 401.
	r = tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn2, "pin": "5678"}).MustStatus(t, 401)
	if r.ErrCode() != "INVALID_PIN" {
		t.Fatalf("code %q", r.ErrCode())
	}
	// Wrong PIN → 401.
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "0000"}).MustStatus(t, 401)
	// Owner has no PIN → 401.
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Owner, "pin": "1234"}).MustStatus(t, 401)

	r = tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)
	var m me
	r.JSON(t, &m)
	if m.User.ID != env.Fx.Users.Tn1 || m.User.Role != "cashier" || m.Device == nil || m.Device.ID != env.Fx.DeviceCN1 || m.Device.Code != "CN1" {
		t.Fatalf("me = %+v", m)
	}
	var kind string
	var dev *int64
	if err := env.Pool.QueryRow(context.Background(),
		"SELECT kind, device_id FROM sessions WHERE token_hash = $1", auth.HashToken(tablet.Cookie(auth.SessionCookie))).Scan(&kind, &dev); err != nil {
		t.Fatal(err)
	}
	if kind != "pin" || dev == nil || *dev != env.Fx.DeviceCN1 {
		t.Fatalf("session kind %s device %v", kind, dev)
	}
	tablet.Get("/api/v1/auth/me").MustStatus(t, 200)

	// A revoked device cookie counts as no device.
	other := env.Anon()
	other.SetCookie(auth.DeviceCookie, "not-a-real-token")
	if r := other.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}); r.ErrCode() != "DEVICE_REQUIRED" {
		t.Fatalf("code %q", r.ErrCode())
	}
}

func TestA04_PinBruteForceLocks(t *testing.T) {
	env := testutil.NewEnv(t)
	tablet := env.Anon()
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)
	for i := 1; i < 5; i++ {
		tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "0000"}).MustStatus(t, 401)
	}
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "0000"}).MustStatus(t, 423)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 423)
}

func TestA05_PinSessionIdleLock(t *testing.T) {
	env := testutil.NewEnv(t)
	tablet := env.Anon()
	tablet.SetCookie(auth.DeviceCookie, env.Fx.DeviceCN1Token)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)
	env.Clock.T = env.Clock.T.Add(14 * time.Minute)
	tablet.Get("/api/v1/auth/me").MustStatus(t, 200) // activity resets the idle timer
	env.Clock.T = env.Clock.T.Add(14 * time.Minute)
	tablet.Get("/api/v1/auth/me").MustStatus(t, 200)
	env.Clock.T = env.Clock.T.Add(16 * time.Minute)
	tablet.Get("/api/v1/auth/me").MustStatus(t, 401)
	// The device is still activated; re-entering the PIN works.
	tablet.Get("/api/v1/device").MustStatus(t, 200)
	tablet.Post("/api/v1/auth/pin", map[string]any{"userId": env.Fx.Users.Tn1, "pin": "1234"}).MustStatus(t, 200)

	// Password sessions are not idle-locked.
	owner := env.Owner()
	env.Clock.T = env.Clock.T.Add(60 * time.Minute)
	owner.Get("/api/v1/auth/me").MustStatus(t, 200)
}

func TestA07_LogoutInvalidatesCookie(t *testing.T) {
	env := testutil.NewEnv(t)
	c := env.Anon()
	login(c, "owner@moc.vn", seed.DefaultPassword).MustStatus(t, 200)
	old := c.Cookie(auth.SessionCookie)
	c.Post("/api/v1/auth/logout", nil).MustStatus(t, 204)
	if c.Cookie(auth.SessionCookie) != "" {
		t.Fatal("cookie not cleared")
	}
	replay := env.Anon()
	replay.SetCookie(auth.SessionCookie, old)
	replay.Get("/api/v1/auth/me").MustStatus(t, 401)

	// Logging out on a POS keeps the device activated.
	tn1 := env.Tn1()
	tn1.Post("/api/v1/auth/logout", nil).MustStatus(t, 204)
	tn1.Get("/api/v1/auth/me").MustStatus(t, 401)
	tn1.Get("/api/v1/device").MustStatus(t, 200)
}

func TestA08_UnauthenticatedGets401(t *testing.T) {
	env := testutil.NewEnv(t)
	c := env.Anon()
	for _, p := range []string{"/api/v1/auth/me", "/api/v1/users", "/api/v1/devices", "/api/v1/settings", "/api/v1/branches"} {
		if r := c.Get(p); r.Status != 401 || r.ErrCode() != "UNAUTHENTICATED" {
			t.Errorf("GET %s = %d %s", p, r.Status, r.Body)
		}
	}
	for _, p := range []string{"/api/v1/users", "/api/v1/branches", "/api/v1/areas", "/api/v1/tables", "/api/v1/users/1/lock"} {
		if r := c.Post(p, map[string]any{}); r.Status != 401 {
			t.Errorf("POST %s = %d %s", p, r.Status, r.Body)
		}
	}
	if r := c.Put("/api/v1/settings", map[string]any{}); r.Status != 401 {
		t.Errorf("PUT /settings = %d", r.Status)
	}
	// A garbage cookie is the same as none.
	c.SetCookie(auth.SessionCookie, "garbage")
	c.Get("/api/v1/auth/me").MustStatus(t, 401)
}

func TestA14_NoHashesInResponses(t *testing.T) {
	env := testutil.NewEnv(t)
	var hash string
	if err := env.Pool.QueryRow(context.Background(), "SELECT password_hash FROM users WHERE id = $1", env.Fx.Users.Ql1).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("password not bcrypt: %q", hash)
	}
	tablet := env.Anon()
	bodies := [][]byte{
		login(env.Anon(), "ql1@moc.vn", seed.DefaultPassword).MustStatus(t, 200).Body,
		env.Owner().Get("/api/v1/users").MustStatus(t, 200).Body,
		env.Owner().Get("/api/v1/auth/me").MustStatus(t, 200).Body,
		tablet.Post("/api/v1/devices/activate", map[string]any{"email": "ql1@moc.vn", "password": seed.DefaultPassword, "branchId": env.Fx.Branches[1], "name": "x"}).MustStatus(t, 201).Body,
		tablet.Get("/api/v1/device").MustStatus(t, 200).Body,
		env.Owner().Post("/api/v1/users", map[string]any{"name": "Hoa", "role": "cashier", "branchId": env.Fx.Branches[1], "pin": "2468"}).MustStatus(t, 201).Body,
	}
	for _, b := range bodies {
		s := string(b)
		if strings.Contains(s, "$2a$") || strings.Contains(s, "$2b$") || strings.Contains(strings.ToLower(s), "hash") ||
			strings.Contains(s, "matkhau123") || strings.Contains(s, "2468") || strings.Contains(s, `"pin"`) || strings.Contains(s, `"password"`) {
			t.Errorf("response leaks secrets: %s", s)
		}
	}
	var pinHash string
	if err := env.Pool.QueryRow(context.Background(), "SELECT pin_hash FROM users WHERE name = 'Hoa'").Scan(&pinHash); err != nil || !strings.HasPrefix(pinHash, "$2") {
		t.Fatalf("pin not bcrypt: %q %v", pinHash, err)
	}
}

func TestA15_WritesFromOtherOriginRejected(t *testing.T) {
	env := testutil.NewEnv(t)
	evil := env.Anon()
	evil.Origin = "https://evil.example"
	r := login(evil, "owner@moc.vn", seed.DefaultPassword).MustStatus(t, 403)
	if r.ErrCode() != "BAD_ORIGIN" {
		t.Fatalf("code %q", r.ErrCode())
	}
	// A logged-in user's cookie is not enough without a good Origin.
	owner := env.Owner()
	owner.Origin = "https://evil.example"
	owner.Post("/api/v1/users/"+itoa(env.Fx.Users.Tn1)+"/lock", nil).MustStatus(t, 403)
	owner.Origin = ""
	if r := owner.Post("/api/v1/users/"+itoa(env.Fx.Users.Tn1)+"/lock", nil); r.Status != 403 || r.ErrCode() != "BAD_ORIGIN" {
		t.Fatalf("no Origin: %d %s", r.Status, r.Body)
	}
	env.Tn1().Get("/api/v1/auth/me").MustStatus(t, 200) // not locked
	// Reads are not affected.
	owner.Get("/api/v1/auth/me").MustStatus(t, 200)
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
