package mcpserver

import (
	"context"
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
	if resource == "Catalog_ВидыЦен" {
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000002","Description":"Retail","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	if params.Get("$inlinecount") != "" {
		return []byte(`{"odata.count":"1","value":[]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЧекККМ") && strings.Contains(resource, "(guid'") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":true,"СуммаДокумента":10,"НомерЧекаККМ":"1","Запасы":[],"БезналичнаяОплата":[]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЧекККМ") {
		if params.Get("$select") == "Date" {
			return []byte(`{"value":[{"Date":"2026-09-23T00:00:00"}]}`), nil
		}
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":true,"СуммаДокумента":10,"НомерЧекаККМ":"1"}]}`), nil
	}
	if strings.HasPrefix(resource, "Document_ЗаказПокупателя(") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":false,"СуммаДокумента":10,"Запасы":[]}`), nil
	}
	if resource == "Document_ЗаказПокупателя" {
		return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"1","Date":"2026-09-23T00:00:00","Posted":false,"СуммаДокумента":10}]}`), nil
	}
	return []byte(`{"value":[{"Ref_Key":"item-1","Description":"Chair"}]}`), nil
}

func TestToolsAreReadOnlyAndCallable(t *testing.T) {
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
	if len(listed.Tools) != 11 {
		t.Fatalf("got %d tools; want 11", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %s is not marked read-only", tool.Name)
		}
	}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{operations.Check.Tool, map[string]any{}},
		{operations.ListGroups.Tool, map[string]any{}},
		{operations.ListPriceTypes.Tool, map[string]any{}},
		{operations.SearchProducts.Tool, map[string]any{"query": "Chair", "limit": 2}},
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
	}
}
