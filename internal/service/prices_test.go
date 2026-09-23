package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const priceTestProduct = "00000000-0000-0000-0000-000000000001"
const priceTestType = "00000000-0000-0000-0000-000000000002"
const priceTestVariant = "00000000-0000-0000-0000-000000000003"
const priceTestCurrency = "00000000-0000-0000-0000-000000000004"

type priceReader struct {
	documents           []map[string]any
	products            []map[string]any
	groupID             string
	documentPages       *int
	currencyUnavailable bool
}

func (priceReader) Check(context.Context) (int, error) { return 1, nil }

func (r priceReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	switch {
	case r.groupID != "" && resource == nomenclatureResource(r.groupID):
		return json.Marshal(map[string]any{"Ref_Key": r.groupID, "Description": "Example group", "IsFolder": true, "DeletionMark": false})
	case strings.HasPrefix(resource, "Catalog_Номенклатура("):
		return []byte(`{"Ref_Key":"` + priceTestProduct + `","Description":"Example product","IsFolder":false,"DeletionMark":false}`), nil
	case resource == "Catalog_Номенклатура":
		skip, _ := strconv.Atoi(params.Get("$skip"))
		top, _ := strconv.Atoi(params.Get("$top"))
		return json.Marshal(map[string]any{"value": r.products[min(skip, len(r.products)):min(skip+top, len(r.products))]})
	case resource == "Catalog_ВидыЦен":
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + priceTestType + `","Description":"Example price","DeletionMark":false,"Недействителен":false}]}`), nil
	case resource == "Catalog_Валюты":
		if r.currencyUnavailable {
			return nil, errors.New("currency catalog unavailable")
		}
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + priceTestCurrency + `","Code":"643","Description":"Ruble","СимвольноеПредставление":"₽","DeletionMark":false}]}`), nil
	case resource == "Document_УстановкаЦенНоменклатуры":
		if params.Get("$inlinecount") != "" {
			return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.documents)), "value": []any{}})
		}
		if r.documentPages != nil {
			*r.documentPages++
		}
		skip, _ := strconv.Atoi(params.Get("$skip"))
		top, _ := strconv.Atoi(params.Get("$top"))
		return json.Marshal(map[string]any{"value": r.documents[skip : skip+top]})
	}
	return nil, nil
}

func TestListPricesScansHistoryOnceForProductPage(t *testing.T) {
	group := salesID(20)
	secondProduct := salesID(4)
	firstDoc := salesID(10)
	secondDoc := salesID(11)
	documentPages := 0
	reader := priceReader{
		groupID:       group,
		documentPages: &documentPages,
		products: []map[string]any{
			{"Ref_Key": group, "Description": "Group", "Parent_Key": emptyGUID, "IsFolder": true, "DeletionMark": false},
			{"Ref_Key": priceTestProduct, "Code": "P001", "Description": "First", "Артикул": "A001", "Parent_Key": group, "IsFolder": false, "DeletionMark": false},
			{"Ref_Key": secondProduct, "Description": "Second", "Parent_Key": group, "IsFolder": false, "DeletionMark": false},
		},
		documents: []map[string]any{
			priceTestDocument(firstDoc, "2026-09-20T10:00:00", true, false, emptyGUID, "10.10"),
			{"Ref_Key": secondDoc, "Date": "2026-09-21T10:00:00", "Posted": true, "DeletionMark": false, "Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": secondProduct, "ВидЦены_Key": priceTestType, "Характеристика_Key": emptyGUID, "Цена": json.Number("20.20")}}},
		},
	}
	svc := Service{OData: reader}
	first, err := svc.ListPrices(context.Background(), "Example price", group, "", "2026-09-23", 1, 0)
	if err != nil || len(first.Items) != 1 || first.Items[0].ProductID != priceTestProduct || first.Items[0].ProductCode != "P001" || first.Items[0].ProductArticle != "A001" || first.Items[0].Price != "10.10" || first.Items[0].CurrencyCode != "643" || first.Items[0].CurrencySymbol != "₽" || first.NextOffset == nil || *first.NextOffset != 2 {
		t.Fatalf("first price page: %+v, %v", first, err)
	}
	if documentPages != 1 {
		t.Fatalf("scanned %d price document pages for one product page", documentPages)
	}
	second, err := svc.ListPrices(context.Background(), "Example price", group, "", "2026-09-23", 1, *first.NextOffset)
	if err != nil || len(second.Items) != 1 || second.Items[0].ProductID != secondProduct || second.Items[0].Price != "20.20" || second.NextOffset != nil {
		t.Fatalf("second price page: %+v, %v", second, err)
	}
	before, err := svc.ListPrices(context.Background(), "Example price", group, "", "2026-09-19", 2, 0)
	if err != nil || len(before.Items) != 2 || before.Items[0].Found || before.Items[1].Found {
		t.Fatalf("prices before documents: %+v, %v", before, err)
	}
}

func priceTestDocument(id, date string, posted, deleted bool, characteristic, amount string) map[string]any {
	return map[string]any{
		"Ref_Key": id, "Date": date, "Posted": posted, "DeletionMark": deleted,
		"Запасы": []map[string]any{{
			"LineNumber": "1", "Номенклатура_Key": priceTestProduct, "ВидЦены_Key": priceTestType,
			"Характеристика_Key": characteristic, "Цена": json.Number(amount), "Валюта_Key": priceTestCurrency,
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

func TestGetPriceKeepsCurrencyIDWhenCatalogUnavailable(t *testing.T) {
	reader := priceReader{currencyUnavailable: true, documents: []map[string]any{
		priceTestDocument(salesID(30), "2026-09-23T10:00:00", true, false, emptyGUID, "12.34"),
	}}
	quote, err := (Service{OData: reader}).GetPrice(context.Background(), priceTestProduct, "Example price", "", "2026-09-23")
	if err != nil || !quote.Found || quote.Price != "12.34" || quote.CurrencyID != priceTestCurrency || quote.CurrencyCode != "" {
		t.Fatalf("quote without currency catalog: %+v, %v", quote, err)
	}
}
