package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func linkFixture() *receiptWriter {
	set, other := salesID(200), salesID(201)
	link, unrelated, plain := salesID(210), salesID(211), salesID(212)
	stringType := map[string]any{"Types": []string{"String"}, "StringQualifiers": map[string]any{"Length": "1024", "AllowedLength": "Variable"}}
	definition := func(id, description string, hyperlink bool, setID string) map[string]any {
		return map[string]any{"Ref_Key": id, "DeletionMark": false, "Description": description, "Заголовок": description, "Имя": "Name_" + id[len(id)-3:], "ValueType": stringType, "ВыводитьВВидеГиперссылки": hyperlink, "ЭтоДополнительноеСведение": false, "НаборСвойств_Key": setID}
	}
	return &receiptWriter{traceReader: &traceReader{rows: map[string][]map[string]any{
		propertyDefinitionsResource: {
			definition(link, "Первичный документ - ссылка", true, set),
			definition(unrelated, "Сайт", true, other),
			definition(plain, "Заметка", false, set),
			{"Ref_Key": salesID(213), "DeletionMark": true, "Description": "Старая ссылка", "ValueType": stringType, "ВыводитьВВидеГиперссылки": true, "НаборСвойств_Key": set},
		},
		propertySetsResource: {
			{"Ref_Key": set, "DeletionMark": false, "Description": "Приходные накладные", "ИмяПредопределенногоНабора": receiptPropertySet, "Используется": true, "ДополнительныеРеквизиты": []map[string]any{{"Свойство_Key": link, "ПометкаУдаления": false}, {"Свойство_Key": plain, "ПометкаУдаления": false}}},
			{"Ref_Key": other, "DeletionMark": false, "Description": "Контрагенты", "ИмяПредопределенногоНабора": "Справочник_Контрагенты", "Используется": true, "ДополнительныеРеквизиты": []map[string]any{{"Свойство_Key": unrelated, "ПометкаУдаления": false}}},
			{"Ref_Key": salesID(202), "DeletionMark": false, "Description": "(не используется) Приходная накладная", "ИмяПредопределенногоНабора": "", "Используется": false, "ДополнительныеРеквизиты": []map[string]any{{"Свойство_Key": unrelated, "ПометкаУдаления": false}}},
		},
		receiptResourceName: {
			{"Ref_Key": salesID(220), "Number": "R20", "Posted": true, "DeletionMark": false, "DataVersion": "v1", "ВидОперации": receiptOperation, "Комментарий": "keep me",
				"ДополнительныеРеквизиты": []map[string]any{
					{"LineNumber": "1", "Свойство_Key": plain, "Значение": "note", "Значение_Type": "Edm.String", "ТекстоваяСтрока": "note"},
					{"LineNumber": "2", "Свойство_Key": link, "Значение": "Накладная №1", "Значение_Type": "Edm.String", "ТекстоваяСтрока": `<a href = "https://drive.example.test/d/1">Накладная №1</a>`},
				}},
			{"Ref_Key": salesID(221), "Number": "R21", "Posted": false, "DeletionMark": false, "DataVersion": "v1", "ВидОперации": receiptOperation},
			{"Ref_Key": salesID(222), "Number": "RET", "Posted": true, "DeletionMark": false, "DataVersion": "v1", "ВидОперации": "ВозвратОтПокупателя"},
		},
	}}}
}

func TestListLinkPropertiesUsesTheReceiptSet(t *testing.T) {
	list, err := Service{OData: linkFixture()}.ListLinkProperties(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Properties) != 2 {
		t.Fatalf("want the two live hyperlink properties: %+v", list.Properties)
	}
	link, unrelated := list.Properties[0], list.Properties[1]
	if !link.ForReceipts || link.ReceiptsUsing != 1 || link.MaxLength != 1024 || link.OwnerSet != "Приходные накладные" {
		t.Fatalf("receipt property: %+v", link)
	}
	if unrelated.ForReceipts {
		t.Fatalf("a property only in a legacy or other set must not be offered: %+v", unrelated)
	}
}

func TestGetReceiptLinkSplitsURLAndCaption(t *testing.T) {
	svc := Service{OData: linkFixture()}
	link, err := svc.GetReceiptLink(context.Background(), salesID(220), "")
	if err != nil || link.URL != "https://drive.example.test/d/1" || link.Caption != "Накладная №1" || link.Format != "hyperlink" || !link.Posted {
		t.Fatalf("link: %+v, %v", link, err)
	}
	link, err = svc.GetReceiptLink(context.Background(), salesID(221), "Первичный документ - ссылка")
	if err != nil || link.Present || link.Format != "absent" {
		t.Fatalf("absent link: %+v, %v", link, err)
	}
	for name, selector := range map[string]string{"missing": "Нет такого", "other set": "Сайт", "not a hyperlink": "Заметка"} {
		if _, err := svc.GetReceiptLink(context.Background(), salesID(220), selector); err == nil {
			t.Fatalf("%s property must be refused", name)
		}
	}
	if _, err := svc.GetReceiptLink(context.Background(), salesID(222), ""); err == nil {
		t.Fatal("a customer return must be refused")
	}
}

func TestSetReceiptLinkRejectsAmbiguousAndInvalidInput(t *testing.T) {
	fake := linkFixture()
	definitions := fake.rows[propertyDefinitionsResource]
	twin := map[string]any{}
	for key, value := range definitions[0] {
		twin[key] = value
	}
	twin["Ref_Key"] = salesID(214)
	fake.rows[propertyDefinitionsResource] = append(definitions, twin)
	svc := Service{OData: fake}
	address := "https://drive.example.test/d/2"
	if _, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Property: "Первичный документ - ссылка", URL: &address}); err == nil || !strings.Contains(err.Error(), "several") {
		t.Fatalf("ambiguous property must be refused: %v", err)
	}
	fake.rows[propertyDefinitionsResource] = definitions
	for name, change := range map[string]ReceiptLinkChange{
		"script url":       {URL: ptr("javascript:alert(1)")},
		"quoted url":       {URL: ptr(`https://drive.example.test/"x`)},
		"relative url":     {URL: ptr("/d/2")},
		"markup caption":   {Caption: ptr("<b>x</b>")},
		"nothing to do":    {},
		"clear and change": {Clear: true, URL: &address},
		"long caption":     {Caption: ptr(strings.Repeat("я", 1025))},
	} {
		if _, err := svc.SetReceiptLink(context.Background(), salesID(220), change); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	if len(fake.writes) != 0 {
		t.Fatalf("refused changes wrote: %v", fake.writes)
	}
}

func TestSetReceiptLinkKeepsOtherValuesAndChecksVersion(t *testing.T) {
	fake := linkFixture()
	svc := Service{OData: fake}
	caption := "Накладная №1"
	unchanged, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Caption: &caption, Apply: true})
	if err != nil || !unchanged.Unchanged || unchanged.Applied || len(fake.writes) != 0 {
		t.Fatalf("unchanged value must not write: %+v, %v", unchanged, err)
	}
	newCaption := "Накладная №1 от 01.10.2026"
	preview, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Caption: &newCaption})
	if err != nil || preview.After.URL != "https://drive.example.test/d/1" || preview.After.Caption != newCaption || preview.Before.Caption != caption || len(fake.writes) != 0 {
		t.Fatalf("caption-only preview must keep the URL: %+v, %v", preview, err)
	}
	if _, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Caption: &newCaption, ExpectedDataVersion: "v0", Apply: true}); err == nil {
		t.Fatal("stale preview must be rejected")
	}
	saved, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Caption: &newCaption, ExpectedDataVersion: "v1", Apply: true})
	if err != nil || !saved.Applied || saved.After.Caption != newCaption || !saved.After.Posted {
		t.Fatalf("save: %+v, %v", saved, err)
	}
	row := fake.rows[receiptResourceName][0]
	data, _ := json.Marshal(row["ДополнительныеРеквизиты"])
	if !strings.Contains(string(data), `"ТекстоваяСтрока":"note"`) || !strings.Contains(fmtRows(row), `<a href = "https://drive.example.test/d/1">`+newCaption+`</a>`) || row["Комментарий"] != "keep me" || row["Posted"] != true {
		t.Fatalf("stored receipt: %s %+v", data, row)
	}

	address := "https://drive.example.test/d/9?share=1"
	added, err := svc.SetReceiptLink(context.Background(), salesID(221), ReceiptLinkChange{URL: &address, Apply: true})
	if err != nil || !added.Applied || added.Before.Present || added.After.Caption != address {
		t.Fatalf("empty caption must show the URL: %+v, %v", added, err)
	}
	cleared, err := svc.SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{Clear: true, Apply: true})
	if err != nil || !cleared.Applied || cleared.After.Present {
		t.Fatalf("clear: %+v, %v", cleared, err)
	}
	data, _ = json.Marshal(fake.rows[receiptResourceName][0]["ДополнительныеРеквизиты"])
	if !strings.Contains(string(data), `"ТекстоваяСтрока":"note"`) || strings.Contains(string(data), "href") {
		t.Fatalf("clear must remove only the link row: %s", data)
	}
}

func TestSetReceiptLinkReportsPostingChangedBy1C(t *testing.T) {
	fake := &unpostingWriter{receiptWriter: linkFixture()}
	address := "https://drive.example.test/d/3"
	if _, err := (Service{OData: fake}).SetReceiptLink(context.Background(), salesID(220), ReceiptLinkChange{URL: &address, Apply: true}); err == nil || !strings.Contains(err.Error(), "posting state") {
		t.Fatalf("a silent unpost must be reported: %v", err)
	}
}

// unpostingWriter imitates a base that unposts a document when it is rewritten.
type unpostingWriter struct{ *receiptWriter }

func (w *unpostingWriter) Write(ctx context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	data, err := w.receiptWriter.Write(ctx, method, resource, body, ifMatch)
	for _, row := range w.rows[receiptResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			row["Posted"] = false
		}
	}
	return data, err
}

func ptr(value string) *string { return &value }

func fmtRows(row map[string]any) string {
	var out strings.Builder
	for _, line := range row["ДополнительныеРеквизиты"].([]any) {
		out.WriteString(anyString(line.(map[string]any)["ТекстоваяСтрока"]) + "\n")
	}
	return out.String()
}
