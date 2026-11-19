package orders_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

const base = "/api/v1"

func orderPath(id int64, suffix string) string {
	return fmt.Sprintf("%s/orders/%d%s", base, id, suffix)
}

func decodeOrder(t *testing.T, r *testutil.Resp) orderview.Order {
	t.Helper()
	var o orderview.Order
	r.JSON(t, &o)
	return o
}

func createDineIn(t *testing.T, c *testutil.Client, tableID int64) orderview.Order {
	t.Helper()
	return decodeOrder(t, c.Post(base+"/orders", map[string]any{"type": "dine_in", "tableId": tableID}).MustStatus(t, 201))
}

func createTakeaway(t *testing.T, c *testutil.Client) orderview.Order {
	t.Helper()
	return decodeOrder(t, c.Post(base+"/orders", map[string]any{"type": "takeaway"}).MustStatus(t, 201))
}

func addItem(c *testutil.Client, orderID, productID int64, size string, qty int, mods ...int64) *testutil.Resp {
	body := map[string]any{"productId": productID, "qty": qty, "modifierIds": mods}
	if size != "" {
		body["sizeCode"] = size
	}
	return c.Post(orderPath(orderID, "/items"), body)
}

func mustAdd(t *testing.T, c *testutil.Client, orderID, productID int64, size string, qty int, mods ...int64) orderview.Order {
	t.Helper()
	return decodeOrder(t, addItem(c, orderID, productID, size, qty, mods...).MustStatus(t, 201))
}

// floorTable returns one table of GET /tables.
type floorTable struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AreaID    int64  `json:"areaId"`
	Status    string `json:"status"`
	OpenOrder *struct {
		ID        int64  `json:"id"`
		Code      string `json:"code"`
		ItemCount int    `json:"itemCount"`
		Total     int64  `json:"total"`
	} `json:"openOrder"`
}

func floor(t *testing.T, c *testutil.Client) map[int64]floorTable {
	t.Helper()
	var areas []struct {
		ID     int64        `json:"id"`
		Name   string       `json:"name"`
		Tables []floorTable `json:"tables"`
	}
	c.Get(base+"/tables").MustStatus(t, 200).JSON(t, &areas)
	out := map[int64]floorTable{}
	for _, a := range areas {
		for _, tb := range a.Tables {
			out[tb.ID] = tb
		}
	}
	return out
}

type auditRow struct {
	ActorID    *int64
	ApproverID *int64
	Note       string
}

func audits(t *testing.T, env *testutil.Env, action string, orderID int64) []auditRow {
	t.Helper()
	rows, err := env.Pool.Query(context.Background(),
		`SELECT actor_id, approver_id, note FROM audit_logs WHERE action = $1 AND entity_type = 'order' AND entity_id = $2 ORDER BY id`,
		action, fmt.Sprint(orderID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []auditRow
	for rows.Next() {
		var a auditRow
		if err := rows.Scan(&a.ActorID, &a.ApproverID, &a.Note); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func exec(t *testing.T, env *testutil.Env, sql string, args ...any) {
	t.Helper()
	if _, err := env.Pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func hasEvent(env *testutil.Env, typ string) bool {
	for _, e := range env.Events.Events {
		if e.Type == typ {
			return true
		}
	}
	return false
}
