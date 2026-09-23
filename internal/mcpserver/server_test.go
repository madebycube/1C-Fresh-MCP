package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubReader struct{}

func (stubReader) Check(context.Context) (int, error) { return 1221, nil }
func (stubReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	if resource == "$metadata" {
		return []byte(`<Schema Namespace="StandardODATA"><EntityType Name="Catalog_ВидыЦен"><Key><PropertyRef Name="Ref_Key"/></Key><Property Name="Ref_Key" Type="Edm.Guid" Nullable="false"/><Property Name="Description" Type="Edm.String"/></EntityType><EntityContainer><EntitySet Name="Catalog_ВидыЦен" EntityType="StandardODATA.Catalog_ВидыЦен"/></EntityContainer></Schema>`), nil
	}
	if resource == "Catalog_Номенклатура" && params.Get("$orderby") == "Ref_Key asc" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000004","Code":"P1","Description":"Chair","IsFolder":false,"DeletionMark":false,"Parent_Key":"00000000-0000-0000-0000-000000000001"}]}`), nil
	}
	if resource == "Catalog_Номенклатура" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Description":"Furniture","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false}]}`), nil
	}
	if resource == "Catalog_КатегорииНоменклатуры" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000014","Description":"Example category","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":false,"DeletionMark":false,"ТипНоменклатурыПоУмолчанию":"Запас","ЕдиницаИзмерения_Key":"00000000-0000-0000-0000-000000000013"}]}`), nil
	}
	if strings.HasPrefix(resource, "Catalog_Номенклатура(") {
		if strings.Contains(resource, "00000000-0000-0000-0000-000000000004") {
			return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000004","Description":"Chair","НаименованиеПолное":"Chair full","Артикул":"A","ТипНоменклатуры":"Товар","ЕдиницаИзмерения_Key":"00000000-0000-0000-0000-000000000002","IsFolder":false,"DeletionMark":false,"DataVersion":"version-1"}`), nil
		}
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Description":"Furniture","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false,"DataVersion":"version-1"}`), nil
	}
	if resource == "Catalog_ВидыЦен" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000002","Description":"Retail","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if resource == "Catalog_КлассификаторЕдиницИзмерения" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000013","Code":"796","Description":"pc","DeletionMark":false}]}`), nil
	}
	if resource == "Catalog_СтруктурныеЕдиницы" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000006","Description":"Example warehouse","ТипСтруктурнойЕдиницы":"Склад","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if resource == "Catalog_Контрагенты" {
		return []byte(`{"odata.count":"2","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Code":"G1","Description":"Example folder","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false},{"Ref_Key":"00000000-0000-0000-0000-000000000007","Code":"C1","Description":"Example customer","IsFolder":false,"DeletionMark":false,"Недействителен":false,"Покупатель":true,"Поставщик":true}]}`), nil
	}
	if resource == "Catalog_Кассы" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000011","Code":"C1","Description":"Cash desk","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if strings.HasPrefix(resource, "Catalog_Контрагенты(") {
		if strings.Contains(resource, "00000000-0000-0000-0000-000000000001") {
			return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Description":"Example folder","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false,"DataVersion":"version-1"}`), nil
		}
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000007","Code":"C1","Description":"Example customer","НаименованиеПолное":"Example customer","IsFolder":false,"DeletionMark":false,"Недействителен":false,"Покупатель":true,"Поставщик":true,"DataVersion":"version-1"}`), nil
	}
	if params.Get("$inlinecount") != "" {
		return []byte(`{"odata.count":"1","value":[]}`), nil
	}
	if strings.HasPrefix(resource, "Document_РасходнаяНакладная(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000008","Number":"S1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":120.50,"Контрагент_Key":"00000000-0000-0000-0000-000000000007","ВидОперации":"ПродажаПокупателю","Заказ":"00000000-0000-0000-0000-000000000001","Заказ_Type":"StandardODATA.Document_ЗаказПокупателя"}`), nil
	}
	if resource == "Document_РасходнаяНакладная" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000008","Number":"S1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":120.50,"Контрагент_Key":"00000000-0000-0000-0000-000000000007","ВидОперации":"ПродажаПокупателю","Заказ":"00000000-0000-0000-0000-000000000001","Заказ_Type":"StandardODATA.Document_ЗаказПокупателя"}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЗаказПоставщику(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000009","Number":"P1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":25.50,"Контрагент_Key":"00000000-0000-0000-0000-000000000007","Запасы":[]}`), nil
	}
	if resource == "Document_ЗаказПоставщику" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000009","Number":"P1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":25.50,"Контрагент_Key":"00000000-0000-0000-0000-000000000007"}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ПеремещениеЗапасов(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000010","Number":"T1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"ВидОперации":"Перемещение","СтруктурнаяЕдиница_Key":"00000000-0000-0000-0000-000000000006","Запасы":[]}`), nil
	}
	if resource == "Document_ПеремещениеЗапасов" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000010","Number":"T1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"ВидОперации":"Перемещение","СтруктурнаяЕдиница_Key":"00000000-0000-0000-0000-000000000006"}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ПоступлениеНаСчет(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000012","Number":"B1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":100,"ВидОперации":"ПоступлениеОплатыПоКартам","БанковскийСчет_Key":"00000000-0000-0000-0000-000000000011"}`), nil
	}
	if resource == "Document_ПоступлениеНаСчет" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000012","Number":"B1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":100,"ВидОперации":"ПоступлениеОплатыПоКартам","БанковскийСчет_Key":"00000000-0000-0000-0000-000000000011"}]}`), nil
	}
	if resource == "Document_УстановкаЦенНоменклатуры" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000005","Date":"2026-09-23T12:00:00","Posted":true,"DeletionMark":false,"Запасы":[{"LineNumber":"1","Номенклатура_Key":"00000000-0000-0000-0000-000000000004","ВидЦены_Key":"00000000-0000-0000-0000-000000000002","Характеристика_Key":"00000000-0000-0000-0000-000000000000","Цена":120.50,"Валюта_Key":"00000000-0000-0000-0000-000000000006"}]}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЧекККМ") && strings.Contains(resource, "(guid'") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":true,"СуммаДокумента":10,"НомерЧекаККМ":"1","Запасы":[],"БезналичнаяОплата":[]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЧекККМ") {
		if params.Get("$select") == "Date" {
			return []byte(`{"value":[{"Date":"2026-09-23T00:00:00"}]}`), nil
		}
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"СуммаДокумента":10,"НомерЧекаККМ":"1"}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЗаказПокупателя(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":false,"СуммаДокумента":10,"Запасы":[]}`), nil
	}
	if resource == "Document_ЗаказПокупателя" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":false,"DeletionMark":false,"СуммаДокумента":10}]}`), nil
	}
	return []byte(`{"value":[{"Ref_Key":"item-1","Description":"Chair"}]}`), nil
}

func (stubReader) Write(_ context.Context, method, _ string, _ []byte, _ string) ([]byte, error) {
	if method == http.MethodPost {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000003"}`), nil
	}
	return nil, nil
}

func (stubReader) GetStockBalance(_ context.Context, productID string, _ url.Values, _ int64) ([]byte, error) {
	return []byte(`{"odata.count":"1","value":[{"Номенклатура_Key":"` + productID + `","СтруктурнаяЕдиница_Key":"00000000-0000-0000-0000-000000000006","Характеристика_Key":"00000000-0000-0000-0000-000000000000","Организация_Key":"00000000-0000-0000-0000-000000000000","Партия_Key":"00000000-0000-0000-0000-000000000000","Ячейка_Key":"00000000-0000-0000-0000-000000000000","КоличествоBalance":3.5}]}`), nil
}

func TestToolsHaveWriteAnnotationsAndAreCallable(t *testing.T) {
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(service.Service{OData: stubReader{}}).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 44 {
		t.Fatalf("got %d tools; want 44", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil {
			t.Fatalf("tool %s has no annotations", tool.Name)
		}
		write := tool.Name == operations.CreateGroup.Tool || tool.Name == operations.UpdateGroup.Tool || tool.Name == operations.CreateCounterpartyGroup.Tool || tool.Name == operations.UpdateCounterpartyGroup.Tool || tool.Name == operations.UpdateProduct.Tool || tool.Name == operations.CreateCustomer.Tool || tool.Name == operations.UpdateCustomer.Tool || tool.Name == operations.CreateSupplier.Tool || tool.Name == operations.UpdateSupplier.Tool
		if tool.Annotations.ReadOnlyHint == write {
			t.Fatalf("tool %s has incorrect read-only annotation", tool.Name)
		}
		if (tool.Name == operations.CreateGroup.Tool || tool.Name == operations.CreateCounterpartyGroup.Tool || tool.Name == operations.CreateCustomer.Tool || tool.Name == operations.CreateSupplier.Tool) && (tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint) {
			t.Fatalf("%s must be marked additive", tool.Name)
		}
		if (tool.Name == operations.UpdateGroup.Tool || tool.Name == operations.UpdateCounterpartyGroup.Tool || tool.Name == operations.UpdateCustomer.Tool || tool.Name == operations.UpdateSupplier.Tool) && !tool.Annotations.IdempotentHint {
			t.Fatalf("%s must be marked idempotent", tool.Name)
		}
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{operations.Check.Tool, map[string]any{}},
		{operations.ListGroups.Tool, map[string]any{}},
		{operations.ListProductCategories.Tool, map[string]any{}},
		{operations.CreateGroup.Tool, map[string]any{"name": "New group"}},
		{operations.UpdateGroup.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000001", "name": "Renamed", "parent_id": "root"}},
		{operations.ListCounterpartyGroups.Tool, map[string]any{}},
		{operations.CreateCounterpartyGroup.Tool, map[string]any{"name": "New folder"}},
		{operations.UpdateCounterpartyGroup.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000001", "name": "Renamed", "parent_id": "root"}},
		{operations.ListPriceTypes.Tool, map[string]any{}},
		{operations.ListUnitTypes.Tool, map[string]any{}},
		{operations.GetPrice.Tool, map[string]any{"product_id": "00000000-0000-0000-0000-000000000004", "price_type": "Retail", "as_of": "2026-09-23"}},
		{operations.ListPrices.Tool, map[string]any{"price_type": "Retail", "group_id": "00000000-0000-0000-0000-000000000001", "as_of": "2026-09-23", "limit": 1}},
		{operations.ListWarehouses.Tool, map[string]any{}},
		{operations.GetStock.Tool, map[string]any{"product_id": "00000000-0000-0000-0000-000000000004"}},
		{operations.SearchProducts.Tool, map[string]any{"query": "Chair", "limit": 2}},
		{operations.ListProducts.Tool, map[string]any{"limit": 2, "group_id": "00000000-0000-0000-0000-000000000001"}},
		{operations.GetProduct.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000004"}},
		{operations.UpdateProduct.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000004", "article": "B", "group_id": "00000000-0000-0000-0000-000000000001"}},
		{operations.ListOrders.Tool, map[string]any{"limit": 2, "offset": 0, "customer_id": "00000000-0000-0000-0000-000000000002", "from": "2026-09-23", "to": "2026-09-23"}},
		{operations.GetOrder.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000001"}},
		{operations.ListCustomers.Tool, map[string]any{"limit": 2, "group_id": "root"}},
		{operations.SearchCustomers.Tool, map[string]any{"query": "Example", "limit": 2, "group_id": "root"}},
		{operations.GetCustomer.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000007"}},
		{operations.CreateCustomer.Tool, map[string]any{"name": "New customer"}},
		{operations.UpdateCustomer.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000007", "name": "Renamed customer"}},
		{operations.ListSuppliers.Tool, map[string]any{"limit": 2, "group_id": "root"}},
		{operations.SearchSuppliers.Tool, map[string]any{"query": "Example", "limit": 2, "group_id": "root"}},
		{operations.GetSupplier.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000007"}},
		{operations.CreateSupplier.Tool, map[string]any{"name": "New supplier"}},
		{operations.UpdateSupplier.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000007", "name": "Renamed supplier"}},
		{operations.ListSales.Tool, map[string]any{"kind": "shipment", "limit": 2}},
		{operations.GetSale.Tool, map[string]any{"kind": "shipment", "id": "00000000-0000-0000-0000-000000000008"}},
		{operations.ListPurchases.Tool, map[string]any{"kind": "order", "limit": 2}},
		{operations.GetPurchase.Tool, map[string]any{"kind": "order", "id": "00000000-0000-0000-0000-000000000009"}},
		{operations.ListWarehouseDocuments.Tool, map[string]any{"kind": "transfer", "limit": 2}},
		{operations.GetWarehouseDocument.Tool, map[string]any{"kind": "transfer", "id": "00000000-0000-0000-0000-000000000010"}},
		{operations.ListMoneyAccounts.Tool, map[string]any{"kind": "cash"}},
		{operations.ListMoney.Tool, map[string]any{"kind": "bank-in", "from": "2026-09-23", "to": "2026-09-23", "account_id": "00000000-0000-0000-0000-000000000011", "limit": 2}},
		{operations.GetMoney.Tool, map[string]any{"kind": "bank-in", "id": "00000000-0000-0000-0000-000000000012"}},
		{operations.ListReceipts.Tool, map[string]any{"kind": "sale", "from": "2026-09-23", "to": "2026-09-23", "limit": 2}},
		{operations.GetReceipt.Tool, map[string]any{"kind": "sale", "id": "00000000-0000-0000-0000-000000000001"}},
		{operations.AuditUnpostedReceipts.Tool, map[string]any{"from": "2026-09-23", "before": "2026-09-24"}},
		{operations.SearchResources.Tool, map[string]any{"query": "Цен"}},
		{operations.DescribeResource.Tool, map[string]any{"name": "Catalog_ВидыЦен"}},
	} {
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError || result.StructuredContent == nil {
			t.Fatalf("tool %s failed: %v, %+v", call.name, err, result)
		}
		if call.name == operations.GetPrice.Tool {
			data, err := json.Marshal(result.StructuredContent)
			if err != nil || !strings.Contains(string(data), `"found":true`) || !strings.Contains(string(data), `"price":"120.50"`) {
				t.Fatalf("price tool returned %s: %v", data, err)
			}
		}
		if call.name == operations.ListPrices.Tool {
			data, err := json.Marshal(result.StructuredContent)
			if err != nil || !strings.Contains(string(data), `"price":"120.50"`) || !strings.Contains(string(data), `"items":[`) {
				t.Fatalf("price list tool returned %s: %v", data, err)
			}
		}
		if call.name == operations.ListOrders.Tool {
			data, err := json.Marshal(result.StructuredContent)
			if err != nil || !strings.Contains(string(data), `"customer_id":"00000000-0000-0000-0000-000000000002"`) || !strings.Contains(string(data), `"total":0`) || !strings.Contains(string(data), `"items":[]`) {
				t.Fatalf("order list tool returned %s: %v", data, err)
			}
		}
	}
}
