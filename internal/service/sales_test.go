package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type salesReader struct {
	rows map[string][]map[string]any
}

func (r salesReader) Check(context.Context) (int, error) { return 1, nil }

func (r salesReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	name := strings.Split(resource, "(")[0]
	rows := r.rows[name]
	if strings.Contains(resource, "(guid'") {
		id := strings.TrimSuffix(strings.TrimPrefix(resource[len(name):], "(guid'"), "')")
		for _, row := range rows {
			if row["Ref_Key"] == id {
				return json.Marshal(row)
			}
		}
	}
	if params.Get("$inlinecount") != "" {
		if name == "Catalog_Контрагенты" {
			skip, _ := strconv.Atoi(params.Get("$skip"))
			return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(rows)), "value": rows[skip:min(skip+100, len(rows))]})
		}
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(rows)), "value": []any{}})
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"value": rows[skip:min(skip+top, len(rows))]})
}

func salesID(index int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012x", index) }

func TestCustomerPagingSearchAndGet(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Code": "B", "Description": "Beta", "IsFolder": false, "DeletionMark": false, "Покупатель": true, "Поставщик": false, "Недействителен": false},
		{"Ref_Key": salesID(2), "Code": "F", "Description": "Folder", "IsFolder": true, "DeletionMark": false},
		{"Ref_Key": salesID(3), "Code": "S", "Description": "Supplier", "IsFolder": false, "DeletionMark": false, "Покупатель": false, "Поставщик": true},
		{"Ref_Key": salesID(4), "Code": "A", "Description": "Alpha", "IsFolder": false, "DeletionMark": false, "Покупатель": true, "Поставщик": true, "Недействителен": false},
		{"Ref_Key": salesID(5), "Code": "D", "Description": "Deleted", "IsFolder": false, "DeletionMark": true, "Покупатель": true},
	}
	svc := Service{OData: salesReader{map[string][]map[string]any{"Catalog_Контрагенты": rows}}}
	first, err := svc.ListCustomers(context.Background(), "", 1, 0)
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].Name != "Alpha" || first.NextOffset == nil || *first.NextOffset != 1 {
		t.Fatalf("first customer page: %+v, %v", first, err)
	}
	second, err := svc.ListCustomers(context.Background(), "", 1, 1)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].Name != "Beta" || second.NextOffset != nil {
		t.Fatalf("second customer page: %+v, %v", second, err)
	}
	matched, err := svc.ListCustomers(context.Background(), "alpha", 20, 0)
	if err != nil || matched.Total != 1 || matched.Items[0].ID != salesID(4) {
		t.Fatalf("customer search: %+v, %v", matched, err)
	}
	customer, err := svc.GetCustomer(context.Background(), salesID(4))
	if err != nil || customer.ID != salesID(4) || customer.Supplier == nil || !*customer.Supplier {
		t.Fatalf("customer detail: %+v, %v", customer, err)
	}
	if _, err := svc.GetCustomer(context.Background(), salesID(3)); err == nil {
		t.Fatal("supplier-only record accepted as customer")
	}
	suppliers, err := svc.ListSuppliers(context.Background(), "", 20, 0)
	if err != nil || suppliers.Total != 2 || suppliers.Items[0].Name != "Alpha" || suppliers.Items[1].Name != "Supplier" {
		t.Fatalf("supplier list: %+v, %v", suppliers, err)
	}
	if _, err := svc.GetSupplier(context.Background(), salesID(3)); err != nil {
		t.Fatalf("supplier detail: %v", err)
	}
	if _, err := svc.GetSupplier(context.Background(), salesID(1)); err == nil {
		t.Fatal("buyer-only record accepted as supplier")
	}
}

func TestSalesDocumentOperationsAndLinks(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Date": "2026-09-03T12:00:00", "DeletionMark": false, "Posted": true, "Number": "S1", "СуммаДокумента": 99.50, "Контрагент_Key": salesID(9), "ВидОперации": "ПродажаПокупателю", "Заказ": salesID(7), "Заказ_Type": "StandardODATA.Document_ЗаказПокупателя", "ДокументОснование": "", "ДокументОснование_Type": "StandardODATA.Undefined", "Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": salesID(10), "Количество": 2, "Цена": 49.75, "Сумма": 99.50, "Всего": 99.50, "СуммаНДС": 10}}},
		{"Ref_Key": salesID(2), "Date": "2026-09-02T12:00:00", "DeletionMark": false, "Posted": false, "Number": "OTHER", "СуммаДокумента": 10, "Контрагент_Key": salesID(9), "ВидОперации": "ДругаяОперация"},
		{"Ref_Key": salesID(3), "Date": "2026-09-01T12:00:00", "DeletionMark": false, "Posted": false, "Number": "S2", "СуммаДокумента": 15, "Контрагент_Key": salesID(8), "ВидОперации": "ПродажаПокупателю", "Заказ": salesID(6), "Заказ_Type": "StandardODATA.Undefined"},
		{"Ref_Key": salesID(4), "Date": "2026-09-04T12:00:00", "DeletionMark": true, "Posted": false, "Number": "DELETED", "СуммаДокумента": 20, "ВидОперации": "ПродажаПокупателю"},
	}
	svc := Service{OData: salesReader{map[string][]map[string]any{"Document_РасходнаяНакладная": rows}}}
	page, err := svc.ListSalesDocuments(context.Background(), "shipment", "", "", "", 1, 0)
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].Number != "S1" || page.Items[0].Amount.String() != "99.5" || page.Items[0].OrderID != salesID(7) || page.NextOffset == nil {
		t.Fatalf("shipment page: %+v, %v", page, err)
	}
	next, err := svc.ListSalesDocuments(context.Background(), "shipment", "", "", "", 1, 1)
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].Number != "S2" || next.Items[0].OrderID != "" {
		t.Fatalf("shipment next page: %+v, %v", next, err)
	}
	filtered, err := svc.ListSalesDocuments(context.Background(), "shipment", salesID(9), "2026-09-03", "2026-09-03", 20, 0)
	if err != nil || filtered.Total != 1 {
		t.Fatalf("shipment filters: %+v, %v", filtered, err)
	}
	if _, err := svc.GetSalesDocument(context.Background(), "shipment", salesID(2)); err == nil {
		t.Fatal("wrong sales operation accepted")
	}
	detail, err := svc.GetSalesDocument(context.Background(), "shipment", salesID(1))
	if err != nil || len(detail.Lines) != 1 || detail.Lines[0].ProductID != salesID(10) || detail.Lines[0].Quantity.String() != "2" {
		t.Fatalf("shipment detail: %+v, %v", detail, err)
	}
	if _, err := svc.ListSalesDocuments(context.Background(), "shipment", "", "2026-09-03", "", 20, 0); err == nil {
		t.Fatal("incomplete date range accepted")
	}
}

func TestSalesDocumentPreservesDecimalLexeme(t *testing.T) {
	var row map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Date":"2026-09-01T10:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":99.50}`), &row); err != nil {
		t.Fatal(err)
	}
	doc, err := salesDocument(row, config.SalesDocuments["invoice"], "invoice")
	if err != nil || doc.Amount.String() != "99.50" {
		t.Fatalf("amount changed: %q, %v", doc.Amount, err)
	}
}
