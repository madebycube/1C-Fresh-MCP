package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"testing"
)

type unreliableFilterReader struct {
	rows []map[string]any
}

func (r unreliableFilterReader) Check(context.Context) (int, error) { return 1, nil }

func (r unreliableFilterReader) Get(_ context.Context, _ string, params url.Values, _ int64) ([]byte, error) {
	if params.Get("$filter") != "" {
		return nil, errors.New("document scan used an unreliable server filter")
	}
	if params.Get("$inlinecount") != "" {
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.rows)), "value": []any{}})
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"value": r.rows[skip:min(skip+top, len(r.rows))]})
}

func TestDocumentScanFiltersDeletedAndSortsLocally(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": "00000000-0000-0000-0000-000000000001", "Date": "2026-09-03T10:00:00", "DeletionMark": false, "Number": "new"},
		{"Ref_Key": "00000000-0000-0000-0000-000000000002", "Date": "2026-09-02T10:00:00", "DeletionMark": true, "Number": "deleted"},
		{"Ref_Key": "00000000-0000-0000-0000-000000000003", "Date": "2026-09-01T10:00:00", "DeletionMark": false, "Number": "old"},
	}
	orders, err := (Service{OData: unreliableFilterReader{rows}}).ListOrders(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 2 || orders[0].Number != "new" || orders[1].Number != "old" {
		t.Fatalf("unexpected orders: %+v", orders)
	}
}
