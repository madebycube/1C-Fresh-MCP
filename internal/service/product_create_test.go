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

type productCreateStub struct {
	unitDeleted bool
	groupRow    string
	response    []byte
	writeErr    error
	writes      int
	method      string
	resource    string
	body        map[string]any
}

func (*productCreateStub) Check(context.Context) (int, error) { return 1, nil }

func (stub *productCreateStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	switch {
	case resource == "Catalog_КлассификаторЕдиницИзмерения":
		data, _ := json.Marshal(map[string]any{
			"odata.count": "1", "value": []map[string]any{{
				"Ref_Key": childID, "Description": "pc", "DeletionMark": stub.unitDeleted,
			}},
		})
		return data, nil
	case strings.HasPrefix(resource, "Catalog_Номенклатура(guid'"):
		return []byte(stub.groupRow), nil
	default:
		return nil, errors.New("unexpected read")
	}
}

func (stub *productCreateStub) Write(_ context.Context, method, resource string, body []byte, _ string) ([]byte, error) {
	stub.writes++
	stub.method, stub.resource = method, resource
	if err := json.Unmarshal(body, &stub.body); err != nil {
		return nil, err
	}
	return stub.response, stub.writeErr
}

func TestCreateProductLinksActiveClassifierUnitAndGroup(t *testing.T) {
	stub := &productCreateStub{
		groupRow: `{"Ref_Key":"` + groupID + `","Description":"Furniture","IsFolder":true,"DeletionMark":false}`,
		response: []byte(`{"Ref_Key":"33333333-3333-3333-3333-333333333333"}`),
	}
	created, err := (Service{OData: stub}).CreateProduct(context.Background(), ProductCreate{
		Name: "Chair", Article: "A1", Type: "stock", UnitID: childID, GroupID: groupID,
	})
	if err != nil || !created.Applied || created.ID != "33333333-3333-3333-3333-333333333333" || created.FullName != "Chair" || stub.writes != 1 || stub.method != http.MethodPost || stub.resource != "Catalog_Номенклатура" {
		t.Fatalf("product creation: %+v, %+v, %v", created, stub, err)
	}
	if stub.body["ТипНоменклатуры"] != "Запас" || stub.body["ЕдиницаИзмерения_Key"] != childID || stub.body["Parent_Key"] != groupID || stub.body["IsFolder"] != false {
		t.Fatalf("product payload: %+v", stub.body)
	}
}

func TestCreateServiceUsesRootAndServiceType(t *testing.T) {
	stub := &productCreateStub{response: []byte(`{"Ref_Key":"33333333-3333-3333-3333-333333333333"}`)}
	created, err := (Service{OData: stub}).CreateProduct(context.Background(), ProductCreate{
		Name: "Assembly", Type: "service", UnitID: childID,
	})
	if err != nil || created.Type != "service" || created.GroupID != emptyGUID || stub.body["ТипНоменклатуры"] != "Услуга" || stub.body["Parent_Key"] != emptyGUID {
		t.Fatalf("service creation: %+v, %+v, %v", created, stub, err)
	}
}

func TestCreateProductRejectsInvalidUnitAndGroup(t *testing.T) {
	stub := &productCreateStub{unitDeleted: true}
	svc := Service{OData: stub}
	input := ProductCreate{Name: "Service", Type: "service", UnitID: childID}
	if _, err := svc.CreateProduct(context.Background(), input); err == nil || stub.writes != 0 {
		t.Fatal("accepted a deleted classifier unit")
	}
	stub.unitDeleted = false
	stub.groupRow = `{"Ref_Key":"` + groupID + `","Description":"Chair","IsFolder":false,"DeletionMark":false}`
	input.GroupID = groupID
	if _, err := svc.CreateProduct(context.Background(), input); err == nil || stub.writes != 0 {
		t.Fatal("accepted a product as parent")
	}
	input.Type = "unknown"
	if _, err := svc.CreateProduct(context.Background(), input); err == nil || stub.writes != 0 {
		t.Fatal("accepted an unknown product type")
	}
}

func TestCreateProductDoesNotRetryAmbiguousPost(t *testing.T) {
	stub := &productCreateStub{response: []byte(`{}`)}
	input := ProductCreate{Name: "Service", Type: "service", UnitID: childID}
	if _, err := (Service{OData: stub}).CreateProduct(context.Background(), input); err == nil || !strings.Contains(err.Error(), "may have created") || stub.writes != 1 {
		t.Fatalf("unexpected ambiguous response: %v, writes %d", err, stub.writes)
	}
	stub.writes = 0
	stub.writeErr = errors.New("connection lost")
	if _, err := (Service{OData: stub}).CreateProduct(context.Background(), input); err == nil || stub.writes != 1 {
		t.Fatalf("unexpected write failure: %v, writes %d", err, stub.writes)
	}
}
