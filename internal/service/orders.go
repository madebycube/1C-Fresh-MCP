package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const MaxOrders = 100

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
	if limit < 1 || limit > MaxOrders {
		return nil, fmt.Errorf("limit must be between 1 and %d", MaxOrders)
	}
	plan := config.CustomerOrders
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return nil, err
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	orders := make([]Order, 0, limit)
	for index := len(rows) - 1; index >= len(rows)-limit; index-- {
		row := rows[index]
		order, err := bindFields[Order](row, plan.Fields)
		if err != nil {
			return nil, errors.New("invalid OData order fields")
		}
		orders = append(orders, order)
	}
	return orders, nil
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
