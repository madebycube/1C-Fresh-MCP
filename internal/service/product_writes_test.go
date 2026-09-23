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

type productMoveStub struct {
	product string
	group   string
	body    map[string]any
	ifMatch string
	writes  int
}

func (*productMoveStub) Check(context.Context) (int, error) { return 1, nil }

func (stub *productMoveStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	if strings.Contains(resource, "(guid'"+groupID+"')") {
		return []byte(stub.group), nil
	}
	if strings.Contains(resource, "(guid'"+childID+"')") {
		return []byte(stub.product), nil
	}
	return nil, errors.New("unexpected read")
}

func (stub *productMoveStub) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	stub.writes++
	stub.ifMatch = ifMatch
	if method != http.MethodPatch || resource != nomenclatureResource(childID) {
		return nil, errors.New("unexpected write")
	}
	return nil, json.Unmarshal(body, &stub.body)
}

func textPointer(value string) *string { return &value }

func TestUpdateProductPatchesOnlyChangedFields(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Chair","НаименованиеПолное":"Chair full","Артикул":"OLD","IsFolder":false,"DeletionMark":false,"DataVersion":"version-4"}`}
	change, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{Name: textPointer("Chair"), Article: textPointer("NEW")})
	if err != nil {
		t.Fatal(err)
	}
	if !change.Applied || change.Article != "NEW" || stub.method != http.MethodPatch || stub.ifMatch != "version-4" || len(stub.body) != 1 || stub.body["Артикул"] != "NEW" {
		t.Fatalf("unexpected product update: change=%+v stub=%+v", change, stub)
	}
}

func TestUpdateProductCanClearArticle(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Chair","Артикул":"OLD","IsFolder":false,"DeletionMark":false,"DataVersion":"v1"}`}
	change, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{Article: textPointer("")})
	if err != nil || !change.Applied || change.Article != "" || stub.body["Артикул"] != "" {
		t.Fatalf("article was not cleared: change=%+v err=%v body=%+v", change, err, stub.body)
	}
}

func TestUpdateProductDoesNotWriteGroupsOrUnchangedValues(t *testing.T) {
	stub := &groupWriteStub{row: `{"Ref_Key":"` + groupID + `","Description":"Chair","Артикул":"A","IsFolder":true,"DeletionMark":false,"DataVersion":"v1"}`}
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{Name: textPointer("New")}); err == nil || stub.writeHits != 0 {
		t.Fatal("updated a group as a product")
	}
	stub.row = `{"Ref_Key":"` + groupID + `","Description":"Chair","Артикул":"A","IsFolder":false,"DeletionMark":false,"DataVersion":"v1"}`
	change, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{Article: textPointer("A")})
	if err != nil || change.Applied || stub.writeHits != 0 {
		t.Fatalf("unchanged product wrote data: change=%+v err=%v", change, err)
	}
}

func TestUpdateProductRejectsMissingFieldsAndInvalidName(t *testing.T) {
	stub := &groupWriteStub{}
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{}); err == nil {
		t.Fatal("accepted empty patch")
	}
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), groupID, ProductPatch{Name: textPointer(" ")}); err == nil {
		t.Fatal("accepted empty product name")
	}
}

func TestUpdateProductMovesOnlyToActiveGroup(t *testing.T) {
	stub := &productMoveStub{
		product: `{"Ref_Key":"` + childID + `","Description":"Chair","Parent_Key":"` + emptyGUID + `","IsFolder":false,"DeletionMark":false,"DataVersion":"v2"}`,
		group:   `{"Ref_Key":"` + groupID + `","IsFolder":true,"DeletionMark":false}`,
	}
	change, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{GroupID: textPointer(groupID)})
	if err != nil || !change.Applied || change.GroupID != groupID || stub.writes != 1 || stub.ifMatch != "v2" || len(stub.body) != 1 || stub.body["Parent_Key"] != groupID {
		t.Fatalf("product move: %+v, %+v, %v", change, stub, err)
	}
	stub.product = `{"Ref_Key":"` + childID + `","Description":"Chair","Parent_Key":"` + groupID + `","IsFolder":false,"DeletionMark":false,"DataVersion":"v2"}`
	change, err = (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{GroupID: textPointer(groupID)})
	if err != nil || change.Applied || stub.writes != 1 {
		t.Fatalf("unchanged group: %+v, %v", change, err)
	}
	for _, row := range []string{
		`{"Ref_Key":"` + groupID + `","IsFolder":false,"DeletionMark":false}`,
		`{"Ref_Key":"` + groupID + `","IsFolder":true,"DeletionMark":true}`,
	} {
		stub.group = row
		if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{GroupID: textPointer(groupID)}); err == nil || stub.writes != 1 {
			t.Fatal("accepted product or deleted group as destination")
		}
	}
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{GroupID: textPointer("")}); err == nil || stub.writes != 1 {
		t.Fatal("accepted an empty destination group")
	}
}
