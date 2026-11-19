package domain

import "testing"

func TestD14_ValidateTaxCode(t *testing.T) {
	for _, ok := range []string{"0101234567", "0101234567-001"} {
		if !ValidateTaxCode(ok) {
			t.Errorf("ValidateTaxCode(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"12345", "abcdefghij", "", "01012345678", "0101234567-01", "0101234567-0011", "0101234567001", " 0101234567", "010123456a"} {
		if ValidateTaxCode(bad) {
			t.Errorf("ValidateTaxCode(%q) = true, want false", bad)
		}
	}
}
