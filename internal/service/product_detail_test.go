package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

type productDetailReader struct {
	row   string
	calls int
}

func (*productDetailReader) Check(context.Context) (int, error) { return 1, nil }

func (reader *productDetailReader) Get(_ context.Context, resource string, params url.Values, maxBytes int64) ([]byte, error) {
	reader.calls++
	if resource != nomenclatureResource(groupID) || maxBytes != 1<<20 || !strings.Contains(params.Get("$select"), "ЕдиницаИзмерения_Key") || !strings.Contains(params.Get("$select"), "ТипНоменклатуры") {
		return nil, errors.New("unexpected product read")
	}
	return []byte(reader.row), nil
}

func TestGetProductReadsNamedFields(t *testing.T) {
	reader := &productDetailReader{row: `{"Ref_Key":"` + groupID + `","Code":"P1","Description":"Chair","НаименованиеПолное":"Chair full","Артикул":"A1","Parent_Key":"` + childID + `","ТипНоменклатуры":"Товар","ЕдиницаИзмерения_Key":"` + childID + `","IsFolder":false,"DeletionMark":false}`}
	product, err := (Service{OData: reader}).GetProduct(context.Background(), groupID)
	if err != nil || product.ID != groupID || product.Name != "Chair" || product.Type != "Товар" || product.UnitID != childID || reader.calls != 1 {
		t.Fatalf("get product: %+v, calls=%d, err=%v", product, reader.calls, err)
	}
}

func TestGetProductRejectsInvalidOrNonProduct(t *testing.T) {
	reader := &productDetailReader{}
	if _, err := (Service{OData: reader}).GetProduct(context.Background(), "not-a-guid"); err == nil || reader.calls != 0 {
		t.Fatal("invalid ID reached OData")
	}
	for _, row := range []string{
		`{"Ref_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false}`,
		`{"Ref_Key":"` + groupID + `","IsFolder":false,"DeletionMark":true}`,
		`{"Ref_Key":"` + groupID + `","IsFolder":false}`,
	} {
		reader.row = row
		if _, err := (Service{OData: reader}).GetProduct(context.Background(), groupID); err == nil {
			t.Fatalf("accepted non-product row: %s", row)
		}
	}
}
