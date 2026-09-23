package service

import (
	"context"
	"testing"
)

func TestBankAccountDiscoveryKeepsOrganizationAccounts(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Description": "Own account", "DeletionMark": false, "Owner": salesID(9), "Owner_Type": "StandardODATA.Catalog_Организации", "Недействителен": false},
		{"Ref_Key": salesID(2), "Description": "Customer account", "DeletionMark": false, "Owner": salesID(8), "Owner_Type": "StandardODATA.Catalog_Контрагенты"},
		{"Ref_Key": salesID(3), "Description": "Deleted own account", "DeletionMark": true, "Owner": salesID(9), "Owner_Type": "StandardODATA.Catalog_Организации"},
	}
	svc := Service{OData: salesReader{rows: map[string][]map[string]any{"Catalog_БанковскиеСчета": rows}}}
	accounts, err := svc.ListMoneyAccounts(context.Background(), "bank")
	if err != nil || len(accounts) != 1 || accounts[0].ID != salesID(1) || accounts[0].OwnerID != salesID(9) {
		t.Fatalf("bank accounts: %+v, %v", accounts, err)
	}
}

func TestMoneyReadsExcludePayrollTaxAndUnknownOperations(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Number": "SUPPLIER", "Date": "2026-09-03T12:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 100.50, "ВидОперации": "Поставщику", "БанковскийСчет_Key": salesID(9)},
		{"Ref_Key": salesID(2), "Number": "PAYROLL", "Date": "2026-09-03T13:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 500, "ВидОперации": "Зарплата", "БанковскийСчет_Key": salesID(9)},
		{"Ref_Key": salesID(3), "Number": "TAX", "Date": "2026-09-03T14:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 200, "ВидОперации": "Налоги", "БанковскийСчет_Key": salesID(9)},
		{"Ref_Key": salesID(4), "Number": "UNKNOWN", "Date": "2026-09-03T15:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 10, "ВидОперации": "НоваяОперация", "БанковскийСчет_Key": salesID(9)},
		{"Ref_Key": salesID(5), "Number": "OTHER ACCOUNT", "Date": "2026-09-03T16:00:00", "Posted": true, "DeletionMark": false, "СуммаДокумента": 20, "ВидОперации": "Поставщику", "БанковскийСчет_Key": salesID(8)},
		{"Ref_Key": salesID(6), "Number": "DELETED", "Date": "2026-09-03T17:00:00", "Posted": true, "DeletionMark": true, "СуммаДокумента": 20, "ВидОперации": "Поставщику", "БанковскийСчет_Key": salesID(9)},
	}
	svc := Service{OData: salesReader{rows: map[string][]map[string]any{"Document_РасходСоСчета": rows}}}
	page, err := svc.ListMoneyDocuments(context.Background(), "bank-out", "2026-09-03", "2026-09-03", salesID(9), "", "", 20, 0)
	if err != nil || page.Total != 1 || page.Items[0].Number != "SUPPLIER" {
		t.Fatalf("bank expenses: %+v, %v", page, err)
	}
	if _, err := svc.GetMoneyDocument(context.Background(), "bank-out", salesID(2)); err == nil {
		t.Fatal("payroll document accessible through money get")
	}
	if _, err := svc.GetMoneyDocument(context.Background(), "bank-out", salesID(3)); err == nil {
		t.Fatal("tax document accessible through money get")
	}
	if _, err := svc.GetMoneyDocument(context.Background(), "bank-out", salesID(4)); err == nil {
		t.Fatal("unknown operation accessible through money get")
	}
}

func TestMoneyScopeRequiredBeforeReading(t *testing.T) {
	svc := Service{OData: salesReader{}}
	for _, test := range []struct{ kind, account, register, terminal string }{
		{"bank-in", "", "", ""},
		{"cash-in", "", "", ""},
		{"card-payment", "", "", ""},
		{"cash-shift", salesID(1), "", ""},
		{"bank-out", "bad", "", ""},
	} {
		if _, err := svc.ListMoneyDocuments(context.Background(), test.kind, "2026-09-01", "2026-09-02", test.account, test.register, test.terminal, 20, 0); err == nil {
			t.Errorf("accepted scope %+v", test)
		}
	}
	if _, err := svc.ListMoneyDocuments(context.Background(), "bank-in", "2026-09-01", "2026-10-10", salesID(1), "", "", 20, 0); err == nil {
		t.Fatal("oversized date range accepted")
	}
}
