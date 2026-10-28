package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// traceReader records filters so tests can check that reads avoid them.
type traceReader struct {
	rows    map[string][]map[string]any
	fail    map[string]bool
	filters []string
}

func (r *traceReader) Check(context.Context) (int, error) { return 1, nil }

func (r *traceReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	name := strings.Split(resource, "(")[0]
	if r.fail[name] {
		return nil, errors.New("OData returned HTTP 403")
	}
	rows := r.rows[name]
	if strings.Contains(resource, "(guid'") {
		id := strings.TrimSuffix(strings.TrimPrefix(resource[len(name):], "(guid'"), "')")
		for _, row := range rows {
			if row["Ref_Key"] == id {
				return json.Marshal(row)
			}
		}
		return nil, errors.New("OData returned HTTP 404")
	}
	filter := params.Get("$filter")
	r.filters = append(r.filters, filter)
	selected := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if supplier, ok := strings.CutPrefix(filter, "Контрагент_Key eq guid'"); ok && row["Контрагент_Key"] != strings.TrimSuffix(supplier, "'") {
			continue
		}
		if invoice, ok := strings.CutPrefix(filter, "СчетНаОплату eq cast(guid'"); ok && row["СчетНаОплату"] != invoice[:36] {
			continue
		}
		selected = append(selected, row)
	}
	if params.Get("$inlinecount") != "" && params.Get("$filter") != "" {
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(selected)), "value": selected[:min(1, len(selected))]})
	}
	if params.Get("$inlinecount") != "" {
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(selected)), "value": []any{}})
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"value": selected[min(skip, len(selected)):min(skip+top, len(selected))]})
}

func traceFixture() *traceReader {
	supplier, other := salesID(50), salesID(51)
	receipt, order, plan := salesID(1), salesID(2), salesID(3)
	return &traceReader{rows: map[string][]map[string]any{
		"Document_ПриходнаяНакладная": {
			{"Ref_Key": receipt, "Number": "R1", "Date": "2026-09-01T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "ПоступлениеОтПоставщика", "СуммаДокумента": 1000.10, "Контрагент_Key": supplier, "Договор_Key": salesID(60), "Заказ": order, "Заказ_Type": "StandardODATA.Document_ЗаказПоставщику", "ДатаВходящегоДокумента": "0001-01-01T00:00:00",
				"Предоплата": []map[string]any{{"LineNumber": "1", "Документ": salesID(30), "Документ_Type": "StandardODATA.Document_РасходСоСчета", "СуммаРасчетов": 100, "СуммаПлатежа": 100}}},
			{"Ref_Key": salesID(9), "Number": "RET", "Date": "2026-09-01T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "ВозвратОтПокупателя", "Контрагент_Key": supplier},
		},
		"Document_СчетНаОплатуПоставщика": {
			{"Ref_Key": plan, "Number": "INV1", "Date": "2026-09-01T11:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 1000.10, "Контрагент_Key": supplier, "ДокументОснование_Key": order, "ЗапланироватьОплату": true, "БанковскийСчет_Key": salesID(70), "Организация_Key": salesID(80),
				"ПлатежныйКалендарь": []map[string]any{{"LineNumber": "1", "ДатаОплаты": "2026-09-05T00:00:00", "ПроцентОплаты": 100, "СуммаОплаты": 1000.10, "СуммаНДСОплаты": 0}}},
			{"Ref_Key": salesID(4), "Number": "INV-OTHER", "Date": "2026-09-01T11:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 5, "Контрагент_Key": supplier, "ДокументОснование_Key": salesID(99)},
			{"Ref_Key": salesID(5), "Number": "INV-ELSE", "Date": "2026-09-02T11:00:00", "Posted": false, "DeletionMark": false, "СуммаДокумента": 7, "Контрагент_Key": other},
		},
		"Document_ПлатежноеПоручение": {
			{"Ref_Key": salesID(20), "Number": "PO1", "Date": "2026-09-02T09:00:00", "Posted": false, "DeletionMark": false, "ВидОперации": "Поставщику", "СуммаДокумента": 600, "Контрагент_Key": supplier, "Оплачено": true,
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 600, "СуммаПлатежа": 600, "ПризнакАванса": false}}},
		},
		"Document_РасходСоСчета": {
			{"Ref_Key": salesID(21), "Number": "B1", "Date": "2026-09-03T09:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Поставщику", "СуммаДокумента": 600, "Контрагент_Key": supplier, "ДокументОснование": salesID(20), "ДокументОснование_Type": "StandardODATA.Document_ПлатежноеПоручение",
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 400.05, "СуммаПлатежа": 400.05}, {"LineNumber": "2", "Документ": salesID(98), "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 199.95, "СуммаПлатежа": 199.95}}},
			{"Ref_Key": salesID(22), "Number": "B-PLAN", "Date": "2026-09-04T09:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Поставщику", "СуммаДокумента": 50, "Контрагент_Key": supplier,
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "СчетНаОплату": plan, "СчетНаОплату_Type": "StandardODATA.Document_СчетНаОплатуПоставщика", "Документ": salesID(97), "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 50, "СуммаПлатежа": 50}}},
			{"Ref_Key": salesID(23), "Number": "EQUAL", "Date": "2026-09-04T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Поставщику", "СуммаДокумента": "1000.1", "Контрагент_Key": supplier,
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Документ": salesID(96), "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 1000.10, "СуммаПлатежа": 1000.10}}},
			{"Ref_Key": salesID(24), "Number": "TAX", "Date": "2026-09-04T11:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Налоги", "СуммаДокумента": 1000.10, "Контрагент_Key": supplier,
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 1000.10}}},
			{"Ref_Key": salesID(25), "Number": "UNPOSTED", "Date": "2026-09-04T12:00:00", "Posted": false, "DeletionMark": false, "ВидОперации": "Поставщику", "СуммаДокумента": 300, "Контрагент_Key": supplier,
				"РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "СуммаРасчетов": 300, "СуммаПлатежа": 300}}},
		},
		"Document_РасходИзКассы": {},
		"AccumulationRegister_РасчетыСПоставщиками_RecordType": {
			{"Recorder": receipt, "Recorder_Type": "StandardODATA.Document_ПриходнаяНакладная", "LineNumber": "1", "Active": true, "RecordType": "Receipt", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "Сумма": 1000.10, "СуммаВал": 1000.10},
			{"Recorder": receipt, "Recorder_Type": "StandardODATA.Document_ПриходнаяНакладная", "LineNumber": "2", "Active": true, "RecordType": "Expense", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "ДокументОплаты": salesID(30), "ДокументОплаты_Type": "StandardODATA.Document_РасходСоСчета", "Сумма": 100, "СуммаВал": 100},
			{"Recorder": receipt, "Recorder_Type": "StandardODATA.Document_ПриходнаяНакладная", "LineNumber": "3", "Active": true, "RecordType": "Receipt", "Документ": salesID(30), "Документ_Type": "StandardODATA.Document_РасходСоСчета", "Сумма": 100, "СуммаВал": 100},
			{"Recorder": salesID(21), "Recorder_Type": "StandardODATA.Document_РасходСоСчета", "LineNumber": "1", "Active": true, "RecordType": "Expense", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "Сумма": 400.05, "СуммаВал": 400.05},
			{"Recorder": salesID(26), "Recorder_Type": "StandardODATA.Document_РасходИзКассы", "LineNumber": "1", "Active": false, "RecordType": "Expense", "Документ": receipt, "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "Сумма": 500, "СуммаВал": 500},
			{"Recorder": salesID(98), "Recorder_Type": "StandardODATA.Document_ПриходнаяНакладная", "LineNumber": "1", "Active": true, "RecordType": "Receipt", "Документ": salesID(98), "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "Сумма": 9, "СуммаВал": 9},
		},
		"AccumulationRegister_ПлатежныйКалендарь_RecordType": {
			{"Recorder": plan, "Recorder_Type": "StandardODATA.Document_СчетНаОплатуПоставщика", "Period": "2026-09-05T00:00:00", "Active": true, "СчетНаОплату": plan, "СчетНаОплату_Type": "StandardODATA.Document_СчетНаОплатуПоставщика", "СтатусУтвержденияПлатежа": "Утвержден", "Сумма": 1000.10, "СуммаОплаты": 0},
			{"Recorder": plan, "Recorder_Type": "StandardODATA.Document_СчетНаОплатуПоставщика", "Period": "2026-09-05T00:00:00", "Active": true, "СчетНаОплату": salesID(4), "СчетНаОплату_Type": "StandardODATA.Document_СчетНаОплатуПоставщика", "СтатусУтвержденияПлатежа": "НеУтвержден", "Сумма": 5},
		},
	}}
}

func TestTraceSupplierReceiptCountsOnlyLinkedRecordedPayments(t *testing.T) {
	reader := traceFixture()
	trace, err := Service{OData: reader}.TraceSupplierReceipt(context.Background(), salesID(1))
	if err != nil {
		t.Fatal(err)
	}
	if trace.Receipt.OrderID != salesID(2) || trace.Receipt.IncomingDate != "" {
		t.Fatalf("receipt: %+v", trace.Receipt)
	}
	if len(trace.PaymentPlans) != 1 || trace.PaymentPlans[0].ID != salesID(3) || trace.PaymentPlans[0].Link != "basis-supplier-order" {
		t.Fatalf("plans: %+v", trace.PaymentPlans)
	}
	kinds := map[string]TracedPayment{}
	for _, payment := range trace.Payments {
		kinds[payment.Number] = payment
	}
	if len(kinds) != 4 || kinds["PO1"].Kind != "payment-order" || kinds["B1"].PaymentOrderID != salesID(20) || len(kinds["B1"].Allocations) != 1 || kinds["B-PLAN"].Allocations[0].Link != "payment-plan" || !kinds["UNPOSTED"].Allocations[0].SettlementAmount.equal("300") {
		t.Fatalf("payments: %+v", trace.Payments)
	}
	if _, ok := kinds["TAX"]; ok {
		t.Fatal("non-supplier operation included")
	}
	// 400.05 recorded bank expense + 100 prepayment; the payment order, unposted expense and plan-level payment are not counted.
	if trace.Summary.LinkedSettlement != "500.05" || trace.Summary.OutstandingByLinks != "500.05" || trace.Summary.LinkStatus != "partly_settled_by_linked_payments" || !trace.Summary.Complete {
		t.Fatalf("summary: %+v", trace.Summary)
	}
	// The register holds the receipt debt, a bank payment and an advance offset; the inactive cash row is listed but not counted.
	settlements := trace.Settlements
	if !settlements.Available || len(settlements.Movements) != 4 || settlements.Debt != "1000.1" || settlements.Settled != "500.05" || settlements.Outstanding != "500.05" || trace.Summary.Status != "partly_closed_in_settlements" {
		t.Fatalf("settlements: %+v, summary %+v", settlements, trace.Summary)
	}
	sources := map[string]bool{}
	for _, movement := range settlements.Movements {
		sources[movement.Source] = true
	}
	if !sources["receipt"] || !sources["advance-offset"] || !sources["bank-out"] || !sources["cash-out"] {
		t.Fatalf("sources: %+v", sources)
	}
	if len(trace.Candidates) != 1 || trace.Candidates[0].Number != "EQUAL" {
		t.Fatalf("equal-amount payment must be a candidate only: %+v", trace.Candidates)
	}
	for _, scan := range trace.Scans {
		if scan.Resource == "Document_РасходСоСчета" && scan.Excluded != 1 {
			t.Fatalf("scan: %+v", scan)
		}
	}
	for _, filter := range reader.filters {
		if filter != "" {
			t.Fatalf("reference filters return HTTP 500 in the tested tenant; got %q", filter)
		}
	}
}

func (d Decimal) equal(value string) bool { return decimalEqual(d, Decimal(value)) }

func TestTraceReportsMissingSourcesAsIncomplete(t *testing.T) {
	reader := traceFixture()
	reader.fail = map[string]bool{"Document_ПлатежноеПоручение": true, "AccumulationRegister_РасчетыСПоставщиками_RecordType": true}
	reader.rows["Document_РасходСоСчета"] = nil
	reader.rows["Document_ПриходнаяНакладная"][0]["Предоплата"] = []map[string]any{}
	trace, err := Service{OData: reader}.TraceSupplierReceipt(context.Background(), salesID(1))
	if err != nil {
		t.Fatal(err)
	}
	if trace.Summary.Complete || trace.Summary.Status != "no_linked_payment_found" || trace.Summary.LinkedSettlement != "0" {
		t.Fatalf("summary: %+v", trace.Summary)
	}
	if !strings.Contains(strings.Join(trace.Summary.Notes, " "), "does not mean unpaid") {
		t.Fatalf("notes: %+v", trace.Summary.Notes)
	}
}

func TestTraceRejectsNonSupplierReceipts(t *testing.T) {
	svc := Service{OData: traceFixture()}
	if _, err := svc.TraceSupplierReceipt(context.Background(), salesID(9)); err == nil {
		t.Fatal("customer return accepted")
	}
	if _, err := svc.TraceSupplierReceipt(context.Background(), emptyGUID); err == nil {
		t.Fatal("empty GUID accepted")
	}
}

func TestSupplierPaymentPlansFilterAndApproval(t *testing.T) {
	svc := Service{OData: traceFixture()}
	page, err := svc.ListSupplierPaymentPlans(context.Background(), SupplierPaymentPlanFilter{SupplierID: salesID(50)}, 1, 0)
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.NextOffset == nil {
		t.Fatalf("page: %+v, %v", page, err)
	}
	page, err = svc.ListSupplierPaymentPlans(context.Background(), SupplierPaymentPlanFilter{AccountID: salesID(70), From: "2026-09-01", To: "2026-09-01"}, 20, 0)
	if err != nil || page.Total != 1 || page.Items[0].Number != "INV1" {
		t.Fatalf("account filter: %+v, %v", page, err)
	}
	if _, err := svc.ListSupplierPaymentPlans(context.Background(), SupplierPaymentPlanFilter{SupplierID: "x"}, 20, 0); err == nil {
		t.Fatal("invalid supplier filter accepted")
	}
	plan, err := svc.GetSupplierPaymentPlan(context.Background(), salesID(3))
	if err != nil || len(plan.Schedule) != 1 || plan.Calendar == nil || !plan.Calendar.Available || len(plan.Calendar.Records) != 1 || plan.Calendar.ApprovalStates[0] != "Утвержден" {
		t.Fatalf("plan: %+v, %v", plan, err)
	}
	reader := traceFixture()
	reader.fail = map[string]bool{"AccumulationRegister_ПлатежныйКалендарь_RecordType": true}
	plan, err = Service{OData: reader}.GetSupplierPaymentPlan(context.Background(), salesID(3))
	if err != nil || plan.Calendar.Available || plan.Calendar.Error == "" {
		t.Fatalf("unavailable calendar must be reported, not hidden: %+v, %v", plan.Calendar, err)
	}
}

func TestTraceSettlementStatusClosedAndOpen(t *testing.T) {
	reader := traceFixture()
	register := reader.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"]
	register = append(register, map[string]any{"Recorder": salesID(27), "Recorder_Type": "StandardODATA.Document_Взаимозачет", "LineNumber": "1", "Active": true, "RecordType": "Expense", "Документ": salesID(1), "Документ_Type": "StandardODATA.Document_ПриходнаяНакладная", "Сумма": 500.05, "СуммаВал": 500.05})
	reader.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"] = register
	trace, err := Service{OData: reader}.TraceSupplierReceipt(context.Background(), salesID(1))
	if err != nil || trace.Summary.Status != "closed_in_settlements" || trace.Settlements.Outstanding != "0" {
		t.Fatalf("closed: %+v %+v, %v", trace.Summary, trace.Settlements, err)
	}
	reader.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"] = register[:1]
	trace, err = Service{OData: reader}.TraceSupplierReceipt(context.Background(), salesID(1))
	if err != nil || trace.Summary.Status != "open_in_settlements" {
		t.Fatalf("open: %+v, %v", trace.Summary, err)
	}
	reader.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"] = append(register[:1:1], register[0])
	trace, err = Service{OData: reader}.TraceSupplierReceipt(context.Background(), salesID(1))
	if err != nil || trace.Settlements.Available || trace.Summary.Status != trace.Summary.LinkStatus {
		t.Fatalf("repeated rows must make the register unavailable: %+v, %v", trace.Settlements, err)
	}
}
