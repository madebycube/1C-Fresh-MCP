package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type Receipt struct {
	Kind              string            `json:"kind"`
	ID                string            `json:"id"`
	Number            string            `json:"number"`
	Date              string            `json:"date"`
	Posted            bool              `json:"posted"`
	Amount            Decimal           `json:"amount"`
	OrderID           string            `json:"order_id"`
	CustomerID        string            `json:"customer_id"`
	RegisterID        string            `json:"register_id"`
	ReceiptNumber     string            `json:"receipt_number"`
	PaymentForm       string            `json:"payment_form,omitempty"`
	OriginalReceiptID string            `json:"original_receipt_id,omitempty"`
	Lines             []ReceiptLine     `json:"lines,omitempty"`
	CashlessPayments  []CashlessPayment `json:"cashless_payments,omitempty"`
}

type ReceiptLine struct {
	LineNumber string  `json:"line_number"`
	ItemID     string  `json:"item_id"`
	Quantity   Decimal `json:"quantity"`
	Unit       string  `json:"unit"`
	Price      Decimal `json:"price"`
	Amount     Decimal `json:"amount"`
	Total      Decimal `json:"total"`
}

type CashlessPayment struct {
	LineNumber string  `json:"line_number"`
	Kind       string  `json:"kind"`
	CardType   string  `json:"card_type"`
	Amount     Decimal `json:"amount"`
	TerminalID string  `json:"terminal_id"`
	Cancelled  *bool   `json:"cancelled,omitempty"`
}

type ReceiptPage struct {
	Kind       string    `json:"kind"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Total      int       `json:"total"`
	Offset     int       `json:"offset"`
	NextOffset *int      `json:"next_offset,omitempty"`
	Items      []Receipt `json:"items"`
}

func receiptPlan(kind string) (config.DocumentResource, error) {
	plan, ok := config.CashReceipts[kind]
	if !ok {
		return config.DocumentResource{}, errors.New("kind must be sale or refund")
	}
	return plan, nil
}

func (s Service) ListReceipts(ctx context.Context, kind, from, to string, limit, offset int) (ReceiptPage, error) {
	plan, err := receiptPlan(kind)
	if err != nil {
		return ReceiptPage{}, err
	}
	if limit == 0 {
		limit = 20
	}
	rows, total, err := s.dateRangePage(ctx, plan, from, to, limit, offset)
	if err != nil {
		return ReceiptPage{}, err
	}
	page := ReceiptPage{Kind: kind, From: from, To: to, Total: total, Offset: offset, Items: make([]Receipt, 0, len(rows))}
	for _, row := range rows {
		receipt, err := bindFields[Receipt](row, plan.Fields)
		if err != nil {
			return ReceiptPage{}, errors.New("invalid OData receipt fields")
		}
		receipt.Kind = kind
		page.Items = append(page.Items, receipt)
	}
	if offset+len(page.Items) < total {
		next := offset + len(page.Items)
		page.NextOffset = &next
	}
	return page, nil
}

func (s Service) GetReceipt(ctx context.Context, kind, id string) (Receipt, error) {
	plan, err := receiptPlan(kind)
	if err != nil {
		return Receipt{}, err
	}
	if !guidPattern.MatchString(id) {
		return Receipt{}, errors.New("receipt ID must be a GUID")
	}
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField + "," + plan.PaymentsField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return Receipt{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return Receipt{}, errors.New("invalid OData receipt response")
	}
	receipt, err := bindFields[Receipt](row, plan.Fields)
	if err != nil {
		return Receipt{}, errors.New("invalid OData receipt fields")
	}
	receipt.Kind = kind
	var lines []map[string]json.RawMessage
	if raw, ok := row[plan.LinesField]; ok {
		if err := json.Unmarshal(raw, &lines); err != nil {
			return Receipt{}, errors.New("invalid OData receipt lines")
		}
	}
	receipt.Lines = make([]ReceiptLine, 0, len(lines))
	for _, row := range lines {
		line, err := bindFields[ReceiptLine](row, plan.LineFields)
		if err != nil {
			return Receipt{}, errors.New("invalid OData receipt line fields")
		}
		receipt.Lines = append(receipt.Lines, line)
	}
	var payments []map[string]json.RawMessage
	if raw, ok := row[plan.PaymentsField]; ok {
		if err := json.Unmarshal(raw, &payments); err != nil {
			return Receipt{}, errors.New("invalid OData receipt payments")
		}
	}
	receipt.CashlessPayments = make([]CashlessPayment, 0, len(payments))
	for _, row := range payments {
		payment, err := bindFields[CashlessPayment](row, plan.PaymentFields)
		if err != nil {
			return Receipt{}, errors.New("invalid OData receipt payment fields")
		}
		receipt.CashlessPayments = append(receipt.CashlessPayments, payment)
	}
	return receipt, nil
}
