package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type ProductCharacteristic struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Article   string `json:"article"`
	ProductID string `json:"product_id"`
}

type productCharacteristicRow struct {
	ProductCharacteristic
	OwnerType string `json:"owner_type"`
	Deleted   bool   `json:"deleted"`
	Inactive  bool   `json:"inactive"`
}

func (s Service) ListProductCharacteristics(ctx context.Context, productID, name string) ([]ProductCharacteristic, error) {
	if productID != "" && !linkedGUID(productID) {
		return nil, errors.New("product ID must be a nonzero GUID")
	}
	rows, err := s.catalogRows(ctx, config.ProductCharacteristics)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(name))
	items := make([]ProductCharacteristic, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) || !catalogBoolean(row["Недействителен"]) {
			return nil, errors.New("invalid OData product characteristic flags")
		}
		item, err := bindFields[productCharacteristicRow](row, config.ProductCharacteristics.Fields)
		if err != nil || !linkedGUID(item.ID) || item.Name == "" {
			return nil, errors.New("invalid OData product characteristic")
		}
		if item.OwnerType != "StandardODATA.Catalog_Номенклатура" || !linkedGUID(item.ProductID) || item.Deleted || item.Inactive {
			continue
		}
		if productID != "" && !strings.EqualFold(item.ProductID, productID) || query != "" && !strings.Contains(strings.ToLower(item.Name), query) {
			continue
		}
		items = append(items, item.ProductCharacteristic)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}
