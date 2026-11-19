package domain

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestD06_Change(t *testing.T) {
	got, err := Change(86_000, 100_000)
	if err != nil || got != 14_000 {
		t.Fatalf("Change(86000, 100000) = %d, %v; want 14000", got, err)
	}
	if got, err := Change(86_000, 86_000); err != nil || got != 0 {
		t.Fatalf("exact cash: %d, %v", got, err)
	}
	if _, err := Change(86_000, 50_000); !errors.Is(err, ErrInsufficientCash) {
		t.Fatalf("less than total: err = %v, want ErrInsufficientCash", err)
	}
}

func TestD07_CashSuggestions(t *testing.T) {
	want := []int64{86_000, 90_000, 100_000, 200_000, 500_000}
	if got := CashSuggestions(86_000); !reflect.DeepEqual(got, want) {
		t.Fatalf("CashSuggestions(86000) = %v, want %v", got, want)
	}
	for _, total := range []int64{1, 10_000, 43_000, 128_000, 199_000, 500_000, 1_250_000} {
		got := CashSuggestions(total)
		if len(got) == 0 || got[0] != total {
			t.Fatalf("%d: first suggestion must be the exact amount: %v", total, got)
		}
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Fatalf("%d: not strictly ascending/distinct: %v", total, got)
			}
		}
		for _, v := range got {
			if v < total {
				t.Fatalf("%d: suggestion %d below total", total, v)
			}
		}
	}
}

func TestD09_VietQRPayload(t *testing.T) {
	s, err := BuildVietQR(VietQRInput{BankBIN: "970436", AccountNo: "0123456789", Amount: 128_000, Purpose: "CN1-20261119-0001"})
	if err != nil {
		t.Fatal(err)
	}
	// CRC: last 4 hex chars are CRC16-CCITT over everything before them (including "6304").
	body, crc := s[:len(s)-4], s[len(s)-4:]
	if !strings.HasSuffix(body, "6304") {
		t.Fatalf("missing CRC tag: %s", s)
	}
	if want := fmt.Sprintf("%04X", CRC16CCITT([]byte(body))); crc != want {
		t.Fatalf("crc = %s, want %s", crc, want)
	}
	top, err := ParseTLV(s)
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]string{"00": "01", "01": "12", "53": "704", "54": "128000", "58": "VN"}
	for id, want := range checks {
		if top[id] != want {
			t.Errorf("tag %s = %q, want %q", id, top[id], want)
		}
	}
	m, err := ParseTLV(top["38"])
	if err != nil {
		t.Fatal(err)
	}
	if m["00"] != VietQRGUID || m["02"] != "QRIBFTTA" {
		t.Errorf("merchant info = %v", m)
	}
	ben, err := ParseTLV(m["01"])
	if err != nil {
		t.Fatal(err)
	}
	if ben["00"] != "970436" || ben["01"] != "0123456789" {
		t.Errorf("beneficiary = %v", ben)
	}
	add, err := ParseTLV(top["62"])
	if err != nil {
		t.Fatal(err)
	}
	if add["08"] != "CN1-20261119-0001" {
		t.Errorf("purpose = %q", add["08"])
	}
	if _, err := BuildVietQR(VietQRInput{Amount: 1}); !errors.Is(err, ErrVietQRAccount) {
		t.Errorf("missing account: err = %v", err)
	}
}

func TestCRC16CCITTKnownVector(t *testing.T) {
	if got := CRC16CCITT([]byte("123456789")); got != 0x29B1 {
		t.Fatalf("crc = %04X, want 29B1", got)
	}
}

func TestD11_ShiftCashReconciliation(t *testing.T) {
	exp := ExpectedCash(1_000_000, 556_000, 43_000)
	if exp != 1_513_000 {
		t.Fatalf("expected = %d, want 1513000", exp)
	}
	if d := CashDiff(1_500_000, exp); d != -13_000 {
		t.Fatalf("diff = %d, want -13000", d)
	}
	if d := CashDiff(1_520_000, exp); d != 7_000 {
		t.Fatalf("diff = %d, want 7000", d)
	}
	if ShiftNoteRequired(-50_000, 50_000) || ShiftNoteRequired(50_000, 50_000) {
		t.Fatal("exactly the threshold does not need a note")
	}
	if !ShiftNoteRequired(-50_001, 50_000) || !ShiftNoteRequired(60_000, 50_000) {
		t.Fatal("above the threshold needs a note")
	}
}

func TestAmountInWords(t *testing.T) {
	cases := map[int64]string{
		0:             "Không đồng",
		5:             "Năm đồng",
		15:            "Mười lăm đồng",
		21:            "Hai mươi mốt đồng",
		105:           "Một trăm linh năm đồng",
		1_000:         "Một nghìn đồng",
		9_481:         "Chín nghìn bốn trăm tám mươi mốt đồng",
		43_000:        "Bốn mươi ba nghìn đồng",
		118_519:       "Một trăm mười tám nghìn năm trăm mười chín đồng",
		128_000:       "Một trăm hai mươi tám nghìn đồng",
		1_005_000:     "Một triệu không trăm linh năm nghìn đồng",
		1_250_000:     "Một triệu hai trăm năm mươi nghìn đồng",
		2_000_015:     "Hai triệu không trăm mười lăm đồng",
		1_000_000_000: "Một tỷ đồng",
		3_200_000_000: "Ba tỷ hai trăm triệu đồng",
	}
	for n, want := range cases {
		if got := AmountInWords(n); got != want {
			t.Errorf("AmountInWords(%d) = %q, want %q", n, got, want)
		}
	}
}
