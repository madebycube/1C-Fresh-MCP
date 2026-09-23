package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type ProductCategory struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	UnitID   string `json:"unit_id"`
	IsFolder bool   `json:"is_folder"`
	Deleted  bool   `json:"deleted"`
}

func (s Service) ListProductCategories(ctx context.Context, name string) ([]ProductCategory, error) {
	rows, err := s.catalogRows(ctx, config.ProductCategories)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]ProductCategory, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData product category flags")
		}
		category, err := bindFields[ProductCategory](row, config.ProductCategories.Fields)
		if err != nil || !linkedGUID(category.ID) || category.Name == "" {
			return nil, errors.New("invalid OData product category")
		}
		byID[strings.ToLower(category.ID)] = category
	}
	query := strings.ToLower(strings.TrimSpace(name))
	selected := make([]ProductCategory, 0, len(rows))
	for _, category := range byID {
		if category.IsFolder || category.Deleted {
			continue
		}
		parts := []string{category.Name}
		seen := map[string]bool{strings.ToLower(category.ID): true}
		parentID := category.ParentID
		for linkedGUID(parentID) {
			key := strings.ToLower(parentID)
			if seen[key] {
				return nil, errors.New("cycle in OData product categories")
			}
			seen[key] = true
			parent, ok := byID[key]
			if !ok {
				break
			}
			parts = append([]string{parent.Name}, parts...)
			parentID = parent.ParentID
		}
		category.Path = strings.Join(parts, " / ")
		if query == "" || strings.Contains(strings.ToLower(category.Path), query) {
			selected = append(selected, category)
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
