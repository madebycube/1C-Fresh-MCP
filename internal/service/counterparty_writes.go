package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type CounterpartyPatch struct {
	Name     *string `json:"name,omitempty"`
	FullName *string `json:"full_name,omitempty"`
}

type CounterpartyChange struct {
	ID       string `json:"id,omitempty"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	ParentID string `json:"parent_id,omitempty"`
	Applied  bool   `json:"applied"`
}

type counterpartyRecord struct {
	ID          string `json:"Ref_Key"`
	Name        string `json:"Description"`
	FullName    string `json:"НаименованиеПолное"`
	ParentID    string `json:"Parent_Key"`
	IsFolder    *bool  `json:"IsFolder"`
	Deleted     *bool  `json:"DeletionMark"`
	Buyer       *bool  `json:"Покупатель"`
	Supplier    *bool  `json:"Поставщик"`
	DataVersion string `json:"DataVersion"`
}

func (s Service) CreateCounterparty(ctx context.Context, role, name, fullName, parentID string) (CounterpartyChange, error) {
	if err := counterpartyRole(role); err != nil {
		return CounterpartyChange{}, err
	}
	var err error
	if name, err = counterpartyText(name, false, 120); err != nil {
		return CounterpartyChange{}, errors.New("counterparty name must be 1–120 characters without control characters")
	}
	if fullName == "" {
		fullName = name
	}
	if fullName, err = counterpartyText(fullName, false, 4096); err != nil {
		return CounterpartyChange{}, errors.New("invalid counterparty full name")
	}
	if parentID == "" {
		parentID = emptyGUID
	}
	if !guidPattern.MatchString(parentID) {
		return CounterpartyChange{}, errors.New("parent ID must be a GUID")
	}
	if !strings.EqualFold(parentID, emptyGUID) {
		parent, err := s.readCounterpartyRecord(ctx, parentID)
		if err != nil {
			return CounterpartyChange{}, err
		}
		if parent.IsFolder == nil || !*parent.IsFolder || parent.Deleted == nil || *parent.Deleted {
			return CounterpartyChange{}, errors.New("parent must be an active counterparty folder")
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return CounterpartyChange{}, errors.New("OData client does not support writes")
	}
	fields := map[string]any{
		"Ref_Key": emptyGUID, "Description": name, "НаименованиеПолное": fullName,
		"IsFolder": false, "Parent_Key": parentID,
		"Покупатель": role == "customer", "Поставщик": role == "supplier",
	}
	body, _ := json.Marshal(fields)
	response, err := writer.Write(ctx, http.MethodPost, config.Customers.Name, body, "")
	if err != nil {
		return CounterpartyChange{}, err
	}
	change := CounterpartyChange{Role: role, Name: name, FullName: fullName, ParentID: parentID, Applied: true}
	if len(response) > 0 {
		var created counterpartyRecord
		if json.Unmarshal(response, &created) == nil && linkedGUID(created.ID) {
			change.ID = created.ID
		}
	}
	return change, nil
}

func (s Service) UpdateCounterparty(ctx context.Context, role, id string, patch CounterpartyPatch) (CounterpartyChange, error) {
	if err := counterpartyRole(role); err != nil {
		return CounterpartyChange{}, err
	}
	if !linkedGUID(id) {
		return CounterpartyChange{}, errors.New("counterparty ID must be a nonzero GUID")
	}
	if patch.Name == nil && patch.FullName == nil {
		return CounterpartyChange{}, errors.New("provide a name or full name to update")
	}
	fields := make(map[string]string)
	var err error
	if patch.Name != nil {
		if fields["Description"], err = counterpartyText(*patch.Name, false, 120); err != nil {
			return CounterpartyChange{}, errors.New("invalid counterparty name")
		}
	}
	if patch.FullName != nil {
		if fields["НаименованиеПолное"], err = counterpartyText(*patch.FullName, true, 4096); err != nil {
			return CounterpartyChange{}, errors.New("invalid counterparty full name")
		}
	}
	current, err := s.readCounterpartyRecord(ctx, id)
	if err != nil {
		return CounterpartyChange{}, err
	}
	if current.IsFolder == nil || *current.IsFolder || current.Deleted == nil || *current.Deleted || role == "customer" && (current.Buyer == nil || !*current.Buyer) || role == "supplier" && (current.Supplier == nil || !*current.Supplier) {
		return CounterpartyChange{}, errors.New("ID does not identify an active counterparty with that role")
	}
	if current.DataVersion == "" {
		return CounterpartyChange{}, errors.New("counterparty has no data version for a safe update")
	}
	change := CounterpartyChange{ID: current.ID, Role: role, Name: current.Name, FullName: current.FullName, ParentID: current.ParentID}
	if value, ok := fields["Description"]; ok {
		change.Name = value
		if value == current.Name {
			delete(fields, "Description")
		}
	}
	if value, ok := fields["НаименованиеПолное"]; ok {
		change.FullName = value
		if value == current.FullName {
			delete(fields, "НаименованиеПолное")
		}
	}
	if len(fields) == 0 {
		return change, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return CounterpartyChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(fields)
	if _, err := writer.Write(ctx, http.MethodPatch, counterpartyResource(id), body, current.DataVersion); err != nil {
		return CounterpartyChange{}, err
	}
	change.Applied = true
	return change, nil
}

func (s Service) readCounterpartyRecord(ctx context.Context, id string) (counterpartyRecord, error) {
	if !linkedGUID(id) {
		return counterpartyRecord{}, errors.New("counterparty ID must be a nonzero GUID")
	}
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,НаименованиеПолное,Parent_Key,IsFolder,DeletionMark,Покупатель,Поставщик,DataVersion"}}
	data, err := s.OData.Get(ctx, counterpartyResource(id), params, 1<<20)
	if err != nil {
		return counterpartyRecord{}, err
	}
	var record counterpartyRecord
	if err := json.Unmarshal(data, &record); err != nil || !strings.EqualFold(record.ID, id) {
		return counterpartyRecord{}, errors.New("invalid OData counterparty response")
	}
	return record, nil
}

func counterpartyResource(id string) string {
	return config.Customers.Name + "(guid'" + strings.ToLower(id) + "')"
}

func counterpartyRole(role string) error {
	if role != "customer" && role != "supplier" {
		return errors.New("role must be customer or supplier")
	}
	return nil
}

func counterpartyText(value string, allowEmpty bool, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if !allowEmpty && value == "" || utf8.RuneCountInString(value) > limit || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", errors.New("invalid text")
	}
	return value, nil
}
