package domain

import "regexp"

var taxCodeRe = regexp.MustCompile(`^[0-9]{10}(-[0-9]{3})?$`)

// ValidateTaxCode reports whether s is a Vietnamese tax code: 10 digits, or
// 10 digits + "-" + 3 digits (dependent unit).
func ValidateTaxCode(s string) bool { return taxCodeRe.MatchString(s) }
