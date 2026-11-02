package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const paymentResourceName = "Document_РасходСоСчета"

// paymentWriter stores payment changes like 1C would and passes other writes to the receipt stub.
type paymentWriter struct {
	*receiptWriter
	paymentWrites []string
}

func (w *paymentWriter) Write(ctx context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	if !strings.HasPrefix(resource, paymentResourceName) {
		return w.receiptWriter.Write(ctx, method, resource, body, ifMatch)
	}
	w.paymentWrites = append(w.paymentWrites, method+" "+resource)
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	row := w.rows[paymentResourceName][0]
	if method != http.MethodPatch || row["DataVersion"] != ifMatch {
		return nil, errors.New("OData record changed since it was read")
	}
	for key, value := range fields {
		row[key] = value
	}
	row["DataVersion"] = "p3"
	return nil, nil
}

func (w *paymentWriter) Action(ctx context.Context, resource, action string) ([]byte, error) {
	if !strings.HasPrefix(resource, paymentResourceName) {
		return w.receiptWriter.Action(ctx, resource, action)
	}
	w.actions = append(w.actions, action)
	row := w.rows[paymentResourceName][0]
	row["Posted"], row["DataVersion"] = action == "Post", "p2"
	return nil, nil
}

func paymentFixture(t *testing.T) (*paymentWriter, string) {
	t.Helper()
	fake := &paymentWriter{receiptWriter: receiptFixture()}
	input := receiptDraftInput()
	input.Apply = true
	receipt, err := Service{OData: fake}.CreateReceiptDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	fake.rows["Catalog_ДоговорыКонтрагентов"][0]["Description"] = "Основной договор"
	fake.rows["Catalog_ДоговорыКонтрагентов"] = append(fake.rows["Catalog_ДоговорыКонтрагентов"],
		map[string]any{"Ref_Key": salesID(160), "DeletionMark": false, "Owner": salesID(51), "Организация_Key": salesID(80), "ВалютаРасчетов_Key": salesID(61), "Недействителен": false, "РасчетыВУсловныхЕдиницах": false})
	fake.rows[paymentResourceName] = []map[string]any{{
		"Ref_Key": salesID(700), "Number": "P-1", "Date": "2026-10-05T00:00:00", "Posted": true, "DeletionMark": false, "DataVersion": "p1",
		"ВидОперации": "Поставщику", "Организация_Key": salesID(80), "Контрагент_Key": salesID(50), "ВалютаДенежныхСредств_Key": salesID(61),
		"СуммаДокумента": 101.05, "Комментарий": "paid\r\nin full", "БанковскийСчет_Key": salesID(82),
		"РасшифровкаПлатежа": []map[string]any{{
			"LineNumber": "1", "Договор_Key": salesID(60), "ПризнакАванса": true, "СуммаПлатежа": 101.05, "СуммаРасчетов": 101.05,
			"СтавкаНДС_Key": salesID(70), "СуммаНДС": 18.22, "Курс": 1, "Кратность": "1", "СтатьяДвиженияДенежныхСредств_Key": salesID(81),
			"Документ": "", "Документ_Type": "StandardODATA.Undefined",
		}},
	}}
	return fake, receipt.ID
}

func TestAmendSupplierPaymentReallocatesAndReposts(t *testing.T) {
	fake, receiptID := paymentFixture(t)
	svc := Service{OData: fake}
	amount, rest := "60", "41.05"
	change := PaymentAmendment{
		ChangeLines: []PaymentAllocationChange{{LineNumber: 1, DocumentID: &receiptID, Amount: &amount}},
		AddLines:    []PaymentAllocationInput{{ContractID: salesID(60), Amount: rest, VATRate: "НДС22"}},
	}
	if _, err := svc.AmendSupplierPayment(context.Background(), "bank-out", salesID(700), change); err == nil || !strings.Contains(err.Error(), "repost") {
		t.Fatalf("posted payment amended without repost: %v", err)
	}
	change.Repost = true
	preview, err := svc.AmendSupplierPayment(context.Background(), "bank-out", salesID(700), change)
	// 60 × 22 / 122 = 10.819… → 10.82; 41.05 × 22 / 122 = 7.402… → 7.40.
	if err != nil || preview.Applied || len(preview.Allocations) != 2 || preview.Allocations[0].VAT != "10.82" || preview.Allocations[0].Advance || !preview.Allocations[1].Advance || preview.Allocations[1].VAT != "7.4" {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if len(fake.paymentWrites) != 0 || len(fake.actions) != 0 {
		t.Fatalf("preview wrote: %v %v", fake.paymentWrites, fake.actions)
	}
	change.Apply, change.ExpectedDataVersion = true, "p1"
	saved, err := svc.AmendSupplierPayment(context.Background(), "bank-out", salesID(700), change)
	if err != nil || !saved.Applied || !saved.Posted || strings.Join(fake.actions, ",") != "Unpost,Post" || fake.paymentWrites[0] != "PATCH "+paymentResourceName+"(guid'"+salesID(700)+"')" {
		t.Fatalf("apply: %+v, %v, actions=%v writes=%v", saved, err, fake.actions, fake.paymentWrites)
	}
	row := fake.rows[paymentResourceName][0]
	lines := row["РасшифровкаПлатежа"].([]any)
	first, second := lines[0].(map[string]any), lines[1].(map[string]any)
	if row["Комментарий"] != "paid\r\nin full" || first["Документ"] != receiptID || first["Документ_Type"] != receiptDocumentType || first["ПризнакАванса"] != false || first["СтатьяДвиженияДенежныхСредств_Key"] != salesID(81) {
		t.Fatalf("first line: %+v", first)
	}
	if second["LineNumber"] != "2" || second["ПризнакАванса"] != true || second["СуммаРасчетов"] != json.Number("41.05") || second["СтатьяДвиженияДенежныхСредств_Key"] != salesID(81) || second["СтавкаНДС_Key"] != salesID(70) {
		t.Fatalf("second line: %+v", second)
	}
}

func TestAmendSupplierPaymentRefusesBadAllocations(t *testing.T) {
	fake, receiptID := paymentFixture(t)
	fake.rows[paymentResourceName][0]["Posted"] = false
	svc := Service{OData: fake}
	half, otherContract, otherReceipt, wrongAmount := "50", salesID(160), salesID(90), "120"
	for name, change := range map[string]PaymentAmendment{
		"lines short of amount":    {ChangeLines: []PaymentAllocationChange{{LineNumber: 1, Amount: &half}}},
		"other supplier contract":  {ChangeLines: []PaymentAllocationChange{{LineNumber: 1, ContractID: &otherContract}}},
		"not a goods receipt":      {ChangeLines: []PaymentAllocationChange{{LineNumber: 1, DocumentID: &otherReceipt}}},
		"amount without lines":     {Amount: &wrongAmount},
		"every line removed":       {RemoveLines: []int{1}},
		"missing line":             {RemoveLines: []int{2}},
		"unknown VAT rate":         {AddLines: []PaymentAllocationInput{{ContractID: salesID(60), Amount: "1", VATRate: "НДС99"}}},
		"advance without contract": {AddLines: []PaymentAllocationInput{{Amount: "1", VATRate: "НДС22"}}},
	} {
		change.Apply = true
		if _, err := svc.AmendSupplierPayment(context.Background(), "bank-out", salesID(700), change); err == nil || len(fake.paymentWrites) != 0 {
			t.Fatalf("%s: accepted or wrote: %v", name, err)
		}
	}
	if _, err := svc.AmendSupplierPayment(context.Background(), "bank-in", salesID(700), PaymentAmendment{}); err == nil {
		t.Fatal("incoming payment accepted")
	}
	// A new total is accepted when the lines add up to it.
	newAmount, lineAmount := "120", "120"
	preview, err := svc.AmendSupplierPayment(context.Background(), "bank-out", salesID(700), PaymentAmendment{Amount: &newAmount, ChangeLines: []PaymentAllocationChange{{LineNumber: 1, Amount: &lineAmount, DocumentID: &receiptID}}})
	if err != nil || preview.Amount != "120" || !strings.Contains(strings.Join(preview.Impact, " "), "bank statement") {
		t.Fatalf("new amount: %+v, %v", preview, err)
	}
}

func TestGetMoneyDocumentReturnsAllocations(t *testing.T) {
	fake, _ := paymentFixture(t)
	doc, err := Service{OData: fake}.GetMoneyDocument(context.Background(), "bank-out", salesID(700))
	if err != nil || doc.Comment != "paid\r\nin full" || len(doc.Allocations) != 1 {
		t.Fatalf("doc: %+v, %v", doc, err)
	}
	line := doc.Allocations[0]
	if line.ContractID != salesID(60) || line.ContractName != "Основной договор" || !line.Advance || line.PaymentAmount != "101.05" || line.VAT != "18.22" || line.VATRateID != salesID(70) || line.DocumentType != "" || line.CashFlowItemID != salesID(81) || line.Multiplicity != "1" {
		t.Fatalf("allocation: %+v", line)
	}
}
