// Package users manages staff accounts: list, create, update, lock/unlock and PIN reset.
//
// Scope rules:
//   - Owner manages everyone. Branch managers manage staff of their own branch only.
//   - Users of other branches (and chain-level users) are invisible to a branch
//     manager: loading them answers 404 (A-10 style).
//   - Explicitly asking to create/move a user into another branch, or to create
//     an owner/warehouse manager, is refused with 403 FORBIDDEN (A-11).
package users

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
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
	errPinTaken   = apperr.Conflict("PIN_TAKEN", "Mã PIN này đã có người dùng trong chi nhánh, vui lòng chọn mã khác")
	errEmailTaken = apperr.Conflict("EMAIL_TAKEN", "Email này đã được dùng cho tài khoản khác")
	errLockSelf   = apperr.BadRequest("CANNOT_LOCK_SELF", "Không thể tự khoá tài khoản của mình")
)

// MinPasswordLen is the minimum password length (UserInput.password minLength).
const MinPasswordLen = 8

type handler struct{ d *platform.Deps }

// Register adds the /users routes.
func Register(g *gin.RouterGroup, d *platform.Deps) {
	h := &handler{d: d}
	staff := auth.RequireAction(domain.ActManageStaff)
	g.GET("/users", staff, h.list)
	g.POST("/users", staff, h.create)
	g.PUT("/users/:id", staff, h.update)
	g.POST("/users/:id/lock", staff, h.lock)
	g.POST("/users/:id/unlock", staff, h.unlock)
	g.POST("/users/:id/reset-pin", staff, h.resetPin)
}

// User is the JSON shape of a user (never includes hashes, A-14).
type User struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Email        *string    `json:"email"`
	Role         string     `json:"role"`
	BranchID     *int64     `json:"branchId"`
	Status       string     `json:"status"`
	HasPin       bool       `json:"hasPin"`
	LastActiveAt *time.Time `json:"lastActiveAt"`
}

func toJSON(u store.User) User {
	status := "active"
	if u.DisabledAt != nil {
		status = "disabled"
	}
	return User{ID: u.ID, Name: u.Name, Email: u.Email, Role: u.Role, BranchID: u.BranchID, Status: status, HasPin: u.PinHash != nil, LastActiveAt: u.LastActiveAt}
}

func bcryptCost() int { return bcrypt.DefaultCost }

func hash(s string) (*string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(s), bcryptCost())
	if err != nil {
		return nil, err
	}
	v := string(b)
	return &v, nil
}

func invalid(field, reason string) *apperr.Error {
	return apperr.ErrValidation.WithDetails(map[string]any{"field": field, "reason": reason})
}

// visible returns 404 when p may not manage u.
func visible(p *auth.Principal, u store.User) error {
	if p.BranchID == nil {
		return nil
	}
	if u.BranchID == nil {
		return apperr.ErrNotFound
	}
	return auth.CheckBranch(p, *u.BranchID)
}

// checkPinFree returns PIN_TAKEN when pin matches the PIN of another active user of branchID (D-13).
func checkPinFree(ctx context.Context, q *store.Queries, branchID int64, pin string, exceptUserID int64) error {
	rows, err := q.UsersPinHashesInBranch(ctx, &branchID)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID == exceptUserID || r.PinHash == nil {
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(*r.PinHash), []byte(pin)) == nil {
			return errPinTaken
		}
	}
	return nil
}

// ---------- handlers ----------

func (h *handler) list(c *gin.Context) {
	p := auth.From(c)
	var requested *int64
	if v := c.Query("branchId"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(c, invalid("branchId", "invalid"))
			return
		}
		requested = &id
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := store.New(h.d.Pool).UsersList(c.Request.Context(), branch)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := make([]User, 0, len(rows))
	for _, u := range rows {
		out = append(out, toJSON(u))
	}
	httpx.OK(c, out)
}

// input is UserInput. Pointer fields distinguish "absent" from empty.
type input struct {
	Name     string  `json:"name"`
	Email    *string `json:"email"`
	Password *string `json:"password"`
	PIN      *string `json:"pin"`
	Role     string  `json:"role"`
	BranchID *int64  `json:"branchId"`
}

// normalized is a validated input.
type normalized struct {
	name     string
	email    *string
	password *string // plain, nil = keep/none
	pin      *string // plain, nil = keep/none
	role     domain.Role
	branchID *int64
}

// validate checks in against the caller's scope. existing is nil on create.
func validate(p *auth.Principal, in input, existing *store.User) (*normalized, error) {
	n := &normalized{name: strings.TrimSpace(in.Name), role: domain.Role(in.Role)}
	if n.name == "" {
		return nil, invalid("name", "required")
	}
	if !n.role.Valid() {
		return nil, invalid("role", "invalid")
	}
	chainRole := n.role == domain.RoleOwner || n.role == domain.RoleWarehouseManager
	if p.BranchID != nil {
		// Branch manager: own branch, branch roles only (A-11).
		if chainRole {
			return nil, apperr.ErrForbidden
		}
		if in.BranchID != nil && *in.BranchID != *p.BranchID {
			return nil, apperr.ErrForbidden
		}
		n.branchID = p.BranchID
	} else if !chainRole {
		if in.BranchID == nil {
			return nil, apperr.BadRequest("BRANCH_REQUIRED", "Vui lòng chọn chi nhánh")
		}
		n.branchID = in.BranchID
	}
	if in.Email != nil {
		if e := strings.ToLower(strings.TrimSpace(*in.Email)); e != "" {
			if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
				return nil, invalid("email", "invalid")
			}
			n.email = &e
		}
	}
	if in.Password != nil && *in.Password != "" {
		if len([]rune(*in.Password)) < MinPasswordLen {
			return nil, invalid("password", "too_short")
		}
		n.password = in.Password
	}
	if in.PIN != nil {
		if err := domain.ValidatePIN(*in.PIN); err != nil {
			return nil, invalid("pin", "4-6 digits")
		}
		n.pin = in.PIN
	}
	hasPassword := n.password != nil || (existing != nil && existing.PasswordHash != nil && n.email != nil)
	hasPin := n.pin != nil || (existing != nil && existing.PinHash != nil)
	switch n.role {
	case domain.RoleOwner, domain.RoleWarehouseManager, domain.RoleBranchManager:
		if n.email == nil {
			return nil, invalid("email", "required")
		}
		if !hasPassword {
			return nil, invalid("password", "required")
		}
	default:
		if !hasPin {
			return nil, invalid("pin", "required")
		}
		if n.email != nil && !hasPassword {
			return nil, invalid("password", "required")
		}
	}
	return n, nil
}

func (h *handler) emailFree(ctx context.Context, q *store.Queries, email *string, exceptID int64) error {
	if email == nil {
		return nil
	}
	u, err := q.GetUserByEmail(ctx, *email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if u.ID != exceptID {
		return errEmailTaken
	}
	return nil
}

func (h *handler) create(c *gin.Context) {
	p := auth.From(c)
	var in input
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	n, err := validate(p, in, nil)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var pwHash, pinHash *string
	if n.password != nil {
		if pwHash, err = hash(*n.password); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	if n.pin != nil {
		if pinHash, err = hash(*n.pin); err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	ctx := c.Request.Context()
	var created store.User
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, tx pgx.Tx) error {
		if n.branchID != nil {
			// Serialize staff changes per branch so two creates cannot take the same PIN.
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7001, $1::int)", *n.branchID); err != nil {
				return err
			}
			if _, err := q.GetBranch(ctx, *n.branchID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return invalid("branchId", "not_found")
				}
				return err
			}
			if n.pin != nil {
				if err := checkPinFree(ctx, q, *n.branchID, *n.pin, 0); err != nil {
					return err
				}
			}
		}
		if err := h.emailFree(ctx, q, n.email, 0); err != nil {
			return err
		}
		u, err := q.UsersCreate(ctx, store.UsersCreateParams{
			Name: n.name, Email: n.email, PasswordHash: pwHash, PinHash: pinHash, Role: string(n.role), BranchID: n.branchID,
		})
		if err != nil {
			return err
		}
		created = u
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: u.BranchID, Action: "user.create", EntityType: "user",
			EntityID: strconv.FormatInt(u.ID, 10), After: toJSON(u),
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, toJSON(created))
}

// loadForUpdate locks the target user and checks the caller may manage them.
func loadForUpdate(ctx context.Context, q *store.Queries, p *auth.Principal, id int64) (store.User, error) {
	u, err := q.UsersGetForUpdate(ctx, id)
	if err != nil {
		return u, err
	}
	return u, visible(p, u)
}

func (h *handler) update(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var in input
	if err := httpx.Bind(c, &in); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var updated store.User
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, tx pgx.Tx) error {
		old, err := loadForUpdate(ctx, q, p, id)
		if err != nil {
			return err
		}
		n, err := validate(p, in, &old)
		if err != nil {
			return err
		}
		if old.ID == p.UserID && n.role != domain.Role(old.Role) {
			return apperr.BadRequest("CANNOT_CHANGE_OWN_ROLE", "Không thể tự đổi vai trò của mình")
		}
		pwHash, pinHash, mustChange := old.PasswordHash, old.PinHash, old.PinMustChange
		if n.email == nil && n.password == nil {
			pwHash = nil // PIN-only staff have no password
		}
		if n.password != nil {
			if pwHash, err = hash(*n.password); err != nil {
				return err
			}
		}
		if n.branchID != nil {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7001, $1::int)", *n.branchID); err != nil {
				return err
			}
			if _, err := q.GetBranch(ctx, *n.branchID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return invalid("branchId", "not_found")
				}
				return err
			}
		}
		if n.pin != nil {
			if pinHash, err = hash(*n.pin); err != nil {
				return err
			}
			mustChange = false
		}
		switch {
		case n.branchID == nil:
			pinHash, mustChange = nil, false // chain-level users log in with email only
		case n.pin != nil:
			if err := checkPinFree(ctx, q, *n.branchID, *n.pin, old.ID); err != nil {
				return err
			}
		case pinHash != nil && !sameBranch(old.BranchID, n.branchID):
			// bcrypt hashes cannot be compared, so a PIN moved to another branch must be re-entered.
			return invalid("pin", "required_when_changing_branch")
		}
		if err := h.emailFree(ctx, q, n.email, old.ID); err != nil {
			return err
		}
		u, err := q.UsersUpdate(ctx, store.UsersUpdateParams{
			ID: old.ID, Name: n.name, Email: n.email, PasswordHash: pwHash, PinHash: pinHash,
			PinMustChange: mustChange, Role: string(n.role), BranchID: n.branchID,
		})
		if err != nil {
			return err
		}
		updated = u
		if n.password != nil || old.Role != u.Role || !sameBranch(old.BranchID, u.BranchID) {
			// Credentials or scope changed: end existing sessions.
			if err := q.RevokeUserSessions(ctx, u.ID); err != nil {
				return err
			}
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: u.BranchID, Action: "user.update", EntityType: "user",
			EntityID: strconv.FormatInt(u.ID, 10), Before: toJSON(old), After: toJSON(u),
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, toJSON(updated))
}

func sameBranch(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (h *handler) lock(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	now := h.d.Clock.Now()
	var target store.User
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		u, err := loadForUpdate(ctx, q, p, id)
		if err != nil {
			return err
		}
		if u.ID == p.UserID {
			return errLockSelf
		}
		target = u
		if err := q.UsersDisable(ctx, store.UsersDisableParams{Now: now, By: &p.UserID, ID: u.ID}); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(ctx, u.ID); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: u.BranchID, Action: "user.lock", EntityType: "user",
			EntityID: strconv.FormatInt(u.ID, 10), Before: map[string]any{"status": toJSON(u).Status}, After: map[string]any{"status": "disabled"},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var branch int64
	if target.BranchID != nil {
		branch = *target.BranchID
	}
	h.d.Events.Publish(platform.Event{Type: "user.locked", BranchID: branch, Data: map[string]any{"userId": target.ID}})
	c.Status(http.StatusNoContent)
}

func (h *handler) unlock(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, _ pgx.Tx) error {
		u, err := loadForUpdate(ctx, q, p, id)
		if err != nil {
			return err
		}
		if err := q.UsersEnable(ctx, u.ID); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: u.BranchID, Action: "user.unlock", EntityType: "user",
			EntityID: strconv.FormatInt(u.ID, 10), Before: map[string]any{"status": toJSON(u).Status}, After: map[string]any{"status": "active"},
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type resetPinReq struct {
	PIN string `json:"pin"`
}

func (h *handler) resetPin(c *gin.Context) {
	p := auth.From(c)
	id, err := httpx.PathID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req resetPinReq
	if err := httpx.Bind(c, &req); err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := domain.ValidatePIN(req.PIN); err != nil {
		httpx.Fail(c, invalid("pin", "4-6 digits"))
		return
	}
	pinHash, err := hash(req.PIN)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	err = store.InTx(ctx, h.d.Pool, func(q *store.Queries, tx pgx.Tx) error {
		u, err := loadForUpdate(ctx, q, p, id)
		if err != nil {
			return err
		}
		if u.BranchID == nil {
			return apperr.BadRequest("PIN_NOT_ALLOWED", "Tài khoản cấp chuỗi không dùng mã PIN")
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(7001, $1::int)", *u.BranchID); err != nil {
			return err
		}
		if err := checkPinFree(ctx, q, *u.BranchID, req.PIN, u.ID); err != nil {
			return err
		}
		if err := q.UsersSetPin(ctx, store.UsersSetPinParams{ID: u.ID, PinHash: pinHash, PinMustChange: true}); err != nil {
			return err
		}
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &p.UserID, BranchID: u.BranchID, Action: "user.reset_pin", EntityType: "user", EntityID: strconv.FormatInt(u.ID, 10),
		})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
