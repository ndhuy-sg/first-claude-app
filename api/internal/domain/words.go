package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var digitWords = [10]string{"không", "một", "hai", "ba", "bốn", "năm", "sáu", "bảy", "tám", "chín"}

// readTriple reads a number 0..999. full=true means a higher group exists, so
// "không trăm" / "linh" are spoken (e.g. 1.005.000 → "một triệu không trăm linh năm nghìn").
func readTriple(n int64, full bool) []string {
	h, t, u := n/100, n/10%10, n%10
	var w []string
	if h > 0 || full {
		w = append(w, digitWords[h], "trăm")
	}
	switch {
	case t == 0 && u == 0:
		return w
	case t == 0:
		if h > 0 || full {
			w = append(w, "linh")
		}
	case t == 1:
		w = append(w, "mười")
	default:
		w = append(w, digitWords[t], "mươi")
	}
	switch {
	case u == 0:
	case u == 1 && t >= 2:
		w = append(w, "mốt")
	case u == 5 && t >= 1:
		w = append(w, "lăm")
	default:
		w = append(w, digitWords[u])
	}
	return w
}

// readNumber reads n ≥ 0 in Vietnamese words (lower case, no unit).
func readNumber(n int64, full bool) []string {
	const ty = 1_000_000_000
	if n >= ty {
		w := readNumber(n/ty, full)
		w = append(w, "tỷ")
		if rest := n % ty; rest > 0 {
			w = append(w, readNumber(rest, true)...)
		}
		return w
	}
	groups := []struct {
		v    int64
		unit string
	}{{n / 1_000_000, "triệu"}, {n / 1000 % 1000, "nghìn"}, {n % 1000, ""}}
	var w []string
	seen := full
	for _, g := range groups {
		if g.v == 0 {
			continue
		}
		w = append(w, readTriple(g.v, seen)...)
		if g.unit != "" {
			w = append(w, g.unit)
		}
		seen = true
	}
	return w
}

// AmountInWords spells a VND amount for invoices, e.g. 128000 →
// "Một trăm hai mươi tám nghìn đồng".
func AmountInWords(amount int64) string {
	prefix := ""
	if amount < 0 {
		prefix = "âm "
		amount = -amount
	}
	var s string
	if amount == 0 {
		s = "không"
	} else {
		s = strings.Join(readNumber(amount, false), " ")
	}
	s = prefix + s + " đồng"
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
