package reports

import "testing"

func TestFormatVND(t *testing.T) {
	for in, want := range map[int64]string{0: "0đ", 500: "500đ", 60000: "60.000đ", -60000: "-60.000đ", 1234567: "1.234.567đ", 100000: "100.000đ"} {
		if got := FormatVND(in); got != want {
			t.Errorf("FormatVND(%d) = %q, want %q", in, got, want)
		}
	}
}
