package service

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type fakeReader struct {
	mu      sync.Mutex
	filters []string
}

func (f *fakeReader) Check(context.Context) (int, error) { return 1221, nil }

func (f *fakeReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	f.mu.Lock()
	f.filters = append(f.filters, params.Get("$filter"))
	f.mu.Unlock()
	if resource != "Catalog_Номенклатура" || params.Get("$top") != "2" {
		return []byte(`{"value":null}`), nil
	}
	return []byte(`{"value":[{"Ref_Key":"item-1","Code":"001","Description":"Chair","НаименованиеПолное":"Chair blue","Артикул":"CH-1","Parent_Key":"group-1"}]}`), nil
}

func TestSearchProductsEscapesAndDeduplicates(t *testing.T) {
	reader := &fakeReader{}
	products, err := (Service{OData: reader}).SearchProducts(context.Background(), "  O'Brien  ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].ID != "item-1" || products[0].Article != "CH-1" {
		t.Fatalf("unexpected products: %+v", products)
	}
	if len(reader.filters) != 3 {
		t.Fatalf("got %d field queries; want 3", len(reader.filters))
	}
	for _, filter := range reader.filters {
		if !strings.Contains(filter, "O''Brien") || !strings.Contains(filter, "IsFolder eq false") || !strings.Contains(filter, "DeletionMark eq false") {
			t.Errorf("unsafe or incomplete filter: %q", filter)
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
