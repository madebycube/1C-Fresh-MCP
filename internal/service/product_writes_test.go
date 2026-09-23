package service

import (
	"context"
	"net/http"
	"testing"
)

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
