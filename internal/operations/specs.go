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

var All = []Spec{Check, SearchProducts}
