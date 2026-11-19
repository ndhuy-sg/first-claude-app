package domain

import "testing"

func TestD13_PINMustBe4To6Digits(t *testing.T) {
	ok := []string{"1234", "12345", "123456", "0000"}
	bad := []string{"", "123", "1234567", "12a4", "abcd", " 1234", "1234 ", "١٢٣٤", "12.4", "-123"}
	for _, p := range ok {
		if err := ValidatePIN(p); err != nil {
			t.Errorf("ValidatePIN(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range bad {
		if err := ValidatePIN(p); err == nil {
			t.Errorf("ValidatePIN(%q) = nil, want error", p)
		}
	}
}
