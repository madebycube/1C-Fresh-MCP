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
	Description: "Rename or move an existing product group by 1C ID.",
	Usage:       "1c update group GUID [--name NAME] [--parent GROUP_GUID|root] [--json]",
	Example:     "1c update group GROUP_GUID --parent PARENT_GUID",
}

var ListPriceTypes = Spec{
	Command:     "list price-types",
	Tool:        "list_price_types",
	Description: "List price types and their 1C IDs.",
	Usage:       "1c list price-types [--json]",
	Example:     "1c list price-types",
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

var ListCustomers = Spec{
	Command:     "list customers",
	Tool:        "list_customers",
	Description: "List customers with stable paging.",
	Usage:       "1c list customers [--limit N] [--offset N] [--json]",
	Example:     "1c list customers --limit 20",
}

var SearchCustomers = Spec{
	Command:     "search customers",
	Tool:        "search_customers",
	Description: "Find customers by name or code.",
	Usage:       "1c search customers [--limit N] [--offset N] [--json] QUERY",
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
	Description: "List suppliers with stable paging.",
	Usage:       "1c list suppliers [--limit N] [--offset N] [--json]",
	Example:     "1c list suppliers --limit 20",
}

var SearchSuppliers = Spec{
	Command:     "search suppliers",
	Tool:        "search_suppliers",
	Description: "Find suppliers by name or code.",
	Usage:       "1c search suppliers [--limit N] [--offset N] [--json] QUERY",
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

var All = []Spec{Check, ListGroups, CreateGroup, UpdateGroup, ListPriceTypes, ListUnitTypes, GetPrice, ListWarehouses, GetStock, ListProducts, GetProduct, SearchProducts, UpdateProduct, ListCustomers, SearchCustomers, GetCustomer, CreateCustomer, UpdateCustomer, ListSuppliers, SearchSuppliers, GetSupplier, CreateSupplier, UpdateSupplier, ListOrders, GetOrder, ListSales, GetSale, ListPurchases, GetPurchase, ListWarehouseDocuments, GetWarehouseDocument, ListMoneyAccounts, ListMoney, GetMoney, ListReceipts, GetReceipt, AuditUnpostedReceipts, SearchResources, DescribeResource}
