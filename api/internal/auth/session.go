// Package auth owns sessions, the current-user principal and access checks.
//
// Wave 0 provides: token helpers, session creation, the middleware that loads
// the principal from the session cookie, RequireAuth/RequireAction and branch
// scoping. The auth agent adds login/PIN/device endpoints, lockout and CSRF.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

const (
	SessionCookie = "moc_session" // user session (password or PIN)
	DeviceCookie  = "moc_device"  // POS device activation, long lived

	PasswordSessionTTL = 12 * time.Hour
	PINSessionTTL      = 16 * time.Hour // a PIN session is also idle-locked by app_settings.pos_idle_lock_minutes
	DeviceCookieTTL    = 400 * 24 * time.Hour
)

// NewToken returns a random URL-safe token and its sha256 hex hash. Only the hash is stored.
func NewToken() (raw, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw)
}

func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// CreateSession stores a new session and returns the raw cookie value.
func CreateSession(ctx context.Context, q *store.Queries, userID int64, deviceID *int64, kind string, now time.Time) (string, error) {
	raw, hash := NewToken()
	ttl := PasswordSessionTTL
	if kind == "pin" {
		ttl = PINSessionTTL
	}
	err := q.CreateSession(ctx, store.CreateSessionParams{
		TokenHash: hash, UserID: userID, DeviceID: deviceID, Kind: kind, ExpiresAt: now.Add(ttl), Now: now,
	})
	return raw, err
}

// SetCookie writes an HttpOnly, SameSite=Lax cookie on path "/".
func SetCookie(c *gin.Context, name, value string, maxAge time.Duration, secure bool) {
	c.SetSameSite(2) // http.SameSiteLaxMode
	c.SetCookie(name, value, int(maxAge.Seconds()), "/", "", secure, true)
}

func ClearCookie(c *gin.Context, name string, secure bool) {
	c.SetSameSite(2)
	c.SetCookie(name, "", -1, "/", "", secure, true)
}

// Principal is the authenticated caller.
type Principal struct {
	UserID      int64
	Name        string
	Role        domain.Role
	BranchID    *int64 // nil for owner and warehouse manager
	DeviceID    *int64 // set for PIN sessions on a POS device
	SessionHash string
	Kind        string // "password" | "pin"
}

// Device is the POS device identified by the device cookie (may be present without a user session).
type Device struct {
	ID       int64
	BranchID int64
	Name     string
}

const (
	ctxPrincipal = "auth.principal"
	ctxDevice    = "auth.device"
)

// Middleware loads the device (moc_device cookie) and principal (moc_session cookie)
// into the gin context. It never rejects; use RequireAuth for that.
// PIN sessions idle longer than idleMinutes() are treated as signed out.
func Middleware(pool *pgxpool.Pool, now func() time.Time, idleMinutes func(ctx context.Context) int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		q := store.New(pool)
		if raw, err := c.Cookie(DeviceCookie); err == nil && raw != "" {
			if d, err := q.GetActiveDeviceByToken(ctx, HashToken(raw)); err == nil {
				c.Set(ctxDevice, &Device{ID: d.ID, BranchID: d.BranchID, Name: d.Name})
				_ = q.TouchDevice(ctx, d.ID)
			}
		}
		raw, err := c.Cookie(SessionCookie)
		if err != nil || raw == "" {
			c.Next()
			return
		}
		hash := HashToken(raw)
		t := now()
		s, err := q.GetActiveSession(ctx, store.GetActiveSessionParams{TokenHash: hash, Now: t})
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				_ = c.Error(err)
			}
			c.Next()
			return
		}
		if s.Kind == "pin" && idleMinutes != nil {
			if t.Sub(s.LastSeenAt) > time.Duration(idleMinutes(ctx))*time.Minute {
				_ = q.RevokeSession(ctx, hash)
				c.Next()
				return
			}
		}
		_ = q.TouchSession(ctx, store.TouchSessionParams{Now: t, TokenHash: hash})
		c.Set(ctxPrincipal, &Principal{
			UserID: s.UserID, Name: s.UserName, Role: domain.Role(s.UserRole), BranchID: s.UserBranchID,
			DeviceID: s.DeviceID, SessionHash: hash, Kind: s.Kind,
		})
		c.Next()
	}
}

// From returns the principal or nil.
func From(c *gin.Context) *Principal {
	if v, ok := c.Get(ctxPrincipal); ok {
		return v.(*Principal)
	}
	return nil
}

// DeviceFrom returns the activated POS device or nil.
func DeviceFrom(c *gin.Context) *Device {
	if v, ok := c.Get(ctxDevice); ok {
		return v.(*Device)
	}
	return nil
}

// RequireAuth rejects requests without a valid session (401).
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if From(c) == nil {
			c.AbortWithStatusJSON(401, gin.H{"error": apperr.ErrUnauthenticated})
			return
		}
		c.Next()
	}
}

// RequireAction rejects callers whose role has no scope for action (403).
// Branch-level checks still happen in the handler via ResolveBranch/CheckBranch.
func RequireAction(action domain.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := From(c)
		if p == nil {
			c.AbortWithStatusJSON(401, gin.H{"error": apperr.ErrUnauthenticated})
			return
		}
		if domain.Can(p.Role, action) == domain.ScopeNone {
			c.AbortWithStatusJSON(403, gin.H{"error": apperr.ErrForbidden})
			return
		}
		c.Next()
	}
}

// ResolveBranch decides which branch a request acts on.
//   - Branch-bound users (manager, cashier, barista) always get their own branch;
//     any requested branch id is ignored (test A-12).
//   - Chain-level users (owner, warehouse manager) must pass a branch id when
//     the action needs one (required=true), else nil means "all branches".
func ResolveBranch(p *Principal, requested *int64, required bool) (*int64, error) {
	if p.BranchID != nil {
		return p.BranchID, nil
	}
	if requested == nil && required {
		return nil, apperr.BadRequest("BRANCH_REQUIRED", "Vui lòng chọn chi nhánh")
	}
	return requested, nil
}

// CheckBranch returns 404 NOT_FOUND when p may not see data of branchID
// (we answer 404 rather than 403 so other branches' data is not revealed, test A-10).
func CheckBranch(p *Principal, branchID int64) error {
	if p.BranchID == nil || *p.BranchID == branchID {
		return nil
	}
	return apperr.ErrNotFound
}
