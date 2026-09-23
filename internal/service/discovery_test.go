package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

const metadataFixture = `<Schema Namespace="StandardODATA">
<EntityType Name="Catalog_ВидыЦен"><Key><PropertyRef Name="Ref_Key"/></Key><Property Name="Ref_Key" Type="Edm.Guid" Nullable="false"/><Property Name="Description" Type="Edm.String"/><NavigationProperty Name="Owner"/></EntityType>
<EntityType Name="Document_ЗаказПокупателя"><Key><PropertyRef Name="Ref_Key"/></Key><Property Name="Ref_Key" Type="Edm.Guid" Nullable="false"/><Property Name="Posted" Type="Edm.Boolean" Nullable="true"/></EntityType>
<EntityContainer><EntitySet Name="Catalog_ВидыЦен" EntityType="StandardODATA.Catalog_ВидыЦен"/><EntitySet Name="Document_ЗаказПокупателя" EntityType="StandardODATA.Document_ЗаказПокупателя"/></EntityContainer>
</Schema>`

type metadataReader struct{ calls int }

func (r *metadataReader) Check(context.Context) (int, error) { return 2, nil }
func (r *metadataReader) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	r.calls++
	if resource != "$metadata" {
		return nil, nil
	}
	return []byte(metadataFixture), nil
}

func TestResourceDiscoveryAndDescription(t *testing.T) {
	reader := &metadataReader{}
	svc := Service{OData: reader}
	search, err := svc.SearchResources(context.Background(), "цен", "catalog", 1)
	if err != nil {
		t.Fatal(err)
	}
	if search.Total != 1 || len(search.Items) != 1 || search.Items[0].Name != "Catalog_ВидыЦен" || search.Items[0].PropertyCount != 2 {
		t.Fatalf("unexpected search: %+v", search)
	}
	detail, err := svc.DescribeResource(context.Background(), "Catalog_ВидыЦен")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Keys) != 1 || detail.Keys[0] != "Ref_Key" || len(detail.Properties) != 2 || detail.Properties[0].Nullable || !detail.Properties[1].Nullable || len(detail.Navigation) != 1 || detail.Navigation[0] != "Owner" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	if _, err := svc.DescribeResource(context.Background(), "Missing"); err == nil {
		t.Fatal("missing resource accepted")
	}
}

func TestResourceDiscoveryRejectsInvalidInputBeforeFetch(t *testing.T) {
	reader := &metadataReader{}
	svc := Service{OData: reader}
	for _, query := range []string{"", "\n", strings.Repeat("x", 121)} {
		if _, err := svc.SearchResources(context.Background(), query, "", 0); err == nil {
			t.Errorf("accepted query %q", query)
		}
	}
	if _, err := svc.SearchResources(context.Background(), "цен", "wrong", 0); err == nil {
		t.Fatal("accepted invalid kind")
	}
	if _, err := svc.SearchResources(context.Background(), "цен", "", 201); err == nil {
		t.Fatal("accepted invalid limit")
	}
	if _, err := svc.DescribeResource(context.Background(), "../Catalog_ВидыЦен"); err == nil {
		t.Fatal("accepted invalid resource name")
	}
	if reader.calls != 0 {
		t.Fatal("invalid input reached OData")
	}
}

func TestResourceDiscoveryRejectsBrokenMetadata(t *testing.T) {
	for _, data := range []string{"<bad", `<Schema Namespace="StandardODATA"><EntityContainer><EntitySet Name="X" EntityType="StandardODATA.Missing"/></EntityContainer></Schema>`} {
		if _, err := parseResources([]byte(data)); err == nil {
			t.Errorf("accepted broken metadata %q", data)
		}
	}
}
