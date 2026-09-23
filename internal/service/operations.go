package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type OperationalDocument struct {
	Domain                 string         `json:"domain"`
	Kind                   string         `json:"kind"`
	ID                     string         `json:"id"`
	Number                 string         `json:"number"`
	Date                   string         `json:"date"`
	Posted                 bool           `json:"posted"`
	Deleted                bool           `json:"deleted"`
	Amount                 Decimal        `json:"amount,omitempty"`
	SupplierID             string         `json:"supplier_id,omitempty"`
	WarehouseID            string         `json:"warehouse_id,omitempty"`
	ReserveWarehouseID     string         `json:"reserve_warehouse_id,omitempty"`
	DestinationWarehouseID string         `json:"destination_warehouse_id,omitempty"`
	StateID                string         `json:"state_id,omitempty"`
	Operation              string         `json:"operation,omitempty"`
	SupplierOrderID        string         `json:"supplier_order_id,omitempty"`
	OrderType              string         `json:"order_type,omitempty"`
	TransferOrderID        string         `json:"transfer_order_id,omitempty"`
	BasisID                string         `json:"basis_id,omitempty"`
	BasisType              string         `json:"basis_type,omitempty"`
	Lines                  []DocumentLine `json:"lines,omitempty"`
}

type OperationalPage struct {
	Domain      string                `json:"domain"`
	Kind        string                `json:"kind"`
	SupplierID  string                `json:"supplier_id,omitempty"`
	WarehouseID string                `json:"warehouse_id,omitempty"`
	From        string                `json:"from,omitempty"`
	To          string                `json:"to,omitempty"`
	Total       int                   `json:"total"`
	Offset      int                   `json:"offset"`
	NextOffset  *int                  `json:"next_offset,omitempty"`
	Items       []OperationalDocument `json:"items"`
}

func (s Service) ListOperationalDocuments(ctx context.Context, domain, kind, supplierID, warehouseID, from, to string, limit, offset int) (OperationalPage, error) {
	plan, key, err := operationalPlan(domain, kind)
	if err != nil {
		return OperationalPage{}, err
	}
	if err := validateListPage(limit, offset); err != nil {
		return OperationalPage{}, err
	}
	if supplierID != "" && (!guidPattern.MatchString(supplierID) || domain != "purchase") {
		return OperationalPage{}, errors.New("supplier filter requires a purchase kind and supplier GUID")
	}
	if warehouseID != "" && !guidPattern.MatchString(warehouseID) {
		return OperationalPage{}, errors.New("warehouse ID must be a GUID")
	}
	startDate, endDate, err := optionalDocumentDateRange(from, to)
	if err != nil {
		return OperationalPage{}, err
	}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return OperationalPage{}, err
	}
	matched := make([]OperationalDocument, 0, len(rows))
	for _, row := range rows {
		doc, err := operationalDocument(row, plan, domain, kind)
		if err != nil {
			return OperationalPage{}, err
		}
		if !operationalMatches(doc, key) || supplierID != "" && !strings.EqualFold(doc.SupplierID, supplierID) || warehouseID != "" && !documentUsesWarehouse(doc, warehouseID) || startDate != "" && doc.Date < startDate || endDate != "" && doc.Date >= endDate {
			continue
		}
		matched = append(matched, doc)
	}
	page := OperationalPage{Domain: domain, Kind: kind, SupplierID: supplierID, WarehouseID: warehouseID, From: from, To: to, Total: len(matched), Offset: offset, Items: make([]OperationalDocument, 0)}
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

func (s Service) GetOperationalDocument(ctx context.Context, domain, kind, id string) (OperationalDocument, error) {
	plan, key, err := operationalPlan(domain, kind)
	if err != nil {
		return OperationalDocument{}, err
	}
	if !guidPattern.MatchString(id) {
		return OperationalDocument{}, errors.New("document ID must be a GUID")
	}
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return OperationalDocument{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return OperationalDocument{}, errors.New("invalid OData document response")
	}
	doc, err := operationalDocument(row, plan, domain, kind)
	if err != nil || !strings.EqualFold(doc.ID, id) {
		return OperationalDocument{}, errors.New("invalid OData document")
	}
	if !operationalMatches(doc, key) {
		return OperationalDocument{}, errors.New("document has a different operation")
	}
	if raw, ok := row[plan.LinesField]; ok {
		var lines []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &lines); err != nil {
			return OperationalDocument{}, errors.New("invalid OData document lines")
		}
		doc.Lines = make([]DocumentLine, 0, len(lines))
		for _, lineRow := range lines {
			line, err := bindFields[DocumentLine](lineRow, plan.LineFields)
			if err != nil {
				return OperationalDocument{}, errors.New("invalid OData document line")
			}
			doc.Lines = append(doc.Lines, line)
		}
	}
	return doc, nil
}

func operationalPlan(domain, kind string) (config.DocumentResource, string, error) {
	key := ""
	switch domain {
	case "purchase":
		switch kind {
		case "order":
			key = "supplier-order"
		case "receipt":
			key = "goods-receipt"
		}
	case "warehouse":
		if kind == "transfer-order" || kind == "transfer" || kind == "stock-receipt" || kind == "stock-writeoff" {
			key = kind
		}
	}
	if key == "" {
		return config.DocumentResource{}, "", errors.New("kind must be order or receipt for purchases, or transfer-order, transfer, stock-receipt, or stock-writeoff for warehouse documents")
	}
	return config.OperationalDocuments[key], key, nil
}

func operationalDocument(row map[string]json.RawMessage, plan config.DocumentResource, domain, kind string) (OperationalDocument, error) {
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return OperationalDocument{}, errors.New("invalid OData document flags")
	}
	doc, err := bindFields[OperationalDocument](row, plan.Fields)
	if err != nil || !guidPattern.MatchString(doc.ID) || len(doc.Date) < 19 {
		return OperationalDocument{}, errors.New("invalid OData document fields")
	}
	doc.Domain, doc.Kind = domain, kind
	if doc.OrderType != "StandardODATA.Document_ЗаказПоставщику" || !linkedGUID(doc.SupplierOrderID) {
		doc.SupplierOrderID = ""
		doc.OrderType = ""
	}
	if !linkedGUID(doc.TransferOrderID) {
		doc.TransferOrderID = ""
	}
	if !linkedGUID(doc.StateID) {
		doc.StateID = ""
	}
	if doc.BasisType == "StandardODATA.Undefined" || !linkedGUID(doc.BasisID) {
		doc.BasisID = ""
		doc.BasisType = ""
	}
	return doc, nil
}

func linkedGUID(id string) bool {
	return guidPattern.MatchString(id) && !strings.EqualFold(id, emptyGUID)
}

func operationalMatches(doc OperationalDocument, key string) bool {
	switch key {
	case "goods-receipt":
		return doc.Operation == "ПоступлениеОтПоставщика"
	case "transfer":
		return doc.Operation == "Перемещение"
	default:
		return true
	}
}

func documentUsesWarehouse(doc OperationalDocument, id string) bool {
	return strings.EqualFold(doc.WarehouseID, id) || strings.EqualFold(doc.ReserveWarehouseID, id) || strings.EqualFold(doc.DestinationWarehouseID, id)
}
