package app_test

import (
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"testing"

	"github.com/ndhuy-sg/first-claude-app/api/internal/testutil"
)

func TestHealthzAndSeed(t *testing.T) {
	env := testutil.NewEnv(t)
	env.Anon().Get("/api/v1/healthz").MustStatus(t, 200)
	if env.Fx.Branches[1] == 0 || env.Fx.Users.Tn1 == 0 || env.Fx.Products.CaPheSua == 0 || env.Fx.TablesCN1[5] == 0 {
		t.Fatalf("fixtures not loaded: %+v", env.Fx)
	}
	var n int
	if err := env.Pool.QueryRow(t.Context(), "SELECT count(*) FROM products").Scan(&n); err != nil || n != 18 {
		t.Fatalf("products = %d, err %v", n, err)
	}
}

func TestFixtureHelpersAndOrderView(t *testing.T) {
	env := testutil.NewEnv(t)
	shift := env.OpenShiftCN1()
	id := env.InsertBan5Order(shift)
	o, err := orderview.Load(t.Context(), env.Queries, "http://x", id)
	if err != nil {
		t.Fatal(err)
	}
	if o.Total != 128_000 || len(o.Items) != 2 || o.Code != "CN1-20261119-0001" || o.Status != "open" {
		t.Fatalf("unexpected order: %+v", o)
	}
}
