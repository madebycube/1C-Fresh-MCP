package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	registerTestCustomer = "00000000-0000-0000-0000-000000000001"
	registerTestOther    = "00000000-0000-0000-0000-000000000002"
	registerTestProduct  = "00000000-0000-0000-0000-000000000003"
	registerTestBank     = "00000000-0000-0000-0000-000000000004"
	registerTestCash     = "00000000-0000-0000-0000-000000000005"
	registerTestRuble    = "00000000-0000-0000-0000-000000000006"
)

type registerReader struct {
	rows    []map[string]any
	queries *[]config.RegisterQuery
}

func (registerReader) Check(context.Context) (int, error) { return 1, nil }

func (registerReader) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	switch {
	case resource == "Catalog_Контрагенты":
		return []byte(`{"odata.count":"2","value":[{"Ref_Key":"` + registerTestCustomer + `","Description":"Пример покупателя"},{"Ref_Key":"` + registerTestOther + `","Description":"Другой"}]}`), nil
	case strings.HasPrefix(resource, "Catalog_Номенклатура("):
		return []byte(`{"Ref_Key":"` + registerTestProduct + `","Code":"P1","Description":"Пример товара"}`), nil
	case resource == "Catalog_БанковскиеСчета":
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + registerTestBank + `","Description":"Расчётный","DeletionMark":false,"Owner_Type":"StandardODATA.Catalog_Организации"}]}`), nil
	case resource == "Catalog_Кассы":
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + registerTestCash + `","Description":"Касса","DeletionMark":false}]}`), nil
	case resource == "Catalog_Валюты":
		return []byte(`{"odata.count":"1","value":[{"Ref_Key":"` + registerTestRuble + `","Code":"643","Description":"Рубль","DeletionMark":false}]}`), nil
	}
	return nil, nil
}

func (r registerReader) GetRegisterTable(_ context.Context, query config.RegisterQuery, params url.Values, _ int64) ([]byte, error) {
	*r.queries = append(*r.queries, query)
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	return json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(r.rows)), "value": r.rows[skip:min(skip+top, len(r.rows))]})
}

func TestListDebtsSplitsDebtAndAdvanceByCounterparty(t *testing.T) {
	queries := []config.RegisterQuery{}
	reader := registerReader{queries: &queries, rows: []map[string]any{
		{"Контрагент_Key": registerTestCustomer, "ТипРасчетов": "Долг", "СуммаBalance": json.Number("100.10")},
		{"Контрагент_Key": registerTestCustomer, "ТипРасчетов": "Аванс", "СуммаBalance": json.Number("-20")},
		{"Контрагент_Key": registerTestOther, "ТипРасчетов": "Долг", "СуммаBalance": json.Number("300")},
		{"Контрагент_Key": registerTestProduct, "ТипРасчетов": "Долг", "СуммаBalance": json.Number("-50")},
		{"Контрагент_Key": registerTestProduct, "ТипРасчетов": "Аванс", "СуммаBalance": json.Number("50")},
	}}
	page, err := Service{OData: reader}.ListDebts(context.Background(), "customer", "2026-09-30", "", 1, 0)
	if err != nil || page.Total != 2 || page.Count != 1 || page.NextOffset == nil || page.Items[0].CounterpartyID != registerTestOther || page.TotalNet != "380.1" || page.TotalAdvance != "30" {
		t.Fatalf("page: %+v, %v", page, err)
	}
	if queries[0] != (config.RegisterQuery{Register: "AccumulationRegister_РасчетыСПокупателями", Table: "Balance", Period: "2026-10-01T00:00:00", Dimensions: "Контрагент,ТипРасчетов"}) {
		t.Fatalf("query: %+v", queries[0])
	}
	page, err = Service{OData: reader}.ListDebts(context.Background(), "customer", "2026-09-30", registerTestCustomer, 20, 0)
	if err != nil || len(page.Items) != 1 || page.Items[0].Debt != "100.1" || page.Items[0].Net != "80.1" || page.Items[0].CounterpartyName != "Пример покупателя" {
		t.Fatalf("filtered: %+v, %v", page, err)
	}
}

func TestListDebtsRejectsBadInputAndUnknownTypes(t *testing.T) {
	queries := []config.RegisterQuery{}
	svc := Service{OData: registerReader{queries: &queries, rows: []map[string]any{{"Контрагент_Key": registerTestCustomer, "ТипРасчетов": "Прочее", "СуммаBalance": 1}}}}
	for _, call := range [][3]string{{"other", "", ""}, {"customer", "2026-13-01", ""}, {"customer", "", "bad"}} {
		if _, err := svc.ListDebts(context.Background(), call[0], call[1], call[2], 20, 0); err == nil {
			t.Fatalf("expected error for %v", call)
		}
	}
	if len(queries) != 0 {
		t.Fatalf("invalid input read the register: %+v", queries)
	}
	if _, err := svc.ListDebts(context.Background(), "supplier", "", "", 20, 0); err == nil {
		t.Fatal("expected unknown settlement type error")
	}
}

func TestReportSalesSumsTurnoversByProduct(t *testing.T) {
	queries := []config.RegisterQuery{}
	reader := registerReader{queries: &queries, rows: []map[string]any{
		{"Номенклатура_Key": registerTestProduct, "Контрагент_Key": emptyGUID, "КоличествоTurnover": 2, "СуммаTurnover": json.Number("240.50"), "СуммаНДСTurnover": 40, "СебестоимостьTurnover": 100},
	}}
	report, err := Service{OData: reader}.ReportSales(context.Background(), "product", "2026-09-01", "2026-09-30", 20, 0)
	if err != nil || len(report.Items) != 1 || report.Items[0].ProductName != "Пример товара" || report.TotalAmount != "240.5" || report.Items[0].Quantity != "2" {
		t.Fatalf("report: %+v, %v", report, err)
	}
	byCustomer, err := Service{OData: reader}.ReportSales(context.Background(), "customer", "2026-09-01", "2026-09-30", 20, 0)
	if err != nil || byCustomer.Items[0].CustomerName != "(no counterparty)" {
		t.Fatalf("by customer: %+v, %v", byCustomer, err)
	}
	if queries[0].StartPeriod != "2026-09-01T00:00:00" || queries[0].EndPeriod != "2026-09-30T23:59:59" || queries[0].Dimensions != "Номенклатура" {
		t.Fatalf("query: %+v", queries[0])
	}
	for _, call := range [][3]string{{"warehouse", "2026-09-01", "2026-09-30"}, {"product", "2026-09-30", "2026-09-01"}, {"product", "2025-01-01", "2026-09-30"}} {
		if _, err := (Service{OData: reader}).ReportSales(context.Background(), call[0], call[1], call[2], 20, 0); err == nil {
			t.Fatalf("expected error for %v", call)
		}
	}
}

func TestListCashBalancesNamesAccounts(t *testing.T) {
	queries := []config.RegisterQuery{}
	reader := registerReader{queries: &queries, rows: []map[string]any{
		{"БанковскийСчетКасса": registerTestBank, "БанковскийСчетКасса_Type": "StandardODATA.Catalog_БанковскиеСчета", "Валюта_Key": registerTestRuble, "СуммаBalance": 1000, "СуммаВалBalance": 1000},
		{"БанковскийСчетКасса": registerTestCash, "БанковскийСчетКасса_Type": "StandardODATA.Catalog_Кассы", "Валюта_Key": registerTestRuble, "СуммаBalance": json.Number("25.5"), "СуммаВалBalance": json.Number("25.5")},
	}}
	balances, err := Service{OData: reader}.ListCashBalances(context.Background(), "2026-09-30")
	if err != nil || balances.Count != 2 || balances.Total != "1025.5" || balances.Items[0].AccountName != "Расчётный" || balances.Items[1].CurrencyCode != "643" {
		t.Fatalf("balances: %+v, %v", balances, err)
	}
	reader.rows = []map[string]any{{"БанковскийСчетКасса": registerTestBank, "БанковскийСчетКасса_Type": "StandardODATA.Catalog_Сотрудники", "Валюта_Key": registerTestRuble, "СуммаBalance": 1, "СуммаВалBalance": 1}}
	if _, err := (Service{OData: reader}).ListCashBalances(context.Background(), ""); err == nil {
		t.Fatal("expected unknown account type error")
	}
}
