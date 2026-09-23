package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeReader struct {
	mu      sync.Mutex
	filters []string
	selects []string
}

func (f *fakeReader) Check(context.Context) (int, error) { return 1221, nil }

func (f *fakeReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	f.mu.Lock()
	f.filters = append(f.filters, params.Get("$filter"))
	f.selects = append(f.selects, params.Get("$select"))
	f.mu.Unlock()
	if resource != "Catalog_Номенклатура" || params.Get("$top") != "100" || params.Get("$orderby") != "Ref_Key asc" {
		return []byte(`{"value":null}`), nil
	}
	return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Code":"001","Description":"O'Brien chair","НаименованиеПолное":"O'Brien chair full","Артикул":"O'Brien-1","Parent_Key":"00000000-0000-0000-0000-000000000000","IsFolder":false,"DeletionMark":false}]}`), nil
}

func TestSearchProductsEscapesAndDeduplicates(t *testing.T) {
	reader := &fakeReader{}
	products, err := (Service{OData: reader}).SearchProducts(context.Background(), "  O'Brien  ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].ID != salesID(1) || products[0].Article != "O'Brien-1" {
		t.Fatalf("unexpected products: %+v", products)
	}
	if len(reader.filters) != 3 {
		t.Fatalf("got %d field queries; want 3", len(reader.filters))
	}
	for _, filter := range reader.filters {
		if !strings.Contains(filter, "O''Brien") || strings.Contains(filter, "IsFolder") || strings.Contains(filter, "DeletionMark") {
			t.Errorf("unsafe or incomplete filter: %q", filter)
		}
	}
	for _, selectFields := range reader.selects {
		if !strings.Contains(selectFields, "IsFolder") || !strings.Contains(selectFields, "DeletionMark") {
			t.Errorf("flags absent from select: %q", selectFields)
		}
	}
}

func TestSearchProductsRejectsInvalidInput(t *testing.T) {
	reader := &fakeReader{}
	for _, test := range []struct {
		query string
		limit int
	}{
		{"", 1}, {"ok\nnext", 1}, {"ok", 51}, {"ok", -1},
	} {
		if _, err := (Service{OData: reader}).SearchProducts(context.Background(), test.query, test.limit); err == nil {
			t.Errorf("accepted query %q, limit %d", test.query, test.limit)
		}
	}
	if len(reader.filters) != 0 {
		t.Fatal("invalid input reached OData")
	}
}

type pagedSearchReader struct {
	rows  []map[string]any
	mu    sync.Mutex
	calls int
}

func (*pagedSearchReader) Check(context.Context) (int, error) { return 1, nil }

func (reader *pagedSearchReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	if resource != "Catalog_Номенклатура" {
		return nil, fmt.Errorf("unexpected resource %s", resource)
	}
	reader.mu.Lock()
	reader.calls++
	reader.mu.Unlock()
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"value": reader.rows[min(skip, len(reader.rows)):min(skip+top, len(reader.rows))]})
}

func TestSearchProductsSkipsInvalidEarlyRows(t *testing.T) {
	rows := make([]map[string]any, 101)
	for index := range rows {
		rows[index] = map[string]any{
			"Ref_Key": salesID(index + 1), "Description": "match",
			"НаименованиеПолное": "match", "Артикул": "match",
			"IsFolder": true, "DeletionMark": false,
		}
	}
	rows[100]["IsFolder"] = false
	reader := &pagedSearchReader{rows: rows}
	products, err := (Service{OData: reader}).SearchProducts(context.Background(), "match", 1)
	if err != nil || len(products) != 1 || products[0].ID != salesID(101) || reader.calls != 6 {
		t.Fatalf("paged search: %+v, calls=%d, err=%v", products, reader.calls, err)
	}
}
