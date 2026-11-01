package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const testInvoiceID = "00000000-0000-0000-0000-000000000201"

// prepaymentFixture adds an unposted supplier invoice with no receipt, a cash desk, and payments that name the invoice.
func prepaymentFixture() *planWriter {
	fake := planFixture()
	org, supplier, contract, currency := salesID(80), salesID(50), salesID(60), salesID(61)
	fake.rows[paymentPlanResourceName] = []map[string]any{{
		"Ref_Key": testInvoiceID, "Number": "SP-12", "Date": "2026-10-20T09:00:00", "DataVersion": "v1", "Posted": false, "DeletionMark": false,
		"Организация_Key": org, "Контрагент_Key": supplier, "Договор_Key": contract, "ВалютаДокумента_Key": currency,
		"БанковскийСчет_Key": emptyGUID, "БанковскийСчетПоставщика_Key": emptyGUID, "Касса_Key": emptyGUID, "ТипДенежныхСредств": "",
		"ДокументОснование_Key": emptyGUID, "НомерВходящегоДокумента": "12", "ДатаВходящегоДокумента": "2026-10-19T00:00:00",
		"Подразделение_Key": salesID(63), "Комментарий": "Счёт поставщика", "ЗапланироватьОплату": false, "СуммаДокумента": 1220,
		"Запасы": []map[string]any{
			{"LineNumber": "1", "Всего": 1000, "СуммаНДС": 180.33},
			{"LineNumber": "2", "Всего": 220, "СуммаНДС": 0},
		},
		"ПлатежныйКалендарь": []map[string]any{},
	}}
	allocation := func(amount float64) []map[string]any {
		return []map[string]any{{"LineNumber": "1", "Договор_Key": contract, "СчетНаОплату": testInvoiceID, "СчетНаОплату_Type": supplierInvoiceType, "СуммаРасчетов": amount, "СуммаПлатежа": amount}}
	}
	fake.rows["Document_РасходСоСчета"] = []map[string]any{
		{"Ref_Key": salesID(210), "Number": "B1", "Date": "2026-10-21T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": supplierPaymentAction, "СуммаДокумента": 300, "Контрагент_Key": supplier, "РасшифровкаПлатежа": allocation(300)},
		{"Ref_Key": salesID(211), "Number": "B2", "Date": "2026-10-21T10:00:00", "Posted": false, "DeletionMark": false, "ВидОперации": supplierPaymentAction, "СуммаДокумента": 100, "Контрагент_Key": supplier, "РасшифровкаПлатежа": allocation(100)},
		{"Ref_Key": salesID(212), "Number": "B3", "Date": "2026-10-21T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": supplierPaymentAction, "СуммаДокумента": 920, "Контрагент_Key": supplier, "РасшифровкаПлатежа": []map[string]any{}},
	}
	fake.rows["Document_ПлатежноеПоручение"] = []map[string]any{
		{"Ref_Key": salesID(213), "Number": "PO1", "Date": "2026-10-21T10:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 920, "Контрагент_Key": supplier, "РасшифровкаПлатежа": allocation(920)},
	}
	fake.rows[cashDesksResourceName] = []map[string]any{
		{"Ref_Key": salesID(220), "DeletionMark": false, "Недействителен": false},
		{"Ref_Key": salesID(221), "DeletionMark": false, "Недействителен": true},
	}
	fake.rows["Catalog_СтруктурныеЕдиницы"] = []map[string]any{
		{"Ref_Key": salesID(230), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Подразделение", "Недействителен": false},
		{"Ref_Key": salesID(231), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Склад", "Недействителен": false},
	}
	return fake
}

func bankPrepayment() PrepaymentRequest {
	return PrepaymentRequest{InvoiceID: testInvoiceID, Source: "bank", AccountID: salesID(110), PaymentDate: "2026-11-05"}
}

func TestPrepaymentPreviewNeedsNoReceiptAndKeepsAccountsApart(t *testing.T) {
	fake := prepaymentFixture()
	preview, err := (Service{OData: fake}).PlanInvoicePrepayment(context.Background(), bankPrepayment())
	if err != nil {
		t.Fatal(err)
	}
	// Only the posted bank expense allocated to the invoice counts: 1220 − 300 = 920.
	if preview.Paid != "300" || preview.Remaining != "920" || preview.Amount != "920" || preview.VAT != "135.99" || preview.Percent != "75.41" {
		t.Fatalf("amounts: %+v", preview)
	}
	if preview.AccountID != salesID(110) || preview.SupplierAccountID != salesID(111) || preview.PaymentArticleID != salesID(120) || preview.CashID != "" {
		t.Fatalf("accounts: %+v", preview)
	}
	if preview.Applied || len(fake.writes) != 0 || preview.Status != "partly_paid" || !preview.Payments.Complete || len(preview.Payments.Payments) != 3 {
		t.Fatalf("preview: %+v %v", preview, fake.writes)
	}
}

func TestPrepaymentApplySavesPlanAndRerunWritesNothing(t *testing.T) {
	fake := prepaymentFixture()
	svc := Service{OData: fake}
	input := bankPrepayment()
	input.Amount, input.Purpose, input.DepartmentID, input.Apply = "500", "Предоплата по счёту 12", salesID(230), true
	saved, err := svc.PlanInvoicePrepayment(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Applied || saved.Posted || saved.Status != "partly_paid" || saved.DataVersion != "v2" || saved.Comment != "Счёт поставщика\nНазначение платежа: Предоплата по счёту 12" {
		t.Fatalf("saved: %+v", saved)
	}
	if len(fake.writes) != 1 || !strings.HasPrefix(fake.writes[0], "PATCH "+paymentPlanResourceName) || len(fake.actions) != 0 {
		t.Fatalf("writes: %v %v", fake.writes, fake.actions)
	}
	body := fake.bodies[0]
	calendar := body["ПлатежныйКалендарь"].([]any)[0].(map[string]any)
	if body["ЗапланироватьОплату"] != true || body["ТипДенежныхСредств"] != paymentPlanMoneyType || body["БанковскийСчет_Key"] != salesID(110) || body["БанковскийСчетПоставщика_Key"] != salesID(111) || body["Подразделение_Key"] != salesID(230) || calendar["СуммаОплаты"] != json.Number("500") {
		t.Fatalf("body: %+v", body)
	}
	if _, ok := body["Касса_Key"]; ok {
		t.Fatalf("bank plan must not touch the cash desk: %+v", body)
	}
	input.ExpectedDataVersion = ""
	again, err := svc.PlanInvoicePrepayment(context.Background(), input)
	if err != nil || !again.AlreadyPlanned || again.Applied || len(fake.writes) != 1 {
		t.Fatalf("rerun must not write: %+v, %v, %v", again, err, fake.writes)
	}
	input.Amount = "600"
	if _, err := svc.PlanInvoicePrepayment(context.Background(), input); err == nil || !strings.Contains(err.Error(), "update payment-plan") {
		t.Fatalf("a different plan must be refused: %v", err)
	}
}

func TestPrepaymentFromCashDesk(t *testing.T) {
	fake := prepaymentFixture()
	svc := Service{OData: fake}
	input := PrepaymentRequest{InvoiceID: testInvoiceID, Source: "cash", CashID: salesID(220), PaymentDate: "2026-11-05", Amount: "100", Apply: true}
	saved, err := svc.PlanInvoicePrepayment(context.Background(), input)
	if err != nil || !saved.Applied || saved.CashID != salesID(220) || saved.AccountID != "" {
		t.Fatalf("cash: %+v, %v", saved, err)
	}
	body := fake.bodies[0]
	if body["ТипДенежныхСредств"] != cashMoneyType || body["Касса_Key"] != salesID(220) {
		t.Fatalf("body: %+v", body)
	}
	if _, ok := body["БанковскийСчет_Key"]; ok {
		t.Fatalf("cash plan must not touch bank accounts: %+v", body)
	}
	bads := []PrepaymentRequest{
		{InvoiceID: testInvoiceID, Source: "cash", CashID: salesID(221), PaymentDate: "2026-11-05"},
		{InvoiceID: testInvoiceID, Source: "cash", CashID: salesID(220), AccountID: salesID(110), PaymentDate: "2026-11-05"},
		{InvoiceID: testInvoiceID, Source: "bank", AccountID: salesID(110), CashID: salesID(220), PaymentDate: "2026-11-05"},
	}
	for _, bad := range bads {
		if _, err := (Service{OData: prepaymentFixture()}).PlanInvoicePrepayment(context.Background(), bad); err == nil {
			t.Fatalf("must refuse %+v", bad)
		}
	}
}

func TestPrepaymentRefusals(t *testing.T) {
	cases := map[string]struct {
		edit func(*planWriter, *PrepaymentRequest)
		want string
	}{
		"posted":  {func(f *planWriter, _ *PrepaymentRequest) { f.rows[paymentPlanResourceName][0]["Posted"] = true }, "unpost it"},
		"overpay": {func(_ *planWriter, in *PrepaymentRequest) { in.Amount = "920.01" }, "exceeds what is left to pay, 920"},
		"swapped": {func(_ *planWriter, in *PrepaymentRequest) { in.AccountID = salesID(111) }, "organization"},
		"incomplete": {func(f *planWriter, _ *PrepaymentRequest) {
			f.fail = map[string]bool{"Document_РасходИзКассы": true}
		}, "incomplete"},
		"version":   {func(_ *planWriter, in *PrepaymentRequest) { in.ExpectedDataVersion = "old" }, "changed since the preview"},
		"warehouse": {func(_ *planWriter, in *PrepaymentRequest) { in.DepartmentID = salesID(231) }, "active department"},
		"source":    {func(_ *planWriter, in *PrepaymentRequest) { in.Source = "" }, "source must be"},
		"lines differ": {func(f *planWriter, _ *PrepaymentRequest) {
			f.rows[paymentPlanResourceName][0]["СуммаДокумента"] = 1221
		}, "do not add up"},
	}
	for name, test := range cases {
		fake := prepaymentFixture()
		input := bankPrepayment()
		input.Apply = true
		test.edit(fake, &input)
		if _, err := (Service{OData: fake}).PlanInvoicePrepayment(context.Background(), input); err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("%s: got %v, want %q", name, err, test.want)
		}
		if len(fake.writes) != 0 {
			t.Fatalf("%s wrote %v", name, fake.writes)
		}
	}
}

func TestPrepaymentCommentKeepsOnePurposeLine(t *testing.T) {
	comment, err := prepaymentComment("Счёт\nНазначение платежа: старое", nil, "  новое \n назначение ")
	if err != nil || comment != "Счёт\nНазначение платежа: новое назначение" {
		t.Fatalf("%q %v", comment, err)
	}
	if _, err := prepaymentComment(strings.Repeat("я", 495), nil, "длинное"); err == nil {
		t.Fatal("too long comment must fail")
	}
}

func TestPaymentRequestStatusSeparatesPostingApprovalAndPayment(t *testing.T) {
	fake := prepaymentFixture()
	row := fake.rows[paymentPlanResourceName][0]
	row["Posted"], row["ЗапланироватьОплату"] = true, true
	row["ПлатежныйКалендарь"] = []map[string]any{{"LineNumber": "1", "ДатаОплаты": "2026-11-05T00:00:00", "ПроцентОплаты": 100, "СуммаОплаты": 1220, "СуммаНДСОплаты": 180.33}}
	fake.rows["AccumulationRegister_ПлатежныйКалендарь_RecordType"] = []map[string]any{
		{"Recorder": testInvoiceID, "Recorder_Type": supplierInvoiceType, "СчетНаОплату": testInvoiceID, "СчетНаОплату_Type": supplierInvoiceType, "Period": "2026-11-05T00:00:00", "Active": true, "СтатусУтвержденияПлатежа": "Утвержден", "Сумма": 1220, "СуммаОплаты": 0},
	}
	svc := Service{OData: fake}
	item, err := svc.GetSupplierPaymentPlan(context.Background(), testInvoiceID)
	if err != nil || item.Status != "partly_paid" || item.Payments == nil || item.Payments.Paid != "300" {
		t.Fatalf("partly paid: %+v, %v", item, err)
	}
	fake.rows["Document_РасходСоСчета"] = fake.rows["Document_РасходСоСчета"][1:]
	if item, err = svc.GetSupplierPaymentPlan(context.Background(), testInvoiceID); err != nil || item.Status != "approved_not_paid" {
		t.Fatalf("approved: %+v, %v", item, err)
	}
	fake.rows["AccumulationRegister_ПлатежныйКалендарь_RecordType"][0]["СтатусУтвержденияПлатежа"] = "НеУтвержден"
	if item, err = svc.GetSupplierPaymentPlan(context.Background(), testInvoiceID); err != nil || item.Status != "posted_not_approved" {
		t.Fatalf("not approved: %+v, %v", item, err)
	}
}
