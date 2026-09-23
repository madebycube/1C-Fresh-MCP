package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type counterpartyGroupStub struct {
	records  map[string]string
	method   string
	resource string
	body     map[string]any
	version  string
	writes   int
}

func (*counterpartyGroupStub) Check(context.Context) (int, error) { return 1, nil }

func (stub *counterpartyGroupStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(resource, "Catalog_Контрагенты(guid'"), "')")
	row, ok := stub.records[id]
	if !ok {
		return nil, errors.New("unknown counterparty")
	}
	return []byte(row), nil
}

func (stub *counterpartyGroupStub) Write(_ context.Context, method, resource string, body []byte, version string) ([]byte, error) {
	stub.writes++
	stub.method, stub.resource, stub.version = method, resource, version
	if err := json.Unmarshal(body, &stub.body); err != nil {
		return nil, err
	}
	return []byte(`{"Ref_Key":"` + childID + `"}`), nil
}

func TestListCounterpartyGroupsBuildsPathsAndSkipsRecords(t *testing.T) {
	reader := &catalogReader{rows: map[string][]map[string]any{"Catalog_Контрагенты": {
		{"Ref_Key": groupID, "Code": "1", "Description": "Partners", "Parent_Key": emptyGUID, "IsFolder": true, "DeletionMark": false},
		{"Ref_Key": childID, "Code": "2", "Description": "Retail", "Parent_Key": groupID, "IsFolder": true, "DeletionMark": true},
		{"Ref_Key": "33333333-3333-3333-3333-333333333333", "Description": "Customer", "IsFolder": false, "DeletionMark": false},
	}}}
	groups, err := (Service{OData: reader}).ListCounterpartyGroups(context.Background(), "partners")
	if err != nil || len(groups) != 2 || groups[0].Path != "Partners" || groups[1].Path != "Partners / Retail" || !groups[1].Deleted {
		t.Fatalf("counterparty groups: %+v, %v", groups, err)
	}
}

func TestCounterpartyGroupWritesValidateFolderAndVersion(t *testing.T) {
	stub := &counterpartyGroupStub{records: map[string]string{
		groupID: `{"Ref_Key":"` + groupID + `","Description":"Partners","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v1"}`,
		childID: `{"Ref_Key":"` + childID + `","Description":"Child","Parent_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v2"}`,
	}}
	svc := Service{OData: stub}
	created, err := svc.CreateCounterpartyGroup(context.Background(), "New", groupID)
	if err != nil || !created.Applied || created.ID != childID || stub.method != http.MethodPost || stub.resource != "Catalog_Контрагенты" || stub.body["IsFolder"] != true || stub.body["Parent_Key"] != groupID {
		t.Fatalf("create folder: %+v, %+v, %v", created, stub, err)
	}
	updated, err := svc.UpdateCounterpartyGroup(context.Background(), childID, CounterpartyGroupPatch{Name: textPointer("Renamed"), ParentID: textPointer("root")})
	if err != nil || !updated.Applied || stub.method != http.MethodPatch || stub.resource != counterpartyResource(childID) || stub.version != "v2" || stub.body["Parent_Key"] != emptyGUID || stub.body["Description"] != "Renamed" {
		t.Fatalf("update folder: %+v, %+v, %v", updated, stub, err)
	}
	stub.records[childID] = `{"Ref_Key":"` + childID + `","Description":"Child","Parent_Key":"` + groupID + `","IsFolder":false,"DeletionMark":false,"DataVersion":"v2"}`
	if _, err := svc.UpdateCounterpartyGroup(context.Background(), childID, CounterpartyGroupPatch{Name: textPointer("Wrong")}); err == nil || stub.writes != 2 {
		t.Fatal("updated a customer as a folder")
	}
	if _, err := svc.CreateCounterpartyGroup(context.Background(), "Wrong", childID); err == nil || stub.writes != 2 {
		t.Fatal("created a folder under a customer")
	}
}

func TestCounterpartyGroupMoveRejectsCycles(t *testing.T) {
	stub := &counterpartyGroupStub{records: map[string]string{
		groupID: `{"Ref_Key":"` + groupID + `","Description":"Root","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v1"}`,
		childID: `{"Ref_Key":"` + childID + `","Description":"Child","Parent_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v2"}`,
	}}
	svc := Service{OData: stub}
	for _, parent := range []string{groupID, childID, "bad-guid", ""} {
		if _, err := svc.UpdateCounterpartyGroup(context.Background(), groupID, CounterpartyGroupPatch{ParentID: &parent}); err == nil || stub.writes != 0 {
			t.Fatalf("accepted invalid parent %q: %v", parent, err)
		}
	}
	unchanged, err := svc.UpdateCounterpartyGroup(context.Background(), groupID, CounterpartyGroupPatch{ParentID: textPointer("root")})
	if err != nil || unchanged.Applied || stub.writes != 0 {
		t.Fatalf("unchanged folder wrote data: %+v, %v", unchanged, err)
	}
}
