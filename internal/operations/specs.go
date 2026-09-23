package operations

type Spec struct {
	Command     string
	Tool        string
	Description string
}

var Check = Spec{
	Command:     "check",
	Tool:        "check_connection",
	Description: "Verify read-only access to the configured 1C-Fresh OData application.",
}

var SearchProducts = Spec{
	Command:     "products search",
	Tool:        "find_nomenclature",
	Description: "Find existing nomenclature by name or article; returns at most 50 items.",
}

var ListOrders = Spec{
	Command:     "orders list",
	Tool:        "list_customer_orders",
	Description: "List recent customer orders without changing them; returns at most 100.",
}

var GetOrder = Spec{
	Command:     "orders get",
	Tool:        "get_customer_order",
	Description: "Read one customer order and its stock line items by GUID.",
}

var ListReceipts = Spec{
	Command:     "receipts list",
	Tool:        "list_cash_receipts",
	Description: "List sales receipts or refunds in a bounded application date range.",
}

var GetReceipt = Spec{
	Command:     "receipts get",
	Tool:        "get_cash_receipt",
	Description: "Read one cash receipt or refund, including stock lines and cashless payments.",
}

var AuditUnpostedReceipts = Spec{
	Command:     "receipts audit-unposted",
	Tool:        "audit_unposted_receipts",
	Description: "Find unposted sale and refund receipts before a date in a bounded read-only scan.",
}

var All = []Spec{Check, SearchProducts, ListOrders, GetOrder, ListReceipts, GetReceipt, AuditUnpostedReceipts}
