package operations

type Spec struct {
	Command     string
	Tool        string
	Description string
	Usage       string
	Example     string
	Advanced    bool
}

var Check = Spec{
	Command:     "check",
	Tool:        "check_connection",
	Description: "Check the connection to 1C.",
	Usage:       "1c check [--json]",
	Example:     "1c check",
}

var ListGroups = Spec{
	Command:     "list groups",
	Tool:        "list_product_groups",
	Description: "List product folders, their paths, and 1C IDs.",
	Usage:       "1c list groups [--name TEXT] [--json]",
	Example:     "1c list groups --name \"Пример группы\"",
}

var CreateGroup = Spec{
	Command:     "create group",
	Tool:        "create_product_group",
	Description: "Create a product group in 1C.",
	Usage:       "1c create group --name NAME [--parent GUID] [--json]",
	Example:     "1c create group --name НоваяГруппа --parent PARENT_GUID",
}

var UpdateGroup = Spec{
	Command:     "update group",
	Tool:        "update_product_group",
	Description: "Rename an existing product group by 1C ID.",
	Usage:       "1c update group GUID --name NAME [--json]",
	Example:     "1c update group GROUP_GUID --name НовоеИмя",
}

var ListPriceTypes = Spec{
	Command:     "list price-types",
	Tool:        "list_price_types",
	Description: "List price types and their 1C IDs.",
	Usage:       "1c list price-types [--json]",
	Example:     "1c list price-types",
}

var GetPrice = Spec{
	Command:     "get price",
	Tool:        "get_product_price",
	Description: "Find a product's effective price for a named price type.",
	Usage:       "1c get price PRODUCT_GUID --price-type NAME [--characteristic GUID] [--as-of YYYY-MM-DD] [--json]",
	Example:     "1c get price PRODUCT_GUID --price-type \"Пример цены\"",
}

var ListWarehouses = Spec{
	Command:     "list warehouses",
	Tool:        "list_warehouses",
	Description: "List warehouses and retail stores with their 1C IDs.",
	Usage:       "1c list warehouses [--json]",
	Example:     "1c list warehouses",
}

var GetStock = Spec{
	Command:     "get stock",
	Tool:        "get_product_stock",
	Description: "Show current product stock by warehouse.",
	Usage:       "1c get stock PRODUCT_GUID [--warehouse GUID] [--characteristic GUID] [--json]",
	Example:     "1c get stock PRODUCT_GUID --warehouse WAREHOUSE_GUID",
}

var SearchProducts = Spec{
	Command:     "search products",
	Tool:        "find_nomenclature",
	Description: "Find products by name or article.",
	Usage:       "1c search products [--limit N] [--json] QUERY",
	Example:     "1c search products \"название товара\"",
}

var UpdateProduct = Spec{
	Command:     "update product",
	Tool:        "update_product",
	Description: "Edit a product name, full name, or article by 1C ID.",
	Usage:       "1c update product GUID [--name TEXT] [--full-name TEXT] [--article TEXT] [--json]",
	Example:     "1c update product PRODUCT_GUID --article NEW-ARTICLE",
}

var ListOrders = Spec{
	Command:     "list orders",
	Tool:        "list_customer_orders",
	Description: "List recent customer orders.",
	Usage:       "1c list orders [--limit N] [--json]",
	Example:     "1c list orders --limit 10",
}

var GetOrder = Spec{
	Command:     "get order",
	Tool:        "get_customer_order",
	Description: "Show an order and its product lines by 1C ID.",
	Usage:       "1c get order [--json] GUID",
	Example:     "1c get order --json ORDER_GUID",
}

var ListReceipts = Spec{
	Command:     "list receipts",
	Tool:        "list_cash_receipts",
	Description: "List sale or refund receipts in a date range.",
	Usage:       "1c list receipts --from YYYY-MM-DD --to YYYY-MM-DD [--kind sale|refund] [--limit N] [--offset N] [--json]",
	Example:     "1c list receipts --from 2026-09-01 --to 2026-09-07",
}

var GetReceipt = Spec{
	Command:     "get receipt",
	Tool:        "get_cash_receipt",
	Description: "Show a sale or refund receipt by 1C ID.",
	Usage:       "1c get receipt [--kind sale|refund] [--json] GUID",
	Example:     "1c get receipt --json RECEIPT_GUID",
}

var AuditUnpostedReceipts = Spec{
	Command:     "audit receipts",
	Tool:        "audit_unposted_receipts",
	Description: "List older unposted receipts for review.",
	Usage:       "1c audit receipts --from YYYY-MM-DD --before YYYY-MM-DD [--kind sale|refund|both] [--json]",
	Example:     "1c audit receipts --from 2026-09-01 --before 2026-09-24",
}

var SearchResources = Spec{
	Command:     "search resources",
	Tool:        "search_odata_resources",
	Description: "Find raw OData entity sets exposed by this 1C application.",
	Usage:       "1c search resources [--kind catalog|document|register|other] [--limit N] [--json] QUERY",
	Example:     "1c search resources --kind document Заказ",
	Advanced:    true,
}

var DescribeResource = Spec{
	Command:     "describe resource",
	Tool:        "describe_odata_resource",
	Description: "Show fields, types, keys, and links for one raw OData entity set.",
	Usage:       "1c describe resource [--json] EXACT_NAME",
	Example:     "1c describe resource Catalog_ВидыЦен",
	Advanced:    true,
}

var All = []Spec{Check, ListGroups, CreateGroup, UpdateGroup, ListPriceTypes, GetPrice, ListWarehouses, GetStock, SearchProducts, UpdateProduct, ListOrders, GetOrder, ListReceipts, GetReceipt, AuditUnpostedReceipts, SearchResources, DescribeResource}
