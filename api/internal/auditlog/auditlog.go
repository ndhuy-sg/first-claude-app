// Package auditlog serves the read-only audit log (GET /audit-logs).
//
// Entries are written by other modules through platform.Audit in the same
// transaction as the change. The table is append-only: there is no API that
// updates or deletes entries, and a database trigger rejects UPDATE/DELETE (L-02).
//
// Visibility (L-03): branch managers see only their own branch's entries
// (?branchId is ignored); the warehouse manager sees chain-level entries
// (without a branch); the owner sees everything and may filter by branch.
package auditlog

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
	"github.com/ndhuy-sg/first-claude-app/api/internal/auth"
	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/httpx"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

// Register mounts GET /audit-logs (and nothing else).
func Register(g *gin.RouterGroup, d *platform.Deps) {
	g.GET("/audit-logs", auth.RequireAction(domain.ActViewAudit), func(c *gin.Context) { list(c, d) })
}

type UserSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type AuditLog struct {
	ID         int64           `json:"id"`
	At         time.Time       `json:"at"`
	Actor      *UserSummary    `json:"actor"`
	Approver   *UserSummary    `json:"approver"`
	BranchID   *int64          `json:"branchId"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
	Note       string          `json:"note"`
}

func summary(id *int64, name *string) *UserSummary {
	if id == nil {
		return nil
	}
	s := &UserSummary{ID: *id}
	if name != nil {
		s.Name = *name
	}
	return s
}

func rawOrNull(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return b
}

func list(c *gin.Context, d *platform.Deps) {
	p := auth.From(c)
	var requested *int64
	if s := c.Query("branchId"); s != "" {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id <= 0 {
			httpx.Fail(c, apperr.ErrValidation.WithDetails(map[string]any{"reason": "branchId must be a positive integer"}))
			return
		}
		requested = &id
	}
	branch, err := auth.ResolveBranch(p, requested, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	// The warehouse manager's audit scope is the warehouse: entries without a branch.
	chainOnly := domain.Can(p.Role, domain.ActViewAudit) == domain.ScopeWarehouse
	if chainOnly {
		branch = nil
	}
	var action *string
	if a := c.Query("action"); a != "" {
		action = &a
	}
	page := httpx.GetPage(c)
	ctx := c.Request.Context()
	q := store.New(d.Pool)
	rows, err := q.ListAuditLogs(ctx, store.ListAuditLogsParams{BranchID: branch, ChainOnly: chainOnly, Action: action, Lim: page.Limit(), Off: page.Offset()})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	total, err := q.CountAuditLogs(ctx, store.CountAuditLogsParams{BranchID: branch, ChainOnly: chainOnly, Action: action})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out := httpx.List[AuditLog]{Items: make([]AuditLog, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, AuditLog{
			ID: r.ID, At: r.At, Actor: summary(r.ActorID, r.ActorName), Approver: summary(r.ApproverID, r.ApproverName),
			BranchID: r.BranchID, Action: r.Action, EntityType: r.EntityType, EntityID: r.EntityID,
			Before: rawOrNull(r.Before), After: rawOrNull(r.After), Note: r.Note,
		})
	}
	httpx.OK(c, out)
}
