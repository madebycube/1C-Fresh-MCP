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

const groupID = "11111111-1111-1111-1111-111111111111"
const childID = "22222222-2222-2222-2222-222222222222"

type groupWriteStub struct {
	row       string
	method    string
	resource  string
	body      map[string]any
	ifMatch   string
	writeErr  error
	writeHits int
}

func (*groupWriteStub) Check(context.Context) (int, error) { return 1, nil }
func (stub *groupWriteStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	if !strings.Contains(resource, "(guid'") {
		return nil, errors.New("unexpected read")
	}
	return []byte(stub.row), nil
}
func (stub *groupWriteStub) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	stub.writeHits++
	stub.method, stub.resource, stub.ifMatch = method, resource, ifMatch
	if err := json.Unmarshal(body, &stub.body); err != nil {
		return nil, err
	}
	if stub.writeErr != nil {
		return nil, stub.writeErr
	}
	return []byte(`{"Ref_Key":"` + childID + `"}`), nil
}

func TestCreateGroupUsesFolderPayloadAndValidatesParent(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Parent","IsFolder":true,"DeletionMark":false}`}
	change, err := (Service{OData: stub}).CreateGroup(context.Background(), "New group", groupID)
	if err != nil {
		t.Fatal(err)
	}
	if change.ID != childID || !change.Applied || stub.method != http.MethodPost || stub.resource != "Catalog_Номенклатура" || stub.body["IsFolder"] != true || stub.body["Parent_Key"] != groupID || stub.body["Ref_Key"] != emptyGUID {
		t.Fatalf("unexpected write: change=%+v stub=%+v", change, stub)
	}
	stub.row = `{"Ref_Key":"` + groupID + `","Description":"Product","IsFolder":false,"DeletionMark":false}`
	if _, err := (Service{OData: stub}).CreateGroup(context.Background(), "Invalid child", groupID); err == nil || stub.writeHits != 1 {
		t.Fatal("created group under a product")
	}
}

func TestCreateRootGroupUsesEmptyParent(t *testing.T) {
	stub := &groupWriteStub{}
	change, err := (Service{OData: stub}).CreateGroup(context.Background(), "Root", "")
	if err != nil || change.ParentID != emptyGUID || stub.body["Parent_Key"] != emptyGUID {
		t.Fatalf("unexpected root group: change=%+v err=%v body=%+v", change, err, stub.body)
	}
}

func TestUpdateGroupUsesDataVersionAndRejectsProducts(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Old","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"AAAAAQ=="}`}
	change, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{Name: textPointer("New")})
	if err != nil {
		t.Fatal(err)
	}
	if !change.Applied || stub.method != http.MethodPatch || stub.resource != "Catalog_Номенклатура(guid'"+groupID+"')" || stub.ifMatch != "AAAAAQ==" || len(stub.body) != 1 || stub.body["Description"] != "New" {
		t.Fatalf("unexpected update: change=%+v stub=%+v", change, stub)
	}
	stub.row = `{"Ref_Key":"` + groupID + `","Description":"Old","IsFolder":false,"DataVersion":"AAAAAQ=="}`
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{Name: textPointer("New")}); err == nil || stub.writeHits != 1 {
		t.Fatal("updated a product as a group")
	}
}

func TestUpdateGroupSkipsUnchangedNameAndRejectsInvalidID(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Same","IsFolder":true,"DataVersion":"v1"}`}
	change, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{Name: textPointer("Same")})
	if err != nil || change.Applied || stub.writeHits != 0 {
		t.Fatalf("unchanged group wrote data: %+v, %v", change, err)
	}
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), "../../etc/passwd", GroupPatch{Name: textPointer("New")}); err == nil {
		t.Fatal("invalid ID accepted")
	}
}

type groupMoveStub struct {
	records map[string]string
	writes  int
	body    map[string]any
	ifMatch string
}

func (*groupMoveStub) Check(context.Context) (int, error) { return 1, nil }

func (stub *groupMoveStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(resource, "Catalog_Номенклатура(guid'"), "')")
	row, ok := stub.records[id]
	if !ok {
		return nil, errors.New("unknown group")
	}
	return []byte(row), nil
}

func (stub *groupMoveStub) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	if method != http.MethodPatch || resource != nomenclatureResource(groupID) {
		return nil, errors.New("unexpected group write")
	}
	stub.writes++
	stub.ifMatch = ifMatch
	return nil, json.Unmarshal(body, &stub.body)
}

func TestUpdateGroupMovesAndSkipsUnchangedParent(t *testing.T) {
	stub := &groupMoveStub{records: map[string]string{
		groupID: `{"Ref_Key":"` + groupID + `","Description":"Moved","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v2"}`,
		childID: `{"Ref_Key":"` + childID + `","Description":"Destination","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false}`,
	}}
	change, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{ParentID: textPointer(childID)})
	if err != nil || !change.Applied || change.ParentID != childID || stub.writes != 1 || stub.ifMatch != "v2" || len(stub.body) != 1 || stub.body["Parent_Key"] != childID {
		t.Fatalf("group move: %+v, %+v, %v", change, stub, err)
	}
	stub.records[groupID] = `{"Ref_Key":"` + groupID + `","Description":"Moved","Parent_Key":"` + childID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v3"}`
	change, err = (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{ParentID: textPointer(childID)})
	if err != nil || change.Applied || stub.writes != 1 {
		t.Fatalf("unchanged parent: %+v, %v", change, err)
	}
	change, err = (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{ParentID: textPointer("root")})
	if err != nil || !change.Applied || change.ParentID != emptyGUID || stub.writes != 2 || stub.body["Parent_Key"] != emptyGUID {
		t.Fatalf("move to root: %+v, %+v, %v", change, stub, err)
	}
}

func TestUpdateGroupRejectsCyclesAndInvalidParents(t *testing.T) {
	stub := &groupMoveStub{records: map[string]string{
		groupID: `{"Ref_Key":"` + groupID + `","Description":"Parent","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":false,"DataVersion":"v2"}`,
		childID: `{"Ref_Key":"` + childID + `","Description":"Child","Parent_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false}`,
	}}
	for _, destination := range []string{groupID, childID, "", "not-a-guid"} {
		if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{ParentID: &destination}); err == nil || stub.writes != 0 {
			t.Fatalf("accepted cyclic or invalid destination %q", destination)
		}
	}
	stub.records[childID] = `{"Ref_Key":"` + childID + `","Description":"Deleted","Parent_Key":"` + emptyGUID + `","IsFolder":true,"DeletionMark":true}`
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{ParentID: textPointer(childID)}); err == nil || stub.writes != 0 {
		t.Fatal("moved under a deleted group")
	}
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, GroupPatch{}); err == nil || stub.writes != 0 {
		t.Fatal("accepted an empty update")
	}
}
