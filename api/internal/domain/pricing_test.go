package domain

import (
	"errors"
	"testing"
)

// Reference product from the test cases: "Cà phê sữa" 29.000đ, size L +6.000đ,
// "Thêm shot" +8.000đ, "Ít đá" +0đ.
const (
	caPheSuaBase = 29_000
	sizeL        = 6_000
	themShot     = 8_000
	itDa         = 0
)

func TestD01_LineTotal(t *testing.T) {
	unit := UnitPrice(caPheSuaBase, sizeL, []int64{themShot})
	if unit != 43_000 {
		t.Fatalf("unit price = %d, want 43000", unit)
	}
	if got := LineTotal(unit, 2); got != 86_000 {
		t.Fatalf("line total = %d, want 86000", got)
	}
	if got := UnitPrice(caPheSuaBase, 0, nil); got != 29_000 {
		t.Fatalf("no size/modifier = %d, want 29000", got)
	}
}

func TestD02_ZeroPriceModifier(t *testing.T) {
	without := UnitPrice(caPheSuaBase, sizeL, []int64{themShot})
	with := UnitPrice(caPheSuaBase, sizeL, []int64{themShot, itDa})
	if with != without {
		t.Fatalf("ít đá changed the price: %d vs %d", with, without)
	}
}

func TestD03_PercentDiscount(t *testing.T) {
	d, err := ComputeDiscount(86_000, DiscountPercent, 10)
	if err != nil {
		t.Fatal(err)
	}
	if d != 8_600 || 86_000-d != 77_400 {
		t.Fatalf("discount = %d (total %d), want 8600 (77400)", d, 86_000-d)
	}
	// rounded down to the đồng: 10% of 12.345 = 1.234,5 → 1.234
	if d, _ := ComputeDiscount(12_345, DiscountPercent, 10); d != 1_234 {
		t.Fatalf("rounding: discount = %d, want 1234", d)
	}
	for _, bad := range []int64{-1, 101} {
		if _, err := ComputeDiscount(86_000, DiscountPercent, bad); !errors.Is(err, ErrInvalidDiscount) {
			t.Fatalf("percent %d: err = %v, want ErrInvalidDiscount", bad, err)
		}
	}
}

func TestD04_AmountDiscountAboveTotalRejected(t *testing.T) {
	if _, err := ComputeDiscount(86_000, DiscountAmount, 86_001); !errors.Is(err, ErrDiscountExceedsTotal) {
		t.Fatalf("err = %v, want ErrDiscountExceedsTotal", err)
	}
	if d, err := ComputeDiscount(86_000, DiscountAmount, 86_000); err != nil || 86_000-d != 0 {
		t.Fatalf("full discount: d=%d err=%v", d, err)
	}
	if _, err := ComputeDiscount(86_000, DiscountAmount, -1); !errors.Is(err, ErrInvalidDiscount) {
		t.Fatalf("negative: err = %v, want ErrInvalidDiscount", err)
	}
	if _, err := ComputeDiscount(86_000, DiscountKind("other"), 1); !errors.Is(err, ErrInvalidDiscount) {
		t.Fatalf("unknown kind: err = %v", err)
	}
}

func TestD05_DiscountAbove20PctNeedsApproval(t *testing.T) {
	cases := []struct {
		discount int64
		want     bool
	}{
		{0, false},
		{8_600, false},  // 10%
		{17_200, false}, // exactly 20%
		{17_201, true},  // just above 20%
		{21_500, true},  // 25%
	}
	for _, c := range cases {
		if got := DiscountNeedsApproval(86_000, c.discount, 20); got != c.want {
			t.Errorf("discount %d of 86000: needsApproval = %v, want %v", c.discount, got, c.want)
		}
	}
}

func TestD08_SplitVAT(t *testing.T) {
	cases := []struct {
		gross       int64
		rateBp      int
		before, vat int64
	}{
		{86_000, 800, 79_630, 6_370},
		{86_000, 1000, 78_182, 7_818},
		{0, 800, 0, 0},
		{43_000, 0, 43_000, 0},
	}
	for _, c := range cases {
		b, v := SplitVAT(c.gross, c.rateBp)
		if b != c.before || v != c.vat {
			t.Errorf("SplitVAT(%d, %d) = (%d, %d), want (%d, %d)", c.gross, c.rateBp, b, v, c.before, c.vat)
		}
	}
	// before + vat == gross for many amounts
	for g := int64(0); g < 200_000; g += 997 {
		b, v := SplitVAT(g, 800)
		if b+v != g || v < 0 {
			t.Fatalf("SplitVAT(%d, 800) = (%d, %d)", g, b, v)
		}
	}
}
