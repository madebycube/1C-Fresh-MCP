package service

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

const markTestID = "00000000-0000-0000-0000-000000000020"

type markStub struct {
	body   string
	writes *[]string
}

func (markStub) Check(context.Context) (int, error) { return 1, nil }

func (m markStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	body := m.body
	if len(*m.writes) > 0 {
		var patch map[string]bool
		_ = json.Unmarshal([]byte(strings.SplitN((*m.writes)[len(*m.writes)-1], " ", 4)[3]), &patch)
		if patch["DeletionMark"] {
			body = strings.Replace(body, `"DeletionMark":false`, `"DeletionMark":true`, 1)
		} else {
			body = strings.Replace(body, `"DeletionMark":true`, `"DeletionMark":false`, 1)
		}
	}
	return []byte(body), nil
}

func (m markStub) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	*m.writes = append(*m.writes, method+" "+resource+" "+ifMatch+" "+string(body))
	return nil, nil
}

func TestSetDeletionMarkPatchesOnlyTheMark(t *testing.T) {
	writes := []string{}
	svc := Service{OData: markStub{writes: &writes, body: `{"Ref_Key":"` + markTestID + `","Number":"T1","DataVersion":"v1","Posted":false,"DeletionMark":false}`}}
	preview, err := svc.SetDeletionMark(context.Background(), "transfer", markTestID, true, "", false)
	if err != nil || preview.Applied || preview.MarkedFrom || preview.Label != "T1" || len(writes) != 0 {
		t.Fatalf("preview: %+v, %v, %v", preview, err, writes)
	}
	done, err := svc.SetDeletionMark(context.Background(), "transfer", markTestID, true, "v1", true)
	want := "PATCH Document_ПеремещениеЗапасов(guid'" + markTestID + "') v1 {\"DeletionMark\":true}"
	if err != nil || !done.Applied || !done.Marked || len(writes) != 1 || writes[0] != want {
		t.Fatalf("mark: %+v, %v, %v", done, err, writes)
	}
	writes = writes[:0]
	product := Service{OData: markStub{writes: &writes, body: `{"Ref_Key":"` + markTestID + `","Description":"Пример товара","DataVersion":"v3","IsFolder":false,"DeletionMark":true}`}}
	cleared, err := product.SetDeletionMark(context.Background(), "product", markTestID, false, "", true)
	if err != nil || !cleared.Applied || cleared.Marked || len(writes) != 1 || !strings.HasPrefix(writes[0], "PATCH Catalog_Номенклатура(") {
		t.Fatalf("clear: %+v, %v, %v", cleared, err, writes)
	}
}

func TestSetDeletionMarkRefusesUnsafeTargets(t *testing.T) {
	writes := []string{}
	cases := []struct {
		kind, id, body string
		mark           bool
		version        string
	}{
		{"sale", markTestID, `{}`, true, ""},
		{"transfer", "bad", `{}`, true, ""},
		{"transfer", markTestID, `{"Ref_Key":"` + markTestID + `","DataVersion":"v1","Posted":true,"DeletionMark":false}`, true, ""},
		{"transfer", markTestID, `{"Ref_Key":"` + markTestID + `","DataVersion":"v1","Posted":false,"DeletionMark":false}`, true, "old"},
		{"transfer", markTestID, `{"Ref_Key":"` + markTestID + `","Posted":false,"DeletionMark":false}`, true, ""},
		{"product", markTestID, `{"Ref_Key":"` + markTestID + `","DataVersion":"v1","IsFolder":true,"DeletionMark":false}`, true, ""},
		{"counterparty", markTestID, `{"Ref_Key":"00000000-0000-0000-0000-000000000099","DataVersion":"v1","DeletionMark":false}`, true, ""},
	}
	for _, c := range cases {
		svc := Service{OData: markStub{writes: &writes, body: c.body}}
		if _, err := svc.SetDeletionMark(context.Background(), c.kind, c.id, c.mark, c.version, true); err == nil {
			t.Fatalf("expected error for %+v", c)
		}
	}
	if len(writes) != 0 {
		t.Fatalf("refused cases wrote: %v", writes)
	}
}
