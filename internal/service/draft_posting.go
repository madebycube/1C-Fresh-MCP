package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type DraftPosting struct {
	Kind        string   `json:"kind"`
	ID          string   `json:"id"`
	Number      string   `json:"number"`
	Action      string   `json:"action"`
	Applied     bool     `json:"applied"`
	PostedFrom  bool     `json:"posted_before"`
	Posted      bool     `json:"posted"`
	DataVersion string   `json:"data_version"`
	Lines       int      `json:"lines"`
	Impact      []string `json:"impact"`
}

type postableDraft struct {
	resource string
	label    string
	post     string
	unpost   string
	check    func(map[string]json.RawMessage) error
}

// postableDrafts lists the documents the CLI creates unposted and may post: stock transfers and price-setting documents.
var postableDrafts = map[string]postableDraft{
	"transfer": {
		resource: transferResourceName, label: "transfer",
		post:   "moves the listed stock between the two warehouses in the stock registers",
		unpost: "removes the transfer's stock movements until it is posted again",
		check: func(row map[string]json.RawMessage) error {
			if rawString(row["ВидОперации"]) != transferOperation {
				return unsupported("only warehouse-to-warehouse transfers can be posted")
			}
			return nil
		},
	},
	"price": {
		resource: config.PriceDocuments.Name, label: "price document",
		post:   "makes the listed prices current from the document date",
		unpost: "withdraws the document's prices; earlier prices apply again",
	},
}

// SetDraftPosting previews and, with apply, posts or unposts a transfer or price-setting document.
func (s Service) SetDraftPosting(ctx context.Context, kind, id, action, expectedDataVersion string, apply bool) (DraftPosting, error) {
	draft, ok := postableDrafts[kind]
	if !ok {
		return DraftPosting{}, errors.New("kind must be transfer or price")
	}
	if action != "post" && action != "unpost" {
		return DraftPosting{}, errors.New("action must be post or unpost")
	}
	if !linkedGUID(id) {
		return DraftPosting{}, errors.New(draft.label + " ID must be a GUID")
	}
	current, err := s.readPostableDraft(ctx, draft, id)
	if err != nil {
		return DraftPosting{}, err
	}
	current.Kind, current.Action = kind, action
	current.Impact = []string{draft.post}
	if action == "unpost" {
		current.Impact = []string{draft.unpost}
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return DraftPosting{}, errors.New(draft.label + " changed since the preview; preview again")
	}
	if !apply || current.Posted == (action == "post") {
		return current, nil
	}
	actor, ok := s.OData.(documentActor)
	if !ok {
		return DraftPosting{}, errors.New("OData client does not support document actions")
	}
	name := "Post"
	if action == "unpost" {
		name = "Unpost"
	}
	if _, err := actor.Action(ctx, draft.resource+"(guid'"+current.ID+"')", name); err != nil {
		return DraftPosting{}, fmt.Errorf("1C rejected %s; read the %s before retrying: %w", action, draft.label, err)
	}
	saved, err := s.readPostableDraft(ctx, draft, current.ID)
	if err != nil {
		return DraftPosting{}, fmt.Errorf("%s was sent but the %s could not be read back: %w", action, draft.label, err)
	}
	if saved.Posted != (action == "post") {
		return DraftPosting{}, fmt.Errorf("%s was accepted but the %s posting flag did not change", action, draft.label)
	}
	current.Applied, current.Posted, current.DataVersion = true, saved.Posted, saved.DataVersion
	return current, nil
}

func (s Service) readPostableDraft(ctx context.Context, draft postableDraft, id string) (DraftPosting, error) {
	id = strings.ToLower(id)
	data, err := s.OData.Get(ctx, draft.resource+"(guid'"+id+"')", url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return DraftPosting{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return DraftPosting{}, errors.New("invalid OData " + draft.label + " response")
	}
	var deleted, posted bool
	if json.Unmarshal(row["DeletionMark"], &deleted) != nil || json.Unmarshal(row["Posted"], &posted) != nil {
		return DraftPosting{}, errors.New("invalid OData " + draft.label + " response")
	}
	if deleted {
		return DraftPosting{}, errors.New(draft.label + " is marked for deletion")
	}
	if draft.check != nil {
		if err := draft.check(row); err != nil {
			return DraftPosting{}, err
		}
	}
	var lines []json.RawMessage
	_ = json.Unmarshal(row["Запасы"], &lines)
	result := DraftPosting{ID: id, Number: rawString(row["Number"]), PostedFrom: posted, Posted: posted, DataVersion: rawString(row["DataVersion"]), Lines: len(lines)}
	return result, nil
}
