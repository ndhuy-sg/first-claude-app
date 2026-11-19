package domain

import (
	"errors"
	"time"
)

// All money is int64 VND. Never use floats for money.

var (
	ErrInvalidDiscount      = errors.New("invalid discount")
	ErrDiscountExceedsTotal = errors.New("discount exceeds subtotal")
	ErrInsufficientCash     = errors.New("cash received is less than amount")
)

// UnitPrice = base price + size delta + sum of modifier prices.
func UnitPrice(base, sizeDelta int64, modifierPrices []int64) int64 {
	p := base + sizeDelta
	for _, m := range modifierPrices {
		p += m
	}
	return p
}

// LineTotal = unit price × quantity.
func LineTotal(unit int64, qty int) int64 { return unit * int64(qty) }

type DiscountKind string

const (
	DiscountPercent DiscountKind = "percent"
	DiscountAmount  DiscountKind = "amount"
)

// ComputeDiscount returns the discount amount in VND for a subtotal.
// Percent discounts are rounded down to the đồng. value is a whole percent (0–100)
// for DiscountPercent and VND for DiscountAmount.
func ComputeDiscount(subtotal int64, kind DiscountKind, value int64) (int64, error) {
	switch kind {
	case DiscountPercent:
		if value < 0 || value > 100 {
			return 0, ErrInvalidDiscount
		}
		return subtotal * value / 100, nil
	case DiscountAmount:
		if value < 0 {
			return 0, ErrInvalidDiscount
		}
		if value > subtotal {
			return 0, ErrDiscountExceedsTotal
		}
		return value, nil
	}
	return 0, ErrInvalidDiscount
}

// DiscountNeedsApproval reports whether a discount is above maxPct of the subtotal
// and therefore needs a manager's PIN.
func DiscountNeedsApproval(subtotal, discountAmount int64, maxPct int) bool {
	return discountAmount*100 > subtotal*int64(maxPct)
}

// Change returns received − amount for a cash payment.
func Change(amount, received int64) (int64, error) {
	if received < amount {
		return 0, ErrInsufficientCash
	}
	return received - amount, nil
}

// CashSuggestions returns quick-pick amounts for the cash keypad: the exact
// amount, then amounts rounded up to 10.000, 50.000 and 100.000, then the
// 200.000 and 500.000 notes. All are ≥ total, distinct and ascending.
func CashSuggestions(total int64) []int64 {
	ceil := func(v, step int64) int64 { return (v + step - 1) / step * step }
	cands := []int64{total, ceil(total, 10_000), ceil(total, 50_000), ceil(total, 100_000), ceil(total, 200_000), ceil(total, 500_000)}
	out := []int64{}
	for _, c := range cands {
		if c >= total && (len(out) == 0 || c > out[len(out)-1]) {
			out = append(out, c)
		}
	}
	return out
}

// SplitVAT splits a VAT-inclusive amount into (before VAT, VAT). rateBp is in
// basis points (800 = 8%). The pre-tax amount is rounded half-up to the đồng
// and VAT is the remainder, so before + vat == gross always holds.
func SplitVAT(gross int64, rateBp int) (before, vat int64) {
	den := int64(10000 + rateBp)
	before = (gross*10000*2 + den) / (2 * den)
	return before, gross - before
}

// VN is the Asia/Ho_Chi_Minh time zone (UTC+7, no DST).
var VN = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("ICT", 7*3600)
	}
	return loc
}()

// BusinessDate returns the calendar date in Vietnam for t (midnight in VN).
func BusinessDate(t time.Time) time.Time {
	v := t.In(VN)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
}
