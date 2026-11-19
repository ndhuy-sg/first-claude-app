package auditlog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

type userSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type auditLog struct {
	ID         int64           `json:"id"`
	At         time.Time       `json:"at"`
	Actor      *userSummary    `json:"actor"`
	Approver   *userSummary    `json:"approver"`
	BranchID   *int64          `json:"branchId"`
	Action     string          `json:"action"`
	EntityType string          `json:"entityType"`
	EntityID   string          `json:"entityId"`
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
	Note       string          `json:"note"`
}

type list struct {
	Items []auditLog `json:"items"`
	Total int        `json:"total"`
}

func insertLog(t *testing.T, env *testutil.Env, at time.Time, actor int64, branch *int64, action string) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(),
		`INSERT INTO audit_logs (at, actor_id, branch_id, action, entity_type, entity_id) VALUES ($1, $2, $3, $4, 'order', '1')`,
		at, actor, branch, action); err != nil {
		t.Fatal(err)
	}
}

// seedLogs: 3 CN1 entries, 2 CN2 entries, 1 chain-level entry (no branch).
func seedLogs(t *testing.T, env *testutil.Env) {
	cn1, cn2 := env.Fx.Branches[1], env.Fx.Branches[2]
	base := testutil.DefaultNow
	insertLog(t, env, base.Add(1*time.Minute), env.Fx.Users.Tn1, &cn1, "order.discount")
	insertLog(t, env, base.Add(2*time.Minute), env.Fx.Users.Ql1, &cn1, "order.cancel")
	insertLog(t, env, base.Add(3*time.Minute), env.Fx.Users.Tn2, &cn2, "order.discount")
	insertLog(t, env, base.Add(4*time.Minute), env.Fx.Users.Owner, nil, "product.price")
	insertLog(t, env, base.Add(5*time.Minute), env.Fx.Users.Ql1, &cn1, "user.lock")
	insertLog(t, env, base.Add(6*time.Minute), env.Fx.Users.Ql2, &cn2, "order.refund")
}

func TestL03_BranchManagerSeesOwnBranchOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	seedLogs(t, env)
	cn1 := env.Fx.Branches[1]
	ql1 := env.Ql1()
	for _, path := range []string{"/api/v1/audit-logs", fmt.Sprintf("/api/v1/audit-logs?branchId=%d", env.Fx.Branches[2])} {
		var l list
		ql1.Get(path).MustStatus(t, 200).JSON(t, &l)
		if l.Total != 3 || len(l.Items) != 3 {
			t.Fatalf("%s: total %d items %d", path, l.Total, len(l.Items))
		}
		for _, it := range l.Items {
			if it.BranchID == nil || *it.BranchID != cn1 {
				t.Fatalf("%s: foreign entry %+v", path, it)
			}
		}
	}
	env.Tn1().Get("/api/v1/audit-logs").MustStatus(t, 403)
	env.Anon().Get("/api/v1/audit-logs").MustStatus(t, 401)
}

func TestAuditLogList_OwnerFiltersAndPaging(t *testing.T) {
	env := testutil.NewEnv(t)
	seedLogs(t, env)
	owner := env.Owner()

	var all list
	owner.Get("/api/v1/audit-logs").MustStatus(t, 200).JSON(t, &all)
	if all.Total != 6 || len(all.Items) != 6 {
		t.Fatalf("owner all: %+v", all)
	}
	for i := 1; i < len(all.Items); i++ { // newest first
		if all.Items[i].At.After(all.Items[i-1].At) {
			t.Fatalf("not newest first: %+v", all.Items)
		}
	}
	if all.Items[0].Action != "order.refund" || all.Items[0].Actor == nil || all.Items[0].Actor.Name == "" || all.Items[0].Approver != nil {
		t.Fatalf("first item: %+v", all.Items[0])
	}
	if string(all.Items[0].Before) != "null" && all.Items[0].Before != nil {
		t.Fatalf("before should be null: %s", all.Items[0].Before)
	}

	var cn2 list
	owner.Get(fmt.Sprintf("/api/v1/audit-logs?branchId=%d", env.Fx.Branches[2])).MustStatus(t, 200).JSON(t, &cn2)
	if cn2.Total != 2 {
		t.Fatalf("owner CN2: %+v", cn2)
	}
	var disc list
	owner.Get("/api/v1/audit-logs?action=order.discount").MustStatus(t, 200).JSON(t, &disc)
	if disc.Total != 2 {
		t.Fatalf("action filter: %+v", disc)
	}
	var p2 list
	owner.Get("/api/v1/audit-logs?page=2&pageSize=4").MustStatus(t, 200).JSON(t, &p2)
	if p2.Total != 6 || len(p2.Items) != 2 || p2.Items[0].Action != "order.cancel" {
		t.Fatalf("page 2: %+v", p2)
	}
	var none list
	res := owner.Get("/api/v1/audit-logs?action=nothing").MustStatus(t, 200)
	res.JSON(t, &none)
	if none.Total != 0 || none.Items == nil {
		t.Fatalf("empty list must be []: %s", res.Body)
	}

	// The warehouse manager sees chain-level entries only (no branch).
	var wh list
	env.As(env.Fx.Users.Warehouse).Get("/api/v1/audit-logs").MustStatus(t, 200).JSON(t, &wh)
	if wh.Total != 1 || wh.Items[0].Action != "product.price" {
		t.Fatalf("warehouse: %+v", wh)
	}
}

// L-02: no API changes or deletes audit logs, and the database refuses it too.
func TestL02_AuditLogsAreAppendOnly(t *testing.T) {
	env := testutil.NewEnv(t)
	seedLogs(t, env)
	owner := env.Owner()
	var l list
	owner.Get("/api/v1/audit-logs").MustStatus(t, 200).JSON(t, &l)
	id := l.Items[0].ID
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, path := range []string{"/api/v1/audit-logs", fmt.Sprintf("/api/v1/audit-logs/%d", id)} {
			res := owner.Do(method, path, map[string]any{"note": "x"})
			if res.Status != 404 && res.Status != 405 {
				t.Errorf("%s %s = %d", method, path, res.Status)
			}
		}
	}
	ctx := context.Background()
	if _, err := env.Pool.Exec(ctx, `UPDATE audit_logs SET note = 'sửa' WHERE id = $1`, id); err == nil {
		t.Fatal("UPDATE on audit_logs succeeded")
	}
	if _, err := env.Pool.Exec(ctx, `DELETE FROM audit_logs WHERE id = $1`, id); err == nil {
		t.Fatal("DELETE on audit_logs succeeded")
	}
	var n int
	_ = env.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&n)
	if n != 6 {
		t.Fatalf("rows = %d", n)
	}
}

// L-01 (helper part): platform.Audit stores who, approver, branch, time and before/after values.
func TestL01_PlatformAuditWritesAllFields(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	cn1 := env.Fx.Branches[1]
	err := store.InTx(ctx, env.Pool, func(q *store.Queries, _ pgx.Tx) error {
		return platform.Audit(ctx, q, platform.AuditEntry{
			ActorID: &env.Fx.Users.Tn1, ApproverID: &env.Fx.Users.Ql1, BranchID: &cn1,
			Action: "order.discount", EntityType: "order", EntityID: "42",
			Before: map[string]any{"discountAmount": 0}, After: map[string]any{"discountAmount": 30000},
			Note: "Khách quen",
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var l list
	env.Ql1().Get("/api/v1/audit-logs").MustStatus(t, 200).JSON(t, &l)
	if len(l.Items) != 1 {
		t.Fatalf("items = %+v", l.Items)
	}
	it := l.Items[0]
	if it.Actor == nil || it.Actor.ID != env.Fx.Users.Tn1 || it.Actor.Name != "Lan" ||
		it.Approver == nil || it.Approver.ID != env.Fx.Users.Ql1 || it.Approver.Name != "Tuấn" ||
		it.BranchID == nil || *it.BranchID != cn1 || it.Action != "order.discount" || it.EntityType != "order" ||
		it.EntityID != "42" || it.Note != "Khách quen" || it.At.IsZero() {
		t.Fatalf("entry = %+v", it)
	}
	var before, after map[string]int64
	if json.Unmarshal(it.Before, &before) != nil || json.Unmarshal(it.After, &after) != nil ||
		before["discountAmount"] != 0 || after["discountAmount"] != 30000 {
		t.Fatalf("before/after = %s / %s", it.Before, it.After)
	}
}
