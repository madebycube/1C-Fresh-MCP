package service

import (
	"context"
	"errors"
	"sort"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type Warehouse struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Deleted  bool   `json:"deleted"`
	Inactive bool   `json:"inactive"`
}

func (s Service) ListWarehouses(ctx context.Context) ([]Warehouse, error) {
	rows, err := s.catalogRows(ctx, config.Warehouses)
	if err != nil {
		return nil, err
	}
	warehouses := make([]Warehouse, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) || !catalogBoolean(row["Недействителен"]) {
			return nil, errors.New("invalid OData warehouse flags")
		}
		warehouse, err := bindFields[Warehouse](row, config.Warehouses.Fields)
		if err != nil || warehouse.ID == "" || warehouse.Name == "" {
			return nil, errors.New("invalid OData warehouse")
		}
		if warehouse.Kind == "Склад" || warehouse.Kind == "Розница" {
			warehouses = append(warehouses, warehouse)
		}
	}
	sort.Slice(warehouses, func(i, j int) bool {
		if warehouses[i].Name == warehouses[j].Name {
			return warehouses[i].ID < warehouses[j].ID
		}
		return warehouses[i].Name < warehouses[j].Name
	})
	return warehouses, nil
}
