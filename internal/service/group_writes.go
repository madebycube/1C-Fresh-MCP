package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const emptyGUID = "00000000-0000-0000-0000-000000000000"

type odataWriter interface {
	Write(context.Context, string, string, []byte, string) ([]byte, error)
}

type GroupChange struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id,omitempty"`
	Applied  bool   `json:"applied"`
}

type groupRecord struct {
	ID          string `json:"Ref_Key"`
	Name        string `json:"Description"`
	ParentID    string `json:"Parent_Key"`
	IsFolder    bool   `json:"IsFolder"`
	Deleted     bool   `json:"DeletionMark"`
	DataVersion string `json:"DataVersion"`
}

func (s Service) CreateGroup(ctx context.Context, name, parentID string) (GroupChange, error) {
	name, err := groupName(name)
	if err != nil {
		return GroupChange{}, err
	}
	if parentID == "" {
		parentID = emptyGUID
	}
	if !guidPattern.MatchString(parentID) {
		return GroupChange{}, errors.New("parent ID must be a GUID")
	}
	if !strings.EqualFold(parentID, emptyGUID) {
		parent, err := s.readGroup(ctx, parentID)
		if err != nil {
			return GroupChange{}, err
		}
		if !strings.EqualFold(parent.ID, parentID) || !parent.IsFolder || parent.Deleted {
			return GroupChange{}, errors.New("parent must be an active product group")
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return GroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]any{
		"Ref_Key": emptyGUID, "Description": name, "IsFolder": true, "Parent_Key": parentID,
	})
	response, err := writer.Write(ctx, http.MethodPost, config.ProductGroups.Name, body, "")
	if err != nil {
		return GroupChange{}, err
	}
	change := GroupChange{Name: name, ParentID: parentID, Applied: true}
	if len(response) > 0 {
		var created groupRecord
		if json.Unmarshal(response, &created) == nil && guidPattern.MatchString(created.ID) {
			change.ID = created.ID
		}
	}
	return change, nil
}

func (s Service) UpdateGroup(ctx context.Context, id, name string) (GroupChange, error) {
	if !guidPattern.MatchString(id) || strings.EqualFold(id, emptyGUID) {
		return GroupChange{}, errors.New("group ID must be a nonzero GUID")
	}
	name, err := groupName(name)
	if err != nil {
		return GroupChange{}, err
	}
	current, err := s.readGroup(ctx, id)
	if err != nil {
		return GroupChange{}, err
	}
	if !current.IsFolder || current.Deleted || !strings.EqualFold(current.ID, id) {
		return GroupChange{}, errors.New("ID does not identify an active product group")
	}
	if current.DataVersion == "" {
		return GroupChange{}, errors.New("group has no data version for a safe update")
	}
	if current.Name == name {
		return GroupChange{ID: id, Name: name, ParentID: current.ParentID, Applied: false}, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return GroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]string{"Description": name})
	_, err = writer.Write(ctx, http.MethodPatch, nomenclatureResource(id), body, current.DataVersion)
	if err != nil {
		return GroupChange{}, err
	}
	return GroupChange{ID: id, Name: name, ParentID: current.ParentID, Applied: true}, nil
}

func (s Service) readGroup(ctx context.Context, id string) (groupRecord, error) {
	if !guidPattern.MatchString(id) || strings.EqualFold(id, emptyGUID) {
		return groupRecord{}, errors.New("group ID must be a nonzero GUID")
	}
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,Parent_Key,IsFolder,DeletionMark,DataVersion"}}
	data, err := s.OData.Get(ctx, nomenclatureResource(id), params, 1<<20)
	if err != nil {
		return groupRecord{}, err
	}
	var record groupRecord
	if err := json.Unmarshal(data, &record); err != nil || record.ID == "" {
		return groupRecord{}, errors.New("invalid OData product group response")
	}
	return record, nil
}

func nomenclatureResource(id string) string {
	return fmt.Sprintf("%s(guid'%s')", config.ProductGroups.Name, id)
}

func groupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 120 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", errors.New("group name must be 1–120 characters without control characters")
	}
	return name, nil
}
