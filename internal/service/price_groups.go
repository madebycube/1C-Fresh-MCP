package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type PriceGroup struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Deleted  bool   `json:"deleted"`
}

type PriceGroupChange struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Applied bool   `json:"applied"`
}

type priceGroupRecord struct {
	ID          string `json:"Ref_Key"`
	Name        string `json:"Description"`
	Deleted     *bool  `json:"DeletionMark"`
	DataVersion string `json:"DataVersion"`
}

// ListPriceGroups returns price groups (Catalog_ЦеновыеГруппы). The catalog has no folders.
func (s Service) ListPriceGroups(ctx context.Context, name string, includeDeleted bool) ([]PriceGroup, error) {
	rows, err := s.catalogRows(ctx, config.PriceGroups)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(name))
	groups := make([]PriceGroup, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData price group flags")
		}
		group, err := bindFields[PriceGroup](row, config.PriceGroups.Fields)
		if err != nil || !linkedGUID(group.ID) {
			return nil, errors.New("invalid OData price group")
		}
		if group.Deleted && !includeDeleted {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(group.Name), query) {
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name == groups[j].Name {
			return groups[i].ID < groups[j].ID
		}
		return groups[i].Name < groups[j].Name
	})
	return groups, nil
}

func (s Service) CreatePriceGroup(ctx context.Context, name string) (PriceGroupChange, error) {
	name, err := priceGroupName(name)
	if err != nil {
		return PriceGroupChange{}, err
	}
	existing, err := s.ListPriceGroups(ctx, "", false)
	if err != nil {
		return PriceGroupChange{}, err
	}
	for _, group := range existing {
		if strings.EqualFold(group.Name, name) {
			return PriceGroupChange{}, errors.New("an active price group with this name already exists: " + group.ID)
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PriceGroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]any{"Ref_Key": emptyGUID, "Description": name})
	response, err := writer.Write(ctx, http.MethodPost, config.PriceGroups.Name, body, "")
	if err != nil {
		return PriceGroupChange{}, err
	}
	change := PriceGroupChange{Name: name, Applied: true}
	var created priceGroupRecord
	if len(response) > 0 && json.Unmarshal(response, &created) == nil && linkedGUID(created.ID) {
		change.ID = created.ID
	}
	return change, nil
}

func (s Service) UpdatePriceGroup(ctx context.Context, id, name string) (PriceGroupChange, error) {
	if !linkedGUID(id) {
		return PriceGroupChange{}, errors.New("price group ID must be a nonzero GUID")
	}
	name, err := priceGroupName(name)
	if err != nil {
		return PriceGroupChange{}, err
	}
	current, err := s.readPriceGroup(ctx, id)
	if err != nil {
		return PriceGroupChange{}, err
	}
	if current.DataVersion == "" {
		return PriceGroupChange{}, errors.New("price group has no data version for a safe update")
	}
	change := PriceGroupChange{ID: current.ID, Name: name}
	if name == current.Name {
		return change, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PriceGroupChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]string{"Description": name})
	if _, err := writer.Write(ctx, http.MethodPatch, priceGroupResource(id), body, current.DataVersion); err != nil {
		return PriceGroupChange{}, err
	}
	change.Applied = true
	return change, nil
}

func (s Service) readPriceGroup(ctx context.Context, id string) (priceGroupRecord, error) {
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,DeletionMark,DataVersion"}}
	data, err := s.OData.Get(ctx, priceGroupResource(id), params, 1<<20)
	if err != nil {
		return priceGroupRecord{}, err
	}
	var row priceGroupRecord
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(row.ID, id) || row.Deleted == nil || *row.Deleted {
		return priceGroupRecord{}, errors.New("ID does not identify an active price group")
	}
	return row, nil
}

func priceGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", errors.New("price group name must be 1–100 characters without control characters")
	}
	return name, nil
}

func priceGroupResource(id string) string {
	return config.PriceGroups.Name + "(guid'" + strings.ToLower(id) + "')"
}
