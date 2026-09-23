package service

import (
	"context"
	"net/http"
	"testing"
)

func TestCreateCounterpartySetsRoleAndValidatesParent(t *testing.T) {
	stub := &groupWriteStub{}
	change, err := (Service{OData: stub}).CreateCounterparty(context.Background(), "customer", "  Example customer  ", "", "")
	if err != nil || !change.Applied || change.ID != childID || change.Name != "Example customer" || change.FullName != change.Name || stub.method != http.MethodPost || stub.resource != "Catalog_Контрагенты" || stub.body["IsFolder"] != false || stub.body["Покупатель"] != true || stub.body["Поставщик"] != false || stub.body["Parent_Key"] != emptyGUID {
		t.Fatalf("customer create: %+v, %+v, %v", change, stub, err)
	}
	stub.row = `{"Ref_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false}`
	change, err = (Service{OData: stub}).CreateCounterparty(context.Background(), "supplier", "Example supplier", "Supplier legal name", groupID)
	if err != nil || !change.Applied || stub.body["Покупатель"] != false || stub.body["Поставщик"] != true || stub.body["Parent_Key"] != groupID {
		t.Fatalf("supplier create: %+v, %+v, %v", change, stub, err)
	}
	stub.row = `{"Ref_Key":"` + groupID + `","IsFolder":false,"DeletionMark":false}`
	if _, err := (Service{OData: stub}).CreateCounterparty(context.Background(), "customer", "Bad parent", "", groupID); err == nil || stub.writeHits != 2 {
		t.Fatal("accepted a non-folder parent")
	}
	if _, err := (Service{OData: stub}).CreateCounterparty(context.Background(), "other", "Wrong role", "", ""); err == nil || stub.writeHits != 2 {
		t.Fatal("accepted an unknown role")
	}
}

func TestUpdateCounterpartyRequiresRoleAndDataVersion(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Old","НаименованиеПолное":"Old full","IsFolder":false,"DeletionMark":false,"Покупатель":true,"Поставщик":false,"DataVersion":"version-1"}`}
	name := "New"
	change, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "customer", groupID, CounterpartyPatch{Name: &name})
	if err != nil || !change.Applied || change.Name != name || stub.method != http.MethodPatch || stub.ifMatch != "version-1" || stub.resource != "Catalog_Контрагенты(guid'"+groupID+"')" || len(stub.body) != 1 || stub.body["Description"] != name {
		t.Fatalf("customer update: %+v, %+v, %v", change, stub, err)
	}
	if _, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "supplier", groupID, CounterpartyPatch{Name: &name}); err == nil || stub.writeHits != 1 {
		t.Fatal("updated a customer through supplier command")
	}
	stub.row = `{"Ref_Key":"` + groupID + `","Description":"Old","IsFolder":false,"DeletionMark":false,"Покупатель":true,"Поставщик":false}`
	if _, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "customer", groupID, CounterpartyPatch{Name: &name}); err == nil || stub.writeHits != 1 {
		t.Fatal("updated without a data version")
	}
}

func TestUpdateCounterpartySkipsUnchangedValueAndRejectsBadInput(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Same","НаименованиеПолное":"Full","IsFolder":false,"DeletionMark":false,"Покупатель":false,"Поставщик":true,"DataVersion":"version-1"}`}
	name := "Same"
	change, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "supplier", groupID, CounterpartyPatch{Name: &name})
	if err != nil || change.Applied || stub.writeHits != 0 {
		t.Fatalf("unchanged supplier: %+v, %v", change, err)
	}
	if _, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "supplier", groupID, CounterpartyPatch{}); err == nil {
		t.Fatal("accepted an empty update")
	}
	name = "\n"
	if _, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "supplier", groupID, CounterpartyPatch{Name: &name}); err == nil || stub.writeHits != 0 {
		t.Fatal("accepted an empty name")
	}
	name = "Renamed"
	for _, row := range []string{
		`{"Ref_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false,"Поставщик":true,"DataVersion":"v1"}`,
		`{"Ref_Key":"` + groupID + `","IsFolder":false,"DeletionMark":true,"Поставщик":true,"DataVersion":"v1"}`,
	} {
		stub.row = row
		if _, err := (Service{OData: stub}).UpdateCounterparty(context.Background(), "supplier", groupID, CounterpartyPatch{Name: &name}); err == nil || stub.writeHits != 0 {
			t.Fatal("updated a folder or deleted counterparty")
		}
	}
}
