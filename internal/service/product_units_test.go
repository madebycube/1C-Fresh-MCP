package service

import (
	"context"
	"strings"
	"testing"
)

func receiptUnitFixture() *receiptWriter {
	fake := receiptFixture()
	fake.rows["Catalog_КлассификаторЕдиницИзмерения"] = []map[string]any{{"Ref_Key": salesID(65), "DeletionMark": false, "Code": "796", "Description": "шт"}}
	fake.rows["Catalog_ЕдиницыИзмерения"] = []map[string]any{
		{"Ref_Key": salesID(67), "DeletionMark": false, "Description": "кор", "Owner": salesID(64), "Owner_Type": productOwnerType, "Коэффициент": 12},
		{"Ref_Key": salesID(68), "DeletionMark": false, "Description": "уп", "Owner": salesID(64), "Owner_Type": productOwnerType, "Коэффициент": 0},
		{"Ref_Key": salesID(69), "DeletionMark": false, "Description": "чужая", "Owner": salesID(66), "Owner_Type": productOwnerType, "Коэффициент": 5},
	}
	return fake
}

func TestListProductUnitsReportsUnknownFactors(t *testing.T) {
	units, err := Service{OData: receiptUnitFixture()}.ListProductUnits(context.Background(), salesID(64))
	if err != nil || units.Count != 3 || !units.Items[0].Base || units.Items[0].Name != "шт" {
		t.Fatalf("units: %+v, %v", units, err)
	}
	if units.Items[1].Name != "кор" || units.Items[1].Factor != "12" || !units.Items[1].FactorKnown || units.Items[2].FactorKnown {
		t.Fatalf("own units: %+v", units.Items)
	}
}

func TestReceiptLineUsesPackagingUnitWithConfirmedFactor(t *testing.T) {
	fake := receiptUnitFixture()
	input := receiptDraftInput()
	input.Lines = []ReceiptLineInput{{ProductID: salesID(64), UnitID: salesID(67), Quantity: "2", Price: "120", VATRate: "НДС22"}}
	preview, err := Service{OData: fake}.CreateReceiptDraft(context.Background(), input)
	if err != nil || preview.Lines[0].UnitType != productUnitType || preview.Lines[0].UnitFactor != "12" || preview.Lines[0].BaseQuantity != "24" {
		t.Fatalf("packaging line: %+v, %v", preview.Lines, err)
	}
	input.Apply = true
	if _, err := (Service{OData: fake}).CreateReceiptDraft(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	line := fake.rows[receiptResourceName][1]["Запасы"].([]any)[0].(map[string]any)
	if line["ЕдиницаИзмерения"] != salesID(67) || line["ЕдиницаИзмерения_Type"] != productUnitType {
		t.Fatalf("saved line: %+v", line)
	}
	for unit, want := range map[string]string{salesID(68): "no confirmed conversion factor", salesID(69): "product's base unit or one of its own units"} {
		fake := receiptUnitFixture()
		input.Lines[0].UnitID = unit
		if _, err := (Service{OData: fake}).CreateReceiptDraft(context.Background(), input); err == nil || !strings.Contains(err.Error(), want) || len(fake.writes) != 0 {
			t.Fatalf("unit %s: %v, writes %v", unit, err, fake.writes)
		}
	}
}

func TestReceiptLineKeepsDocumentAmountsWithinRounding(t *testing.T) {
	// 7 × 14.29 = 100.03, but the supplier's document says 100.00 for 7 units at 14.2857…, shown as 14.29.
	input := receiptDraftInput()
	input.Lines = []ReceiptLineInput{{ProductID: salesID(64), Quantity: "7", Price: "14.29", VATRate: "НДС22", Amount: "100", VAT: "18.03"}}
	preview, err := Service{OData: receiptFixture()}.CreateReceiptDraft(context.Background(), input)
	line := preview.Lines[0]
	if err != nil || line.Amount != "100" || line.ComputedAmount != "100.03" || line.VAT != "18.03" || line.ComputedVAT != "" || preview.Amount != "100" {
		t.Fatalf("explicit amounts: %+v, %v", preview.Lines, err)
	}
	for name, mutate := range map[string]func(*ReceiptLineInput){
		"amount outside rounding": func(l *ReceiptLineInput) { l.Amount = "99.9" },
		"VAT off by two kopecks":  func(l *ReceiptLineInput) { l.VAT = "18.05" },
		"VAT on untaxed line":     func(l *ReceiptLineInput) { l.VATRate = "БезНДС"; l.VAT = "1" },
		"amount with 3 decimals":  func(l *ReceiptLineInput) { l.Amount = "100.001" },
	} {
		fake := receiptFixture()
		input := receiptDraftInput()
		line := ReceiptLineInput{ProductID: salesID(64), Quantity: "7", Price: "14.29", VATRate: "НДС22", Amount: "100"}
		mutate(&line)
		input.Lines, input.Apply = []ReceiptLineInput{line}, true
		if _, err := (Service{OData: fake}).CreateReceiptDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
			t.Fatalf("%s accepted: writes %v", name, fake.writes)
		}
	}
}
