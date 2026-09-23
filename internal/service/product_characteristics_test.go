package service

import (
	"context"
	"testing"
)

func TestListProductCharacteristicsFiltersOwnerAndStatus(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": groupID, "Description": "Blue", "Owner": childID, "Owner_Type": "StandardODATA.Catalog_Номенклатура", "DeletionMark": false, "Недействителен": false},
		{"Ref_Key": "33333333-3333-3333-3333-333333333333", "Description": "Blue", "Owner": groupID, "Owner_Type": "StandardODATA.Catalog_Номенклатура", "DeletionMark": false, "Недействителен": false},
		{"Ref_Key": "44444444-4444-4444-4444-444444444444", "Description": "Blue", "Owner": childID, "Owner_Type": "StandardODATA.Catalog_Другое", "DeletionMark": false, "Недействителен": false},
		{"Ref_Key": "55555555-5555-5555-5555-555555555555", "Description": "Blue", "Owner": childID, "Owner_Type": "StandardODATA.Catalog_Номенклатура", "DeletionMark": true, "Недействителен": false},
		{"Ref_Key": "66666666-6666-6666-6666-666666666666", "Description": "Blue", "Owner": childID, "Owner_Type": "StandardODATA.Catalog_Номенклатура", "DeletionMark": false, "Недействителен": true},
	}
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_ХарактеристикиНоменклатуры": rows}}
	items, err := (Service{OData: reader}).ListProductCharacteristics(context.Background(), childID, "blue")
	if err != nil || len(items) != 1 || items[0].ID != groupID || items[0].ProductID != childID {
		t.Fatalf("characteristics: %+v, %v", items, err)
	}
	if _, err := (Service{OData: reader}).ListProductCharacteristics(context.Background(), "invalid", ""); err == nil {
		t.Fatal("accepted invalid product ID")
	}
	rows[0]["Недействителен"] = nil
	if _, err := (Service{OData: reader}).ListProductCharacteristics(context.Background(), "", ""); err == nil {
		t.Fatal("accepted missing inactive flag")
	}
}
