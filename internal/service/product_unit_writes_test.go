package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type unitWriter struct {
	*traceReader
	method, resource, ifMatch string
	body                      map[string]any
	writes                    int
}

func (w *unitWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes++
	w.method, w.resource, w.ifMatch = method, resource, ifMatch
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&w.body); err != nil {
		return nil, err
	}
	return []byte(`{"Ref_Key":"` + salesID(600) + `"}`), nil
}

func unitWriteFixture() *unitWriter {
	fake := &unitWriter{traceReader: receiptUnitFixture().traceReader}
	fake.rows["Catalog_КлассификаторЕдиницИзмерения"] = append(fake.rows["Catalog_КлассификаторЕдиницИзмерения"],
		map[string]any{"Ref_Key": salesID(77), "DeletionMark": false, "Code": "778", "Description": "упак"},
		map[string]any{"Ref_Key": salesID(78), "DeletionMark": true, "Code": "779", "Description": "старая"})
	fake.rows["Catalog_ЕдиницыИзмерения"][0]["DataVersion"] = "u1"
	return fake
}

func TestCreateProductUnitPostsPackagingUnit(t *testing.T) {
	fake := unitWriteFixture()
	created, err := Service{OData: fake}.CreateProductUnit(context.Background(), ProductUnitCreate{ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "6.500"})
	if err != nil || !created.Applied || created.ID != salesID(600) || created.Name != "упак" || created.Factor != "6.5" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	if fake.method != http.MethodPost || fake.resource != "Catalog_ЕдиницыИзмерения" || fake.body["Owner"] != salesID(64) || fake.body["Owner_Type"] != productOwnerType ||
		fake.body["Коэффициент"] != json.Number("6.5") || fake.body["ЕдиницаИзмеренияПоКлассификатору_Key"] != salesID(77) || fake.body["Description"] != "упак" {
		t.Fatalf("body=%+v", fake.body)
	}
	for name, input := range map[string]ProductUnitCreate{
		"duplicate name":    {ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "3", Name: "КОР"},
		"base unit again":   {ProductID: salesID(64), UnitTypeID: salesID(65), Factor: "1"},
		"zero factor":       {ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "0"},
		"exponent factor":   {ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "1e3"},
		"negative factor":   {ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "-2"},
		"deleted unit type": {ProductID: salesID(64), UnitTypeID: salesID(78), Factor: "2"},
		"unknown product":   {ProductID: salesID(99), UnitTypeID: salesID(77), Factor: "2"},
		"long name":         {ProductID: salesID(64), UnitTypeID: salesID(77), Factor: "2", Name: strings.Repeat("я", 26)},
		"missing unit type": {ProductID: salesID(64), Factor: "2"},
	} {
		if _, err := (Service{OData: fake}).CreateProductUnit(context.Background(), input); err == nil || fake.writes != 1 {
			t.Fatalf("%s: accepted or wrote (%d): %v", name, fake.writes, err)
		}
	}
}

func TestUpdateProductUnitPatchesChangedFields(t *testing.T) {
	fake := unitWriteFixture()
	name, factor := "коробка", "10"
	change, err := Service{OData: fake}.UpdateProductUnit(context.Background(), salesID(67), ProductUnitPatch{Name: &name, Factor: &factor})
	if err != nil || !change.Applied || len(change.Warnings) != 1 || change.ProductID != salesID(64) {
		t.Fatalf("change=%+v err=%v", change, err)
	}
	if fake.method != http.MethodPatch || fake.resource != "Catalog_ЕдиницыИзмерения(guid'"+salesID(67)+"')" || fake.ifMatch != "u1" || len(fake.body) != 2 || fake.body["Коэффициент"] != json.Number("10") {
		t.Fatalf("write %s %s %s %+v", fake.method, fake.resource, fake.ifMatch, fake.body)
	}
	same, twelve := "кор", "12.0"
	if change, err := (Service{OData: fake}).UpdateProductUnit(context.Background(), salesID(67), ProductUnitPatch{Name: &same, Factor: &twelve}); err != nil || change.Applied || fake.writes != 1 {
		t.Fatalf("unchanged unit wrote: %+v %v", change, err)
	}
	if _, err := (Service{OData: fake}).UpdateProductUnit(context.Background(), salesID(68), ProductUnitPatch{Name: &name}); err == nil || fake.writes != 1 {
		t.Fatalf("unit without data version updated: %v", err)
	}
	if _, err := (Service{OData: fake}).UpdateProductUnit(context.Background(), salesID(67), ProductUnitPatch{}); err == nil {
		t.Fatal("empty patch accepted")
	}
}
