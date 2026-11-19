package domain

// ExpectedCash is the cash that should be in the drawer when a shift closes:
// opening cash + cash sales − cash refunds (test D-11).
func ExpectedCash(openingCash, cashSales, cashRefunds int64) int64 {
	return openingCash + cashSales - cashRefunds
}

// CashDiff is counted − expected: positive = surplus, negative = shortage.
func CashDiff(counted, expected int64) int64 { return counted - expected }

// ShiftNoteRequired reports whether a close note is mandatory: the absolute
// difference is above threshold (app_settings.shift_note_threshold, test S-04).
func ShiftNoteRequired(diff, threshold int64) bool {
	if diff < 0 {
		diff = -diff
	}
	return diff > threshold
}
