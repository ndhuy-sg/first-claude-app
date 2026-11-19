// Package orderview builds the Order JSON of openapi.yaml (schema "Order").
// Shared by the orders and payments modules so both return the same shape.
package orderview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ndhuy-sg/first-claude-app/api/internal/store"
)

type UserSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type Modifier struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

type Item struct {
	ID        int64      `json:"id"`
	ProductID int64      `json:"productId"`
	Name      string     `json:"name"`
	SizeCode  string     `json:"sizeCode"`
	SizeName  string     `json:"sizeName"`
	Modifiers []Modifier `json:"modifiers"`
	Qty       int32      `json:"qty"`
	UnitPrice int64      `json:"unitPrice"`
	LineTotal int64      `json:"lineTotal"`
	Note      string     `json:"note"`
}

type Discount struct {
	Type       string       `json:"type"`
	Value      int64        `json:"value"`
	Amount     int64        `json:"amount"`
	Reason     string       `json:"reason"`
	ApprovedBy *UserSummary `json:"approvedBy"`
}

type Payment struct {
	ID           int64      `json:"id"`
	Method       string     `json:"method"`
	Status       string     `json:"status"`
	Amount       int64      `json:"amount"`
	CashReceived *int64     `json:"cashReceived"`
	ChangeAmount *int64     `json:"changeAmount"`
	QRPayload    string     `json:"qrPayload"`
	CreatedAt    time.Time  `json:"createdAt"`
	ConfirmedAt  *time.Time `json:"confirmedAt"`
}

type Buyer struct {
	Name    string `json:"name"`
	TaxCode string `json:"taxCode"`
	Address string `json:"address"`
	Email   string `json:"email"`
}

type InvoiceSummary struct {
	ID         int64  `json:"id"`
	Symbol     string `json:"symbol"`
	Number     string `json:"number"`
	LookupCode string `json:"lookupCode"`
	LookupURL  string `json:"lookupUrl"`
	Status     string `json:"status"`
	Kind       string `json:"kind"`
}

type Order struct {
	ID         int64           `json:"id"`
	Code       string          `json:"code"`
	BranchID   int64           `json:"branchId"`
	Type       string          `json:"type"`
	TableID    *int64          `json:"tableId"`
	TableName  *string         `json:"tableName"`
	Status     string          `json:"status"`
	Items      []Item          `json:"items"`
	Subtotal   int64           `json:"subtotal"`
	Discount   *Discount       `json:"discount"`
	Total      int64           `json:"total"`
	PaidAmount int64           `json:"paidAmount"`
	Payments   []Payment       `json:"payments"`
	Buyer      Buyer           `json:"buyer"`
	Invoice    *InvoiceSummary `json:"invoice"`
	CreatedBy  UserSummary     `json:"createdBy"`
	CreatedAt  time.Time       `json:"createdAt"`
	PaidAt     *time.Time      `json:"paidAt"`
}

// InvoiceNumber formats an invoice number as 8 zero-padded digits.
func InvoiceNumber(n int64) string { return fmt.Sprintf("%08d", n) }

// LookupURL builds the public lookup link for an invoice code.
func LookupURL(publicBaseURL, code string) string { return publicBaseURL + "/tra-cuu/" + code }

// Load reads an order with items, modifiers, payments and its invoice.
// It does not check branch access; callers use auth.CheckBranch on Order.BranchID.
func Load(ctx context.Context, q *store.Queries, publicBaseURL string, id int64) (*Order, error) {
	h, err := q.OrderViewHeader(ctx, id)
	if err != nil {
		return nil, err
	}
	o := &Order{
		ID: h.ID, Code: h.Code, BranchID: h.BranchID, Type: h.OrderType, TableID: h.TableID, TableName: h.TableName,
		Status: h.Status, Subtotal: h.Subtotal, Total: h.Total,
		Buyer:     Buyer{Name: h.BuyerName, TaxCode: h.BuyerTaxCode, Address: h.BuyerAddress, Email: h.BuyerEmail},
		CreatedBy: UserSummary{ID: h.CreatedBy, Name: h.CreatedByName},
		CreatedAt: h.CreatedAt, PaidAt: h.PaidAt,
		Items: []Item{}, Payments: []Payment{},
	}
	if h.DiscountType != nil {
		d := &Discount{Type: *h.DiscountType, Value: h.DiscountValue, Amount: h.DiscountAmount, Reason: h.DiscountReason}
		if h.DiscountApprovedBy != nil && h.ApprovedByName != nil {
			d.ApprovedBy = &UserSummary{ID: *h.DiscountApprovedBy, Name: *h.ApprovedByName}
		}
		o.Discount = d
	}
	items, err := q.OrderViewItems(ctx, id)
	if err != nil {
		return nil, err
	}
	mods, err := q.OrderViewItemModifiers(ctx, id)
	if err != nil {
		return nil, err
	}
	byItem := map[int64][]Modifier{}
	for _, m := range mods {
		byItem[m.OrderItemID] = append(byItem[m.OrderItemID], Modifier{ID: m.ModifierID, Name: m.Name, Price: m.Price})
	}
	for _, it := range items {
		ms := byItem[it.ID]
		if ms == nil {
			ms = []Modifier{}
		}
		o.Items = append(o.Items, Item{
			ID: it.ID, ProductID: it.ProductID, Name: it.ProductName, SizeCode: it.SizeCode, SizeName: it.SizeName,
			Modifiers: ms, Qty: it.Qty, UnitPrice: it.UnitPrice, LineTotal: it.LineTotal, Note: it.Note,
		})
	}
	pays, err := q.OrderViewPayments(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, p := range pays {
		if p.Status == "confirmed" {
			o.PaidAmount += p.Amount
		}
		o.Payments = append(o.Payments, Payment{
			ID: p.ID, Method: p.Method, Status: p.Status, Amount: p.Amount, CashReceived: p.CashReceived,
			ChangeAmount: p.ChangeAmount, QRPayload: p.QrPayload, CreatedAt: p.CreatedAt, ConfirmedAt: p.ConfirmedAt,
		})
	}
	inv, err := q.OrderViewInvoice(ctx, id)
	if err == nil {
		o.Invoice = &InvoiceSummary{
			ID: inv.ID, Symbol: inv.Symbol, Number: InvoiceNumber(inv.Number), LookupCode: inv.LookupCode,
			LookupURL: LookupURL(publicBaseURL, inv.LookupCode), Status: inv.Status, Kind: inv.Kind,
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return o, nil
}
