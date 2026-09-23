package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type catalogReader struct {
	rows  map[string][]map[string]any
	calls int
}

func (r *catalogReader) Check(context.Context) (int, error) { return 1, nil }

func (r *catalogReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	r.calls++
	rows := r.rows[resource]
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	if skip > len(rows) {
		skip = len(rows)
	}
	end := min(skip+top, len(rows))
	data, _ := json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(rows)), "value": rows[skip:end]})
	return data, nil
}

func TestListGroupsPathsAndSearchAcrossPages(t *testing.T) {
	rows := make([]map[string]any, 0, 102)
	rows = append(rows, map[string]any{"Ref_Key": "root", "Description": "КЛИМОВО НОМЕНКЛАТУРА", "IsFolder": true, "DeletionMark": false})
	for index := 1; index < 101; index++ {
		rows = append(rows, map[string]any{"Ref_Key": strconv.Itoa(index), "Description": "Other " + strconv.Itoa(index), "IsFolder": true, "DeletionMark": false})
	}
	rows = append(rows, map[string]any{"Ref_Key": "child", "Parent_Key": "root", "Description": "Столы", "IsFolder": true, "DeletionMark": true})
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_Номенклатура": rows}}
	groups, err := (Service{OData: reader}).ListGroups(context.Background(), "климово")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Path != "КЛИМОВО НОМЕНКЛАТУРА" || groups[1].Path != "КЛИМОВО НОМЕНКЛАТУРА / Столы" || !groups[1].Deleted || reader.calls != 2 {
		t.Fatalf("unexpected groups: %+v, calls %d", groups, reader.calls)
	}
}

func TestListPriceTypesIncludesInactiveAndDeleted(t *testing.T) {
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_ВидыЦен": {
		{"Ref_Key": "retail", "Description": "Розничная", "DeletionMark": false, "Недействителен": false},
		{"Ref_Key": "old", "Description": "Старая", "DeletionMark": true, "Недействителен": true},
	}}}
	types, err := (Service{OData: reader}).ListPriceTypes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(types) != 2 || types[0].Name != "Розничная" || !types[1].Deleted || !types[1].Inactive {
		t.Fatalf("unexpected types: %+v", types)
	}
}

func TestCatalogRejectsRepeatedIDAndBrokenGroup(t *testing.T) {
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_Номенклатура": {
		{"Ref_Key": "same", "Description": "A", "IsFolder": true},
		{"Ref_Key": "same", "Description": "B", "IsFolder": true},
	}}}
	if _, err := (Service{OData: reader}).ListGroups(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatal("repeated ID accepted")
	}
	reader.rows["Catalog_Номенклатура"] = []map[string]any{{"Ref_Key": "product", "Description": "Not a group", "IsFolder": false, "DeletionMark": false}}
	if _, err := (Service{OData: reader}).ListGroups(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatal("non-group row accepted")
	}
}
