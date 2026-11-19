package domain

import (
	"testing"
	"time"
)

func TestD10_OrderCode(t *testing.T) {
	d := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		branch string
		seq    int
		want   string
	}{
		{"CN1", 1, "CN1-20261119-0001"},
		{"CN1", 2, "CN1-20261119-0002"},
		{"CN2", 42, "CN2-20261119-0042"},
		{"CN1", 12345, "CN1-20261119-12345"},
	}
	for _, c := range cases {
		if got := OrderCode(c.branch, d, c.seq); got != c.want {
			t.Errorf("OrderCode(%q, %d) = %q, want %q", c.branch, c.seq, got, c.want)
		}
	}
	// The date part is the VN business date: 18/11 17:30 UTC is already 19/11 00:30 in Vietnam.
	late := time.Date(2026, 11, 18, 17, 30, 0, 0, time.UTC)
	if got := OrderCode("CN1", BusinessDate(late), 1); got != "CN1-20261119-0001" {
		t.Errorf("VN business date code = %q", got)
	}
	early := time.Date(2026, 11, 18, 16, 59, 0, 0, time.UTC) // 23:59 VN on 18/11
	if got := OrderCode("CN1", BusinessDate(early), 1); got != "CN1-20261118-0001" {
		t.Errorf("VN business date code = %q", got)
	}
}
