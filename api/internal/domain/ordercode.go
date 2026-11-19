package domain

import (
	"fmt"
	"time"
)

// OrderCode formats an order code such as CN1-20261119-0001: branch code, VN
// business date (as returned by BusinessDate) and a per-branch, per-day
// sequence padded to 4 digits.
func OrderCode(branchCode string, businessDate time.Time, seq int) string {
	return fmt.Sprintf("%s-%s-%04d", branchCode, businessDate.Format("20060102"), seq)
}
