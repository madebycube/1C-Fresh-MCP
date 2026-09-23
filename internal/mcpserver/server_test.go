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
	if resource == "Catalog_Номенклатура" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Description":"Furniture","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false}]}`), nil
	}
	if strings.HasPrefix(resource, "Catalog_Номенклатура(") {
		if strings.Contains(resource, "00000000-0000-0000-0000-000000000004") {
			return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000004","Description":"Chair","НаименованиеПолное":"Chair full","Артикул":"A","IsFolder":false,"DeletionMark":false,"DataVersion":"version-1"}`), nil
		}
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Description":"Furniture","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":true,"DeletionMark":false,"DataVersion":"version-1"}`), nil
	}
	if resource == "Catalog_ВидыЦен" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000002","Description":"Retail","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if resource == "Catalog_СтруктурныеЕдиницы" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000006","Description":"Example warehouse","ТипСтруктурнойЕдиницы":"Склад","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if params.Get("$inlinecount") != "" {
		return []byte(`{"odata.count":"1","value":[]}`), nil
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
	if len(listed.Tools) != 17 {
		t.Fatalf("got %d tools; want 17", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil {
			t.Fatalf("tool %s has no annotations", tool.Name)
		}
		write := tool.Name == operations.CreateGroup.Tool || tool.Name == operations.UpdateGroup.Tool || tool.Name == operations.UpdateProduct.Tool
		if tool.Annotations.ReadOnlyHint == write {
			t.Fatalf("tool %s has incorrect read-only annotation", tool.Name)
		}
		if tool.Name == operations.CreateGroup.Tool && (tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint) {
			t.Fatal("create group must be marked additive")
		}
		if tool.Name == operations.UpdateGroup.Tool && !tool.Annotations.IdempotentHint {
			t.Fatal("update group must be marked idempotent")
		}
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{operations.Check.Tool, map[string]any{}},
		{operations.ListGroups.Tool, map[string]any{}},
		{operations.CreateGroup.Tool, map[string]any{"name": "New group"}},
		{operations.UpdateGroup.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000001", "name": "Renamed"}},
		{operations.ListPriceTypes.Tool, map[string]any{}},
		{operations.GetPrice.Tool, map[string]any{"product_id": "00000000-0000-0000-0000-000000000004", "price_type": "Retail", "as_of": "2026-09-23"}},
		{operations.ListWarehouses.Tool, map[string]any{}},
		{operations.GetStock.Tool, map[string]any{"product_id": "00000000-0000-0000-0000-000000000004"}},
		{operations.SearchProducts.Tool, map[string]any{"query": "Chair", "limit": 2}},
		{operations.UpdateProduct.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000004", "article": "B"}},
		{operations.ListOrders.Tool, map[string]any{"limit": 2}},
		{operations.GetOrder.Tool, map[string]any{"id": "00000000-0000-0000-0000-000000000001"}},
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
	}
}
