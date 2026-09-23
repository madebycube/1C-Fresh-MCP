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

type updateProductInput struct {
	ID       string  `json:"id" jsonschema:"Existing product GUID"`
	Name     *string `json:"name,omitempty" jsonschema:"Replacement product name"`
	FullName *string `json:"full_name,omitempty" jsonschema:"Replacement full name; empty string clears it"`
	Article  *string `json:"article,omitempty" jsonschema:"Replacement article; empty string clears it"`
}

type listGroupsInput struct {
	Name string `json:"name,omitempty" jsonschema:"Optional substring of a group name or path"`
}

type listGroupsOutput struct {
	Groups []service.Group `json:"groups"`
	Count  int             `json:"count"`
}

type createGroupInput struct {
	Name     string `json:"name" jsonschema:"Name for the new product group"`
	ParentID string `json:"parent_id,omitempty" jsonschema:"Optional parent product group GUID; omit for root"`
}

type updateGroupInput struct {
	ID   string `json:"id" jsonschema:"Existing product group GUID"`
	Name string `json:"name" jsonschema:"Replacement product group name"`
}

type listPriceTypesInput struct{}

type listPriceTypesOutput struct {
	PriceTypes []service.PriceType `json:"price_types"`
	Count      int                 `json:"count"`
}

type getPriceInput struct {
	ProductID        string `json:"product_id" jsonschema:"Product GUID from find_nomenclature"`
	PriceType        string `json:"price_type" jsonschema:"Exact price type name from list_price_types"`
	CharacteristicID string `json:"characteristic_id,omitempty" jsonschema:"Optional product characteristic GUID; omit for the default characteristic"`
	AsOf             string `json:"as_of,omitempty" jsonschema:"Application date, YYYY-MM-DD; defaults to today"`
}

type listWarehousesInput struct{}

type listWarehousesOutput struct {
	Warehouses []service.Warehouse `json:"warehouses"`
	Count      int                 `json:"count"`
}

type getStockInput struct {
	ProductID        string `json:"product_id" jsonschema:"Product GUID from find_nomenclature"`
	WarehouseID      string `json:"warehouse_id,omitempty" jsonschema:"Optional warehouse GUID from list_warehouses"`
	CharacteristicID string `json:"characteristic_id,omitempty" jsonschema:"Optional product characteristic GUID; omit for all characteristics"`
}

type searchResourcesInput struct {
	Query string `json:"query" jsonschema:"Text to find in an OData resource name"`
	Kind  string `json:"kind,omitempty" jsonschema:"Optional catalog, document, register, or other filter"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum results, 1 to 200; defaults to 50"`
}

type describeResourceInput struct {
	Name string `json:"name" jsonschema:"Exact OData entity-set name from search_odata_resources"`
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

type listCustomersInput struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 100; defaults to 20"`
	Offset int `json:"offset,omitempty" jsonschema:"Offset within the customer list"`
}

type searchCustomersInput struct {
	Query  string `json:"query" jsonschema:"Customer name or code substring"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 100; defaults to 20"`
	Offset int    `json:"offset,omitempty" jsonschema:"Offset within matching customers"`
}

type getCustomerInput struct {
	ID string `json:"id" jsonschema:"Customer GUID"`
}

type listSalesInput struct {
	Kind       string `json:"kind" jsonschema:"invoice, shipment, or return"`
	CustomerID string `json:"customer_id,omitempty" jsonschema:"Optional customer GUID"`
	From       string `json:"from,omitempty" jsonschema:"Optional first date YYYY-MM-DD; use with to"`
	To         string `json:"to,omitempty" jsonschema:"Optional last date YYYY-MM-DD; use with from"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 100; defaults to 20"`
	Offset     int    `json:"offset,omitempty" jsonschema:"Offset within matching documents"`
}

type getSaleInput struct {
	Kind string `json:"kind" jsonschema:"invoice, shipment, or return"`
	ID   string `json:"id" jsonschema:"Sales document GUID"`
}

type listReceiptsInput struct {
	Kind   string `json:"kind" jsonschema:"sale or refund"`
	From   string `json:"from" jsonschema:"First application date, YYYY-MM-DD"`
	To     string `json:"to" jsonschema:"Last application date, YYYY-MM-DD; no more than 31 days after from"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results, from 1 to 100; defaults to 20"`
	Offset int    `json:"offset,omitempty" jsonschema:"Offset within matching receipts; defaults to zero"`
}

type getReceiptInput struct {
	Kind string `json:"kind" jsonschema:"sale or refund"`
	ID   string `json:"id" jsonschema:"Receipt GUID"`
}

type auditUnpostedInput struct {
	Kind   string `json:"kind,omitempty" jsonschema:"sale, refund, or both; defaults to both"`
	From   string `json:"from" jsonschema:"First application date, YYYY-MM-DD"`
	Before string `json:"before" jsonschema:"Exclusive application date cutoff, YYYY-MM-DD; range at most 31 days"`
}

func New(svc service.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "1c-fresh", Version: "0.1.0"}, nil)
	additive := false
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.Check.Tool, Description: operations.Check.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ checkInput) (*mcp.CallToolResult, checkOutput, error) {
		count, err := svc.Check(ctx)
		return nil, checkOutput{OK: err == nil, Resources: count}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListGroups.Tool, Description: operations.ListGroups.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listGroupsInput) (*mcp.CallToolResult, listGroupsOutput, error) {
		groups, err := svc.ListGroups(ctx, input.Name)
		return nil, listGroupsOutput{Groups: groups, Count: len(groups)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.CreateGroup.Tool, Description: operations.CreateGroup.Description,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &additive},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input createGroupInput) (*mcp.CallToolResult, service.GroupChange, error) {
		change, err := svc.CreateGroup(ctx, input.Name, input.ParentID)
		return nil, change, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.UpdateGroup.Tool, Description: operations.UpdateGroup.Description,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateGroupInput) (*mcp.CallToolResult, service.GroupChange, error) {
		change, err := svc.UpdateGroup(ctx, input.ID, input.Name)
		return nil, change, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListPriceTypes.Tool, Description: operations.ListPriceTypes.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listPriceTypesInput) (*mcp.CallToolResult, listPriceTypesOutput, error) {
		priceTypes, err := svc.ListPriceTypes(ctx)
		return nil, listPriceTypesOutput{PriceTypes: priceTypes, Count: len(priceTypes)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetPrice.Tool, Description: operations.GetPrice.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getPriceInput) (*mcp.CallToolResult, service.PriceQuote, error) {
		quote, err := svc.GetPrice(ctx, input.ProductID, input.PriceType, input.CharacteristicID, input.AsOf)
		return nil, quote, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListWarehouses.Tool, Description: operations.ListWarehouses.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listWarehousesInput) (*mcp.CallToolResult, listWarehousesOutput, error) {
		warehouses, err := svc.ListWarehouses(ctx)
		return nil, listWarehousesOutput{Warehouses: warehouses, Count: len(warehouses)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetStock.Tool, Description: operations.GetStock.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getStockInput) (*mcp.CallToolResult, service.StockResult, error) {
		stock, err := svc.GetStock(ctx, input.ProductID, input.WarehouseID, input.CharacteristicID)
		return nil, stock, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.SearchProducts.Tool, Description: operations.SearchProducts.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, searchOutput, error) {
		products, err := svc.SearchProducts(ctx, input.Query, input.Limit)
		return nil, searchOutput{Products: products, Count: len(products)}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.UpdateProduct.Tool, Description: operations.UpdateProduct.Description,
		Annotations: &mcp.ToolAnnotations{IdempotentHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updateProductInput) (*mcp.CallToolResult, service.ProductChange, error) {
		change, err := svc.UpdateProduct(ctx, input.ID, service.ProductPatch{Name: input.Name, FullName: input.FullName, Article: input.Article})
		return nil, change, err
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
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListCustomers.Tool, Description: operations.ListCustomers.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listCustomersInput) (*mcp.CallToolResult, service.CustomerPage, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}
		page, err := svc.ListCustomers(ctx, "", limit, input.Offset)
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.SearchCustomers.Tool, Description: operations.SearchCustomers.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchCustomersInput) (*mcp.CallToolResult, service.CustomerPage, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}
		page, err := svc.ListCustomers(ctx, input.Query, limit, input.Offset)
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetCustomer.Tool, Description: operations.GetCustomer.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getCustomerInput) (*mcp.CallToolResult, service.Customer, error) {
		customer, err := svc.GetCustomer(ctx, input.ID)
		return nil, customer, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListSales.Tool, Description: operations.ListSales.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listSalesInput) (*mcp.CallToolResult, service.SalesDocumentPage, error) {
		limit := input.Limit
		if limit == 0 {
			limit = 20
		}
		page, err := svc.ListSalesDocuments(ctx, input.Kind, input.CustomerID, input.From, input.To, limit, input.Offset)
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetSale.Tool, Description: operations.GetSale.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getSaleInput) (*mcp.CallToolResult, service.SalesDocument, error) {
		doc, err := svc.GetSalesDocument(ctx, input.Kind, input.ID)
		return nil, doc, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.ListReceipts.Tool, Description: operations.ListReceipts.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listReceiptsInput) (*mcp.CallToolResult, service.ReceiptPage, error) {
		page, err := svc.ListReceipts(ctx, input.Kind, input.From, input.To, input.Limit, input.Offset)
		return nil, page, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.GetReceipt.Tool, Description: operations.GetReceipt.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getReceiptInput) (*mcp.CallToolResult, service.Receipt, error) {
		receipt, err := svc.GetReceipt(ctx, input.Kind, input.ID)
		return nil, receipt, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.AuditUnpostedReceipts.Tool, Description: operations.AuditUnpostedReceipts.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input auditUnpostedInput) (*mcp.CallToolResult, service.ReceiptAudit, error) {
		report, err := svc.AuditUnpostedReceipts(ctx, input.Kind, input.From, input.Before)
		return nil, report, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.SearchResources.Tool, Description: operations.SearchResources.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input searchResourcesInput) (*mcp.CallToolResult, service.ResourceSearch, error) {
		result, err := svc.SearchResources(ctx, input.Query, input.Kind, input.Limit)
		return nil, result, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: operations.DescribeResource.Tool, Description: operations.DescribeResource.Description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input describeResourceInput) (*mcp.CallToolResult, service.ResourceDetail, error) {
		result, err := svc.DescribeResource(ctx, input.Name)
		return nil, result, err
	})
	return server
}
