package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const priceTestProduct = "00000000-0000-0000-0000-000000000001"
const priceTestType = "00000000-0000-0000-0000-000000000002"
const priceTestVariant = "00000000-0000-0000-0000-000000000003"

type priceReader struct {
	documents []map[string]any
}

func (priceReader) Check(context.Context) (int, error) { return 1, nil }

func (r priceReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	switch {
	case strings.HasPrefix(resource, "Catalog_Номенклатура("):
		return []byte(`{"Ref_Key":"` + priceTestProduct + `","Description":"Example product","IsFolder":false,"DeletionMark":false}`), nil
	case resource == "Catalog_ВидыЦен":
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + priceTestType + `","Description":"Example price","DeletionMark":false,"Недействителен":false}]}`), nil
	case resource == "Document_УстановкаЦенНоменклатуры":
		if params.Get("$inlinecount") != "" {
			return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.documents)), "value": []any{}})
		}
		skip, _ := strconv.Atoi(params.Get("$skip"))
		top, _ := strconv.Atoi(params.Get("$top"))
		return json.Marshal(map[string]any{"value": r.documents[skip : skip+top]})
	}
	return nil, nil
}

func priceTestDocument(id, date string, posted, deleted bool, characteristic, amount string) map[string]any {
	return map[string]any{
		"Ref_Key": id, "Date": date, "Posted": posted, "DeletionMark": deleted,
		"Запасы": []map[string]any{{
			"LineNumber": "1", "Номенклатура_Key": priceTestProduct, "ВидЦены_Key": priceTestType,
			"Характеристика_Key": characteristic, "Цена": json.Number(amount),
		}},
	}
}

func TestGetPriceSelectsLatestPostedDocumentAndExactCharacteristic(t *testing.T) {
	id := func(last string) string { return "00000000-0000-0000-0000-0000000000" + last }
	reader := priceReader{documents: []map[string]any{
		priceTestDocument(id("10"), "2026-09-20T10:00:00", true, false, emptyGUID, "10.10"),
		priceTestDocument(id("20"), "2026-09-22T12:00:00", true, false, emptyGUID, "20.20"),
		priceTestDocument(id("21"), "2026-09-22T12:00:00", true, false, emptyGUID, "21.21"),
		priceTestDocument(id("30"), "2026-09-22T13:00:00", false, false, emptyGUID, "30.30"),
		priceTestDocument(id("40"), "2026-09-22T14:00:00", true, true, emptyGUID, "40.40"),
		priceTestDocument(id("50"), "2026-09-23T09:00:00", true, false, priceTestVariant, "50.50"),
		priceTestDocument(id("60"), "2026-09-24T09:00:00", true, false, emptyGUID, "60.60"),
	}}
	for index := 100; index < 201; index++ {
		reader.documents = append(reader.documents, priceTestDocument(fmt.Sprintf("00000000-0000-0000-0000-%012x", index), "2026-01-01T00:00:00", true, false, emptyGUID, "1.10"))
	}
	svc := Service{OData: reader}
	quote, err := svc.GetPrice(context.Background(), priceTestProduct, "Example price", "", "2026-09-23")
	if err != nil || !quote.Found || quote.Price != "21.21" || quote.SourceDocumentID != id("21") {
		t.Fatalf("default quote: %+v, %v", quote, err)
	}
	variant, err := svc.GetPrice(context.Background(), priceTestProduct, "Example price", priceTestVariant, "2026-09-23")
	if err != nil || !variant.Found || variant.Price != "50.50" || variant.SourceDocumentID != id("50") {
		t.Fatalf("variant quote: %+v, %v", variant, err)
	}
	earlier, err := svc.GetPrice(context.Background(), priceTestProduct, "Example price", "", "2026-09-19")
	if err != nil || !earlier.Found || earlier.Price != "1.10" {
		t.Fatalf("earlier quote: %+v, %v", earlier, err)
	}
	before, err := svc.GetPrice(context.Background(), priceTestProduct, "Example price", "", "2025-12-31")
	if err != nil || before.Found {
		t.Fatalf("before first price: %+v, %v", before, err)
	}
}
