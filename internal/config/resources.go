package config

type FieldBinding struct {
	Output string
	Source string
}

type SearchResource struct {
	Name         string
	SearchFields []string
	Fields       []FieldBinding
	FolderField  string
	DeletedField string
}

var Products = SearchResource{
	Name:         "Catalog_Номенклатура",
	SearchFields: []string{"Description", "НаименованиеПолное", "Артикул"},
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "code", Source: "Code"},
		{Output: "name", Source: "Description"},
		{Output: "full_name", Source: "НаименованиеПолное"},
		{Output: "article", Source: "Артикул"},
		{Output: "parent_id", Source: "Parent_Key"},
	},
	FolderField:  "IsFolder",
	DeletedField: "DeletionMark",
}
