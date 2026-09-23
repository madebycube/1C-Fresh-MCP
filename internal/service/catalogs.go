package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const catalogPageSize = 100
const maxCatalogRows = 5000

type Group struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Deleted  bool   `json:"deleted"`
	IsFolder bool   `json:"is_folder"`
}

type PriceType struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Deleted  bool   `json:"deleted"`
	Inactive bool   `json:"inactive"`
}

type UnitType struct {
	ID                        string `json:"id"`
	Code                      string `json:"code"`
	Name                      string `json:"name"`
	FullName                  string `json:"full_name"`
	InternationalAbbreviation string `json:"international_abbreviation"`
	QuantityType              string `json:"quantity_type"`
	Deleted                   bool   `json:"deleted"`
}

func (s Service) ListGroups(ctx context.Context, name string) ([]Group, error) {
	rows, err := s.catalogRows(ctx, config.ProductGroups)
	if err != nil {
		return nil, err
	}
	groups := make([]Group, 0, len(rows))
	byID := make(map[string]Group, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData product group flags")
		}
		group, err := bindFields[Group](row, config.ProductGroups.Fields)
		if err != nil || group.ID == "" || group.Name == "" {
			return nil, errors.New("invalid OData product group")
		}
		if !group.IsFolder {
			return nil, errors.New("OData returned a product outside the group list")
		}
		groups = append(groups, group)
		byID[group.ID] = group
	}
	query := strings.ToLower(strings.TrimSpace(name))
	selected := make([]Group, 0, len(groups))
	for _, group := range groups {
		parts := []string{group.Name}
		seen := map[string]bool{group.ID: true}
		parent := group.ParentID
		for parent != "" {
			if seen[parent] {
				return nil, errors.New("cycle in OData product groups")
			}
			seen[parent] = true
			ancestor, ok := byID[parent]
			if !ok {
				break
			}
			parts = append([]string{ancestor.Name}, parts...)
			parent = ancestor.ParentID
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

func (s Service) ListPriceTypes(ctx context.Context) ([]PriceType, error) {
	rows, err := s.catalogRows(ctx, config.PriceTypes)
	if err != nil {
		return nil, err
	}
	types := make([]PriceType, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) || !catalogBoolean(row["Недействителен"]) {
			return nil, errors.New("invalid OData price type flags")
		}
		priceType, err := bindFields[PriceType](row, config.PriceTypes.Fields)
		if err != nil || priceType.ID == "" || priceType.Name == "" {
			return nil, errors.New("invalid OData price type")
		}
		types = append(types, priceType)
	}
	sort.Slice(types, func(i, j int) bool {
		if types[i].Name == types[j].Name {
			return types[i].ID < types[j].ID
		}
		return types[i].Name < types[j].Name
	})
	return types, nil
}

func (s Service) ListUnitTypes(ctx context.Context) ([]UnitType, error) {
	rows, err := s.catalogRows(ctx, config.UnitTypes)
	if err != nil {
		return nil, err
	}
	units := make([]UnitType, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData unit type deletion mark")
		}
		unit, err := bindFields[UnitType](row, config.UnitTypes.Fields)
		if err != nil || !linkedGUID(unit.ID) || unit.Name == "" {
			return nil, errors.New("invalid OData unit type")
		}
		if !unit.Deleted {
			units = append(units, unit)
		}
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Name == units[j].Name {
			return units[i].ID < units[j].ID
		}
		return units[i].Name < units[j].Name
	})
	return units, nil
}

func catalogBoolean(value json.RawMessage) bool {
	return string(value) == "true" || string(value) == "false"
}

func (s Service) catalogRows(ctx context.Context, plan config.CatalogListResource) ([]map[string]json.RawMessage, error) {
	params := url.Values{
		"$format":      {"json"},
		"$select":      {sourceFields(plan.Fields)},
		"$orderby":     {"Ref_Key"},
		"$top":         {strconv.Itoa(catalogPageSize)},
		"$inlinecount": {"allpages"},
	}
	if plan.Filter != "" {
		params.Set("$filter", plan.Filter)
	}
	rows := make([]map[string]json.RawMessage, 0)
	seen := make(map[string]bool)
	count := -1
	for len(rows) < maxCatalogRows {
		params.Set("$skip", strconv.Itoa(len(rows)))
		data, err := s.OData.Get(ctx, plan.Name, params, 4<<20)
		if err != nil {
			return nil, err
		}
		var response struct {
			Count string                       `json:"odata.count"`
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
			return nil, errors.New("invalid OData catalog response")
		}
		pageCount, err := strconv.Atoi(response.Count)
		if err != nil || pageCount < 0 || pageCount > maxCatalogRows || count >= 0 && pageCount != count {
			return nil, errors.New("invalid or excessive OData catalog count")
		}
		count = pageCount
		if len(response.Value) == 0 && len(rows) < count || len(response.Value) > catalogPageSize || len(rows)+len(response.Value) > count {
			return nil, errors.New("incomplete OData catalog page")
		}
		for _, row := range response.Value {
			var id string
			if err := json.Unmarshal(row["Ref_Key"], &id); err != nil || id == "" || seen[id] {
				return nil, errors.New("invalid or repeated OData catalog ID")
			}
			seen[id] = true
			rows = append(rows, row)
		}
		if len(rows) == count {
			return rows, nil
		}
	}
	return nil, errors.New("OData catalog exceeds 5000 rows")
}
