package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type orderReader struct {
	resource string
	params   url.Values
}

func (o *orderReader) Check(context.Context) (int, error) { return 1, nil }

func (o *orderReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	o.resource = resource
	o.params = params
	if params.Get("$inlinecount") != "" {
		return []byte(`{"odata.count":"1","value":[]}`), nil
	}
	if strings.Contains(resource, "(guid'") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"42","Date":"2026-09-23T10:00:00","Posted":false,"СуммаДокумента":99.50,"СостояниеЗаказа":"Открыт","Контрагент_Key":"00000000-0000-0000-0000-000000000002","Запасы":[{"LineNumber":"1","Номенклатура":"Chair","Количество":2,"ЕдиницаИзмерения":"шт","Цена":49.75,"Сумма":99.50,"Всего":99.50}]}`), nil
	}
	return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"42","Date":"2026-09-23T10:00:00","Posted":false,"DeletionMark":false,"СуммаДокумента":99.50}]}`), nil
}

func TestListOrdersBuildsBoundedRecentQuery(t *testing.T) {
	reader := &orderReader{}
	orders, err := (Service{OData: reader}).ListOrders(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].Amount.String() != "99.50" {
		t.Fatalf("unexpected orders: %+v", orders)
	}
	if reader.params.Get("$filter") != "" || reader.params.Get("$orderby") != "Ref_Key asc" || reader.params.Get("$top") != "1" {
		t.Fatalf("unexpected query: %v", reader.params)
	}
}

func TestGetOrderMapsLinesAndValidatesGUID(t *testing.T) {
	reader := &orderReader{}
	svc := Service{OData: reader}
	if _, err := svc.GetOrder(context.Background(), "not-a-guid"); err == nil || reader.resource != "" {
		t.Fatal("invalid GUID reached OData")
	}
	order, err := svc.GetOrder(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if order.Number != "42" || len(order.Lines) != 1 || order.Lines[0].Price.String() != "49.75" || !strings.Contains(reader.params.Get("$select"), "Запасы") {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestListOrdersRejectsUnboundedInput(t *testing.T) {
	reader := &orderReader{}
	for _, limit := range []int{-1, 101} {
		if _, err := (Service{OData: reader}).ListOrders(context.Background(), limit); err == nil {
			t.Errorf("accepted limit %d", limit)
		}
	}
	if reader.resource != "" {
		t.Fatal("invalid input reached OData")
	}
}

type orderPageReader struct {
	rows []map[string]any
}

func (r orderPageReader) Check(context.Context) (int, error) { return 1, nil }

func (r orderPageReader) Get(_ context.Context, _ string, params url.Values, _ int64) ([]byte, error) {
	if params.Get("$inlinecount") != "" {
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.rows)), "value": []any{}})
	}
	return json.Marshal(map[string]any{"value": r.rows})
}

func TestListOrdersPageFiltersBeforePaging(t *testing.T) {
	customer := salesID(10)
	other := salesID(11)
	reader := orderPageReader{rows: []map[string]any{
		{"Ref_Key": salesID(1), "Number": "1", "Date": "2026-09-20T09:00:00", "DeletionMark": false, "Контрагент_Key": customer},
		{"Ref_Key": salesID(2), "Number": "2", "Date": "2026-09-21T09:00:00", "DeletionMark": false, "Контрагент_Key": other},
		{"Ref_Key": salesID(3), "Number": "3", "Date": "2026-09-22T09:00:00", "DeletionMark": false, "Контрагент_Key": customer},
		{"Ref_Key": salesID(4), "Number": "4", "Date": "2026-09-23T09:00:00", "DeletionMark": false, "Контрагент_Key": customer},
		{"Ref_Key": salesID(5), "Number": "5", "Date": "2026-09-23T10:00:00", "DeletionMark": true, "Контрагент_Key": customer},
	}}
	svc := Service{OData: reader}
	first, err := svc.ListOrdersPage(context.Background(), customer, "2026-09-22", "2026-09-23", 1, 0)
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Items[0].Number != "4" || first.NextOffset == nil || *first.NextOffset != 1 {
		t.Fatalf("first page: %+v, %v", first, err)
	}
	second, err := svc.ListOrdersPage(context.Background(), customer, "2026-09-22", "2026-09-23", 1, 1)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || second.Items[0].Number != "3" || second.NextOffset != nil {
		t.Fatalf("second page: %+v, %v", second, err)
	}
	for _, input := range []struct {
		customer, from, to string
		limit, offset      int
	}{
		{customer: "bad-guid", limit: 1},
		{from: "2026-09-22", limit: 1},
		{limit: 101},
		{limit: 1, offset: -1},
	} {
		if _, err := svc.ListOrdersPage(context.Background(), input.customer, input.from, input.to, input.limit, input.offset); err == nil {
			t.Fatalf("accepted invalid input: %+v", input)
		}
	}
}
