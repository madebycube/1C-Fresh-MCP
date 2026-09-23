package service

import (
	"context"
	"net/url"
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
	return []byte(`{"value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"42","Date":"2026-09-23T10:00:00","Posted":false,"СуммаДокумента":99.50}]}`), nil
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
	filter := reader.params.Get("$filter")
	if filter != "DeletionMark eq false" || reader.params.Get("$orderby") != "Date asc,Ref_Key asc" || reader.params.Get("$top") != "1" {
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
