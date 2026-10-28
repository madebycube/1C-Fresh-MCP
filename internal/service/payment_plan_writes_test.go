package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// planWriter stores written payment plans so reads and traces after a write see what 1C would return.
type planWriter struct {
	*traceReader
	writes  []string
	bodies  []map[string]any
	actions []string
}

func (w *planWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes = append(w.writes, method+" "+resource)
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	w.bodies = append(w.bodies, fields)
	if method == http.MethodPost {
		id := salesID(500 + len(w.rows[paymentPlanResourceName]))
		fields["Ref_Key"], fields["Number"], fields["DataVersion"], fields["Posted"], fields["DeletionMark"] = id, "P"+id[len(id)-3:], "v1", false, false
		w.rows[paymentPlanResourceName] = append(w.rows[paymentPlanResourceName], fields)
		return json.Marshal(map[string]any{"Ref_Key": id})
	}
	for _, row := range w.rows[paymentPlanResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			if row["DataVersion"] != ifMatch {
				return nil, errors.New("OData record changed since it was read")
			}
			for key, value := range fields {
				row[key] = value
			}
			row["DataVersion"] = "v2"
		}
	}
	return nil, nil
}

func (w *planWriter) Action(_ context.Context, resource, action string) ([]byte, error) {
	w.actions = append(w.actions, action)
	for _, row := range w.rows[paymentPlanResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			row["Posted"] = action == "Post"
		}
	}
	return nil, nil
}

func planFixture() *planWriter {
	org, supplier, contract, currency, receipt := salesID(80), salesID(50), salesID(60), salesID(61), salesID(101)
	settlement := func(recorder, recorderType, line, kind string, amount float64) map[string]any {
		return map[string]any{"Recorder": recorder, "Recorder_Type": recorderType, "LineNumber": line, "Active": true, "RecordType": kind, "Документ": receipt, "Документ_Type": supplierReceiptType, "Сумма": amount, "СуммаВал": amount}
	}
	return &planWriter{traceReader: &traceReader{rows: map[string][]map[string]any{
		receiptResourceName: {{
			"Ref_Key": receipt, "Number": "R7", "Date": "2026-09-15T10:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": receiptOperation,
			"Организация_Key": org, "Контрагент_Key": supplier, "Договор_Key": contract, "ВалютаДокумента_Key": currency, "Подразделение_Key": salesID(63),
			"НомерВходящегоДокумента": "ВХ-7", "ДатаВходящегоДокумента": "2026-09-14T00:00:00", "СуммаВключаетНДС": true, "НалогообложениеНДС": receiptTaxed,
			"СуммаДокумента": 1220.10, "Заказ": emptyGUID, "Заказ_Type": "StandardODATA.Undefined",
			"Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": salesID(64), "Характеристика_Key": emptyGUID, "ЕдиницаИзмерения": salesID(65), "ЕдиницаИзмерения_Type": unitClassifierType,
				"Количество": 3, "Цена": 333.3667, "Сумма": 1000.10, "СтавкаНДС_Key": salesID(70), "СуммаНДС": 166.68, "Всего": 1000.10}},
			"Расходы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": salesID(66), "ЕдиницаИзмерения": salesID(65), "ЕдиницаИзмерения_Type": unitClassifierType,
				"Количество": 1, "Цена": 220, "Сумма": 220, "СтавкаНДС_Key": salesID(72), "СуммаНДС": 0, "Всего": 220}},
		}},
		paymentPlanResourceName:       {},
		"Document_ПлатежноеПоручение": {},
		"Document_РасходСоСчета":      {},
		"Document_РасходИзКассы":      {},
		"AccumulationRegister_РасчетыСПоставщиками_RecordType": {
			settlement(receipt, supplierReceiptType, "1", "Receipt", 1220.10),
			settlement(salesID(30), "StandardODATA.Document_РасходСоСчета", "1", "Expense", 220.10),
		},
		"AccumulationRegister_ПлатежныйКалендарь_RecordType": {},
		bankAccountsResourceName: {
			{"Ref_Key": salesID(110), "DeletionMark": false, "Owner": org, "Owner_Type": organizationOwnerType, "ВалютаДенежныхСредств_Key": currency, "ДатаЗакрытия": "0001-01-01T00:00:00"},
			{"Ref_Key": salesID(111), "DeletionMark": false, "Owner": supplier, "Owner_Type": counterpartyOwnerType, "ВалютаДенежныхСредств_Key": currency},
			{"Ref_Key": salesID(114), "DeletionMark": false, "Owner": org, "Owner_Type": organizationOwnerType, "ВалютаДенежныхСредств_Key": salesID(99)},
			{"Ref_Key": salesID(115), "DeletionMark": false, "Owner": org, "Owner_Type": organizationOwnerType, "ВалютаДенежныхСредств_Key": currency, "ДатаЗакрытия": "2026-09-01T00:00:00"},
			{"Ref_Key": salesID(112), "DeletionMark": true, "Owner": supplier, "Owner_Type": counterpartyOwnerType},
			{"Ref_Key": salesID(113), "DeletionMark": false, "Owner": salesID(81), "Owner_Type": organizationOwnerType},
		},
		"Catalog_ДоговорыКонтрагентов":  {{"Ref_Key": contract, "DeletionMark": false, "СтатьяДДСПоУмолчанию_Key": salesID(120)}},
		"Catalog_ХозяйственныеОперации": {{"Ref_Key": salesID(77), "DeletionMark": false, "PredefinedDataName": paymentPlanOperation}},
	}}}
}

func planDraftInput() PaymentPlanDraft {
	return PaymentPlanDraft{ReceiptID: salesID(101), AccountID: salesID(110), Date: "2026-10-01", PaymentDate: "2026-10-10"}
}

func TestPaymentPlanPreviewDefaultsToOutstandingWithoutWriting(t *testing.T) {
	fake := planFixture()
	preview, err := Service{OData: fake}.CreatePaymentPlanDraft(context.Background(), planDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	// Outstanding 1220.10 − 220.10 = 1000; VAT 166.68 × 1000 / 1220.10 = 136.61…; 1000 / 1220.10 = 81.96 %.
	if preview.Amount != "1000" || preview.VAT != "136.61" || preview.Percent != "81.96" || preview.DocumentAmount != "1220.1" || preview.Outstanding != "1000" {
		t.Fatalf("amounts: %+v", preview)
	}
	if preview.SupplierAccountID != salesID(111) || preview.PaymentArticleID != salesID(120) || preview.Comment != "Приходная накладная № R7 от 15.09.2026" || preview.IncomingNumber != "ВХ-7" || preview.IncomingDate != "2026-09-14" {
		t.Fatalf("references: %+v", preview)
	}
	if preview.Applied || len(fake.writes) != 0 || len(preview.Duplicates) != 0 || len(preview.Incomplete) != 0 || preview.ReceiptStatus != "partly_closed_in_settlements" {
		t.Fatalf("preview wrote or flagged: %+v %v", preview, fake.writes)
	}
}

func TestPaymentPlanCreateBlocksDuplicatesAndOverplanning(t *testing.T) {
	fake := planFixture()
	svc := Service{OData: fake}
	input := planDraftInput()
	input.Amount, input.Apply, input.Comment = "1000.01", true, "first half"
	if _, err := svc.CreatePaymentPlanDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "exceeds the outstanding 1000") {
		t.Fatalf("overplanning must fail: %v", err)
	}
	input.Amount = "400"
	created, err := svc.CreatePaymentPlanDraft(context.Background(), input)
	if err != nil || !created.Applied || created.Posted || created.Amount != "400" || created.Comment != "Приходная накладная № R7 от 15.09.2026. first half" {
		t.Fatalf("create: %+v, %v", created, err)
	}
	body := fake.bodies[0]
	calendar := body["ПлатежныйКалендарь"].([]any)[0].(map[string]any)
	if body["ЗапланироватьОплату"] != true || body["ДокументОснование_Key"] != emptyGUID || body["ТипДенежныхСредств"] != paymentPlanMoneyType || body["ХозяйственнаяОперация_Key"] != salesID(77) || len(body["Запасы"].([]any)) != 2 || calendar["СуммаОплаты"] != json.Number("400") || calendar["ДатаОплаты"] != "2026-10-10T00:00:00" {
		t.Fatalf("body: %+v", body)
	}
	trace, err := svc.TraceSupplierReceipt(context.Background(), salesID(101))
	if err != nil || len(trace.PaymentPlans) != 1 || trace.PaymentPlans[0].Link != "incoming-document" {
		t.Fatalf("trace must link the new plan: %+v, %v", trace.PaymentPlans, err)
	}
	input.Comment = ""
	if _, err := svc.CreatePaymentPlanDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("duplicate must be blocked: %v", err)
	}
	input.AllowDuplicate = true
	if second, err := svc.CreatePaymentPlanDraft(context.Background(), input); err != nil || !second.Applied || len(second.Duplicates) != 1 {
		t.Fatalf("explicit duplicate: %+v, %v", second, err)
	}
}

func TestPaymentPlanCreateRefusesPaidOrUnknownReceipts(t *testing.T) {
	fake := planFixture()
	register := "AccumulationRegister_РасчетыСПоставщиками_RecordType"
	fake.rows[register] = append(fake.rows[register], map[string]any{"Recorder": salesID(31), "Recorder_Type": "StandardODATA.Document_РасходИзКассы", "LineNumber": "1", "Active": true, "RecordType": "Expense", "Документ": salesID(101), "Документ_Type": supplierReceiptType, "СуммаВал": 1000})
	input := planDraftInput()
	input.Apply = true
	if _, err := (Service{OData: fake}).CreatePaymentPlanDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "closed_in_settlements") {
		t.Fatalf("paid receipt must be refused: %v", err)
	}
	fake = planFixture()
	fake.fail = map[string]bool{register: true}
	input.Amount = "10"
	preview, err := Service{OData: fake}.CreatePaymentPlanDraft(context.Background(), PaymentPlanDraft{ReceiptID: salesID(101), AccountID: salesID(110), Date: "2026-10-01", PaymentDate: "2026-10-10", Amount: "10"})
	if err != nil || len(preview.Incomplete) != 1 {
		t.Fatalf("preview must report incomplete evidence: %+v, %v", preview, err)
	}
	if _, err := (Service{OData: fake}).CreatePaymentPlanDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("incomplete evidence must block saving: %v", err)
	}
	if len(fake.writes) != 0 {
		t.Fatalf("refused creates wrote: %v", fake.writes)
	}
}

func TestPaymentPlanRejectsWrongAccountsAndInputs(t *testing.T) {
	svc := Service{OData: planFixture()}
	for name, mutate := range map[string]func(*PaymentPlanDraft){
		"no paying account":          func(d *PaymentPlanDraft) { d.AccountID = "" },
		"other organization account": func(d *PaymentPlanDraft) { d.AccountID = salesID(113) },
		"supplier account as payer":  func(d *PaymentPlanDraft) { d.AccountID = salesID(111) },
		"organization as recipient":  func(d *PaymentPlanDraft) { d.SupplierAccountID = salesID(110) },
		"deleted supplier account":   func(d *PaymentPlanDraft) { d.SupplierAccountID = salesID(112) },
		"amount precision":           func(d *PaymentPlanDraft) { d.Amount = "1.001" },
		"negative amount":            func(d *PaymentPlanDraft) { d.Amount = "-5" },
		"bad payment date":           func(d *PaymentPlanDraft) { d.PaymentDate = "10.10.2026" },
		"not a receipt":              func(d *PaymentPlanDraft) { d.ReceiptID = salesID(102) },
	} {
		input := planDraftInput()
		mutate(&input)
		if _, err := svc.CreatePaymentPlanDraft(context.Background(), input); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

func TestPaymentPlanLifecycleAmendPostAndUnpost(t *testing.T) {
	fake := planFixture()
	svc := Service{OData: fake}
	input := planDraftInput()
	input.Amount, input.Apply = "400", true
	created, err := svc.CreatePaymentPlanDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	amount, date := "500", "2026-10-12"
	if _, err := svc.AmendPaymentPlan(context.Background(), created.ID, PaymentPlanAmendment{Amount: &amount}); err == nil || !strings.Contains(err.Error(), "receipt_id") {
		t.Fatalf("amount change without receipt must fail: %v", err)
	}
	if _, err := svc.AmendPaymentPlan(context.Background(), created.ID, PaymentPlanAmendment{PaymentDate: &date, ExpectedDataVersion: "v0"}); err == nil {
		t.Fatal("stale preview must be rejected")
	}
	tooMuch := "1000.5"
	if _, err := svc.AmendPaymentPlan(context.Background(), created.ID, PaymentPlanAmendment{ReceiptID: salesID(101), Amount: &tooMuch}); err == nil {
		t.Fatal("amount above outstanding must be rejected")
	}
	amended, err := svc.AmendPaymentPlan(context.Background(), created.ID, PaymentPlanAmendment{ReceiptID: salesID(101), Amount: &amount, PaymentDate: &date, ExpectedDataVersion: "v1", Apply: true})
	if err != nil || !amended.Applied || amended.Amount != "500" || amended.VAT != "68.31" || amended.Before.Amount != "400" || strings.Join(amended.Changes, ",") != "payment date,amount" {
		t.Fatalf("amend: %+v, %v", amended, err)
	}
	patch := fake.bodies[len(fake.bodies)-1]
	if fake.writes[len(fake.writes)-1] != "PATCH "+planResource(created.ID) || patch["Запасы"] != nil || patch["БанковскийСчет_Key"] != salesID(110) {
		t.Fatalf("patch: %v %+v", fake.writes, patch)
	}
	posting, err := svc.SetPaymentPlanPosting(context.Background(), created.ID, "post", "", false)
	if err != nil || posting.Applied || len(fake.actions) != 0 || !strings.Contains(strings.Join(posting.Impact, " "), "does not approve") {
		t.Fatalf("post preview: %+v, %v", posting, err)
	}
	posting, err = svc.SetPaymentPlanPosting(context.Background(), created.ID, "post", "v2", true)
	if err != nil || !posting.Applied || !posting.Posted || posting.Calendar == nil || !posting.Calendar.Available || fake.actions[0] != "Post" {
		t.Fatalf("post: %+v, %v", posting, err)
	}
	if _, err := svc.AmendPaymentPlan(context.Background(), created.ID, PaymentPlanAmendment{PaymentDate: &date}); err == nil || !strings.Contains(err.Error(), "unpost") {
		t.Fatalf("posted plan must not be amended: %v", err)
	}
	if posting, err = svc.SetPaymentPlanPosting(context.Background(), created.ID, "unpost", "", true); err != nil || posting.Posted || fake.actions[1] != "Unpost" {
		t.Fatalf("unpost: %+v, %v", posting, err)
	}
}

func TestPaymentPlanRefusesAccountsInOtherCurrencyOrClosed(t *testing.T) {
	for account, want := range map[string]string{salesID(114): "currency differs", salesID(115): "is closed on 2026-09-01"} {
		fake := planFixture()
		input := planDraftInput()
		input.AccountID, input.Apply = account, true
		if _, err := (Service{OData: fake}).CreatePaymentPlanDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), want) || len(fake.writes) != 0 {
			t.Fatalf("account %s: %v, writes %v", account, err, fake.writes)
		}
	}
}
