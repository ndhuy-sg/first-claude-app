package taxgateway

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

func ban5Invoice() Invoice {
	return Invoice{
		InternalID: "MOC-INV-1", Kind: "normal", Symbol: "1C26MAA", Number: 1,
		IssuedAt:      time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC),
		Seller:        Party{Name: "Chuỗi cà phê Mộc", TaxCode: "0100000000", Address: "Địa chỉ CN1"},
		Buyer:         Party{Name: "Khách lẻ"},
		PaymentMethod: "Tiền mặt", VatRateBp: 800,
		Lines: []Line{
			{No: 1, Name: "Cà phê sữa (L)", Qty: 2, UnitPrice: 43_000, Amount: 86_000},
			{No: 2, Name: "Trà đào cam sả (M)", Qty: 1, UnitPrice: 42_000, Amount: 42_000},
		},
		AmountBeforeVat: 118_519, VatAmount: 9_481, Total: 128_000,
		TotalInWords: "Một trăm hai mươi tám nghìn đồng",
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("XML differs from %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestE12_InvoiceXMLGolden(t *testing.T) {
	x, err := BuildXML(ban5Invoice())
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "invoice_ban5.golden.xml", x)

	c := ban5Invoice()
	c.InternalID, c.Kind, c.Number = "MOC-INV-2", "cancel", 2
	c.Original = &Reference{Symbol: "1C26MAA", Number: 1, IssuedAt: c.IssuedAt}
	x, err = BuildXML(c)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "invoice_cancel.golden.xml", x)
}

func TestRateText(t *testing.T) {
	for bp, want := range map[int]string{800: "8%", 1000: "10%", 0: "0%", 525: "5.25%"} {
		if got := RateText(bp); got != want {
			t.Errorf("RateText(%d) = %q, want %q", bp, got, want)
		}
	}
}

func TestStubsAndFake(t *testing.T) {
	ctx := context.Background()
	for _, g := range []Gateway{Direct{}, TVAN{}} {
		if _, err := g.Submit(ctx, ban5Invoice()); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("%s: err = %v, want ErrNotConfigured", g.Name(), err)
		}
	}
	if _, err := (Off{}).Submit(ctx, ban5Invoice()); !errors.Is(err, ErrOff) {
		t.Errorf("off: %v", err)
	}
	f := NewFake()
	for i := 0; i < 3; i++ { // same InternalID re-sent: accepted once (E-05)
		if _, err := f.Submit(ctx, ban5Invoice()); err != nil {
			t.Fatal(err)
		}
	}
	if f.Accepted() != 1 || f.Calls != 3 {
		t.Fatalf("accepted = %d, calls = %d", f.Accepted(), f.Calls)
	}
	f.SetMode(FakeError)
	if _, err := f.Submit(ctx, ban5Invoice()); err == nil {
		t.Fatal("error mode must fail")
	}
	f.SetMode(FakeTimeout)
	tctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if _, err := f.Submit(tctx, ban5Invoice()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout mode: %v", err)
	}
	if ForName("off", nil).Name() != "off" || ForName("fake", f) != f || ForName("tvan", nil).Name() != "tvan" {
		t.Fatal("ForName")
	}
}
