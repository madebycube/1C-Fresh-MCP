package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type ProductDetail struct {
	Product
	Type   string `json:"type"`
	UnitID string `json:"unit_id"`
}

func (s Service) GetProduct(ctx context.Context, id string) (ProductDetail, error) {
	if !linkedGUID(id) {
		return ProductDetail{}, errors.New("product ID must be a nonzero GUID")
	}
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(config.Products.Fields) + ",ТипНоменклатуры,ЕдиницаИзмерения_Key,IsFolder,DeletionMark"}}
	data, err := s.OData.Get(ctx, nomenclatureResource(id), params, 1<<20)
	if err != nil {
		return ProductDetail{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil || !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) || string(row["IsFolder"]) != "false" || string(row["DeletionMark"]) != "false" {
		return ProductDetail{}, errors.New("ID does not identify an active product")
	}
	product, err := bindFields[Product](row, config.Products.Fields)
	if err != nil || !strings.EqualFold(product.ID, id) {
		return ProductDetail{}, errors.New("invalid OData product response")
	}
	var detail ProductDetail
	detail.Product = product
	if err := json.Unmarshal(row["ТипНоменклатуры"], &detail.Type); err != nil {
		return ProductDetail{}, errors.New("invalid OData product type")
	}
	if err := json.Unmarshal(row["ЕдиницаИзмерения_Key"], &detail.UnitID); err != nil {
		return ProductDetail{}, errors.New("invalid OData product unit")
	}
	return detail, nil
}
