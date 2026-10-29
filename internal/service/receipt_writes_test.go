package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// receiptWriter stores written receipts so reads after a write see what 1C would return.
type receiptWriter struct {
	*traceReader
	writes   []string
	actions  []string
	failPost bool
}

func (w *receiptWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes = append(w.writes, method+" "+resource)
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	if method == http.MethodPost {
		if w.failPost {
			return nil, errors.New("OData connection failed")
		}
		fields["Ref_Key"], fields["DataVersion"], fields["Posted"], fields["DeletionMark"] = salesID(500), "v1", false, false
		w.rows[receiptResourceName] = append(w.rows[receiptResourceName], fields)
		return json.Marshal(map[string]any{"Ref_Key": salesID(500)})
	}
	for _, row := range w.rows[receiptResourceName] {
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

func (w *receiptWriter) Action(_ context.Context, resource, action string) ([]byte, error) {
	w.actions = append(w.actions, action)
	for _, row := range w.rows[receiptResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			row["Posted"] = action == "Post"
		}
	}
	return nil, nil
}

func receiptFixture() *receiptWriter {
	org, supplier, contract, currency, warehouse, department, product := salesID(80), salesID(50), salesID(60), salesID(61), salesID(62), salesID(63), salesID(64)
	return &receiptWriter{traceReader: &traceReader{rows: map[string][]map[string]any{
		"Catalog_Организации":          {{"Ref_Key": org, "DeletionMark": false}},
		"Catalog_Контрагенты":          {{"Ref_Key": supplier, "Description": "Example supplier", "IsFolder": false, "DeletionMark": false, "Поставщик": true, "Покупатель": false, "DataVersion": "c1"}},
		"Catalog_ДоговорыКонтрагентов": {{"Ref_Key": contract, "DeletionMark": false, "Owner": supplier, "Организация_Key": org, "ВалютаРасчетов_Key": currency, "Недействителен": false, "РасчетыВУсловныхЕдиницах": false}},
		"Catalog_Валюты":               {{"Ref_Key": currency, "DeletionMark": false, "Code": "643"}},
		"Catalog_СтруктурныеЕдиницы": {
			{"Ref_Key": warehouse, "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Склад", "Недействителен": false},
			{"Ref_Key": department, "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Подразделение", "Недействителен": false},
		},
		"Catalog_Номенклатура": {
			{"Ref_Key": product, "DeletionMark": false, "IsFolder": false, "ТипНоменклатуры": "Запас", "ЕдиницаИзмерения_Key": salesID(65)},
			{"Ref_Key": salesID(66), "DeletionMark": false, "IsFolder": false, "ТипНоменклатуры": "Услуга", "ЕдиницаИзмерения_Key": salesID(65)},
		},
		"Catalog_СтавкиНДС": {
			{"Ref_Key": salesID(70), "DeletionMark": false, "Ставка": 22, "НеОблагается": false, "Расчетная": false, "ПеречислениеСтавкаНДС": "НДС22"},
			{"Ref_Key": salesID(71), "DeletionMark": false, "Ставка": 22, "НеОблагается": false, "Расчетная": false, "ПеречислениеСтавкаНДС": ""},
			{"Ref_Key": salesID(72), "DeletionMark": false, "Ставка": 0, "НеОблагается": true, "Расчетная": false, "ПеречислениеСтавкаНДС": "БезНДС"},
			{"Ref_Key": salesID(73), "DeletionMark": false, "Ставка": 10, "НеОблагается": false, "Расчетная": false, "ПеречислениеСтавкаНДС": "НДС10"},
			{"Ref_Key": salesID(74), "DeletionMark": false, "Ставка": 10, "НеОблагается": false, "Расчетная": false, "ПеречислениеСтавкаНДС": "НДС10"},
			{"Ref_Key": salesID(75), "DeletionMark": false, "Ставка": 22, "НеОблагается": false, "Расчетная": true, "ПеречислениеСтавкаНДС": "НДС22_122"},
		},
		"Catalog_ХозяйственныеОперации": {{"Ref_Key": salesID(76), "DeletionMark": false, "PredefinedDataName": "ПоступлениеОтПоставщика"}},
		receiptResourceName: {
			{"Ref_Key": salesID(90), "Number": "OLD", "Date": "2026-09-01T00:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": org, "Контрагент_Key": supplier, "НомерВходящегоДокумента": " ВХ-1 ", "ДатаВходящегоДокумента": "2026-09-01T00:00:00"},
		},
	}}}
}

func receiptDraftInput() ReceiptDraft {
	return ReceiptDraft{
		OrganizationID: salesID(80), SupplierID: salesID(50), ContractID: salesID(60), WarehouseID: salesID(62), DepartmentID: salesID(63),
		Date: "2026-10-01", IncomingNumber: "ВХ-2", IncomingDate: "2026-09-30", VATMode: receiptVATIncluded,
		Lines: []ReceiptLineInput{{ProductID: salesID(64), Quantity: "3", Price: "33.35", VATRate: "НДС22"}, {ProductID: salesID(64), Quantity: "0.125", Price: "8", VATRate: "БезНДС"}},
	}
}

func TestReceiptDraftPreviewComputesExactAmountsWithoutWriting(t *testing.T) {
	fake := receiptFixture()
	preview, err := Service{OData: fake}.CreateReceiptDraft(context.Background(), receiptDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	// 3 × 33.35 = 100.05 including VAT; 100.05 × 22 / 122 = 18.0418… → 18.04.
	first, second := preview.Lines[0], preview.Lines[1]
	if first.Amount != "100.05" || first.VAT != "18.04" || first.Total != "100.05" || first.VATRateID != salesID(70) || first.UnitID != salesID(65) {
		t.Fatalf("first line: %+v", first)
	}
	if second.Amount != "1" || second.VAT != "0" || preview.Amount != "101.05" || preview.VAT != "18.04" || preview.Taxation != receiptTaxed {
		t.Fatalf("totals: %+v", preview)
	}
	if preview.Applied || len(fake.writes) != 0 || len(preview.Duplicates) != 0 {
		t.Fatalf("preview wrote or flagged: %+v %v", preview, fake.writes)
	}
	input := receiptDraftInput()
	input.VATMode = receiptVATExcluded
	preview, err = Service{OData: fake}.CreateReceiptDraft(context.Background(), input)
	if err != nil || preview.Lines[0].VAT != "22.01" || preview.Lines[0].Total != "122.06" || preview.Amount != "123.06" {
		t.Fatalf("exclusive VAT: %+v, %v", preview.Lines, err)
	}
}

func TestReceiptDraftRejectsInvalidReferencesAndValues(t *testing.T) {
	svc := Service{OData: receiptFixture()}
	for name, mutate := range map[string]func(*ReceiptDraft){
		"service product":   func(d *ReceiptDraft) { d.Lines[0].ProductID = salesID(66) },
		"ambiguous rate":    func(d *ReceiptDraft) { d.Lines[0].VATRate = "НДС10" },
		"calculated rate":   func(d *ReceiptDraft) { d.Lines[0].VATRate = "НДС22_122" },
		"rate by percent":   func(d *ReceiptDraft) { d.Lines[0].VATRate = "22%" },
		"price precision":   func(d *ReceiptDraft) { d.Lines[0].Price = "1.005" },
		"exponent":          func(d *ReceiptDraft) { d.Lines[0].Quantity = "1e3" },
		"zero quantity":     func(d *ReceiptDraft) { d.Lines[0].Quantity = "0" },
		"warehouse as dept": func(d *ReceiptDraft) { d.DepartmentID = salesID(62) },
		"other supplier":    func(d *ReceiptDraft) { d.SupplierID = salesID(99) },
		"vat mode":          func(d *ReceiptDraft) { d.VATMode = "" },
		"bad date":          func(d *ReceiptDraft) { d.Date = "01.10.2026" },
		"no lines":          func(d *ReceiptDraft) { d.Lines = nil },
		"characteristic":    func(d *ReceiptDraft) { d.Lines[0].CharacteristicID = salesID(98) },
	} {
		input := receiptDraftInput()
		mutate(&input)
		if _, err := svc.CreateReceiptDraft(context.Background(), input); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestReceiptDraftBlocksDuplicatesAndNeverRetriesUncertainCreate(t *testing.T) {
	fake := receiptFixture()
	input := receiptDraftInput()
	input.IncomingNumber, input.IncomingDate, input.Apply = "вх-1", "2026-09-01", true
	if _, err := (Service{OData: fake}).CreateReceiptDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
		t.Fatalf("duplicate created: %v %v", err, fake.writes)
	}
	input = receiptDraftInput()
	input.Apply = true
	fake.failPost = true
	if _, err := (Service{OData: fake}).CreateReceiptDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "before retrying") || len(fake.writes) != 1 {
		t.Fatalf("uncertain create: %v %v", err, fake.writes)
	}
}

func TestReceiptLifecycleCreateAmendPost(t *testing.T) {
	fake := receiptFixture()
	svc := Service{OData: fake}
	input := receiptDraftInput()
	input.Apply = true
	created, err := svc.CreateReceiptDraft(context.Background(), input)
	if err != nil || !created.Applied || created.ID != salesID(500) || created.Posted {
		t.Fatalf("create: %+v, %v", created, err)
	}
	saved := fake.rows[receiptResourceName][1]
	if saved["ВидОперации"] != receiptOperation || saved["ХозяйственнаяОперация_Key"] != salesID(76) || saved["Posted"] != false || saved["СуммаДокумента"] != json.Number("101.05") {
		t.Fatalf("saved: %+v", saved)
	}
	// 1C adds fields of its own to lines; an amendment must send them back unchanged.
	lines := saved["Запасы"].([]any)
	for _, line := range lines {
		line.(map[string]any)["ИдентификаторСтроки"] = "keep-me"
	}
	quantity := "4"
	note := "corrected"
	preview, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{Comment: &note, ChangeLines: []ReceiptLineChange{{LineNumber: 1, Quantity: &quantity}}})
	if err != nil || preview.Applied || preview.Before.Amount != "101.05" || preview.Amount != "134.4" || len(preview.Changes) != 2 {
		t.Fatalf("amend preview: %+v, %v", preview, err)
	}
	amended, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{Comment: &note, ChangeLines: []ReceiptLineChange{{LineNumber: 1, Quantity: &quantity}}, ExpectedDataVersion: preview.DataVersion, Apply: true})
	if err != nil || !amended.Applied || amended.DataVersion != "v2" {
		t.Fatalf("amend: %+v, %v", amended, err)
	}
	for _, line := range saved["Запасы"].([]any) {
		if line.(map[string]any)["ИдентификаторСтроки"] != "keep-me" {
			t.Fatalf("line identity lost: %+v", line)
		}
	}
	if _, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{Comment: &note, ExpectedDataVersion: "v1", Apply: true}); err == nil {
		t.Fatal("stale data version accepted")
	}
	posting, err := svc.SetReceiptPosting(context.Background(), created.ID, "post", "", false)
	if err != nil || posting.Applied || len(fake.actions) != 0 || len(posting.Impact) == 0 {
		t.Fatalf("post preview: %+v, %v", posting, err)
	}
	posting, err = svc.SetReceiptPosting(context.Background(), created.ID, "post", "v2", true)
	if err != nil || !posting.Applied || !posting.Posted || fake.actions[0] != "Post" {
		t.Fatalf("post: %+v, %v", posting, err)
	}
	if _, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{Comment: &note, Apply: true}); err == nil || !strings.Contains(err.Error(), "unpost it explicitly") {
		t.Fatalf("posted receipt amended: %v", err)
	}
	if _, err := svc.SetReceiptPosting(context.Background(), salesID(90), "unpost", "", true); err == nil {
		t.Fatal("receipt without data version unposted")
	}
}

func TestAmendReceiptChangesVATModeAndRatesKeepingAmounts(t *testing.T) {
	fake := receiptFixture()
	svc := Service{OData: fake}
	input := receiptDraftInput()
	input.Apply = true
	created, err := svc.CreateReceiptDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	exclusive := receiptVATExcluded
	preview, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{VATMode: &exclusive})
	if err != nil || preview.Applied || preview.VATMode != receiptVATExcluded {
		t.Fatalf("mode preview: %+v, %v", preview, err)
	}
	// 100.05 × 22 / 100 = 22.011 → 22.01 on top of the unchanged amount.
	if first := preview.Lines[0]; first.Amount != "100.05" || first.VAT != "22.01" || first.Total != "122.06" || preview.Amount != "123.06" || len(fake.writes) != 1 {
		t.Fatalf("mode lines: %+v writes=%v", preview, fake.writes)
	}
	rate := "НДС22"
	preview, err = svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{VATRate: &rate})
	// Only the untaxed line changes: 1 × 22 / 122 = 0.1803… → 0.18.
	if err != nil || preview.Lines[0].VAT != "18.04" || preview.Lines[1].VAT != "0.18" || preview.Lines[1].VATRateID != salesID(70) || preview.Amount != "101.05" || preview.VAT != "18.22" || len(preview.Changes) != 1 || preview.Taxation != receiptTaxed {
		t.Fatalf("rate preview: %+v, %v", preview, err)
	}
	// НДС10 has two active copies in 1C, so it is ambiguous and refused.
	for _, bad := range []string{"НДС99", "НДС10", ""} {
		if _, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{VATRate: &bad, Apply: true}); err == nil || len(fake.writes) != 1 {
			t.Fatalf("accepted VAT rate %q: %v", bad, err)
		}
	}
	wrongMode := "gross"
	if _, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{VATMode: &wrongMode, Apply: true}); err == nil || len(fake.writes) != 1 {
		t.Fatalf("accepted VAT mode %q: %v", wrongMode, err)
	}
	amended, err := svc.AmendReceipt(context.Background(), created.ID, ReceiptAmendment{VATMode: &exclusive, ExpectedDataVersion: "v1", Apply: true})
	if err != nil || !amended.Applied {
		t.Fatalf("mode apply: %+v, %v", amended, err)
	}
	saved := fake.rows[receiptResourceName][1]
	if saved["СуммаВключаетНДС"] != false || saved["СуммаДокумента"] != json.Number("123.06") || saved["Запасы"].([]any)[0].(map[string]any)["СуммаНДС"] != json.Number("22.01") {
		t.Fatalf("saved: %+v", saved)
	}
}

type productVATWriter struct {
	*traceReader
	body    map[string]any
	ifMatch string
	writes  int
}

func (w *productVATWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes++
	w.ifMatch = ifMatch
	if method != http.MethodPatch || resource != nomenclatureResource(salesID(64)) {
		return nil, errors.New("unexpected write")
	}
	return nil, json.Unmarshal(body, &w.body)
}

func TestUpdateProductSetsVATRateByCode(t *testing.T) {
	fake := &productVATWriter{traceReader: receiptFixture().traceReader}
	product := fake.rows["Catalog_Номенклатура"][0]
	product["DataVersion"], product["СтавкаНДС_Key"] = "p1", salesID(72)
	svc := Service{OData: fake}
	rate := "НДС22"
	change, err := svc.UpdateProduct(context.Background(), salesID(64), ProductPatch{VATRate: &rate})
	if err != nil || !change.Applied || change.VATRate != "НДС22" || change.VATRateID != salesID(70) || fake.ifMatch != "p1" || len(fake.body) != 1 || fake.body["СтавкаНДС_Key"] != salesID(70) {
		t.Fatalf("change=%+v body=%+v err=%v", change, fake.body, err)
	}
	same := "БезНДС"
	if change, err := svc.UpdateProduct(context.Background(), salesID(64), ProductPatch{VATRate: &same}); err != nil || change.Applied || fake.writes != 1 {
		t.Fatalf("unchanged rate wrote: %+v %v", change, err)
	}
	// НДС10 has two active copies in 1C, so it is ambiguous and refused.
	for _, bad := range []string{"НДС10", "22%", "НДС22_122"} {
		if _, err := svc.UpdateProduct(context.Background(), salesID(64), ProductPatch{VATRate: &bad}); err == nil || fake.writes != 1 {
			t.Fatalf("accepted VAT rate %q: %v", bad, err)
		}
	}
}
