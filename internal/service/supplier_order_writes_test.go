package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// docWriter stores written supplier orders and receipts so reads after a write see what 1C would return.
type docWriter struct {
	*traceReader
	writes  []string
	bodies  []map[string]any
	actions []string
	created int
}

func (w *docWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes = append(w.writes, method+" "+resource)
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	w.bodies = append(w.bodies, fields)
	name := strings.Split(resource, "(")[0]
	if method == http.MethodPost {
		w.created++
		id := salesID(500 + w.created)
		fields["Ref_Key"], fields["Number"], fields["DataVersion"], fields["Posted"], fields["DeletionMark"] = id, "N"+id[len(id)-3:], "v1", false, false
		w.rows[name] = append(w.rows[name], fields)
		return json.Marshal(map[string]any{"Ref_Key": id})
	}
	for _, row := range w.rows[name] {
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

func (w *docWriter) Action(_ context.Context, resource, action string) ([]byte, error) {
	w.actions = append(w.actions, action+" "+strings.Split(resource, "(")[0])
	for _, row := range w.rows[strings.Split(resource, "(")[0]] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			row["Posted"] = action == "Post"
			row["DataVersion"] = "v3"
		}
	}
	return nil, nil
}

var orderProducts = []string{salesID(64), salesID(67), salesID(68), salesID(69), salesID(77)}

func orderFixture() *docWriter {
	base := receiptFixture()
	rows := base.rows
	for _, product := range orderProducts[1:] {
		rows["Catalog_Номенклатура"] = append(rows["Catalog_Номенклатура"], map[string]any{"Ref_Key": product, "DeletionMark": false, "IsFolder": false, "ТипНоменклатуры": "Запас", "ЕдиницаИзмерения_Key": salesID(65)})
	}
	rows[supplierOrderResourceName] = []map[string]any{{
		"Ref_Key": salesID(200), "Number": "Z1", "Date": "2026-08-01T00:00:00", "Posted": true, "DeletionMark": false, "DataVersion": "t1",
		"ВидОперации": supplierOrderPurchase, "ХозяйственнаяОперация_Key": salesID(201), "СостояниеЗаказа_Key": salesID(202), "НДСВключатьВСтоимость": true,
		"Организация_Key": salesID(80), "Контрагент_Key": salesID(51), "СуммаДокумента": 10,
	}}
	rows[bankAccountsResourceName] = []map[string]any{
		{"Ref_Key": salesID(110), "DeletionMark": false, "Owner": salesID(80), "Owner_Type": organizationOwnerType, "ВалютаДенежныхСредств_Key": salesID(61)},
		{"Ref_Key": salesID(111), "DeletionMark": false, "Owner": salesID(50), "Owner_Type": counterpartyOwnerType, "ВалютаДенежныхСредств_Key": salesID(61)},
	}
	rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"] = []map[string]any{}
	rows["Document_РасходСоСчета"] = []map[string]any{}
	rows["Document_РасходИзКассы"] = []map[string]any{}
	return &docWriter{traceReader: base.traceReader}
}

func orderDraftInput() SupplierOrderDraft {
	lines := make([]SupplierOrderLineInput, 0, len(orderProducts))
	for _, product := range orderProducts {
		lines = append(lines, SupplierOrderLineInput{ProductID: product, Quantity: "10", Price: "12.50", VATRate: "НДС22"})
	}
	lines[0].DiscountPercent = "10"
	return SupplierOrderDraft{
		OrganizationID: salesID(80), SupplierID: salesID(50), ContractID: salesID(60), WarehouseID: salesID(62), DepartmentID: salesID(63),
		Date: "2026-10-01", InvoiceNumber: "СЧ-15", InvoiceDate: "2026-09-30", DeliveryDate: "2026-10-10", VATMode: receiptVATIncluded, Lines: lines,
		Payments: []SupplierOrderPaymentInput{{Date: "2026-10-02", Percent: "30"}, {Date: "2026-10-12", Percent: "70"}}, BankAccountID: salesID(110),
	}
}

func TestSupplierOrderPreviewComputesDiscountsScheduleAndWritesNothing(t *testing.T) {
	fake := orderFixture()
	preview, err := Service{OData: fake}.CreateSupplierOrderDraft(context.Background(), orderDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	// 10 × 12.50 = 125, less 10 % = 112.50; VAT 22/122 = 20.29. Others: 125 each, VAT 22.54.
	first := preview.Lines[0]
	if first.Discount != "12.5" || first.Amount != "112.5" || first.VAT != "20.29" || first.Total != "112.5" {
		t.Fatalf("discount line: %+v", first)
	}
	if preview.Amount != "612.5" || len(preview.Payments) != 2 || preview.Payments[0].Amount != "183.75" || preview.Payments[1].Amount != "428.75" {
		t.Fatalf("totals or schedule: %s %+v", preview.Amount, preview.Payments)
	}
	if preview.StateID != salesID(202) || preview.StateSource == "" || preview.BankAccountID != salesID(110) || len(fake.writes) != 0 || preview.Applied {
		t.Fatalf("preview: %+v %v", preview, fake.writes)
	}
}

func TestSupplierOrderRejectsBadInputWithoutWriting(t *testing.T) {
	for name, mutate := range map[string]func(*SupplierOrderDraft){
		"supplier account as ours": func(d *SupplierOrderDraft) { d.BankAccountID = salesID(111) },
		"schedule short":           func(d *SupplierOrderDraft) { d.Payments[1].Percent = "60" },
		"percent and amount":       func(d *SupplierOrderDraft) { d.Payments[0].Amount = "1" },
		"discount over 100":        func(d *SupplierOrderDraft) { d.Lines[0].DiscountPercent = "101" },
		"service product":          func(d *SupplierOrderDraft) { d.Lines[0].ProductID = salesID(66) },
		"delivery before order":    func(d *SupplierOrderDraft) { d.DeliveryDate = "2026-09-01" },
		"no lines":                 func(d *SupplierOrderDraft) { d.Lines = nil },
		"cash box on bank":         func(d *SupplierOrderDraft) { d.CashBoxID = salesID(5) },
	} {
		fake := orderFixture()
		input := orderDraftInput()
		mutate(&input)
		input.Apply = true
		if _, err := (Service{OData: fake}).CreateSupplierOrderDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
			t.Fatalf("%s: err %v writes %v", name, err, fake.writes)
		}
	}
	fake := orderFixture()
	fake.rows[supplierOrderResourceName][0]["ВидОперации"] = "ЗаказНаПереработку"
	input := orderDraftInput()
	input.Apply = true
	if _, err := (Service{OData: fake}).CreateSupplierOrderDraft(context.Background(), input); err == nil || !isUnsupported(err) || len(fake.writes) != 0 {
		t.Fatalf("no template: %v %v", err, fake.writes)
	}
}

// One order with five lines receives four, keeps the fifth expected, and never duplicates on retry.
func TestSupplierOrderPartialReceiptAndPaymentsFlow(t *testing.T) {
	ctx := context.Background()
	fake := orderFixture()
	svc := Service{OData: fake}
	input := orderDraftInput()
	input.Apply = true
	created, err := svc.CreateSupplierOrderDraft(ctx, input)
	if err != nil || !created.Applied || created.Posted || created.Number == "" {
		t.Fatalf("create: %+v %v", created, err)
	}
	body := fake.bodies[0]
	if body["ВидОперации"] != supplierOrderPurchase || body["ХозяйственнаяОперация_Key"] != salesID(201) || body["СтруктурнаяЕдиницаРезерв_Key"] != salesID(62) || body["СтруктурнаяЕдиница_Key"] != salesID(63) || body["ЗапланироватьОплату"] != true || body["Posted"] != false {
		t.Fatalf("order body: %+v", body)
	}
	if _, err := svc.CreateSupplierOrderDraft(ctx, input); err == nil || len(fake.writes) != 1 {
		t.Fatalf("retry created a duplicate: %v %v", err, fake.writes)
	}
	if _, err := svc.CreateReceiptFromOrder(ctx, SupplierOrderReceiptDraft{OrderID: created.ID, Date: "2026-10-05", AllRemaining: true, Apply: true}); err == nil || len(fake.writes) != 1 {
		t.Fatalf("receipt from unposted order: %v", err)
	}
	if posting, err := svc.SetSupplierOrderPosting(ctx, created.ID, "post", "", true); err != nil || !posting.Posted {
		t.Fatalf("post: %+v %v", posting, err)
	}
	receiptInput := SupplierOrderReceiptDraft{OrderID: created.ID, Date: "2026-10-05", IncomingNumber: "УПД-1", Apply: true, Lines: []OrderReceiptLine{
		{LineNumber: 1, Quantity: "4"}, {LineNumber: 2, Quantity: "10"}, {LineNumber: 3, Quantity: "10"}, {LineNumber: 4, Quantity: "10"},
	}}
	receipt, err := svc.CreateReceiptFromOrder(ctx, receiptInput)
	if err != nil || !receipt.Applied || receipt.OrderID != created.ID || len(receipt.Lines) != 4 {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}
	// 4 of 10 at a discounted 112.50 is exactly 45.
	if receipt.Lines[0].Amount != "45" || receipt.Lines[0].Price != "11.25" || receipt.Amount != "420" {
		t.Fatalf("receipt amounts: %+v %s", receipt.Lines[0], receipt.Amount)
	}
	saved := fake.bodies[len(fake.bodies)-1]
	if saved["Заказ"] != created.ID || saved["Заказ_Type"] != supplierOrderType || saved["Запасы"].([]any)[0].(map[string]any)["Заказ"] != created.ID {
		t.Fatalf("receipt not linked: %+v", saved)
	}
	writes := len(fake.writes)
	receiptInput.IncomingNumber = "УПД-1-retry"
	if _, err := svc.CreateReceiptFromOrder(ctx, receiptInput); err == nil || !strings.Contains(err.Error(), "still expected") || len(fake.writes) != writes {
		t.Fatalf("retry received twice: %v", err)
	}
	status, err := svc.GetSupplierOrderStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Lines[0].Pending != "4" || status.Lines[0].Expected != "6" || status.Lines[4].Expected != "10" || status.Lines[1].Received != "0" || status.DeliveryStatus != "awaiting" {
		t.Fatalf("pending status: %+v %s", status.Lines, status.DeliveryStatus)
	}
	if _, err := svc.SetReceiptPosting(ctx, receipt.ID, "post", "", true); err != nil {
		t.Fatal(err)
	}
	register := fake.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"]
	movement := func(recorder, kind, record string, amount float64) map[string]any {
		return map[string]any{"Recorder": recorder, "Recorder_Type": "StandardODATA.Document_" + kind, "LineNumber": "1", "Active": true, "RecordType": record, "Заказ_Key": created.ID, "Сумма": amount, "СуммаВал": amount}
	}
	fake.rows["AccumulationRegister_РасчетыСПоставщиками_RecordType"] = append(register,
		movement(salesID(301), "РасходСоСчета", "Expense", 183.75),
		movement(salesID(302), "РасходИзКассы", "Expense", 100),
		movement(salesID(303), "ПоступлениеНаСчет", "Receipt", 50),
		movement(receipt.ID, "ПриходнаяНакладная", "Receipt", 420),
		map[string]any{"Recorder": salesID(304), "Recorder_Type": "StandardODATA.Document_РасходСоСчета", "LineNumber": "1", "Active": false, "RecordType": "Expense", "Заказ_Key": created.ID, "СуммаВал": 999},
	)
	fake.rows["Document_РасходСоСчета"] = []map[string]any{
		{"Ref_Key": salesID(301), "Number": "B1", "Date": "2026-10-02T10:00:00", "Posted": true, "DeletionMark": false, "Контрагент_Key": salesID(50), "РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Заказ": created.ID, "Заказ_Type": supplierOrderType, "СуммаРасчетов": 183.75}}},
		{"Ref_Key": salesID(305), "Number": "B2", "Date": "2026-10-03T10:00:00", "Posted": false, "DeletionMark": false, "Контрагент_Key": salesID(50), "РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Заказ": created.ID, "Заказ_Type": supplierOrderType, "СуммаРасчетов": 500}}},
	}
	fake.rows["Document_РасходИзКассы"] = []map[string]any{
		{"Ref_Key": salesID(302), "Number": "C1", "Date": "2026-10-03T10:00:00", "Posted": true, "DeletionMark": false, "Контрагент_Key": salesID(50), "РасшифровкаПлатежа": []map[string]any{{"LineNumber": "1", "Заказ": created.ID, "Заказ_Type": supplierOrderType, "СуммаРасчетов": 100}}},
	}
	status, err = svc.GetSupplierOrderStatus(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Lines[0].Received != "4" || status.Lines[0].Expected != "6" || status.Lines[3].Expected != "0" || status.Lines[4].Expected != "10" || status.DeliveryStatus != "partly_received" || len(status.Receipts) != 1 {
		t.Fatalf("received status: %+v %s", status.Lines, status.DeliveryStatus)
	}
	payments := status.Payments
	if payments.Paid != "283.75" || payments.Returned != "50" || payments.Outstanding != "378.75" || payments.Status != "partly_paid" || len(payments.Documents) != 3 {
		t.Fatalf("payments: %+v", payments)
	}
	if payments.Documents[1].Counted {
		t.Fatalf("unposted payment counted: %+v", payments.Documents)
	}
	rest, err := svc.CreateReceiptFromOrder(ctx, SupplierOrderReceiptDraft{OrderID: created.ID, Date: "2026-10-09", AllRemaining: true})
	if err != nil || len(rest.Lines) != 2 || rest.Lines[0].Quantity != "6" || rest.Lines[1].Quantity != "10" {
		t.Fatalf("remaining preview: %+v %v", rest.Lines, err)
	}
}

func TestLinkExistingReceiptToOrder(t *testing.T) {
	ctx := context.Background()
	fake := orderFixture()
	svc := Service{OData: fake}
	input := orderDraftInput()
	input.Apply = true
	order, err := svc.CreateSupplierOrderDraft(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetSupplierOrderPosting(ctx, order.ID, "post", "", true); err != nil {
		t.Fatal(err)
	}
	receiptRow := func(id, product string, quantity int) map[string]any {
		return map[string]any{
			"Ref_Key": id, "Number": "R" + id[len(id)-2:], "Date": "2026-10-04T00:00:00", "DataVersion": "r1", "Posted": true, "DeletionMark": false, "ВидОперации": receiptOperation,
			"Организация_Key": salesID(80), "Контрагент_Key": salesID(50), "Договор_Key": salesID(60), "СтруктурнаяЕдиница_Key": salesID(62), "Подразделение_Key": salesID(63),
			"СуммаДокумента": 12.5 * float64(quantity), "СуммаВключаетНДС": true, "Заказ": emptyGUID, "Заказ_Type": "StandardODATA.Undefined",
			"Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": product, "Характеристика_Key": emptyGUID, "ЕдиницаИзмерения": salesID(65), "ЕдиницаИзмерения_Type": unitClassifierType, "Количество": quantity, "Цена": 12.5, "Сумма": 12.5 * float64(quantity), "СтавкаНДС_Key": salesID(70), "СуммаНДС": 1, "Всего": 12.5 * float64(quantity), "Комментарий": "kept"}},
		}
	}
	fake.rows[receiptResourceName] = append(fake.rows[receiptResourceName], receiptRow(salesID(91), salesID(77), 3), receiptRow(salesID(92), salesID(66), 1), receiptRow(salesID(93), salesID(77), 11))
	writes := len(fake.writes)
	for _, id := range []string{salesID(92), salesID(93)} {
		preview, err := svc.LinkReceiptToOrder(ctx, id, order.ID, ReceiptOrderLinkChange{})
		if err != nil || len(preview.Problems) == 0 {
			t.Fatalf("mismatch not reported for %s: %+v %v", id, preview, err)
		}
		if _, err := svc.LinkReceiptToOrder(ctx, id, order.ID, ReceiptOrderLinkChange{Repost: true, Apply: true}); err == nil || len(fake.writes) != writes {
			t.Fatalf("mismatched link written for %s: %v", id, err)
		}
	}
	if _, err := svc.LinkReceiptToOrder(ctx, salesID(91), order.ID, ReceiptOrderLinkChange{Apply: true}); err == nil || !strings.Contains(err.Error(), "repost") {
		t.Fatalf("posted receipt linked without repost: %v", err)
	}
	linked, err := svc.LinkReceiptToOrder(ctx, salesID(91), order.ID, ReceiptOrderLinkChange{Repost: true, Apply: true})
	if err != nil || !linked.Applied || !linked.Posted || linked.Lines[0].OrderLine != 5 {
		t.Fatalf("link: %+v %v", linked, err)
	}
	row := fake.rows[receiptResourceName][len(fake.rows[receiptResourceName])-3]
	line := row["Запасы"].([]any)[0].(map[string]any)
	if row["Заказ"] != order.ID || line["Заказ"] != order.ID || line["Комментарий"] != "kept" {
		t.Fatalf("linked receipt: %+v", row)
	}
	again, err := svc.LinkReceiptToOrder(ctx, salesID(91), order.ID, ReceiptOrderLinkChange{Repost: true, Apply: true})
	if err != nil || !again.Unchanged || again.Applied {
		t.Fatalf("relink: %+v %v", again, err)
	}
	status, err := svc.GetSupplierOrderStatus(ctx, order.ID)
	if err != nil || status.Lines[4].Received != "3" || status.Lines[4].Expected != "7" {
		t.Fatalf("status after link: %+v %v", status.Lines, err)
	}
}

func TestAmendSupplierOrderKeepsUntouchedFieldsAndChecksVersion(t *testing.T) {
	ctx := context.Background()
	fake := orderFixture()
	svc := Service{OData: fake}
	input := orderDraftInput()
	input.Payments, input.BankAccountID = nil, ""
	input.Apply = true
	order, err := svc.CreateSupplierOrderDraft(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	stored := fake.rows[supplierOrderResourceName][1]
	stored["Запасы"].([]any)[1].(map[string]any)["Содержание"] = "kept"
	quantity := "8"
	if _, err := svc.AmendSupplierOrder(ctx, order.ID, SupplierOrderAmendment{ChangeLines: []SupplierOrderLineChange{{LineNumber: 1, Quantity: &quantity}}, ExpectedDataVersion: "stale", Apply: true}); err == nil {
		t.Fatal("stale version accepted")
	}
	writes := len(fake.writes)
	changed, err := svc.AmendSupplierOrder(ctx, order.ID, SupplierOrderAmendment{ChangeLines: []SupplierOrderLineChange{{LineNumber: 1, Quantity: &quantity}}, Apply: true})
	if err != nil || !changed.Applied || len(fake.writes) != writes+1 {
		t.Fatalf("amend: %+v %v", changed, err)
	}
	// 8 × 12.50 = 100, less 10 % = 90.
	if changed.Lines[0].Amount != "90" || changed.Amount != "590" {
		t.Fatalf("amended totals: %+v %s", changed.Lines[0], changed.Amount)
	}
	body := fake.bodies[len(fake.bodies)-1]
	if _, ok := body["ПлатежныйКалендарь"]; ok {
		t.Fatalf("untouched schedule rewritten: %+v", body)
	}
	if body["Запасы"].([]any)[1].(map[string]any)["Содержание"] != "kept" {
		t.Fatal("untouched line field lost")
	}
	if _, err := svc.SetSupplierOrderPosting(ctx, order.ID, "post", "", true); err != nil {
		t.Fatal(err)
	}
	comment := "new"
	if _, err := svc.AmendSupplierOrder(ctx, order.ID, SupplierOrderAmendment{Comment: &comment, Apply: true}); err == nil {
		t.Fatal("posted order amended without repost")
	}
	reposted, err := svc.AmendSupplierOrder(ctx, order.ID, SupplierOrderAmendment{Comment: &comment, Repost: true, Apply: true})
	if err != nil || !reposted.Posted {
		t.Fatalf("repost: %+v %v", reposted, err)
	}
	if got := fake.actions[len(fake.actions)-2:]; got[0] != "Unpost "+supplierOrderResourceName || got[1] != "Post "+supplierOrderResourceName {
		t.Fatalf("actions: %v", fake.actions)
	}
}
