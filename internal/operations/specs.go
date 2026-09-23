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

var All = []Spec{Check, SearchProducts, ListOrders, GetOrder}
