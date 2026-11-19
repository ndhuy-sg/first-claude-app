// Package platform holds the dependencies every module receives (database
// pool, config, clock, event publisher) and small cross-module helpers:
// audit logging, job enqueueing and events.
//
// Modules register their routes with:
//
//	func Register(g *gin.RouterGroup, d *platform.Deps)
//
// and are wired in internal/app/router.go.
package platform

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ndhuy-sg/first-claude-app/api/internal/config"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Cfg    config.Config
	Clock  Clock
	Events Publisher
}

// Clock lets tests control time.
type Clock interface{ Now() time.Time }

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// FixedClock is a settable clock for tests.
type FixedClock struct{ T time.Time }

func (c *FixedClock) Now() time.Time { return c.T }

// ---------- Events (Server-Sent Events) ----------

// Event is pushed to POS devices of one branch (BranchID) or to everyone (BranchID == 0).
type Event struct {
	Type     string `json:"type"` // e.g. "product.sold_out", "user.locked", "transfer.shipped", "order.updated"
	BranchID int64  `json:"branchId,omitempty"`
	Data     any    `json:"data,omitempty"`
}

type Publisher interface{ Publish(e Event) }

// NoopPublisher drops events. Used until the SSE hub is wired, and in tests that don't care.
type NoopPublisher struct{}

func (NoopPublisher) Publish(Event) {}

// RecordingPublisher keeps published events in memory (tests).
type RecordingPublisher struct{ Events []Event }

func (r *RecordingPublisher) Publish(e Event) { r.Events = append(r.Events, e) }

// ---------- Audit log ----------

// AuditEntry is one append-only audit record.
type AuditEntry struct {
	ActorID    *int64
	ApproverID *int64
	BranchID   *int64
	Action     string // e.g. "order.discount", "order.refund", "order.cancel", "product.price", "product.sold_out", "user.lock"
	EntityType string // e.g. "order", "product", "user"
	EntityID   string
	Before     any
	After      any
	Note       string
}

// Audit writes an audit record using q, which should be the transaction of the change itself.
func Audit(ctx context.Context, q *store.Queries, e AuditEntry) error {
	var before, after json.RawMessage
	if e.Before != nil {
		b, err := json.Marshal(e.Before)
		if err != nil {
			return err
		}
		before = b
	}
	if e.After != nil {
		b, err := json.Marshal(e.After)
		if err != nil {
			return err
		}
		after = b
	}
	return q.InsertAuditLog(ctx, store.InsertAuditLogParams{
		ActorID: e.ActorID, ApproverID: e.ApproverID, BranchID: e.BranchID,
		Action: e.Action, EntityType: e.EntityType, EntityID: e.EntityID,
		Before: before, After: after, Note: e.Note,
	})
}

// ---------- Background jobs ----------

// EnqueueJob adds a job in the caller's transaction. dedupeKey (optional) makes
// enqueueing idempotent: the same key never creates a second job.
func EnqueueJob(ctx context.Context, q *store.Queries, kind string, payload any, dedupeKey string, runAt time.Time) (int64, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	var key *string
	if dedupeKey != "" {
		key = &dedupeKey
	}
	return q.EnqueueJob(ctx, store.EnqueueJobParams{Kind: kind, Payload: b, DedupeKey: key, RunAt: runAt, MaxAttempts: 10})
}

// Ptr returns a pointer to v (handy for nullable columns).
func Ptr[T any](v T) *T { return &v }
