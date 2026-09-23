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
	change, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, "New")
	if err != nil {
		t.Fatal(err)
	}
	if !change.Applied || stub.method != http.MethodPatch || stub.resource != "Catalog_Номенклатура(guid'"+groupID+"')" || stub.ifMatch != "AAAAAQ==" || len(stub.body) != 1 || stub.body["Description"] != "New" {
		t.Fatalf("unexpected update: change=%+v stub=%+v", change, stub)
	}
	stub.row = `{"Ref_Key":"` + groupID + `","Description":"Old","IsFolder":false,"DataVersion":"AAAAAQ=="}`
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, "New"); err == nil || stub.writeHits != 1 {
		t.Fatal("updated a product as a group")
	}
}

func TestUpdateGroupSkipsUnchangedNameAndRejectsInvalidID(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Same","IsFolder":true,"DataVersion":"v1"}`}
	change, err := (Service{OData: stub}).UpdateGroup(context.Background(), groupID, "Same")
	if err != nil || change.Applied || stub.writeHits != 0 {
		t.Fatalf("unchanged group wrote data: %+v, %v", change, err)
	}
	if _, err := (Service{OData: stub}).UpdateGroup(context.Background(), "../../etc/passwd", "New"); err == nil {
		t.Fatal("invalid ID accepted")
	}
}
