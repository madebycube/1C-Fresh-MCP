package operations

type Spec struct {
	Command     string
	Tool        string
	Description string
	Usage       string
	Example     string
}

type Topic struct {
	Name        string
	Description string
	Commands    []Spec
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

var ListProductCategories = Spec{
	Command:     "list product-categories",
	Tool:        "list_product_categories",
	Description: "List active product categories, paths, default types, units, and 1C IDs.",
	Usage:       "1c list product-categories [--name TEXT] [--json]",
	Example:     "1c list product-categories",
}

var ListProductCharacteristics = Spec{
	Command:     "list characteristics",
	Tool:        "list_product_characteristics",
	Description: "List active product characteristics and their 1C IDs.",
	Usage:       "1c list characteristics [--product PRODUCT_GUID] [--name TEXT] [--json]",
	Example:     "1c list characteristics --product PRODUCT_GUID",
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
	Description: "Rename or move an existing product group by 1C ID.",
	Usage:       "1c update group GUID [--name NAME] [--parent GROUP_GUID|root] [--json]",
	Example:     "1c update group GROUP_GUID --parent PARENT_GUID",
}

var ListCounterpartyGroups = Spec{
	Command:     "list counterparty-groups",
	Tool:        "list_counterparty_groups",
	Description: "List customer and supplier folders, paths, and 1C IDs.",
	Usage:       "1c list counterparty-groups [--name TEXT] [--json]",
	Example:     "1c list counterparty-groups",
}

var CreateCounterpartyGroup = Spec{
	Command:     "create counterparty-group",
	Tool:        "create_counterparty_group",
	Description: "Create a customer and supplier folder in 1C.",
	Usage:       "1c create counterparty-group --name NAME [--parent GUID] [--json]",
	Example:     "1c create counterparty-group --name \"Example partners\"",
}

var UpdateCounterpartyGroup = Spec{
	Command:     "update counterparty-group",
	Tool:        "update_counterparty_group",
	Description: "Rename or move a customer and supplier folder by 1C ID.",
	Usage:       "1c update counterparty-group GUID [--name NAME] [--parent GUID|root] [--json]",
	Example:     "1c update counterparty-group FOLDER_GUID --parent root",
}

var ListPriceTypes = Spec{
	Command:     "list price-types",
	Tool:        "list_price_types",
	Description: "List price types and their 1C IDs.",
	Usage:       "1c list price-types [--json]",
	Example:     "1c list price-types",
}

var ListCurrencies = Spec{
	Command:     "list currencies",
	Tool:        "list_currencies",
	Description: "List active currencies with codes, symbols, and 1C IDs.",
	Usage:       "1c list currencies [--json]",
	Example:     "1c list currencies",
}

var ListUnitTypes = Spec{
	Command:     "list unit-types",
	Tool:        "list_unit_types",
	Description: "List measurement-unit classifier entries and their 1C IDs.",
	Usage:       "1c list unit-types [--json]",
	Example:     "1c list unit-types",
}

var GetPrice = Spec{
	Command:     "get price",
	Tool:        "get_product_price",
	Description: "Find a product's effective price for a named price type.",
	Usage:       "1c get price PRODUCT_GUID --price-type NAME [--characteristic GUID] [--as-of YYYY-MM-DD] [--json]",
	Example:     "1c get price PRODUCT_GUID --price-type \"Пример цены\"",
}

var GetPriceDocument = Spec{
	Command:     "get price-document",
	Tool:        "get_price_document",
	Description: "Inspect a price-setting document and its price lines.",
	Usage:       "1c get price-document GUID [--product PRODUCT_GUID] [--limit N] [--offset N] [--json]",
	Example:     "1c get price-document DOCUMENT_GUID --product PRODUCT_GUID",
}

var ListPriceDocuments = Spec{
	Command:     "list price-documents",
	Tool:        "list_price_documents",
	Description: "Browse price-setting documents and their posting status.",
	Usage:       "1c list price-documents [--from YYYY-MM-DD --to YYYY-MM-DD] [--posted true|false] [--limit N] [--offset N] [--json]",
	Example:     "1c list price-documents --posted false --limit 20",
}

var ListPrices = Spec{
	Command:     "list prices",
	Tool:        "list_product_prices",
	Description: "List effective prices for a bounded page of products.",
	Usage:       "1c list prices --price-type NAME [--group GROUP_GUID|root] [--characteristic GUID] [--as-of YYYY-MM-DD] [--limit N] [--offset N] [--json]",
	Example:     "1c list prices --price-type \"Пример цены\" --group GROUP_GUID",
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

var ListProducts = Spec{
	Command:     "list products",
	Tool:        "list_products",
	Description: "Browse products, optionally within one group, with a bounded raw-catalog cursor.",
	Usage:       "1c list products [--group GROUP_GUID|root] [--limit N] [--offset N] [--json]",
	Example:     "1c list products --group GROUP_GUID --limit 20",
}

var GetProduct = Spec{
	Command:     "get product",
	Tool:        "get_product",
	Description: "Show a product, its type, and base unit ID by 1C ID.",
	Usage:       "1c get product [--json] GUID",
	Example:     "1c get product PRODUCT_GUID",
}

var UpdateProduct = Spec{
	Command:     "update product",
	Tool:        "update_product",
	Description: "Edit a product name, full name, article, or group by 1C ID.",
	Usage:       "1c update product GUID [--name TEXT] [--full-name TEXT] [--article TEXT] [--group GUID] [--json]",
	Example:     "1c update product PRODUCT_GUID --group GROUP_GUID",
}

var ListOrders = Spec{
	Command:     "list orders",
	Tool:        "list_customer_orders",
	Description: "List customer orders with optional customer and date filters, and paging.",
	Usage:       "1c list orders [--customer GUID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--limit N] [--offset N] [--json]",
	Example:     "1c list orders --customer CUSTOMER_GUID --limit 20",
}

var GetOrder = Spec{
	Command:     "get order",
	Tool:        "get_customer_order",
	Description: "Show an order and its product lines by 1C ID.",
	Usage:       "1c get order [--json] GUID",
	Example:     "1c get order --json ORDER_GUID",
}

var ListCustomers = Spec{
	Command:     "list customers",
	Tool:        "list_customers",
	Description: "List customers, optionally in one counterparty folder, with stable paging.",
	Usage:       "1c list customers [--group FOLDER_GUID|root] [--limit N] [--offset N] [--json]",
	Example:     "1c list customers --group FOLDER_GUID --limit 20",
}

var SearchCustomers = Spec{
	Command:     "search customers",
	Tool:        "search_customers",
	Description: "Find customers by name or code, optionally in one folder.",
	Usage:       "1c search customers [--group FOLDER_GUID|root] [--limit N] [--offset N] [--json] QUERY",
	Example:     "1c search customers \"Пример компании\"",
}

var GetCustomer = Spec{
	Command:     "get customer",
	Tool:        "get_customer",
	Description: "Show a customer by 1C ID.",
	Usage:       "1c get customer [--json] GUID",
	Example:     "1c get customer CUSTOMER_GUID",
}

var CreateCustomer = Spec{
	Command:     "create customer",
	Tool:        "create_customer",
	Description: "Create a customer counterparty in 1C.",
	Usage:       "1c create customer --name NAME [--full-name TEXT] [--parent GUID] [--json]",
	Example:     "1c create customer --name \"Example customer\"",
}

var UpdateCustomer = Spec{
	Command:     "update customer",
	Tool:        "update_customer",
	Description: "Rename an existing customer by 1C ID.",
	Usage:       "1c update customer GUID [--name TEXT] [--full-name TEXT] [--json]",
	Example:     "1c update customer CUSTOMER_GUID --name \"New name\"",
}

var ListSuppliers = Spec{
	Command:     "list suppliers",
	Tool:        "list_suppliers",
	Description: "List suppliers, optionally in one counterparty folder, with stable paging.",
	Usage:       "1c list suppliers [--group FOLDER_GUID|root] [--limit N] [--offset N] [--json]",
	Example:     "1c list suppliers --limit 20",
}

var SearchSuppliers = Spec{
	Command:     "search suppliers",
	Tool:        "search_suppliers",
	Description: "Find suppliers by name or code, optionally in one folder.",
	Usage:       "1c search suppliers [--group FOLDER_GUID|root] [--limit N] [--offset N] [--json] QUERY",
	Example:     "1c search suppliers \"Пример поставщика\"",
}

var GetSupplier = Spec{
	Command:     "get supplier",
	Tool:        "get_supplier",
	Description: "Show a supplier by 1C ID.",
	Usage:       "1c get supplier [--json] GUID",
	Example:     "1c get supplier SUPPLIER_GUID",
}

var CreateSupplier = Spec{
	Command:     "create supplier",
	Tool:        "create_supplier",
	Description: "Create a supplier counterparty in 1C.",
	Usage:       "1c create supplier --name NAME [--full-name TEXT] [--parent GUID] [--json]",
	Example:     "1c create supplier --name \"Example supplier\"",
}

var UpdateSupplier = Spec{
	Command:     "update supplier",
	Tool:        "update_supplier",
	Description: "Rename an existing supplier by 1C ID.",
	Usage:       "1c update supplier GUID [--name TEXT] [--full-name TEXT] [--json]",
	Example:     "1c update supplier SUPPLIER_GUID --name \"New name\"",
}

var ListSales = Spec{
	Command:     "list sales",
	Tool:        "list_sales_documents",
	Description: "List invoices, customer shipments, or customer returns.",
	Usage:       "1c list sales --kind invoice|shipment|return [--customer GUID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--limit N] [--offset N] [--json]",
	Example:     "1c list sales --kind shipment --limit 20",
}

var GetSale = Spec{
	Command:     "get sale",
	Tool:        "get_sales_document",
	Description: "Show an invoice, shipment, or return by 1C ID.",
	Usage:       "1c get sale --kind invoice|shipment|return [--json] GUID",
	Example:     "1c get sale --kind shipment DOCUMENT_GUID",
}

var ListPurchases = Spec{
	Command:     "list purchases",
	Tool:        "list_purchase_documents",
	Description: "List supplier orders or goods receipts.",
	Usage:       "1c list purchases --kind order|receipt [--supplier GUID] [--warehouse GUID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--limit N] [--offset N] [--json]",
	Example:     "1c list purchases --kind receipt --limit 20",
}

var GetPurchase = Spec{
	Command:     "get purchase",
	Tool:        "get_purchase_document",
	Description: "Show a supplier order or goods receipt by 1C ID.",
	Usage:       "1c get purchase --kind order|receipt [--json] GUID",
	Example:     "1c get purchase --kind order DOCUMENT_GUID",
}

var ListWarehouseDocuments = Spec{
	Command:     "list warehouse-docs",
	Tool:        "list_warehouse_documents",
	Description: "List transfer orders, transfers, stock receipts, or stock writeoffs.",
	Usage:       "1c list warehouse-docs --kind transfer-order|transfer|stock-receipt|stock-writeoff [--warehouse GUID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--limit N] [--offset N] [--json]",
	Example:     "1c list warehouse-docs --kind transfer --limit 20",
}

var GetWarehouseDocument = Spec{
	Command:     "get warehouse-doc",
	Tool:        "get_warehouse_document",
	Description: "Show a warehouse document and its product lines by 1C ID.",
	Usage:       "1c get warehouse-doc --kind transfer-order|transfer|stock-receipt|stock-writeoff [--json] GUID",
	Example:     "1c get warehouse-doc --kind transfer DOCUMENT_GUID",
}

var ListMoneyAccounts = Spec{
	Command:     "list accounts",
	Tool:        "list_money_accounts",
	Description: "List cash accounts, own bank accounts, or retail registers.",
	Usage:       "1c list accounts --kind cash|bank|register [--json]",
	Example:     "1c list accounts --kind bank",
}

var ListMoney = Spec{
	Command:     "list money",
	Tool:        "list_money_documents",
	Description: "List scoped cash, bank, card, or retail shift documents.",
	Usage:       "1c list money --kind cash-in|cash-out|bank-in|bank-out|card-payment|cash-shift --from YYYY-MM-DD --to YYYY-MM-DD [--account GUID] [--register GUID] [--terminal GUID] [--limit N] [--offset N] [--json]",
	Example:     "1c list money --kind bank-in --from 2026-09-01 --to 2026-09-30 --account BANK_ACCOUNT_GUID",
}

var GetMoney = Spec{
	Command:     "get money",
	Tool:        "get_money_document",
	Description: "Show a scoped cash, bank, card, or retail shift document by ID.",
	Usage:       "1c get money --kind cash-in|cash-out|bank-in|bank-out|card-payment|cash-shift [--json] GUID",
	Example:     "1c get money --kind card-payment DOCUMENT_GUID",
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
}

var DescribeResource = Spec{
	Command:     "describe resource",
	Tool:        "describe_odata_resource",
	Description: "Show fields, types, keys, and links for one raw OData entity set.",
	Usage:       "1c describe resource [--json] EXACT_NAME",
	Example:     "1c describe resource Catalog_ВидыЦен",
}

var Topics = []Topic{
	{Name: "setup", Description: "Connection and local MCP server", Commands: []Spec{Check}},
	{Name: "products", Description: "Products, groups, categories, and characteristics", Commands: []Spec{ListProducts, GetProduct, SearchProducts, UpdateProduct, ListGroups, CreateGroup, UpdateGroup, ListProductCategories, ListProductCharacteristics, ListUnitTypes}},
	{Name: "prices", Description: "Price types, currencies, quotes, and source documents", Commands: []Spec{ListPriceTypes, ListCurrencies, GetPrice, ListPrices, ListPriceDocuments, GetPriceDocument}},
	{Name: "inventory", Description: "Warehouses, stock, and warehouse documents", Commands: []Spec{ListWarehouses, GetStock, ListWarehouseDocuments, GetWarehouseDocument}},
	{Name: "partners", Description: "Customer and supplier records and folders", Commands: []Spec{ListCounterpartyGroups, CreateCounterpartyGroup, UpdateCounterpartyGroup, ListCustomers, SearchCustomers, GetCustomer, CreateCustomer, UpdateCustomer, ListSuppliers, SearchSuppliers, GetSupplier, CreateSupplier, UpdateSupplier}},
	{Name: "sales", Description: "Customer orders and sales documents", Commands: []Spec{ListOrders, GetOrder, ListSales, GetSale}},
	{Name: "purchases", Description: "Supplier orders and goods receipts", Commands: []Spec{ListPurchases, GetPurchase}},
	{Name: "money", Description: "Accounts, cash, bank, and card movements", Commands: []Spec{ListMoneyAccounts, ListMoney, GetMoney}},
	{Name: "receipts", Description: "Fiscal receipts and posting audit", Commands: []Spec{ListReceipts, GetReceipt, AuditUnpostedReceipts}},
	{Name: "schema", Description: "Search and inspect exposed OData resources", Commands: []Spec{SearchResources, DescribeResource}},
}

var All = func() []Spec {
	var commands []Spec
	for _, topic := range Topics {
		commands = append(commands, topic.Commands...)
	}
	return commands
}()
