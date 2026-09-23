package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"testing"
)

type productBrowseReader struct {
	rows       []map[string]any
	calls      int
	groupCalls int
	groupRow   string
}

func (*productBrowseReader) Check(context.Context) (int, error) { return 1, nil }

func (reader *productBrowseReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	if resource == nomenclatureResource(groupID) {
		reader.groupCalls++
		return []byte(reader.groupRow), nil
	}
	if resource != "Catalog_Номенклатура" || params.Get("$filter") != "" || params.Get("$orderby") != "Ref_Key asc" {
		return nil, errors.New("unexpected product browse request")
	}
	reader.calls++
	skip, err := strconv.Atoi(params.Get("$skip"))
	if err != nil {
		return nil, err
	}
	top, err := strconv.Atoi(params.Get("$top"))
	if err != nil || top < 1 || top > productBrowseBatch {
		return nil, errors.New("invalid batch size")
	}
	if skip > len(reader.rows) {
		skip = len(reader.rows)
	}
	return json.Marshal(map[string]any{"value": reader.rows[skip:min(skip+top, len(reader.rows))]})
}

func TestListProductsInGroupUsesLocalFilterAndRawCursor(t *testing.T) {
	rows := make([]map[string]any, 501)
	for index := range rows {
		rows[index] = browseRow(index+1, false, false)
	}
	rows[500]["Parent_Key"] = groupID
	reader := &productBrowseReader{rows: rows, groupRow: `{"Ref_Key":"` + groupID + `","Description":"Destination","IsFolder":true,"DeletionMark":false}`}
	svc := Service{OData: reader}
	first, err := svc.ListProductsInGroup(context.Background(), 1, 0, groupID)
	if err != nil || len(first.Items) != 0 || first.Scanned != 500 || first.NextOffset == nil || *first.NextOffset != 500 || first.GroupID != groupID || reader.groupCalls != 1 {
		t.Fatalf("first group page: %+v, calls=%d, err=%v", first, reader.groupCalls, err)
	}
	second, err := svc.ListProductsInGroup(context.Background(), 1, *first.NextOffset, groupID)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != salesID(501) || second.NextOffset != nil {
		t.Fatalf("second group page: %+v, %v", second, err)
	}
	root, err := svc.ListProductsInGroup(context.Background(), 1, 0, "root")
	if err != nil || len(root.Items) != 1 || root.Items[0].ID != salesID(1) || root.GroupID != emptyGUID {
		t.Fatalf("root page: %+v, %v", root, err)
	}
	for _, invalid := range []string{"not-a-guid", emptyGUID[:35]} {
		if _, err := svc.ListProductsInGroup(context.Background(), 1, 0, invalid); err == nil {
			t.Fatalf("accepted invalid group %q", invalid)
		}
	}
	reader.groupRow = `{"Ref_Key":"` + groupID + `","IsFolder":false,"DeletionMark":false}`
	if _, err := svc.ListProductsInGroup(context.Background(), 1, 0, groupID); err == nil {
		t.Fatal("accepted product as group")
	}
}

func browseRow(index int, folder, deleted bool) map[string]any {
	return map[string]any{
		"Ref_Key": salesID(index), "Code": "P", "Description": "Product",
		"Parent_Key": emptyGUID, "IsFolder": folder, "DeletionMark": deleted,
	}
}

func TestListProductsUsesRawCursorAndLocalFlags(t *testing.T) {
	reader := &productBrowseReader{rows: []map[string]any{
		browseRow(1, true, false), browseRow(2, false, true),
		browseRow(3, false, false), browseRow(4, false, false), browseRow(5, false, false),
	}}
	svc := Service{OData: reader}
	first, err := svc.ListProducts(context.Background(), 2, 0)
	if err != nil || len(first.Items) != 2 || first.Items[0].ID != salesID(3) || first.Items[1].ID != salesID(4) || first.Scanned != 4 || first.NextOffset == nil || *first.NextOffset != 4 {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	second, err := svc.ListProducts(context.Background(), 2, *first.NextOffset)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != salesID(5) || second.Scanned != 1 || second.NextOffset != nil {
		t.Fatalf("second page: %+v, %v", second, err)
	}
	if reader.calls != 2 {
		t.Fatalf("got %d reads; want 2", reader.calls)
	}
}

func TestListProductsBoundsScanAndRejectsInvalidPages(t *testing.T) {
	rows := make([]map[string]any, 600)
	for index := range rows {
		rows[index] = browseRow(index+1, true, false)
	}
	reader := &productBrowseReader{rows: rows}
	svc := Service{OData: reader}
	page, err := svc.ListProducts(context.Background(), 20, 0)
	if err != nil || len(page.Items) != 0 || page.Scanned != productBrowseScanLimit || page.NextOffset == nil || *page.NextOffset != productBrowseScanLimit || reader.calls != 5 {
		t.Fatalf("bounded scan: %+v, calls=%d, err=%v", page, reader.calls, err)
	}
	for _, test := range []struct{ limit, offset int }{{0, 0}, {101, 0}, {20, -1}} {
		if _, err := svc.ListProducts(context.Background(), test.limit, test.offset); err == nil || reader.calls != 5 {
			t.Fatalf("accepted invalid page %+v", test)
		}
	}
}
