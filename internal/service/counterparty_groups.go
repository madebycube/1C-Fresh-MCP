package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type CounterpartyGroup struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Deleted  bool   `json:"deleted"`
	IsFolder bool   `json:"is_folder"`
}

type CounterpartyGroupChange struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Applied  bool   `json:"applied"`
}

type CounterpartyGroupPatch struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}

func (s Service) ListCounterpartyGroups(ctx context.Context, name string) ([]CounterpartyGroup, error) {
	rows, err := s.catalogRows(ctx, config.CounterpartyGroups)
	if err != nil {
		return nil, err
	}
	groups := make([]CounterpartyGroup, 0)
	byID := make(map[string]CounterpartyGroup)
	for _, row := range rows {
		if !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData counterparty folder flags")
		}
		if string(row["IsFolder"]) != "true" {
			continue
		}
		group, err := bindFields[CounterpartyGroup](row, config.CounterpartyGroups.Fields)
		if err != nil || !linkedGUID(group.ID) || group.Name == "" {
			return nil, errors.New("invalid OData counterparty folder")
		}
		groups = append(groups, group)
		byID[strings.ToLower(group.ID)] = group
	}
	query := strings.ToLower(strings.TrimSpace(name))
	selected := make([]CounterpartyGroup, 0, len(groups))
	for _, group := range groups {
		parts := []string{group.Name}
		seen := map[string]bool{strings.ToLower(group.ID): true}
		parentID := group.ParentID
		for linkedGUID(parentID) {
			key := strings.ToLower(parentID)
			if seen[key] {
				return nil, errors.New("cycle in OData counterparty folders")
			}
			seen[key] = true
			parent, ok := byID[key]
			if !ok {
				break
			}
			parts = append([]string{parent.Name}, parts...)
			parentID = parent.ParentID
		}
		group.Path = strings.Join(parts, " / ")
		if query == "" || strings.Contains(strings.ToLower(group.Path), query) {
			selected = append(selected, group)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		if selected[i].Path == selected[j].Path {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].Path < selected[j].Path
	})
	return selected, nil
}

func (s Service) CreateCounterpartyGroup(ctx context.Context, name, parentID string) (CounterpartyGroupChange, error) {
	name, err := groupName(name)
	if err != nil {
		return CounterpartyGroupChange{}, err
	}
	if parentID == "" || parentID == "root" {
		parentID = emptyGUID
	}
	if !guidPattern.MatchString(parentID) {
		return CounterpartyGroupChange{}, errors.New("parent ID must be a GUID or root")
	}
	if linkedGUID(parentID) {
		parent, err := s.readCounterpartyRecord(ctx, parentID)
		if err != nil {
			return CounterpartyGroupChange{}, err
		}
		if parent.IsFolder == nil || !*parent.IsFolder || parent.Deleted == nil || *parent.Deleted {
			return CounterpartyGroupChange{}, errors.New("parent must be an active counterparty folder")
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return CounterpartyGroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]any{
		"Ref_Key": emptyGUID, "Description": name, "IsFolder": true, "Parent_Key": parentID,
	})
	response, err := writer.Write(ctx, http.MethodPost, config.CounterpartyGroups.Name, body, "")
	if err != nil {
		return CounterpartyGroupChange{}, err
	}
	change := CounterpartyGroupChange{Name: name, ParentID: strings.ToLower(parentID), Applied: true}
	if len(response) > 0 {
		var created counterpartyRecord
		if json.Unmarshal(response, &created) == nil && linkedGUID(created.ID) {
			change.ID = created.ID
		}
	}
	return change, nil
}

func (s Service) UpdateCounterpartyGroup(ctx context.Context, id string, patch CounterpartyGroupPatch) (CounterpartyGroupChange, error) {
	if !linkedGUID(id) {
		return CounterpartyGroupChange{}, errors.New("counterparty folder ID must be a nonzero GUID")
	}
	if patch.Name == nil && patch.ParentID == nil {
		return CounterpartyGroupChange{}, errors.New("provide a name or parent to update")
	}
	fields := make(map[string]string)
	if patch.Name != nil {
		name, err := groupName(*patch.Name)
		if err != nil {
			return CounterpartyGroupChange{}, err
		}
		fields["Description"] = name
	}
	current, err := s.readCounterpartyRecord(ctx, id)
	if err != nil {
		return CounterpartyGroupChange{}, err
	}
	if current.IsFolder == nil || !*current.IsFolder || current.Deleted == nil || *current.Deleted {
		return CounterpartyGroupChange{}, errors.New("ID does not identify an active counterparty folder")
	}
	if current.DataVersion == "" {
		return CounterpartyGroupChange{}, errors.New("counterparty folder has no data version for a safe update")
	}
	change := CounterpartyGroupChange{ID: current.ID, Name: current.Name, ParentID: current.ParentID}
	if name, ok := fields["Description"]; ok {
		change.Name = name
		if name == current.Name {
			delete(fields, "Description")
		}
	}
	if patch.ParentID != nil {
		parentID := strings.TrimSpace(*patch.ParentID)
		if parentID == "root" || strings.EqualFold(parentID, emptyGUID) {
			parentID = emptyGUID
		} else if !linkedGUID(parentID) {
			return CounterpartyGroupChange{}, errors.New("parent must be a folder GUID or root")
		}
		if linkedGUID(parentID) {
			if err := s.validateCounterpartyGroupParent(ctx, id, parentID); err != nil {
				return CounterpartyGroupChange{}, err
			}
		}
		change.ParentID = strings.ToLower(parentID)
		if !strings.EqualFold(parentID, current.ParentID) && !(current.ParentID == "" && parentID == emptyGUID) {
			fields["Parent_Key"] = change.ParentID
		}
	}
	if len(fields) == 0 {
		return change, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return CounterpartyGroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(fields)
	if _, err := writer.Write(ctx, http.MethodPatch, counterpartyResource(id), body, current.DataVersion); err != nil {
		return CounterpartyGroupChange{}, err
	}
	change.Applied = true
	return change, nil
}

func (s Service) validateCounterpartyGroupParent(ctx context.Context, id, parentID string) error {
	seen := map[string]bool{strings.ToLower(id): true}
	for depth := 0; depth < 100; depth++ {
		key := strings.ToLower(parentID)
		if seen[key] {
			return errors.New("parent would create a counterparty folder cycle")
		}
		seen[key] = true
		parent, err := s.readCounterpartyRecord(ctx, parentID)
		if err != nil {
			return err
		}
		if parent.IsFolder == nil || !*parent.IsFolder || parent.Deleted == nil || *parent.Deleted {
			return errors.New("parent must be an active counterparty folder")
		}
		if parent.ParentID == "" || strings.EqualFold(parent.ParentID, emptyGUID) {
			return nil
		}
		if !linkedGUID(parent.ParentID) {
			return errors.New("invalid counterparty folder parent chain")
		}
		parentID = parent.ParentID
	}
	return errors.New("counterparty folder parent chain is too deep")
}
