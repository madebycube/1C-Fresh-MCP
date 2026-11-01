package service

import (
	"context"
	"strings"
	"testing"
)

func orderPaymentsFixture() *traceReader {
	org, customer, other, contract := salesID(80), salesID(50), salesID(51), salesID(60)
	order, order2 := salesID(1), salesID(2)
	cashIn, card, receipt, bankIn, bankOut, shipment, offset, unallocated := salesID(10), salesID(11), salesID(12), salesID(13), salesID(14), salesID(15), salesID(16), salesID(17)
	const (
		cashInType   = "StandardODATA.Document_ПоступлениеВКассу"
		bankInType   = "StandardODATA.Document_ПоступлениеНаСчет"
		bankOutType  = "StandardODATA.Document_РасходСоСчета"
		cardType     = "StandardODATA.Document_ОперацияПоПлатежнымКартам"
		receiptType  = "StandardODATA.Document_ЧекККМ"
		shipmentType = "StandardODATA.Document_РасходнаяНакладная"
		offsetType   = "StandardODATA.Document_Взаимозачет"
	)
	record := func(recorder, recorderType, line, period, kind, settlement, orderID string, amount float64) map[string]any {
		orderType := customerOrderType
		if orderID == "" {
			orderID, orderType = emptyGUID, "StandardODATA.Undefined"
		}
		return map[string]any{"Recorder": recorder, "Recorder_Type": recorderType, "LineNumber": line, "Period": period, "Active": true, "RecordType": kind, "ТипРасчетов": settlement,
			"Организация_Key": org, "Контрагент_Key": customer, "Договор_Key": contract, "Документ": recorder, "Документ_Type": recorderType, "Заказ": orderID, "Заказ_Type": orderType, "Сумма": amount, "СуммаВал": amount}
	}
	inactive := record(salesID(20), cashInType, "1", "2026-09-03T09:00:00", "Expense", "Аванс", order, 999)
	inactive["Active"] = false
	foreign := record(salesID(21), cashInType, "1", "2026-09-03T09:00:00", "Expense", "Аванс", "", 5)
	foreign["Контрагент_Key"] = other
	money := func(id, number, date string, posted, deleted bool, amount float64, basis string) map[string]any {
		row := map[string]any{"Ref_Key": id, "Number": number, "Date": date, "Posted": posted, "DeletionMark": deleted, "СуммаДокумента": amount, "ВидОперации": "ОтПокупателя", "ДокументОснование": emptyGUID, "ДокументОснование_Type": "StandardODATA.Undefined"}
		if basis != "" {
			row["ДокументОснование"], row["ДокументОснование_Type"] = basis, customerOrderType
		}
		return row
	}
	return &traceReader{rows: map[string][]map[string]any{
		"Document_ЗаказПокупателя": {
			{"Ref_Key": order, "Number": "ЗП-0001 ", "Date": "2026-09-01T10:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 1000, "ВалютаДокумента_Key": salesID(70), "Организация_Key": org, "Контрагент_Key": customer, "Договор_Key": contract},
			{"Ref_Key": order2, "Number": "ЗП-0002", "Date": "2026-09-01T11:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 350, "Организация_Key": org, "Контрагент_Key": customer, "Договор_Key": contract},
			{"Ref_Key": salesID(3), "Number": "ЗП-0002", "Date": "2026-09-02T11:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 10, "Организация_Key": org, "Контрагент_Key": other, "Договор_Key": contract},
		},
		"AccumulationRegister_РасчетыСПокупателями_RecordType": {
			record(cashIn, cashInType, "1", "2026-09-02T09:00:00", "Expense", "Аванс", order, 300),
			record(card, cardType, "1", "2026-09-03T09:00:00", "Expense", "Аванс", order, 200),
			record(receipt, receiptType, "1", "2026-09-04T09:00:00", "Receipt", "Долг", order, 400),
			record(receipt, receiptType, "2", "2026-09-04T09:00:00", "Expense", "Долг", order, 400),
			record(bankIn, bankInType, "1", "2026-09-05T09:00:00", "Expense", "Аванс", order, 150),
			record(bankIn, bankInType, "2", "2026-09-05T09:00:00", "Expense", "Аванс", order2, 350),
			record(shipment, shipmentType, "1", "2026-09-06T09:00:00", "Receipt", "Долг", order, 1000),
			record(shipment, shipmentType, "2", "2026-09-06T09:00:00", "Receipt", "Аванс", order, 300),
			record(shipment, shipmentType, "3", "2026-09-06T09:00:00", "Expense", "Долг", order, 300),
			record(bankOut, bankOutType, "1", "2026-09-07T09:00:00", "Receipt", "Аванс", order, 100),
			record(offset, offsetType, "1", "2026-09-08T09:00:00", "Expense", "Долг", order, 50),
			record(unallocated, cashInType, "1", "2026-09-09T09:00:00", "Expense", "Аванс", "", 70),
			record(salesID(22), cashInType, "1", "2026-10-02T09:00:00", "Expense", "Аванс", order, 500),
			inactive,
			foreign,
		},
		"Document_ПоступлениеВКассу": {
			money(cashIn, "К-1", "2026-09-02T09:00:00", true, false, 300, order),
			money(unallocated, "К-2", "2026-09-09T09:00:00", true, false, 70, ""),
			money(salesID(23), "К-DRAFT", "2026-09-10T09:00:00", false, false, 999, order),
			money(salesID(24), "К-DEL", "2026-09-10T09:00:00", false, true, 998, order),
		},
		"Document_ПоступлениеНаСчет": {
			money(bankIn, "Б-1", "2026-09-05T09:00:00", true, false, 500, ""),
			money(salesID(25), "Б-ЭКВ", "2026-09-06T09:00:00", true, false, 196, ""),
		},
		"Document_ОперацияПоПлатежнымКартам": {money(card, "Е-1", "2026-09-03T09:00:00", true, false, 200, order)},
		"Document_РасходСоСчета":             {money(bankOut, "В-1", "2026-09-07T09:00:00", true, false, 100, "")},
		"Document_ЧекККМ":                    {{"Ref_Key": receipt, "Number": "Ч-1", "Date": "2026-09-04T09:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 400, "Заказ_Key": order}},
		"Document_Взаимозачет":               {{"Ref_Key": offset, "Number": "ВЗ-1", "Date": "2026-09-08T09:00:00", "Posted": true, "DeletionMark": false}},
	}}
}

func TestOrderPaymentsCollectsAllocatedPayments(t *testing.T) {
	reader := orderPaymentsFixture()
	result, err := Service{OData: reader}.GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range reader.filters {
		if filter != "" {
			t.Fatalf("order payments must not use $filter: %q", filter)
		}
	}
	summary := result.Summary
	got := strings.Join([]string{summary.Status, string(summary.Payments), string(summary.Refunds), string(summary.Offsets), string(summary.NetPaid), string(summary.Remaining), string(summary.Overpaid), string(summary.Shipped), string(summary.RegisterBalance)}, " ")
	if got != "paid 1050 100 50 1000 0 0 1400 400" {
		t.Fatalf("summary = %s", got)
	}
	if result.Order.Number != "ЗП-0001" || result.Movements.Inactive != 1 || result.Movements.AfterAsOf != 1 || !summary.Complete {
		t.Fatalf("order or movement counts = %+v %+v", result.Order, result.Movements)
	}
	kinds := make([]string, 0)
	for _, doc := range result.Documents {
		kinds = append(kinds, doc.Kind+":"+doc.Role+":"+string(doc.Allocated)+":"+doc.Number+":"+string(doc.AllocatedOutside))
	}
	want := "cash-in:payment:300:К-1:,card-payment:payment:200:Е-1:,cash-register-receipt:payment:400:Ч-1:,bank-in:payment:150:Б-1:350,bank-out:refund:-100:В-1:,debt-offset:offset:50:ВЗ-1:"
	if strings.Join(kinds, ",") != want {
		t.Fatalf("documents = %s", strings.Join(kinds, ","))
	}
	if result.Unallocated.AdvanceBalance != "70" || len(result.Unallocated.Documents) != 1 || result.Unallocated.Documents[0].Number != "К-2" {
		t.Fatalf("unallocated = %+v", result.Unallocated)
	}
	notCounted := make([]string, 0)
	for _, doc := range result.NotCounted {
		notCounted = append(notCounted, doc.Number)
	}
	if strings.Join(notCounted, ",") != "К-DRAFT,К-DEL" {
		t.Fatalf("not counted = %v", notCounted)
	}
	if !summary.NeedsReview || len(result.Issues) != 1 || result.Issues[0].Kind != "unallocated-advance" {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestOrderPaymentsBeforeRefundAndAsOf(t *testing.T) {
	result, err := Service{OData: orderPaymentsFixture()}.GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-05"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Status != "overpaid" || result.Summary.NetPaid != "1050" || result.Summary.Overpaid != "50" || result.Summary.Refunds != "0" {
		t.Fatalf("summary = %+v", result.Summary)
	}
	result, err = Service{OData: orderPaymentsFixture()}.GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-02"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.Status != "partly_paid" || result.Summary.Remaining != "700" {
		t.Fatalf("summary = %+v", result.Summary)
	}
}

func TestOrderPaymentsFlagsPossibleDuplicate(t *testing.T) {
	reader := orderPaymentsFixture()
	register := reader.rows["AccumulationRegister_РасчетыСПокупателями_RecordType"]
	for _, row := range register {
		if row["Recorder"] == salesID(11) {
			row["Period"], row["СуммаВал"] = "2026-09-04T12:00:00", 400.0
		}
	}
	result, err := Service{OData: reader}.GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range result.Issues {
		found = found || issue.Kind == "possible-duplicate"
	}
	if !found || result.Summary.Payments != "1250" {
		t.Fatalf("duplicate must be flagged and still counted as 1C recorded it: %+v %+v", result.Issues, result.Summary)
	}
}

func TestOrderPaymentsByNumber(t *testing.T) {
	svc := Service{OData: orderPaymentsFixture()}
	result, err := svc.GetOrderPayments(context.Background(), OrderPaymentsQuery{Number: "зп-0001", AsOf: "2026-09-30"})
	if err != nil || result.Order.ID != salesID(1) {
		t.Fatalf("by number = %+v %v", result.Order, err)
	}
	_, err = svc.GetOrderPayments(context.Background(), OrderPaymentsQuery{Number: "ЗП-0002", AsOf: "2026-09-30"})
	if err == nil || !strings.Contains(err.Error(), "2 customer orders match") {
		t.Fatalf("ambiguous number error = %v", err)
	}
	result, err = svc.GetOrderPayments(context.Background(), OrderPaymentsQuery{Number: "ЗП-0002", CustomerID: salesID(50), AsOf: "2026-09-30"})
	if err != nil || result.Order.ID != salesID(2) || result.Summary.Payments != "350" || result.Summary.Status != "paid" {
		t.Fatalf("narrowed by customer = %+v %v", result.Summary, err)
	}
}

func TestOrderPaymentsValidationAndUnavailableRegister(t *testing.T) {
	svc := Service{OData: orderPaymentsFixture()}
	for _, query := range []OrderPaymentsQuery{{}, {OrderID: salesID(1), Number: "1"}, {OrderID: "bad"}, {OrderID: salesID(1), AsOf: "2026-13-01"}, {Number: "1", Date: "01.09.2026"}} {
		if _, err := svc.GetOrderPayments(context.Background(), query); err == nil {
			t.Fatalf("query %+v must fail", query)
		}
	}
	reader := orderPaymentsFixture()
	reader.fail = map[string]bool{"AccumulationRegister_РасчетыСПокупателями_RecordType": true}
	if _, err := (Service{OData: reader}).GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-30"}); err == nil || !strings.Contains(err.Error(), "cannot be confirmed") {
		t.Fatalf("unavailable register error = %v", err)
	}
	reader = orderPaymentsFixture()
	reader.fail = map[string]bool{"Document_ЧекККМ": true}
	result, err := Service{OData: reader}.GetOrderPayments(context.Background(), OrderPaymentsQuery{OrderID: salesID(1), AsOf: "2026-09-30"})
	if err != nil || result.Summary.Status != "incomplete" || result.Summary.Complete {
		t.Fatalf("partial scan = %+v %v", result.Summary, err)
	}
}
