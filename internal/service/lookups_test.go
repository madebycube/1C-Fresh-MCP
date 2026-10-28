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
	items, err := svc.ListContracts(context.Background(), ContractFilter{CounterpartyID: "00000000-0000-0000-0000-00000000000A", Kind: "supplier"})
	if err != nil || len(items) != 1 || items[0].ID != "00000000-0000-0000-0000-000000000005" || items[0].CounterpartyID != "00000000-0000-0000-0000-00000000000a" {
		t.Fatalf("contracts: %+v, %v", items, err)
	}
	items, err = svc.ListContracts(context.Background(), ContractFilter{CounterpartyID: "00000000-0000-0000-0000-00000000000a", Kind: "supplier", All: true})
	if err != nil || len(items) != 2 {
		t.Fatalf("all contracts: %+v, %v", items, err)
	}
	for _, filter := range []ContractFilter{{Kind: "other"}, {CounterpartyID: "x"}, {OrganizationID: "00000000-0000-0000-0000-000000000000"}} {
		if _, err := svc.ListContracts(context.Background(), filter); err == nil {
			t.Fatalf("expected error for %+v", filter)
		}
	}
}
