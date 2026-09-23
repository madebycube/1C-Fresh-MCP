package mcpserver

import (
	"context"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type checkInput struct{}

type checkOutput struct {
	OK        bool `json:"ok"`
	Resources int  `json:"resources"`
}

type searchInput struct {
	Query string `json:"query" jsonschema:"Text to find in product name or article"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 50; defaults to 20"`
}

type searchOutput struct {
	Products []service.Product `json:"products"`
	Count    int               `json:"count"`
}

type listOrdersInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 100; defaults to 20"`
}

type listOrdersOutput struct {
	Orders []service.Order `json:"orders"`
	Count  int             `json:"count"`
}

type getOrderInput struct {
	ID string `json:"id" jsonschema:"Customer order GUID"`
}

func New(svc service.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "1c-fresh", Version: "0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.Check.Tool, Description: operations.Check.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ checkInput) (*mcp.CallToolResult, checkOutput, error) {
		count, err := svc.Check(ctx)
		return nil, checkOutput{OK: err == nil, Resources: count}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.SearchProducts.Tool, Description: operations.SearchProducts.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		products, err := svc.SearchProducts(ctx, input.Query, input.Limit)
		return nil, searchOutput{Products: products, Count: len(products)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListOrders.Tool, Description: operations.ListOrders.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listOrdersInput) (*mcp.CallToolResult, listOrdersOutput, error) {
		orders, err := svc.ListOrders(ctx, input.Limit)
		return nil, listOrdersOutput{Orders: orders, Count: len(orders)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetOrder.Tool, Description: operations.GetOrder.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getOrderInput) (*mcp.CallToolResult, service.Order, error) {
		order, err := svc.GetOrder(ctx, input.ID)
		return nil, order, err
	})
	return server
}
