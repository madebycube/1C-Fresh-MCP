package mcpserver

import (
	"context"
	"net/url"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stubReader struct{}

func (stubReader) Check(context.Context) (int, error) { return 1221, nil }
func (stubReader) Get(context.Context, string, url.Values, int64) ([]byte, error) {
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
	if len(listed.Tools) != 2 {
		t.Fatalf("got %d tools; want 2", len(listed.Tools))
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
		{operations.SearchProducts.Tool, map[string]any{"query": "Chair", "limit": 2}},
	} {
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError || result.StructuredContent == nil {
			t.Fatalf("tool %s failed: %v, %+v", call.name, err, result)
		}
	}
}
