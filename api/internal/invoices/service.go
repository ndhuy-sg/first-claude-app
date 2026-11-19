// Package invoices issues self-issued e-invoices (numbering, lookup codes,
// seller/buyer data, VAT split) and serves the invoice list, detail, resend,
// email and public lookup endpoints.
//
// Issue / IssueCancel are called by the payments module inside its own
// transaction, so a paid order always has its invoice and submit job (E-01).
//
// Discounts: an order discount is written as a separate negative line
// ("Chiết khấu thương mại", qty 1) so item lines keep their real prices and the
// lines always add up to the invoice total.
package invoices

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ndhuy-sg/first-claude-app/api/internal/domain"
	"github.com/ndhuy-sg/first-claude-app/api/internal/orderview"
	"github.com/ndhuy-sg/first-claude-app/api/internal/platform"
	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
	"github.com/ndhuy-sg/first-claude-app/api/internal/taxgateway"
)

const (
	JobSubmit = "invoice.submit"
	JobEmail  = "invoice.email"

	RetailBuyer   = "Khách lẻ"
	DiscountLine  = "Chiết khấu thương mại"
	MethodCash    = "Tiền mặt"
	MethodQR      = "Chuyển khoản"
	MethodMixed   = "Tiền mặt/Chuyển khoản"
	lookupAlpha   = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O, 1/I
	lookupCodeLen = 8
)

// SubmitDedupeKey is the dedupe key of an invoice's submit job.
func SubmitDedupeKey(invoiceID int64) string { return fmt.Sprintf("%s:%d", JobSubmit, invoiceID) }

// SubmitPayload is the payload of invoice.submit jobs.
type SubmitPayload struct {
	InvoiceID int64 `json:"invoiceId"`
}

// EmailPayload is the payload of invoice.email jobs.
type EmailPayload struct {
	InvoiceID int64  `json:"invoiceId"`
	Email     string `json:"email"`
}

// Issued describes a newly issued invoice.
type Issued struct {
	ID         int64
	Symbol     string
	Number     int64
	LookupCode string
}

// NewLookupCode returns a random code "XXXX-XXXX" (32^8 ≈ 1.1e12 combinations, crypto/rand).
func NewLookupCode() string {
	b := make([]byte, 0, lookupCodeLen+1)
	for i := 0; i < lookupCodeLen; i++ {
		if i == 4 {
			b = append(b, '-')
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(lookupAlpha))))
		if err != nil {
			panic(err)
		}
		b = append(b, lookupAlpha[n.Int64()])
	}
	return string(b)
}

// NormalizeLookupCode upper-cases a code and restores the dash ("7k3p9qxa" → "7K3P-9QXA").
func NormalizeLookupCode(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ""))
	if len(s) != lookupCodeLen {
		return s
	}
	return s[:4] + "-" + s[4:]
}

// PaymentMethodText describes how an order was paid, from the confirmed amounts per method.
func PaymentMethodText(cash, qr int64) string {
	switch {
	case cash > 0 && qr > 0:
		return MethodMixed
	case qr > 0:
		return MethodQR
	}
	return MethodCash
}

// LineName is the invoice line text of an order item: "Cà phê sữa (L, Thêm shot)".
func LineName(product, size string, modifiers []string) string {
	extra := []string{}
	if size != "" {
		extra = append(extra, size)
	}
	extra = append(extra, modifiers...)
	if len(extra) == 0 {
		return product
	}
	return product + " (" + strings.Join(extra, ", ") + ")"
}

type header struct {
	branchID          int64
	orderID           int64
	kind              string
	originalInvoiceID *int64
	sellerName        string
	sellerTaxCode     string
	sellerAddress     string
	buyer             orderview.Buyer
	paymentMethod     string
	vatRateBp         int32
	before, vat       int64
	total             int64
}

type line struct {
	name      string
	qty       int32
	unitPrice int64
	amount    int64
}

// insert takes the next number of the branch series (row locked until commit, so
// concurrent payments get consecutive numbers without gaps, E-10), stores the
// invoice with its lines and enqueues the submit job.
func insert(ctx context.Context, q *store.Queries, now time.Time, h header, lines []line) (*Issued, error) {
	series, err := q.InvLockInvoiceSeries(ctx, h.branchID)
	if err != nil {
		return nil, fmt.Errorf("invoice series of branch %d: %w", h.branchID, err)
	}
	if err := q.InvBumpInvoiceSeries(ctx, h.branchID); err != nil {
		return nil, err
	}
	var code string
	for i := 0; ; i++ {
		code = NewLookupCode()
		exists, err := q.InvLookupCodeExists(ctx, code)
		if err != nil {
			return nil, err
		}
		if !exists {
			break
		}
		if i > 10 {
			return nil, errors.New("cannot find a free lookup code")
		}
	}
	id, err := q.InvInsertInvoice(ctx, store.InvInsertInvoiceParams{
		BranchID: h.branchID, OrderID: h.orderID, Kind: h.kind, OriginalInvoiceID: h.originalInvoiceID,
		Symbol: series.Symbol, Number: series.NextNumber, LookupCode: code,
		SellerName: h.sellerName, SellerTaxCode: h.sellerTaxCode, SellerAddress: h.sellerAddress,
		BuyerName: h.buyer.Name, BuyerTaxCode: h.buyer.TaxCode, BuyerAddress: h.buyer.Address, BuyerEmail: h.buyer.Email,
		PaymentMethod: h.paymentMethod, VatRateBp: h.vatRateBp, AmountBeforeVat: h.before, VatAmount: h.vat, Total: h.total,
		IssuedAt: now,
	})
	if err != nil {
		return nil, err
	}
	for i, l := range lines {
		if err := q.InvInsertInvoiceLine(ctx, store.InvInsertInvoiceLineParams{
			InvoiceID: id, LineNo: int32(i + 1), Name: l.name, Qty: l.qty, UnitPrice: l.unitPrice, Amount: l.amount,
		}); err != nil {
			return nil, err
		}
	}
	if _, err := platform.EnqueueJob(ctx, q, JobSubmit, SubmitPayload{InvoiceID: id}, SubmitDedupeKey(id), now); err != nil {
		return nil, err
	}
	return &Issued{ID: id, Symbol: series.Symbol, Number: series.NextNumber, LookupCode: code}, nil
}

// Issue creates the normal invoice of a just-paid order and enqueues its submit job
// (and an email job when the buyer gave an email). Call it inside the payment transaction.
func Issue(ctx context.Context, q *store.Queries, d *platform.Deps, orderID int64) (*Issued, error) {
	now := d.Clock.Now()
	o, err := q.OrderViewHeader(ctx, orderID)
	if err != nil {
		return nil, err
	}
	br, err := q.GetBranch(ctx, o.BranchID)
	if err != nil {
		return nil, err
	}
	st, err := q.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	sums, err := q.PayOrderPaymentSums(ctx, orderID)
	if err != nil {
		return nil, err
	}
	items, err := q.OrderViewItems(ctx, orderID)
	if err != nil {
		return nil, err
	}
	mods, err := q.OrderViewItemModifiers(ctx, orderID)
	if err != nil {
		return nil, err
	}
	modNames := map[int64][]string{}
	for _, m := range mods {
		modNames[m.OrderItemID] = append(modNames[m.OrderItemID], m.Name)
	}
	lines := make([]line, 0, len(items)+1)
	for _, it := range items {
		size := it.SizeName
		if size == "" {
			size = it.SizeCode
		}
		lines = append(lines, line{
			name: LineName(it.ProductName, size, modNames[it.ID]), qty: it.Qty, unitPrice: it.UnitPrice, amount: it.LineTotal,
		})
	}
	if o.DiscountAmount > 0 {
		lines = append(lines, line{name: DiscountLine, qty: 1, unitPrice: -o.DiscountAmount, amount: -o.DiscountAmount})
	}
	taxCode := br.TaxCode
	if taxCode == "" {
		taxCode = st.CompanyTaxCode
	}
	buyer := orderview.Buyer{Name: o.BuyerName, TaxCode: o.BuyerTaxCode, Address: o.BuyerAddress, Email: o.BuyerEmail}
	if strings.TrimSpace(buyer.Name) == "" {
		buyer.Name = RetailBuyer
	}
	before, vat := domain.SplitVAT(o.Total, int(br.VatRateBp))
	iss, err := insert(ctx, q, now, header{
		branchID: o.BranchID, orderID: orderID, kind: "normal",
		sellerName: st.CompanyName, sellerTaxCode: taxCode, sellerAddress: br.Address,
		buyer: buyer, paymentMethod: PaymentMethodText(sums.ConfirmedCash, sums.ConfirmedQr),
		vatRateBp: br.VatRateBp, before: before, vat: vat, total: o.Total,
	}, lines)
	if err != nil {
		return nil, err
	}
	if buyer.Email != "" {
		if _, err := platform.EnqueueJob(ctx, q, JobEmail, EmailPayload{InvoiceID: iss.ID, Email: buyer.Email},
			fmt.Sprintf("%s:%d:auto", JobEmail, iss.ID), now); err != nil {
			return nil, err
		}
	}
	return iss, nil
}

// IssueCancel creates a cancel invoice (kind 'cancel', new number from the same
// series, same lines and amounts, original_invoice_id set) for a refunded order
// and enqueues its submission (E-08). Returns nil when the order has no invoice.
func IssueCancel(ctx context.Context, q *store.Queries, d *platform.Deps, orderID int64) (*Issued, error) {
	orig, err := q.InvGetNormalInvoiceByOrder(ctx, orderID)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	ls, err := q.InvListInvoiceLines(ctx, orig.ID)
	if err != nil {
		return nil, err
	}
	lines := make([]line, 0, len(ls))
	for _, l := range ls {
		lines = append(lines, line{name: l.Name, qty: l.Qty, unitPrice: l.UnitPrice, amount: l.Amount})
	}
	return insert(ctx, q, d.Clock.Now(), header{
		branchID: orig.BranchID, orderID: orderID, kind: "cancel", originalInvoiceID: &orig.ID,
		sellerName: orig.SellerName, sellerTaxCode: orig.SellerTaxCode, sellerAddress: orig.SellerAddress,
		buyer:         orderview.Buyer{Name: orig.BuyerName, TaxCode: orig.BuyerTaxCode, Address: orig.BuyerAddress, Email: orig.BuyerEmail},
		paymentMethod: orig.PaymentMethod, vatRateBp: orig.VatRateBp, before: orig.AmountBeforeVat, vat: orig.VatAmount, total: orig.Total,
	}, lines)
}

// ---------- Read model ----------

type Seller struct {
	Name    string `json:"name"`
	TaxCode string `json:"taxCode"`
	Address string `json:"address"`
}

type Line struct {
	LineNo    int32  `json:"lineNo"`
	Name      string `json:"name"`
	Qty       int32  `json:"qty"`
	UnitPrice int64  `json:"unitPrice"`
	Amount    int64  `json:"amount"`
}

// Invoice is the "Invoice" schema of openapi.yaml.
type Invoice struct {
	orderview.InvoiceSummary
	BranchID        int64           `json:"branchId"`
	OrderID         int64           `json:"orderId"`
	OrderCode       string          `json:"orderCode"`
	Seller          Seller          `json:"seller"`
	Buyer           orderview.Buyer `json:"buyer"`
	PaymentMethod   string          `json:"paymentMethod"`
	Lines           []Line          `json:"lines"`
	VatRateBp       int32           `json:"vatRateBp"`
	AmountBeforeVat int64           `json:"amountBeforeVat"`
	VatAmount       int64           `json:"vatAmount"`
	Total           int64           `json:"total"`
	TotalInWords    string          `json:"totalInWords"`
	IssuedAt        time.Time       `json:"issuedAt"`
	SentAt          *time.Time      `json:"sentAt"`
	Cancelled       bool            `json:"cancelled"`

	originalInvoiceID *int64
}

// Load reads one invoice with its lines. It does not check branch access.
func Load(ctx context.Context, q *store.Queries, publicBaseURL string, id int64) (*Invoice, error) {
	r, err := q.InvGetInvoiceRow(ctx, id)
	if err != nil {
		return nil, err
	}
	ls, err := q.InvListInvoiceLines(ctx, id)
	if err != nil {
		return nil, err
	}
	inv := &Invoice{
		InvoiceSummary: orderview.InvoiceSummary{
			ID: r.ID, Symbol: r.Symbol, Number: orderview.InvoiceNumber(r.Number), LookupCode: r.LookupCode,
			LookupURL: orderview.LookupURL(publicBaseURL, r.LookupCode), Status: r.Status, Kind: r.Kind,
		},
		BranchID: r.BranchID, OrderID: r.OrderID, OrderCode: r.OrderCode,
		Seller:        Seller{Name: r.SellerName, TaxCode: r.SellerTaxCode, Address: r.SellerAddress},
		Buyer:         orderview.Buyer{Name: r.BuyerName, TaxCode: r.BuyerTaxCode, Address: r.BuyerAddress, Email: r.BuyerEmail},
		PaymentMethod: r.PaymentMethod, Lines: make([]Line, 0, len(ls)), VatRateBp: r.VatRateBp,
		AmountBeforeVat: r.AmountBeforeVat, VatAmount: r.VatAmount, Total: r.Total,
		TotalInWords: domain.AmountInWords(r.Total), IssuedAt: r.IssuedAt, SentAt: r.SentAt, Cancelled: r.Cancelled,
		originalInvoiceID: r.OriginalInvoiceID,
	}
	for _, l := range ls {
		inv.Lines = append(inv.Lines, Line{LineNo: l.LineNo, Name: l.Name, Qty: l.Qty, UnitPrice: l.UnitPrice, Amount: l.Amount})
	}
	return inv, nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// InternalID is the stable id sent with every submission of an invoice (E-05).
func InternalID(invoiceID int64) string { return fmt.Sprintf("MOC-INV-%d", invoiceID) }

// LoadPayload builds the tax gateway payload of an invoice.
func LoadPayload(ctx context.Context, q *store.Queries, id int64) (taxgateway.Invoice, error) {
	inv, err := Load(ctx, q, "", id)
	if err != nil {
		return taxgateway.Invoice{}, err
	}
	r, err := q.InvGetInvoiceRow(ctx, id)
	if err != nil {
		return taxgateway.Invoice{}, err
	}
	p := taxgateway.Invoice{
		InternalID: InternalID(id), Kind: inv.Kind, Symbol: inv.Symbol, Number: r.Number, IssuedAt: inv.IssuedAt,
		Seller:        taxgateway.Party{Name: inv.Seller.Name, TaxCode: inv.Seller.TaxCode, Address: inv.Seller.Address},
		Buyer:         taxgateway.Party{Name: inv.Buyer.Name, TaxCode: inv.Buyer.TaxCode, Address: inv.Buyer.Address, Email: inv.Buyer.Email},
		PaymentMethod: inv.PaymentMethod, VatRateBp: int(inv.VatRateBp),
		AmountBeforeVat: inv.AmountBeforeVat, VatAmount: inv.VatAmount, Total: inv.Total, TotalInWords: inv.TotalInWords,
	}
	for _, l := range inv.Lines {
		p.Lines = append(p.Lines, taxgateway.Line{No: int(l.LineNo), Name: l.Name, Qty: int(l.Qty), UnitPrice: l.UnitPrice, Amount: l.Amount})
	}
	if inv.originalInvoiceID != nil {
		o, err := q.InvGetInvoiceRow(ctx, *inv.originalInvoiceID)
		if err != nil {
			return taxgateway.Invoice{}, err
		}
		p.Original = &taxgateway.Reference{Symbol: o.Symbol, Number: o.Number, IssuedAt: o.IssuedAt}
	}
	return p, nil
}
