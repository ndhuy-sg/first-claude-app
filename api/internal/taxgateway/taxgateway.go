// Package taxgateway sends self-issued e-invoice data to the tax authority.
//
// The real transport is not known yet: the owner must first register with the
// tax authority (directly, or through a T-VAN data transmission provider), see
// docs/PLAN.md section 6. Until then the switch app_settings.tax_submission_enabled
// is OFF and nothing is sent. The pieces are kept apart so the real protocol can
// replace them without touching the rest of the system:
//
//   - Invoice:   the invoice data in a transport-neutral form (built by package invoices)
//   - BuildXML:  the XML document (structure modelled on the Vietnamese e-invoice
//     XML layout: TTChung / NDHDon / NBan / NMua / DSHHDVu / TToan); replace it
//     with the official schema once known (golden-file test: testdata/*.golden.xml)
//   - Gateway:   the transport (Off, Fake for tests, Direct and TVAN stubs)
package taxgateway

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Invoice is one invoice to submit. InternalID never changes between retries,
// so the receiver can recognise a re-sent invoice and never create a duplicate (E-05).
type Invoice struct {
	InternalID      string // e.g. "MOC-INV-42"
	Kind            string // normal | cancel | adjust
	Symbol          string // e.g. 1C26MAA
	Number          int64
	IssuedAt        time.Time
	Seller          Party
	Buyer           Party
	PaymentMethod   string
	VatRateBp       int
	Lines           []Line
	AmountBeforeVat int64
	VatAmount       int64
	Total           int64
	TotalInWords    string
	Original        *Reference // for cancel/adjust invoices
}

type Party struct {
	Name    string
	TaxCode string
	Address string
	Email   string
}

type Line struct {
	No        int
	Name      string
	Qty       int
	UnitPrice int64
	Amount    int64
}

// Reference points at the invoice a cancel/adjust invoice replaces.
type Reference struct {
	Symbol   string
	Number   int64
	IssuedAt time.Time
}

// Result is the tax authority's answer to a successful submission.
type Result struct {
	Reference string // receiver's id / code for the invoice
	Response  string // raw response, stored in tax_submissions.response
}

// Gateway submits invoice data. Implementations must be safe for concurrent use.
type Gateway interface {
	Name() string
	Submit(ctx context.Context, inv Invoice) (Result, error)
}

// ErrNotConfigured is returned by the transports that cannot work yet.
var ErrNotConfigured = errors.New("chưa cấu hình kết nối cơ quan thuế")

// ErrOff is returned by the Off gateway.
var ErrOff = errors.New("chưa chọn cách gửi dữ liệu tới cơ quan thuế")

// ---------- Off ----------

// Off is used when app_settings.tax_gateway = 'off'.
type Off struct{}

func (Off) Name() string                                    { return "off" }
func (Off) Submit(context.Context, Invoice) (Result, error) { return Result{}, ErrOff }

// ---------- Direct / TVAN stubs ----------

// Direct will submit straight to the tax authority once the business is approved for it.
// STUB: the protocol is unknown until the owner registers.
type Direct struct{}

func (Direct) Name() string { return "direct" }
func (Direct) Submit(context.Context, Invoice) (Result, error) {
	return Result{}, fmt.Errorf("direct: %w", ErrNotConfigured)
}

// TVAN will submit through a T-VAN data transmission provider.
// STUB: the provider and protocol are unknown until the owner registers.
type TVAN struct{}

func (TVAN) Name() string { return "tvan" }
func (TVAN) Submit(context.Context, Invoice) (Result, error) {
	return Result{}, fmt.Errorf("tvan: %w", ErrNotConfigured)
}

// ---------- Fake (tests, demo) ----------

type FakeMode int

const (
	FakeSuccess FakeMode = iota
	FakeError
	FakeTimeout // blocks until ctx is done
)

// Fake simulates the tax authority. It accepts each InternalID once: a re-sent
// invoice is acknowledged with the same reference and not stored twice.
type Fake struct {
	mu       sync.Mutex
	Mode     FakeMode
	Received map[string][]byte // InternalID -> XML of the first accepted submission
	Calls    int
}

func NewFake() *Fake { return &Fake{Received: map[string][]byte{}} }

func (f *Fake) Name() string { return "fake" }

func (f *Fake) SetMode(m FakeMode) {
	f.mu.Lock()
	f.Mode = m
	f.mu.Unlock()
}

// Accepted returns how many distinct invoices were accepted.
func (f *Fake) Accepted() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Received)
}

func (f *Fake) Submit(ctx context.Context, inv Invoice) (Result, error) {
	f.mu.Lock()
	f.Calls++
	mode := f.Mode
	f.mu.Unlock()
	switch mode {
	case FakeError:
		return Result{}, errors.New("fake: cơ quan thuế trả lỗi")
	case FakeTimeout:
		<-ctx.Done()
		return Result{}, fmt.Errorf("fake: không phản hồi: %w", ctx.Err())
	}
	x, err := BuildXML(inv)
	if err != nil {
		return Result{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Received == nil {
		f.Received = map[string][]byte{}
	}
	if _, ok := f.Received[inv.InternalID]; !ok {
		f.Received[inv.InternalID] = x
	}
	return Result{Reference: "FAKE-" + inv.InternalID, Response: "accepted " + inv.InternalID}, nil
}

// ForName returns the gateway for app_settings.tax_gateway. fake is used for "fake"
// (pass nil to get a fresh one that always succeeds).
func ForName(name string, fake Gateway) Gateway {
	switch name {
	case "direct":
		return Direct{}
	case "tvan":
		return TVAN{}
	case "fake":
		if fake != nil {
			return fake
		}
		return NewFake()
	}
	return Off{}
}
