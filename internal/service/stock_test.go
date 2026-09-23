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

const stockTestProduct = "00000000-0000-0000-0000-000000000001"
const stockTestWarehouse = "00000000-0000-0000-0000-000000000002"
const stockTestStore = "00000000-0000-0000-0000-000000000003"
const stockTestUnit = "00000000-0000-0000-0000-000000000004"
const stockTestCharacteristic = "00000000-0000-0000-0000-000000000006"

type stockReader struct {
	balances []map[string]any
}

func (stockReader) Check(context.Context) (int, error) { return 1, nil }

func (r stockReader) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	switch {
	case strings.HasPrefix(resource, "Catalog_Номенклатура("):
		return []byte(`{"Ref_Key":"` + stockTestProduct + `","Description":"Example product","ЕдиницаИзмерения_Key":"` + stockTestUnit + `","IsFolder":false,"DeletionMark":false}`), nil
	case strings.HasPrefix(resource, "Catalog_ЕдиницыИзмерения("):
		return []byte(`{"Ref_Key":"` + stockTestUnit + `","Description":"pcs"}`), nil
	case resource == "Catalog_СтруктурныеЕдиницы":
		return []byte(`{"odata.count":"3","value":[{"Ref_Key":"` + stockTestWarehouse + `","Description":"Example warehouse","ТипСтруктурнойЕдиницы":"Склад","DeletionMark":false,"Недействителен":false},{"Ref_Key":"` + stockTestStore + `","Description":"Example store","ТипСтруктурнойЕдиницы":"Розница","DeletionMark":false,"Недействителен":false},{"Ref_Key":"00000000-0000-0000-0000-000000000005","Description":"Office","ТипСтруктурнойЕдиницы":"Подразделение","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	return nil, nil
}

func (r stockReader) GetStockBalance(_ context.Context, productID string, params url.Values, _ int64) ([]byte, error) {
	if productID != stockTestProduct {
		return nil, nil
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.balances)), "value": r.balances[skip:min(skip+top, len(r.balances))]})
}

func TestGetStockAggregatesBalanceDimensionsAndFiltersWarehouses(t *testing.T) {
	reader := stockReader{balances: []map[string]any{
		{"Номенклатура_Key": stockTestProduct, "СтруктурнаяЕдиница_Key": stockTestWarehouse, "Характеристика_Key": emptyGUID, "Организация_Key": emptyGUID, "Партия_Key": "00000000-0000-0000-0000-000000000010", "Ячейка_Key": emptyGUID, "КоличествоBalance": json.Number("1.25")},
		{"Номенклатура_Key": stockTestProduct, "СтруктурнаяЕдиница_Key": stockTestWarehouse, "Характеристика_Key": emptyGUID, "Организация_Key": emptyGUID, "Партия_Key": "00000000-0000-0000-0000-000000000011", "Ячейка_Key": emptyGUID, "КоличествоBalance": json.Number("-0.10")},
		{"Номенклатура_Key": stockTestProduct, "СтруктурнаяЕдиница_Key": stockTestStore, "Характеристика_Key": stockTestCharacteristic, "Организация_Key": emptyGUID, "Партия_Key": emptyGUID, "Ячейка_Key": emptyGUID, "КоличествоBalance": json.Number("2.50")},
	}}
	svc := Service{OData: reader}
	warehouses, err := svc.ListWarehouses(context.Background())
	if err != nil || len(warehouses) != 2 {
		t.Fatalf("warehouses: %+v, %v", warehouses, err)
	}
	stock, err := svc.GetStock(context.Background(), stockTestProduct, "", "")
	if err != nil || stock.Total != "3.65" || stock.UnitName != "pcs" || len(stock.Balances) != 3 || len(stock.Warehouses) != 2 {
		t.Fatalf("stock: %+v, %v", stock, err)
	}
	filtered, err := svc.GetStock(context.Background(), stockTestProduct, stockTestWarehouse, "")
	if err != nil || filtered.Total != "1.15" || len(filtered.Warehouses) != 1 || len(filtered.Balances) != 2 {
		t.Fatalf("filtered stock: %+v, %v", filtered, err)
	}
	variant, err := svc.GetStock(context.Background(), stockTestProduct, "", stockTestCharacteristic)
	if err != nil || variant.Total != "2.5" || len(variant.Balances) != 1 || variant.Warehouses[0].ID != stockTestStore {
		t.Fatalf("variant stock: %+v, %v", variant, err)
	}
}

func TestStockSumPreservesExponentPrecision(t *testing.T) {
	var sum stockSum
	for _, value := range []Decimal{"1e-3", "0.009"} {
		if err := sum.Add(value); err != nil {
			t.Fatal(err)
		}
	}
	if sum.Decimal() != "0.01" {
		t.Fatalf("sum = %s", sum.Decimal())
	}
}

func TestGetStockShowsZeroForWarehouseWithoutBalance(t *testing.T) {
	stock, err := (Service{OData: stockReader{balances: []map[string]any{}}}).GetStock(context.Background(), stockTestProduct, stockTestWarehouse, "")
	if err != nil || stock.Total != "0" || len(stock.Warehouses) != 1 || stock.Warehouses[0].Quantity != "0" || len(stock.Balances) != 0 {
		t.Fatalf("zero stock: %+v, %v", stock, err)
	}
}

func TestGetStockPaginatesBalanceRows(t *testing.T) {
	rows := make([]map[string]any, 101)
	for index := range rows {
		rows[index] = map[string]any{
			"Номенклатура_Key":       stockTestProduct,
			"СтруктурнаяЕдиница_Key": stockTestWarehouse,
			"Характеристика_Key":     emptyGUID,
			"Партия_Key":             fmt.Sprintf("00000000-0000-0000-0000-%012x", index+100),
			"КоличествоBalance":      json.Number("1"),
		}
	}
	stock, err := (Service{OData: stockReader{balances: rows}}).GetStock(context.Background(), stockTestProduct, "", "")
	if err != nil || stock.Total != "101" || len(stock.Balances) != 101 {
		t.Fatalf("paged stock: %+v, %v", stock, err)
	}
}
