package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

type receiptReader struct {
	dates []string
	calls int
}

func (r *receiptReader) Check(context.Context) (int, error) { return 1, nil }

func (r *receiptReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	r.calls++
	if strings.Contains(resource, "(guid'") {
		return []byte(`{"Ref_Key":"00000000-0000-0000-0000-000000000001","Number":"R1","Date":"2026-09-03T10:00:00","Posted":true,"СуммаДокумента":49.75,"ЧекККМ_Key":"00000000-0000-0000-0000-000000000002","НомерЧекаККМ":"1","Запасы":[{"LineNumber":"1","Номенклатура_Key":"00000000-0000-0000-0000-000000000003","Количество":1,"Цена":49.75,"Сумма":49.75,"Всего":49.75}],"БезналичнаяОплата":[{"LineNumber":"1","ВидОплаты":"card","Сумма":49.75,"ОплатаОтменена":false}]}`), nil
	}
	if params.Get("$inlinecount") != "" {
		data, _ := json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.dates)), "value": []any{}})
		return data, nil
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	rows := make([]map[string]any, 0)
	for index := skip; index < skip+top && index < len(r.dates); index++ {
		rows = append(rows, map[string]any{"Ref_Key": "00000000-0000-0000-0000-00000000000" + strconv.Itoa(index), "Number": strconv.Itoa(index), "Date": r.dates[index], "Posted": true, "DeletionMark": false, "СуммаДокумента": 10, "НомерЧекаККМ": strconv.Itoa(index)})
	}
	data, _ := json.Marshal(map[string]any{"value": rows})
	return data, nil
}

func TestListReceiptsPagesDateRange(t *testing.T) {
	reader := &receiptReader{dates: []string{
		"2026-09-01T09:00:00", "2026-09-02T10:00:00", "2026-09-03T08:00:00", "2026-09-03T18:00:00", "2026-09-05T11:00:00",
	}}
	svc := Service{OData: reader}
	first, err := svc.ListReceipts(context.Background(), "sale", "2026-09-02", "2026-09-03", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || len(first.Items) != 2 || first.NextOffset == nil || *first.NextOffset != 2 || first.Items[0].Date != reader.dates[1] {
		t.Fatalf("unexpected first page: %+v", first)
	}
	second, err := svc.ListReceipts(context.Background(), "sale", "2026-09-02", "2026-09-03", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 3 || len(second.Items) != 1 || second.NextOffset != nil || second.Items[0].Date != reader.dates[3] {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestGetRefundMapsLinesAndPayments(t *testing.T) {
	reader := &receiptReader{}
	svc := Service{OData: reader}
	if _, err := svc.GetReceipt(context.Background(), "refund", "bad-id"); err == nil || reader.calls != 0 {
		t.Fatal("invalid ID reached OData")
	}
	receipt, err := svc.GetReceipt(context.Background(), "refund", "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != "refund" || receipt.Amount.String() != "49.75" || len(receipt.Lines) != 1 || len(receipt.CashlessPayments) != 1 || receipt.CashlessPayments[0].Cancelled == nil || *receipt.CashlessPayments[0].Cancelled {
		t.Fatalf("unexpected refund: %+v", receipt)
	}
}

func TestListReceiptsRejectsInvalidRange(t *testing.T) {
	reader := &receiptReader{}
	svc := Service{OData: reader}
	for _, test := range []struct {
		kind, from, to string
		limit, offset  int
	}{
		{"unknown", "2026-09-01", "2026-09-02", 1, 0},
		{"sale", "bad", "2026-09-02", 1, 0},
		{"sale", "2026-09-03", "2026-09-02", 1, 0},
		{"sale", "2026-01-01", "2026-09-02", 1, 0},
		{"sale", "2026-09-01", "2026-09-02", 101, 0},
		{"sale", "2026-09-01", "2026-09-02", 1, -1},
	} {
		if _, err := svc.ListReceipts(context.Background(), test.kind, test.from, test.to, test.limit, test.offset); err == nil {
			t.Errorf("accepted %+v", test)
		}
	}
	if reader.calls != 0 {
		t.Fatal("invalid input reached OData")
	}
}
