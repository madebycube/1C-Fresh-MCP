package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

type transferWriter struct {
	*traceReader
	writes   []string
	failPost bool
}

func (w *transferWriter) Write(_ context.Context, method, resource string, body []byte, _ string) ([]byte, error) {
	w.writes = append(w.writes, method+" "+resource)
	if method != http.MethodPost || w.failPost {
		return nil, errors.New("OData connection failed")
	}
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	fields["Ref_Key"], fields["DataVersion"], fields["Posted"], fields["DeletionMark"] = salesID(510), "v1", false, false
	w.rows[transferResourceName] = append(w.rows[transferResourceName], fields)
	return json.Marshal(map[string]any{"Ref_Key": salesID(510)})
}

func transferFixture() *transferWriter {
	rows := receiptFixture().rows
	rows["Catalog_СтруктурныеЕдиницы"] = append(rows["Catalog_СтруктурныеЕдиницы"],
		map[string]any{"Ref_Key": salesID(67), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Склад", "Недействителен": false},
		map[string]any{"Ref_Key": salesID(68), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Склад", "Недействителен": true},
	)
	rows["Catalog_ХозяйственныеОперации"] = append(rows["Catalog_ХозяйственныеОперации"], map[string]any{"Ref_Key": salesID(77), "DeletionMark": false, "PredefinedDataName": "Перемещение"})
	rows["Catalog_ЕдиницыИзмерения"] = []map[string]any{
		{"Ref_Key": salesID(78), "DeletionMark": false, "Owner": salesID(64), "Owner_Type": productOwnerType, "Коэффициент": 12},
		{"Ref_Key": salesID(79), "DeletionMark": false, "Owner": salesID(66), "Owner_Type": productOwnerType, "Коэффициент": 6},
	}
	return &transferWriter{traceReader: &traceReader{rows: rows}}
}

func transferDraftInput() TransferDraft {
	return TransferDraft{
		OrganizationID: salesID(80), FromWarehouseID: salesID(62), ToWarehouseID: salesID(67), Date: "2026-10-05",
		Lines: []TransferLineInput{{ProductID: salesID(64), Quantity: "2.5"}, {ProductID: salesID(64), Quantity: "2", UnitID: salesID(78)}},
	}
}

func TestTransferDraftPreviewWritesNothing(t *testing.T) {
	fake := transferFixture()
	preview, err := Service{OData: fake}.CreateTransferDraft(context.Background(), transferDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(fake.writes) != 0 || preview.CurrencyID != salesID(61) || preview.Date != "2026-10-05T00:00:00" {
		t.Fatalf("preview: %+v %v", preview, fake.writes)
	}
	first, second := preview.Lines[0], preview.Lines[1]
	if first.UnitID != salesID(65) || first.UnitType != unitClassifierType || first.Quantity != "2.5" {
		t.Fatalf("first line: %+v", first)
	}
	if second.UnitType != productUnitType || second.BaseQuantity != "24" {
		t.Fatalf("packaging line: %+v", second)
	}
}

func TestTransferDraftRejectsInvalidInputWithoutWriting(t *testing.T) {
	for name, mutate := range map[string]func(*TransferDraft){
		"same warehouse":     func(d *TransferDraft) { d.ToWarehouseID = salesID(62) },
		"department":         func(d *TransferDraft) { d.ToWarehouseID = salesID(63) },
		"inactive warehouse": func(d *TransferDraft) { d.ToWarehouseID = salesID(68) },
		"zero organization":  func(d *TransferDraft) { d.OrganizationID = emptyGUID },
		"service":            func(d *TransferDraft) { d.Lines[0].ProductID = salesID(66) },
		"foreign unit":       func(d *TransferDraft) { d.Lines[0].UnitID = salesID(79) },
		"zero quantity":      func(d *TransferDraft) { d.Lines[0].Quantity = "0" },
		"precision":          func(d *TransferDraft) { d.Lines[0].Quantity = "1.0001" },
		"characteristic":     func(d *TransferDraft) { d.Lines[0].CharacteristicID = salesID(98) },
		"bad date":           func(d *TransferDraft) { d.Date = "05.10.2026" },
		"no lines":           func(d *TransferDraft) { d.Lines = nil },
	} {
		fake := transferFixture()
		input := transferDraftInput()
		input.Apply = true
		mutate(&input)
		if _, err := (Service{OData: fake}).CreateTransferDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
			t.Errorf("%s: %v %v", name, err, fake.writes)
		}
	}
}

func TestTransferDraftSavesUnpostedAndNeverRetries(t *testing.T) {
	fake := transferFixture()
	input := transferDraftInput()
	input.Apply = true
	created, err := Service{OData: fake}.CreateTransferDraft(context.Background(), input)
	if err != nil || !created.Applied || created.ID != salesID(510) || created.Posted || len(fake.writes) != 1 {
		t.Fatalf("create: %+v, %v, %v", created, err, fake.writes)
	}
	saved := fake.rows[transferResourceName][0]
	if saved["ВидОперации"] != "Перемещение" || saved["ХозяйственнаяОперация_Key"] != salesID(77) || saved["СтруктурнаяЕдиницаПолучатель_Key"] != salesID(67) || saved["ВалютаДокумента_Key"] != salesID(61) {
		t.Fatalf("saved: %+v", saved)
	}
	line := saved["Запасы"].([]any)[1].(map[string]any)
	if line["ЕдиницаИзмерения_Type"] != productUnitType || line["Количество"] != json.Number("2") || line["КлючСвязи"] != json.Number("2") {
		t.Fatalf("line: %+v", line)
	}
	fake = transferFixture()
	fake.failPost = true
	if _, err := (Service{OData: fake}).CreateTransferDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "before retrying") || len(fake.writes) != 1 {
		t.Fatalf("uncertain create: %v %v", err, fake.writes)
	}
}
