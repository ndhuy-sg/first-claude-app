// Package authapi serves login (email+password and PIN), logout, the current
// user, and POS device activation/revocation.
//
// Lockout (A-02): every failed password or PIN attempt increments
// users.failed_logins; reaching app_settings.login_max_failures locks the
// account for login_lock_minutes (423 ACCOUNT_LOCKED). Unknown emails get the
// same 401 INVALID_CREDENTIALS body as a wrong password.
package authapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

var (
	errInvalidCredentials = apperr.Unauthorized("INVALID_CREDENTIALS", "Email hoặc mật khẩu không đúng")
	errInvalidPIN         = apperr.Unauthorized("INVALID_PIN", "Mã PIN không đúng")
	errDeviceRequired     = apperr.Unauthorized("DEVICE_REQUIRED", "Máy này chưa được kích hoạt cho chi nhánh nào")
	errAccountLocked      = apperr.New(http.StatusLocked, "ACCOUNT_LOCKED", "Tài khoản tạm khoá do nhập sai nhiều lần, vui lòng thử lại sau")
	errAccountDisabled    = apperr.Forbidden("ACCOUNT_DISABLED", "Tài khoản đã bị khoá, vui lòng liên hệ quản lý")
)

// dummyHash is compared against when the email is unknown so the response time
// does not reveal whether the account exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("moc-dummy-password"), bcrypt.DefaultCost)

type handler struct{ d *platform.Deps }

// Register adds the auth and device routes.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	g.POST("/auth/login", h.login)
	g.POST("/auth/pin", h.pinLogin)
	g.POST("/auth/logout", h.logout)
	g.GET("/auth/me", auth.RequireAuth(), h.me)
	g.GET("/device", h.currentDevice)
	g.POST("/devices/activate", h.activate)
	g.GET("/devices", auth.RequireAction(domain.ActManageDevices), h.listDevices)
	g.POST("/devices/:id/revoke", auth.RequireAction(domain.ActManageDevices), h.revokeDevice)
}

// ---------- JSON shapes ----------

type deviceJSON struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	BranchID   int64      `json:"branchId"`
	BranchCode string     `json:"branchCode"`
	LastSeenAt *time.Time `json:"lastSeenAt"`
	Revoked    bool       `json:"revoked"`
}

type staffJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type deviceInfoJSON struct {
	Device deviceJSON  `json:"device"`
	Staff  []staffJSON `json:"staff"`
}

type meUser struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	BranchID      *int64 `json:"branchId"`
	PinMustChange bool   `json:"pinMustChange"`
}

type meSettings struct {
	DiscountMaxPct     int32 `json:"discountMaxPct"`
	PosIdleLockMinutes int32 `json:"posIdleLockMinutes"`
}

type meJSON struct {
	User     meUser      `json:"user"`
	Device   *deviceJSON `json:"device"`
	Settings meSettings  `json:"settings"`
}

func toDevice(r store.AuthGetDeviceRow) deviceJSON {
	return deviceJSON{ID: r.ID, Name: r.Name, BranchID: r.BranchID, BranchCode: r.BranchCode, LastSeenAt: r.LastSeenAt, Revoked: r.RevokedAt != nil}
}

func (h *handler) buildMe(ctx context.Context, userID int64, deviceID *int64) (*meJSON, error) {
	q := store.New(h.d.Pool)
	u, err := q.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	s, err := q.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	m := &meJSON{
		User:     meUser{ID: u.ID, Name: u.Name, Role: u.Role, BranchID: u.BranchID, PinMustChange: u.PinMustChange},
		Settings: meSettings{DiscountMaxPct: s.DiscountMaxPct, PosIdleLockMinutes: s.PosIdleLockMinutes},
	}
	if deviceID != nil {
		d, err := q.AuthGetDevice(ctx, *deviceID)
		if err != nil {
			return nil, err
		}
		dj := toDevice(d)
		m.Device = &dj
	}
	return m, nil
}

func (h *handler) deviceInfo(ctx context.Context, deviceID int64) (*deviceInfoJSON, error) {
	q := store.New(h.d.Pool)
	d, err := q.AuthGetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	rows, err := q.AuthListPinStaff(ctx, &d.BranchID)
	if err != nil {
		return nil, err
	}
	out := &deviceInfoJSON{Device: toDevice(d), Staff: make([]staffJSON, 0, len(rows))}
	for _, r := range rows {
		out.Staff = append(out.Staff, staffJSON{ID: r.ID, Name: r.Name, Role: r.Role})
	}
	return out, nil
}

// ---------- credential checks with lockout ----------

func lockedErr(until time.Time) *apperr.Error {
	return errAccountLocked.WithDetails(map[string]any{"lockedUntil": until.UTC().Format(time.RFC3339)})
}

// checkSecret verifies secret against the hash of user id (password or PIN),
// counting failures and locking the account (A-02). It commits its own
// transaction so that failures are recorded even though the request fails.
// wrong is the error returned for a bad secret; details.remainingAttempts is added to it.
func (h *handler) checkSecret(ctx context.Context, id int64, secret string, pin bool, wrong *apperr.Error) (store.User, error) {
	now := h.d.Clock.Now()
	var user store.User
	var result error
	err := store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		u, err := q.AuthGetUserForUpdate(ctx, id)
		if err != nil {
			return err
		}
		user = u
		if u.LockedUntil != nil && u.LockedUntil.After(now) {
			result = lockedErr(*u.LockedUntil)
			return nil
		}
		hash := u.PasswordHash
		if pin {
			hash = u.PinHash
		}
		if hash != nil && bcrypt.CompareHashAndPassword([]byte(*hash), []byte(secret)) == nil {
			if u.DisabledAt != nil {
				result = errAccountDisabled
				return nil
			}
			return q.AuthRecordLoginSuccess(ctx, store.AuthRecordLoginSuccessParams{Now: now, ID: u.ID})
		}
		if hash == nil {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(secret))
		}
		s, err := q.GetSettings(ctx)
		if err != nil {
			return err
		}
		r, err := q.AuthRecordLoginFailure(ctx, store.AuthRecordLoginFailureParams{
			MaxFailures: s.LoginMaxFailures, LockUntil: now.Add(time.Duration(s.LoginLockMinutes) * time.Minute), ID: u.ID,
		})
		if err != nil {
			return err
		}
		if r.LockedUntil != nil && r.LockedUntil.After(now) {
			result = lockedErr(*r.LockedUntil)
			return nil
		}
		result = wrong.WithDetails(map[string]any{"remainingAttempts": s.LoginMaxFailures - r.FailedLogins})
		return nil
	})
	if err != nil {
		return user, err
	}
	return user, result
}

// checkPassword authenticates email + password.
func (h *handler) checkPassword(ctx context.Context, email, password string) (store.User, error) {
	q := store.New(h.d.Pool)
	u, err := q.GetUserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && u.PasswordHash == nil) {
		// Same work and same answer as a wrong password.
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		s, err := q.GetSettings(ctx)
		if err != nil {
			return u, err
		}
		return u, errInvalidCredentials.WithDetails(map[string]any{"remainingAttempts": s.LoginMaxFailures - 1})
	}
	if err != nil {
		return u, err
	}
	return h.checkSecret(ctx, u.ID, password, false, errInvalidCredentials)
}

// startSession creates a session and sets the session cookie.
func (h *handler) startSession(c *gin.Context, userID int64, deviceID *int64, kind string) error {
	ctx := c.Request.Context()
	// Drop the session this browser had before, if any.
	if raw, err := c.Cookie(auth.SessionCookie); err == nil && raw != "" {
		_ = store.New(h.d.Pool).RevokeSession(ctx, auth.HashToken(raw))
	}
	raw, err := auth.CreateSession(ctx, store.New(h.d.Pool), userID, deviceID, kind, h.d.Clock.Now())
	if err != nil {
		return err
	}
	ttl := auth.PasswordSessionTTL
	if kind == "pin" {
		ttl = auth.PINSessionTTL
	}
	auth.SetCookie(c, auth.SessionCookie, raw, ttl, h.d.Cfg.CookieSecure)
	return nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.ErrValidation.WithDetails(map[string]any{"field": "branchId"})
	}
	return id, nil
}

// ---------- handlers ----------

type loginReq struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *handler) login(c *gin.Context) {
	var req loginReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	u, err := h.checkPassword(ctx, req.Email, req.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.startSession(c, u.ID, nil, "password"); err != nil {
		httpx.Fail(c, err)
		return
	}
	var devID *int64
	if d := auth.DeviceFrom(c); d != nil {
		devID = &d.ID
	}
	m, err := h.buildMe(ctx, u.ID, devID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

type pinReq struct {
	UserID int64  `json:"userId" binding:"required"`
	PIN    string `json:"pin" binding:"required"`
}

func (h *handler) pinLogin(c *gin.Context) {
	dev := auth.DeviceFrom(c)
	if dev == nil {
		httpx.Fail(c, errDeviceRequired)
		return
	}
	var req pinReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	u, err := store.New(h.d.Pool).GetUser(ctx, req.UserID)
	if err != nil || u.BranchID == nil || *u.BranchID != dev.BranchID || u.PinHash == nil || u.DisabledAt != nil {
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(c, err)
			return
		}
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.PIN))
		httpx.Fail(c, errInvalidPIN)
		return
	}
	if _, err := h.checkSecret(ctx, u.ID, req.PIN, true, errInvalidPIN); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.startSession(c, u.ID, &dev.ID, "pin"); err != nil {
		httpx.Fail(c, err)
		return
	}
	m, err := h.buildMe(ctx, u.ID, &dev.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) logout(c *gin.Context) {
	if raw, err := c.Cookie(auth.SessionCookie); err == nil && raw != "" {
		if err := store.New(h.d.Pool).RevokeSession(c.Request.Context(), auth.HashToken(raw)); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	auth.ClearCookie(c, auth.SessionCookie, h.d.Cfg.CookieSecure)
	c.Status(http.StatusNoContent)
}

func (h *handler) me(c *gin.Context) {
	p := auth.From(c)
	m, err := h.buildMe(c.Request.Context(), p.UserID, p.DeviceID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, m)
}

func (h *handler) currentDevice(c *gin.Context) {
	dev := auth.DeviceFrom(c)
	if dev == nil {
		httpx.Fail(c, errDeviceRequired)
		return
	}
	info, err := h.deviceInfo(c.Request.Context(), dev.ID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, info)
}

type activateReq struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	BranchID int64  `json:"branchId" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

func (h *handler) activate(c *gin.Context) {
	var req activateReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"field": "name"}))
		return
	}
	ctx := c.Request.Context()
	u, err := h.checkPassword(ctx, req.Email, req.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	role := domain.Role(u.Role)
	switch domain.Can(role, domain.ActManageDevices) {
	case domain.ScopeChain:
	case domain.ScopeOwnBranch:
		if u.BranchID == nil || *u.BranchID != req.BranchID {
			httpx.Fail(c, apperr.ErrForbidden)
			return
		}
	default:
		httpx.Fail(c, apperr.ErrForbidden)
		return
	}
	now := h.d.Clock.Now()
	raw, hash := auth.NewToken()
	var devID int64
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		b, err := q.GetBranch(ctx, req.BranchID)
		if err != nil {
			return err
		}
		if !b.IsActive {
			return apperr.ErrNotFound
		}
		devID, err = q.AuthCreateDevice(ctx, store.AuthCreateDeviceParams{BranchID: b.ID, Name: req.Name, TokenHash: hash, ActivatedBy: u.ID, Now: now})
		if err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &u.ID, BranchID: &b.ID, Action: "device.activate", EntityType: "device",
			EntityID: itoa(devID), After: map[string]any{"name": req.Name},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	auth.SetCookie(c, auth.DeviceCookie, raw, auth.DeviceCookieTTL, h.d.Cfg.CookieSecure)
	if err := h.startSession(c, u.ID, &devID, "password"); err != nil {
		httpx.Fail(c, err)
		return
	}
	info, err := h.deviceInfo(ctx, devID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, info)
}

func (h *handler) listDevices(c *gin.Context) {
	p := auth.From(c)
	var requested *int64
	if v := c.Query("branchId"); v != "" {
		id, err := parseID(v)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		requested = &id
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := store.New(h.d.Pool).AuthListDevices(c.Request.Context(), branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]deviceJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, toDevice(store.AuthGetDeviceRow(r)))
	}
	httpx.OK(c, out)
}

func (h *handler) revokeDevice(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	now := h.d.Clock.Now()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		d, err := q.AuthGetDevice(ctx, id)
		if err != nil {
			return err
		}
		if err := auth.CheckBranch(p, d.BranchID); err != nil {
			return err
		}
		if err := q.AuthRevokeDevice(ctx, store.AuthRevokeDeviceParams{Now: now, ID: id}); err != nil {
			return err
		}
		if err := q.AuthRevokeDeviceSessions(ctx, store.AuthRevokeDeviceSessionsParams{Now: now, DeviceID: &id}); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: &d.BranchID, Action: "device.revoke", EntityType: "device", EntityID: itoa(id),
			Before: map[string]any{"name": d.Name},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
