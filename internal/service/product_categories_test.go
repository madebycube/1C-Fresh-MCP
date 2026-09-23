package service

import (
	"context"
	"testing"
)

func TestListProductCategoriesShowsActiveLeavesWithPaths(t *testing.T) {
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_КатегорииНоменклатуры": {
		{"Ref_Key": groupID, "Description": "Furniture", "Parent_Key": emptyGUID, "IsFolder": true, "DeletionMark": false},
		{"Ref_Key": childID, "Description": "Chairs", "Parent_Key": groupID, "IsFolder": false, "DeletionMark": false, "ТипНоменклатурыПоУмолчанию": "Запас", "ЕдиницаИзмерения_Key": emptyGUID},
		{"Ref_Key": "33333333-3333-3333-3333-333333333333", "Description": "Old", "Parent_Key": groupID, "IsFolder": false, "DeletionMark": true},
	}}}
	categories, err := (Service{OData: reader}).ListProductCategories(context.Background(), "furniture")
	if err != nil || len(categories) != 1 || categories[0].Path != "Furniture / Chairs" || categories[0].Type != "Запас" || categories[0].ID != childID {
		t.Fatalf("categories: %+v, %v", categories, err)
	}
}
