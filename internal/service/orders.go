package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Order struct {
	ID         string      `json:"id"`
	Number     string      `json:"number"`
	Date       string      `json:"date"`
	Posted     bool        `json:"posted"`
	State      string      `json:"state"`
	Amount     Decimal     `json:"amount"`
	CustomerID string      `json:"customer_id"`
	Lines      []OrderLine `json:"lines,omitempty"`
}

type OrderPage struct {
	CustomerID string  `json:"customer_id,omitempty"`
	From       string  `json:"from,omitempty"`
	To         string  `json:"to,omitempty"`
	Total      int     `json:"total"`
	Offset     int     `json:"offset"`
	NextOffset *int    `json:"next_offset,omitempty"`
	Items      []Order `json:"items"`
}

type OrderLine struct {
	LineNumber string  `json:"line_number"`
	Item       string  `json:"item"`
	Quantity   Decimal `json:"quantity"`
	Unit       string  `json:"unit"`
	Price      Decimal `json:"price"`
	Amount     Decimal `json:"amount"`
	Total      Decimal `json:"total"`
}

type Decimal string

func (d *Decimal) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*d = ""
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return err
	}
	*d = Decimal(number)
	return nil
}

func (d Decimal) String() string { return string(d) }

func (s Service) ListOrders(ctx context.Context, limit int) ([]Order, error) {
	if limit == 0 {
		limit = 20
	}
	page, err := s.ListOrdersPage(ctx, "", "", "", limit, 0)
	return page.Items, err
}

func (s Service) ListOrdersPage(ctx context.Context, customerID, from, to string, limit, offset int) (OrderPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return OrderPage{}, err
	}
	if customerID != "" && !guidPattern.MatchString(customerID) {
		return OrderPage{}, errors.New("customer ID must be a GUID")
	}
	startDate, endDate, err := optionalDocumentDateRange(from, to)
	if err != nil {
		return OrderPage{}, err
	}
	plan := config.CustomerOrders
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return OrderPage{}, err
	}
	matched := make([]Order, 0, len(rows))
	for _, row := range rows {
		order, err := bindFields[Order](row, plan.Fields)
		if err != nil {
			return OrderPage{}, errors.New("invalid OData order fields")
		}
		if customerID != "" && !strings.EqualFold(order.CustomerID, customerID) || startDate != "" && order.Date < startDate || endDate != "" && order.Date >= endDate {
			continue
		}
		matched = append(matched, order)
	}
	page := OrderPage{CustomerID: customerID, From: from, To: to, Total: len(matched), Offset: offset, Items: make([]Order, 0)}
	start := min(offset, len(matched))
	end := start + min(limit, len(matched)-start)
	for index := len(matched) - start - 1; index >= len(matched)-end; index-- {
		page.Items = append(page.Items, matched[index])
	}
	if end < len(matched) {
		page.NextOffset = &end
	}
	return page, nil
}

func (s Service) GetOrder(ctx context.Context, id string) (Order, error) {
	if !guidPattern.MatchString(id) {
		return Order{}, errors.New("order ID must be a GUID")
	}
	plan := config.CustomerOrders
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return Order{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return Order{}, errors.New("invalid OData order response")
	}
	order, err := bindFields[Order](row, plan.Fields)
	if err != nil {
		return Order{}, errors.New("invalid OData order fields")
	}
	var lines []map[string]json.RawMessage
	if raw, ok := row[plan.LinesField]; ok {
		if err := json.Unmarshal(raw, &lines); err != nil {
			return Order{}, errors.New("invalid OData order lines")
		}
	}
	order.Lines = make([]OrderLine, 0, len(lines))
	for _, row := range lines {
		line, err := bindFields[OrderLine](row, plan.LineFields)
		if err != nil {
			return Order{}, errors.New("invalid OData order line fields")
		}
		order.Lines = append(order.Lines, line)
	}
	return order, nil
}
