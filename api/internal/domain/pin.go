package domain

import "errors"

// ErrInvalidPIN is returned by ValidatePIN.
var ErrInvalidPIN = errors.New("PIN phải gồm 4–6 chữ số")

// ValidatePIN accepts only 4 to 6 ASCII digits (test D-13).
// Uniqueness within a branch needs the database and is checked by the users module.
func ValidatePIN(pin string) error {
	if len(pin) < 4 || len(pin) > 6 {
		return ErrInvalidPIN
	}
	for i := 0; i < len(pin); i++ {
		if pin[i] < '0' || pin[i] > '9' {
			return ErrInvalidPIN
		}
	}
	return nil
}
