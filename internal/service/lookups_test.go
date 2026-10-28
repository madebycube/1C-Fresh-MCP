package service

import (
	"context"
	"net/url"
	"testing"
)

type lookupReader struct{}

func (lookupReader) Check(context.Context) (int, error) { return 1, nil }

func (lookupReader) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	switch resource {
	case "Catalog_Организации":
		return []byte(`{"odata.count":"2","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000001","Code":"1","Description":"Пример организации","НаименованиеПолное":"ООО Пример","DeletionMark":false},{"Ref_Key":"00000000-0000-0000-0000-000000000002","Code":"2","Description":"Удалённая","DeletionMark":true}]}`), nil
	case "Catalog_СтруктурныеЕдиницы":
		return []byte(`{"odata.count":"2","value":[{"Ref_Key":"00000000-0000-0000-0000-000000000003","Description":"Склад","ТипСтруктурнойЕдиницы":"Склад","DeletionMark":false,"Недействителен":false},{"Ref_Key":"00000000-0000-0000-0000-000000000004","Description":"Офис","ТипСтруктурнойЕдиницы":"Подразделение","DeletionMark":false,"Недействителен":false}]}`), nil
	case "Catalog_БанковскиеСчета":
		return []byte(`{"odata.count":"2","value":[{"Ref_Key":"00000000-0000-0000-0000-00000000000c","Description":"Счёт 1","DeletionMark":false,"Owner":"00000000-0000-0000-0000-000000000001","Owner_Type":"StandardODATA.Catalog_Организации"},{"Ref_Key":"00000000-0000-0000-0000-00000000000d","Description":"Счёт 2","DeletionMark":false,"Owner":"00000000-0000-0000-0000-000000000002","Owner_Type":"StandardODATA.Catalog_Организации"}]}`), nil
	case "Catalog_ДоговорыКонтрагентов":
		return []byte(`{"odata.count":"4","value":[
{"Ref_Key":"00000000-0000-0000-0000-000000000005","Description":"Поставка","ВидДоговора":"СПоставщиком","Owner":"00000000-0000-0000-0000-00000000000A","Организация_Key":"00000000-0000-0000-0000-000000000001","ВалютаРасчетов_Key":"00000000-0000-0000-0000-000000000009","DeletionMark":false,"Недействителен":false},
{"Ref_Key":"00000000-0000-0000-0000-000000000006","Description":"Продажа","ВидДоговора":"СПокупателем","Owner":"00000000-0000-0000-0000-00000000000a","Организация_Key":"00000000-0000-0000-0000-000000000001","DeletionMark":false,"Недействителен":false},
{"Ref_Key":"00000000-0000-0000-0000-000000000007","Description":"Старый","ВидДоговора":"СПоставщиком","Owner":"00000000-0000-0000-0000-00000000000a","Организация_Key":"00000000-0000-0000-0000-000000000001","DeletionMark":false,"Недействителен":true},
{"Ref_Key":"00000000-0000-0000-0000-000000000008","Description":"Чужой","ВидДоговора":"СПоставщиком","Owner":"00000000-0000-0000-0000-00000000000b","Организация_Key":"00000000-0000-0000-0000-000000000001","DeletionMark":false,"Недействителен":false}]}`), nil
	}
	return nil, nil
}

func TestListOrganizationsSkipsDeleted(t *testing.T) {
	items, err := Service{OData: lookupReader{}}.ListOrganizations(context.Background())
	if err != nil || len(items) != 1 || items[0].FullName != "ООО Пример" {
		t.Fatalf("organizations: %+v, %v", items, err)
	}
}

func TestListDepartmentsKeepsOnlyDepartments(t *testing.T) {
	items, err := Service{OData: lookupReader{}}.ListDepartments(context.Background())
	if err != nil || len(items) != 1 || items[0].Name != "Офис" {
		t.Fatalf("departments: %+v, %v", items, err)
	}
}

func TestListContractsFiltersLocally(t *testing.T) {
	svc := Service{OData: lookupReader{}}
	page, err := svc.ListContracts(context.Background(), ContractFilter{CounterpartyID: "00000000-0000-0000-0000-00000000000A", Kind: "supplier"}, 20, 0)
	if err != nil || page.Count != 1 || page.Items[0].ID != "00000000-0000-0000-0000-000000000005" || page.Items[0].CounterpartyID != "00000000-0000-0000-0000-00000000000a" || page.Items[0].Kind != "supplier" {
		t.Fatalf("contracts: %+v, %v", page, err)
	}
	page, err = svc.ListContracts(context.Background(), ContractFilter{CounterpartyID: "00000000-0000-0000-0000-00000000000a", Kind: "supplier", All: true}, 1, 0)
	if err != nil || page.Total != 2 || page.Count != 1 || page.NextOffset == nil || *page.NextOffset != 1 {
		t.Fatalf("all contracts: %+v, %v", page, err)
	}
	for _, filter := range []ContractFilter{{Kind: "lease"}, {CounterpartyID: "x"}, {OrganizationID: "00000000-0000-0000-0000-000000000000"}} {
		if _, err := svc.ListContracts(context.Background(), filter, 20, 0); err == nil {
			t.Fatalf("expected error for %+v", filter)
		}
	}
}

func TestListOrganizationAccountsKeepsOwnedBankAccounts(t *testing.T) {
	svc := Service{OData: lookupReader{}}
	accounts, err := svc.ListOrganizationAccounts(context.Background(), "bank", "00000000-0000-0000-0000-000000000001")
	if err != nil || len(accounts) != 1 || accounts[0].Name != "Счёт 1" {
		t.Fatalf("accounts: %+v, %v", accounts, err)
	}
	if _, err := svc.ListOrganizationAccounts(context.Background(), "cash", "00000000-0000-0000-0000-000000000001"); err == nil {
		t.Fatal("organization filter accepted for cash desks")
	}
}
