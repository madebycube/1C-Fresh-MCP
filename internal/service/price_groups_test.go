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

const priceGroupA = "00000000-0000-0000-0000-00000000000a"
const priceGroupB = "00000000-0000-0000-0000-00000000000b"

type priceGroupStub struct {
	list     string
	record   string
	product  string
	method   string
	resource string
	body     map[string]any
	ifMatch  string
	writes   int
}

func (*priceGroupStub) Check(context.Context) (int, error) { return 1, nil }

func (stub *priceGroupStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	switch {
	case resource == "Catalog_ЦеновыеГруппы":
		return []byte(stub.list), nil
	case strings.HasPrefix(resource, "Catalog_ЦеновыеГруппы(guid'"):
		return []byte(stub.record), nil
	case strings.HasPrefix(resource, "Catalog_Номенклатура(guid'"):
		return []byte(stub.product), nil
	}
	return nil, errors.New("unexpected read " + resource)
}

func (stub *priceGroupStub) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	stub.writes++
	stub.method, stub.resource, stub.ifMatch = method, resource, ifMatch
	if err := json.Unmarshal(body, &stub.body); err != nil {
		return nil, err
	}
	return []byte(`{"Ref_Key":"` + priceGroupB + `"}`), nil
}

const priceGroupList = `{"odata.count":"3","value":[
{"Ref_Key":"` + priceGroupA + `","Code":"001","Description":"Мебель","Parent_Key":"00000000-0000-0000-0000-000000000000","DeletionMark":false},
{"Ref_Key":"00000000-0000-0000-0000-00000000000c","Code":"002","Description":"Старая","Parent_Key":"00000000-0000-0000-0000-000000000000","DeletionMark":true},
{"Ref_Key":"00000000-0000-0000-0000-00000000000d","Code":"003","Description":"Аксессуары","Parent_Key":"00000000-0000-0000-0000-000000000000","DeletionMark":false}]}`

func TestListPriceGroupsHidesDeletedAndFilters(t *testing.T) {
	stub := &priceGroupStub{list: priceGroupList}
	groups, err := (Service{OData: stub}).ListPriceGroups(context.Background(), "", false)
	if err != nil || len(groups) != 2 || groups[0].Name != "Аксессуары" || groups[1].ID != priceGroupA {
		t.Fatalf("groups=%+v err=%v", groups, err)
	}
	groups, err = (Service{OData: stub}).ListPriceGroups(context.Background(), "мебел", true)
	if err != nil || len(groups) != 1 || groups[0].Code != "001" {
		t.Fatalf("filtered groups=%+v err=%v", groups, err)
	}
	groups, _ = (Service{OData: stub}).ListPriceGroups(context.Background(), "", true)
	if len(groups) != 3 {
		t.Fatalf("deleted group missing: %+v", groups)
	}
}

func TestCreatePriceGroupPostsNameAndRejectsDuplicates(t *testing.T) {
	stub := &priceGroupStub{list: priceGroupList}
	change, err := (Service{OData: stub}).CreatePriceGroup(context.Background(), " Освещение ")
	if err != nil || !change.Applied || change.ID != priceGroupB || stub.method != http.MethodPost || stub.resource != "Catalog_ЦеновыеГруппы" || stub.body["Description"] != "Освещение" || len(stub.body) != 2 {
		t.Fatalf("change=%+v err=%v stub=%+v", change, err, stub)
	}
	if _, err := (Service{OData: stub}).CreatePriceGroup(context.Background(), "мебель"); err == nil || stub.writes != 1 {
		t.Fatal("created a duplicate price group")
	}
	if _, err := (Service{OData: stub}).CreatePriceGroup(context.Background(), " "); err == nil || stub.writes != 1 {
		t.Fatal("accepted empty name")
	}
}

func TestUpdatePriceGroupRenamesWithDataVersion(t *testing.T) {
	stub := &priceGroupStub{record: `{"Ref_Key":"` + priceGroupA + `","Description":"Мебель","DeletionMark":false,"DataVersion":"v7"}`}
	change, err := (Service{OData: stub}).UpdatePriceGroup(context.Background(), priceGroupA, "Мебель для дома")
	if err != nil || !change.Applied || stub.method != http.MethodPatch || stub.ifMatch != "v7" || stub.resource != priceGroupResource(priceGroupA) || stub.body["Description"] != "Мебель для дома" || len(stub.body) != 1 {
		t.Fatalf("change=%+v err=%v stub=%+v", change, err, stub)
	}
	change, err = (Service{OData: stub}).UpdatePriceGroup(context.Background(), priceGroupA, "Мебель")
	if err != nil || change.Applied || stub.writes != 1 {
		t.Fatalf("unchanged name wrote data: %+v %v", change, err)
	}
	stub.record = `{"Ref_Key":"` + priceGroupA + `","Description":"Мебель","DeletionMark":true,"DataVersion":"v7"}`
	if _, err := (Service{OData: stub}).UpdatePriceGroup(context.Background(), priceGroupA, "Новое"); err == nil || stub.writes != 1 {
		t.Fatal("renamed a deleted price group")
	}
	stub.record = `{"Ref_Key":"` + priceGroupA + `","Description":"Мебель","DeletionMark":false,"DataVersion":""}`
	if _, err := (Service{OData: stub}).UpdatePriceGroup(context.Background(), priceGroupA, "Новое"); err == nil || stub.writes != 1 {
		t.Fatal("updated without data version")
	}
}

func TestUpdateProductSetsAndClearsPriceGroup(t *testing.T) {
	stub := &priceGroupStub{
		record:  `{"Ref_Key":"` + priceGroupA + `","Description":"Мебель","DeletionMark":false,"DataVersion":"v1"}`,
		product: `{"Ref_Key":"` + childID + `","Description":"Chair","IsFolder":false,"DeletionMark":false,"DataVersion":"p1","ЦеноваяГруппа_Key":"00000000-0000-0000-0000-000000000000"}`,
	}
	change, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{PriceGroupID: textPointer(strings.ToUpper(priceGroupA))})
	if err != nil || !change.Applied || change.PriceGroupID != priceGroupA || stub.ifMatch != "p1" || len(stub.body) != 1 || stub.body["ЦеноваяГруппа_Key"] != priceGroupA {
		t.Fatalf("change=%+v err=%v body=%+v", change, err, stub.body)
	}
	change, err = (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{PriceGroupID: textPointer("none")})
	if err != nil || change.Applied || stub.writes != 1 {
		t.Fatalf("clearing an empty price group wrote data: %+v %v", change, err)
	}
	stub.product = strings.Replace(stub.product, `"ЦеноваяГруппа_Key":"00000000-0000-0000-0000-000000000000"`, `"ЦеноваяГруппа_Key":"`+priceGroupA+`"`, 1)
	change, err = (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{PriceGroupID: textPointer("none")})
	if err != nil || !change.Applied || stub.body["ЦеноваяГруппа_Key"] != emptyGUID {
		t.Fatalf("price group not cleared: %+v %v %+v", change, err, stub.body)
	}
	stub.record = `{"Ref_Key":"` + priceGroupA + `","Description":"Мебель","DeletionMark":true,"DataVersion":"v1"}`
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{PriceGroupID: textPointer(priceGroupA)}); err == nil || stub.writes != 2 {
		t.Fatal("assigned a deleted price group")
	}
	if _, err := (Service{OData: stub}).UpdateProduct(context.Background(), childID, ProductPatch{PriceGroupID: textPointer("bad")}); err == nil {
		t.Fatal("accepted a non-GUID price group")
	}
}
