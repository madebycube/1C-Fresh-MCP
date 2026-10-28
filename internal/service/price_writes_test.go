package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func priceWriteFixture() *priceDocumentWriter {
	fake := &priceDocumentWriter{transferFixture()}
	fake.rows["Catalog_ВидыЦен"] = []map[string]any{
		{"Ref_Key": salesID(81), "Description": "Розничная", "DeletionMark": false, "Недействителен": false, "РассчитываетсяДинамически": false, "ВалютаЦены_Key": salesID(61)},
		{"Ref_Key": salesID(82), "Description": "Мелкий опт", "DeletionMark": false, "Недействителен": false, "РассчитываетсяДинамически": true},
		{"Ref_Key": salesID(83), "Description": "Валютная", "DeletionMark": false, "Недействителен": false, "ВалютаЦены_Key": salesID(84)},
	}
	fake.rows["Catalog_ХарактеристикиНоменклатуры"] = []map[string]any{{"Ref_Key": salesID(85), "DeletionMark": false, "Owner": salesID(66), "Недействителен": false}}
	return fake
}

func priceDraftInput() PriceDraft {
	return PriceDraft{ProductID: salesID(64), PriceType: "розничная", Price: "149.90", Date: "2026-11-01"}
}

func TestPriceDraftPreviewWritesNothing(t *testing.T) {
	fake := priceWriteFixture()
	preview, err := Service{OData: fake}.CreatePriceDraft(context.Background(), priceDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || len(fake.writes) != 0 || preview.Price != "149.9" || preview.PriceTypeID != salesID(81) || preview.UnitID != salesID(65) || preview.CurrencyID != salesID(61) {
		t.Fatalf("preview: %+v %v", preview, fake.writes)
	}
}

func TestPriceDraftRejectsInvalidInputWithoutWriting(t *testing.T) {
	for name, mutate := range map[string]func(*PriceDraft){
		"zero price":        func(d *PriceDraft) { d.Price = "0" },
		"precision":         func(d *PriceDraft) { d.Price = "1.005" },
		"unknown type":      func(d *PriceDraft) { d.PriceType = "Закупочная" },
		"dynamic type":      func(d *PriceDraft) { d.PriceType = "Мелкий опт" },
		"foreign currency":  func(d *PriceDraft) { d.PriceType = "Валютная" },
		"other product":     func(d *PriceDraft) { d.ProductID = salesID(99) },
		"foreign variation": func(d *PriceDraft) { d.CharacteristicID = salesID(85) },
		"bad date":          func(d *PriceDraft) { d.Date = "2026-13-01" },
	} {
		fake := priceWriteFixture()
		input := priceDraftInput()
		input.Apply = true
		mutate(&input)
		if _, err := (Service{OData: fake}).CreatePriceDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
			t.Errorf("%s: %v %v", name, err, fake.writes)
		}
	}
}

func TestPriceDraftSavesUnpostedAndNeverRetries(t *testing.T) {
	fake := priceWriteFixture()
	input := priceDraftInput()
	input.Apply = true
	// The test writer files every POST under the transfer resource; point reads of the price document there too.
	fake.rows["Document_УстановкаЦенНоменклатуры"] = nil
	created, err := Service{OData: fake}.CreatePriceDraft(context.Background(), input)
	if err != nil || !created.Applied || created.ID != salesID(510) || len(fake.writes) != 1 || !strings.HasSuffix(fake.writes[0], "Document_УстановкаЦенНоменклатуры") {
		t.Fatalf("create: %+v, %v, %v", created, err, fake.writes)
	}
	saved := fake.rows["Document_УстановкаЦенНоменклатуры"][0]
	line := saved["Запасы"].([]any)[0].(map[string]any)
	if saved["ЗаписыватьНовыеЦеныПоверхУстановленных"] != true || line["Цена"] != json.Number("149.9") || line["ВидЦены_Key"] != salesID(81) || line["ЕдиницаИзмерения_Type"] != unitClassifierType {
		t.Fatalf("saved: %+v", saved)
	}
	fake = priceWriteFixture()
	fake.failPost = true
	if _, err := (Service{OData: fake}).CreatePriceDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), "before retrying") || len(fake.writes) != 1 {
		t.Fatalf("uncertain create: %v %v", err, fake.writes)
	}
}

// priceDocumentWriter files created documents under the price document resource and pages the price type catalog.
type priceDocumentWriter struct{ *transferWriter }

func (w *priceDocumentWriter) Get(ctx context.Context, resource string, params url.Values, max int64) ([]byte, error) {
	if resource == "Catalog_ВидыЦен" && params.Get("$inlinecount") != "" {
		rows := w.rows[resource]
		skip, _ := strconv.Atoi(params.Get("$skip"))
		return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(rows)), "value": rows[min(skip, len(rows)):]})
	}
	return w.transferWriter.Get(ctx, resource, params, max)
}

func (w *priceDocumentWriter) Write(ctx context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	response, err := w.transferWriter.Write(ctx, method, resource, body, ifMatch)
	if err == nil {
		rows := w.rows[transferResourceName]
		w.rows[resource] = append(w.rows[resource], rows[len(rows)-1])
		w.rows[transferResourceName] = rows[:len(rows)-1]
	}
	return response, err
}
