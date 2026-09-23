package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type Customer struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	IsFolder bool   `json:"is_folder"`
	Deleted  bool   `json:"deleted"`
	Inactive *bool  `json:"inactive"`
	Buyer    *bool  `json:"buyer"`
	Supplier *bool  `json:"supplier"`
}

type CustomerPage struct {
	Query      string     `json:"query,omitempty"`
	Total      int        `json:"total"`
	Offset     int        `json:"offset"`
	NextOffset *int       `json:"next_offset,omitempty"`
	Items      []Customer `json:"items"`
}

type Supplier = Customer
type SupplierPage = CustomerPage

type SalesDocument struct {
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	Number     string         `json:"number"`
	Date       string         `json:"date"`
	Posted     bool           `json:"posted"`
	Deleted    bool           `json:"deleted"`
	Amount     Decimal        `json:"amount"`
	CustomerID string         `json:"customer_id"`
	Operation  string         `json:"operation,omitempty"`
	OrderID    string         `json:"order_id,omitempty"`
	OrderType  string         `json:"order_type,omitempty"`
	BasisID    string         `json:"basis_id,omitempty"`
	BasisType  string         `json:"basis_type,omitempty"`
	Lines      []DocumentLine `json:"lines,omitempty"`
}

type SalesDocumentPage struct {
	Kind       string          `json:"kind"`
	CustomerID string          `json:"customer_id,omitempty"`
	From       string          `json:"from,omitempty"`
	To         string          `json:"to,omitempty"`
	Total      int             `json:"total"`
	Offset     int             `json:"offset"`
	NextOffset *int            `json:"next_offset,omitempty"`
	Items      []SalesDocument `json:"items"`
}

func (s Service) ListCustomers(ctx context.Context, query string, limit, offset int) (CustomerPage, error) {
	return s.listCounterparties(ctx, query, limit, offset, true)
}

func (s Service) ListSuppliers(ctx context.Context, query string, limit, offset int) (SupplierPage, error) {
	return s.listCounterparties(ctx, query, limit, offset, false)
}

func (s Service) listCounterparties(ctx context.Context, query string, limit, offset int, buyer bool) (CustomerPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return CustomerPage{}, err
	}
	rows, err := s.catalogRows(ctx, config.Customers)
	if err != nil {
		return CustomerPage{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	customers := make([]Customer, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
			return CustomerPage{}, errors.New("invalid OData customer flags")
		}
		if string(row["IsFolder"]) == "true" || string(row["DeletionMark"]) == "true" {
			continue
		}
		customer, err := bindFields[Customer](row, config.Customers.Fields)
		if err != nil || !guidPattern.MatchString(customer.ID) {
			return CustomerPage{}, errors.New("invalid OData customer")
		}
		if buyer && (customer.Buyer == nil || !*customer.Buyer) || !buyer && (customer.Supplier == nil || !*customer.Supplier) {
			continue
		}
		if needle == "" || strings.Contains(strings.ToLower(customer.Name), needle) || strings.Contains(strings.ToLower(customer.FullName), needle) || strings.Contains(strings.ToLower(customer.Code), needle) {
			customers = append(customers, customer)
		}
	}
	sort.Slice(customers, func(i, j int) bool {
		first, second := strings.ToLower(customers[i].Name), strings.ToLower(customers[j].Name)
		if first == second {
			return customers[i].ID < customers[j].ID
		}
		return first < second
	})
	page := CustomerPage{Query: query, Total: len(customers), Offset: offset, Items: make([]Customer, 0)}
	start := min(offset, len(customers))
	end := start + min(limit, len(customers)-start)
	page.Items = append(page.Items, customers[start:end]...)
	if end < len(customers) {
		page.NextOffset = &end
	}
	return page, nil
}

func (s Service) GetCustomer(ctx context.Context, id string) (Customer, error) {
	return s.getCounterparty(ctx, id, true)
}

func (s Service) GetSupplier(ctx context.Context, id string) (Supplier, error) {
	return s.getCounterparty(ctx, id, false)
}

func (s Service) getCounterparty(ctx context.Context, id string, buyer bool) (Customer, error) {
	if !guidPattern.MatchString(id) {
		return Customer{}, errors.New("counterparty ID must be a GUID")
	}
	resource := config.Customers.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(config.Customers.Fields)}}
	data, err := s.OData.Get(ctx, resource, params, 1<<20)
	if err != nil {
		return Customer{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil || !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
		return Customer{}, errors.New("invalid OData customer response")
	}
	if string(row["IsFolder"]) == "true" {
		return Customer{}, errors.New("customer ID refers to a folder")
	}
	customer, err := bindFields[Customer](row, config.Customers.Fields)
	if err != nil || !strings.EqualFold(customer.ID, id) {
		return Customer{}, errors.New("invalid OData customer")
	}
	if buyer && (customer.Buyer == nil || !*customer.Buyer) {
		return Customer{}, errors.New("record is not marked as a customer")
	}
	if !buyer && (customer.Supplier == nil || !*customer.Supplier) {
		return Customer{}, errors.New("record is not marked as a supplier")
	}
	return customer, nil
}

func (s Service) ListSalesDocuments(ctx context.Context, kind, customerID, from, to string, limit, offset int) (SalesDocumentPage, error) {
	plan, err := salesPlan(kind)
	if err != nil {
		return SalesDocumentPage{}, err
	}
	if err := validateListPage(limit, offset); err != nil {
		return SalesDocumentPage{}, err
	}
	if customerID != "" && !guidPattern.MatchString(customerID) {
		return SalesDocumentPage{}, errors.New("customer ID must be a GUID")
	}
	startDate, endDate, err := optionalDocumentDateRange(from, to)
	if err != nil {
		return SalesDocumentPage{}, err
	}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return SalesDocumentPage{}, err
	}
	matched := make([]SalesDocument, 0, len(rows))
	for _, row := range rows {
		doc, err := salesDocument(row, plan, kind)
		if err != nil {
			return SalesDocumentPage{}, err
		}
		if !salesOperationMatches(doc) || customerID != "" && !strings.EqualFold(doc.CustomerID, customerID) || startDate != "" && doc.Date < startDate || endDate != "" && doc.Date >= endDate {
			continue
		}
		matched = append(matched, doc)
	}
	page := SalesDocumentPage{Kind: kind, CustomerID: customerID, From: from, To: to, Total: len(matched), Offset: offset, Items: make([]SalesDocument, 0)}
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

func (s Service) GetSalesDocument(ctx context.Context, kind, id string) (SalesDocument, error) {
	plan, err := salesPlan(kind)
	if err != nil {
		return SalesDocument{}, err
	}
	if !guidPattern.MatchString(id) {
		return SalesDocument{}, errors.New("sales document ID must be a GUID")
	}
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return SalesDocument{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return SalesDocument{}, errors.New("invalid OData sales document response")
	}
	doc, err := salesDocument(row, plan, kind)
	if err != nil || !strings.EqualFold(doc.ID, id) {
		return SalesDocument{}, errors.New("invalid OData sales document")
	}
	if !salesOperationMatches(doc) {
		return SalesDocument{}, errors.New("sales document has a different operation")
	}
	if raw, ok := row[plan.LinesField]; ok {
		var lines []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &lines); err != nil {
			return SalesDocument{}, errors.New("invalid OData sales document lines")
		}
		doc.Lines = make([]DocumentLine, 0, len(lines))
		for _, row := range lines {
			line, err := bindFields[DocumentLine](row, plan.LineFields)
			if err != nil {
				return SalesDocument{}, errors.New("invalid OData sales document line")
			}
			doc.Lines = append(doc.Lines, line)
		}
	}
	return doc, nil
}

func salesPlan(kind string) (config.DocumentResource, error) {
	plan, ok := config.SalesDocuments[kind]
	if !ok {
		return config.DocumentResource{}, errors.New("kind must be invoice, shipment, or return")
	}
	return plan, nil
}

func salesDocument(row map[string]json.RawMessage, plan config.DocumentResource, kind string) (SalesDocument, error) {
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return SalesDocument{}, errors.New("invalid OData sales document flags")
	}
	doc, err := bindFields[SalesDocument](row, plan.Fields)
	if err != nil || !guidPattern.MatchString(doc.ID) || len(doc.Date) < 19 {
		return SalesDocument{}, errors.New("invalid OData sales document")
	}
	doc.Kind = kind
	if doc.OrderType != "StandardODATA.Document_ЗаказПокупателя" || !guidPattern.MatchString(doc.OrderID) || strings.EqualFold(doc.OrderID, emptyGUID) {
		doc.OrderID = ""
		doc.OrderType = ""
	}
	if doc.BasisType == "StandardODATA.Undefined" || !guidPattern.MatchString(doc.BasisID) || strings.EqualFold(doc.BasisID, emptyGUID) {
		doc.BasisID = ""
		doc.BasisType = ""
	}
	return doc, nil
}

func salesOperationMatches(doc SalesDocument) bool {
	switch doc.Kind {
	case "shipment":
		return doc.Operation == "ПродажаПокупателю"
	case "return":
		return doc.Operation == "ВозвратОтПокупателя"
	default:
		return true
	}
}

func optionalDocumentDateRange(from, to string) (string, string, error) {
	if from == "" && to == "" {
		return "", "", nil
	}
	if from == "" || to == "" {
		return "", "", errors.New("from and to must be provided together")
	}
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return "", "", errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Format("2006-01-02") != to || end.Before(start) {
		return "", "", errors.New("to must be YYYY-MM-DD and no earlier than from")
	}
	return from + "T00:00:00", end.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00", nil
}

func validateListPage(limit, offset int) error {
	if limit < 1 || limit > 100 || offset < 0 {
		return errors.New("limit must be 1–100 and offset must be nonnegative")
	}
	return nil
}
