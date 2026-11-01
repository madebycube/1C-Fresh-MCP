package service

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

const draftTestID = "00000000-0000-0000-0000-000000000010"

type draftActor struct {
	docs    map[string]string
	actions *[]string
}

func (draftActor) Check(context.Context) (int, error) { return 1, nil }

func (a draftActor) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	for prefix, body := range a.docs {
		if strings.HasPrefix(resource, prefix+"(") {
			if len(*a.actions) > 0 {
				body = strings.Replace(body, `"Posted":false`, `"Posted":true`, 1)
			}
			return []byte(body), nil
		}
	}
	return nil, errNotFoundForTest
}

func (a draftActor) Action(_ context.Context, resource, action string) ([]byte, error) {
	*a.actions = append(*a.actions, action+" "+resource)
	return nil, nil
}

var errNotFoundForTest = errors.New("not found")

func TestSetDraftPostingPostsTransfersAndPriceDocuments(t *testing.T) {
	actions := []string{}
	actor := draftActor{actions: &actions, docs: map[string]string{
		"Document_ПеремещениеЗапасов":       `{"Ref_Key":"` + draftTestID + `","Number":"T1","DataVersion":"v1","Posted":false,"DeletionMark":false,"ВидОперации":"Перемещение","Запасы":[{},{}]}`,
		"Document_УстановкаЦенНоменклатуры": `{"Ref_Key":"` + draftTestID + `","Number":"P1","DataVersion":"v2","Posted":false,"DeletionMark":false,"Запасы":[{}]}`,
	}}
	svc := Service{OData: actor}
	preview, err := svc.SetDraftPosting(context.Background(), "transfer", draftTestID, "post", "", false)
	if err != nil || preview.Applied || preview.Lines != 2 || preview.DataVersion != "v1" || len(actions) != 0 {
		t.Fatalf("preview: %+v, %v, %v", preview, err, actions)
	}
	if _, err := svc.SetDraftPosting(context.Background(), "transfer", draftTestID, "post", "stale", true); err == nil || len(actions) != 0 {
		t.Fatalf("stale version must refuse without acting: %v, %v", err, actions)
	}
	done, err := svc.SetDraftPosting(context.Background(), "price", draftTestID, "post", "v2", true)
	if err != nil || !done.Applied || !done.Posted || len(actions) != 1 || actions[0] != "Post Document_УстановкаЦенНоменклатуры(guid'"+draftTestID+"')" {
		t.Fatalf("post: %+v, %v, %v", done, err, actions)
	}
	actions = actions[:0]
	unchanged, err := svc.SetDraftPosting(context.Background(), "price", draftTestID, "unpost", "", true)
	if err != nil || unchanged.Applied || len(actions) != 0 {
		t.Fatalf("unposting an unposted document must not act: %+v, %v, %v", unchanged, err, actions)
	}
}

func TestSetDraftPostingRejectsBadInput(t *testing.T) {
	actions := []string{}
	actor := draftActor{actions: &actions, docs: map[string]string{
		"Document_ПеремещениеЗапасов":       `{"Ref_Key":"` + draftTestID + `","Posted":false,"DeletionMark":false,"ВидОперации":"СписаниеНаРасходы"}`,
		"Document_УстановкаЦенНоменклатуры": `{"Ref_Key":"` + draftTestID + `","Posted":false,"DeletionMark":true}`,
	}}
	svc := Service{OData: actor}
	for _, call := range [][3]string{
		{"receipt", draftTestID, "post"},
		{"transfer", draftTestID, "delete"},
		{"transfer", "not-a-guid", "post"},
		{"transfer", "00000000-0000-0000-0000-000000000000", "post"},
		{"transfer", draftTestID, "post"},
		{"price", draftTestID, "post"},
	} {
		if _, err := svc.SetDraftPosting(context.Background(), call[0], call[1], call[2], "", true); err == nil {
			t.Fatalf("expected error for %v", call)
		}
	}
	if len(actions) != 0 {
		t.Fatalf("invalid input acted: %v", actions)
	}
}
