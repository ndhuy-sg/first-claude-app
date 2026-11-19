package app_test

import (
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
