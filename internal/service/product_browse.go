package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const productBrowseBatch = 100
const productBrowseScanLimit = 500

type ProductPage struct {
	Offset     int       `json:"offset"`
	Scanned    int       `json:"scanned"`
	NextOffset *int      `json:"next_offset,omitempty"`
	Items      []Product `json:"items"`
}

func (s Service) ListProducts(ctx context.Context, limit, offset int) (ProductPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return ProductPage{}, err
	}
	page := ProductPage{Offset: offset, Items: make([]Product, 0, limit)}
	next := offset
	for page.Scanned < productBrowseScanLimit && len(page.Items) < limit {
		batchStart := next
		top := min(productBrowseBatch, productBrowseScanLimit-page.Scanned)
		params := url.Values{
			"$format": {"json"}, "$select": {sourceFields(config.Products.Fields) + ",IsFolder,DeletionMark"},
			"$orderby": {"Ref_Key asc"}, "$skip": {strconv.Itoa(next)}, "$top": {strconv.Itoa(top)},
		}
		data, err := s.OData.Get(ctx, config.Products.Name, params, 4<<20)
		if err != nil {
			return ProductPage{}, err
		}
		var response struct {
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil || len(response.Value) > top {
			return ProductPage{}, errors.New("invalid OData product page")
		}
		if len(response.Value) == 0 {
			return page, nil
		}
		for _, row := range response.Value {
			if !catalogBoolean(row["IsFolder"]) || !catalogBoolean(row["DeletionMark"]) {
				return ProductPage{}, errors.New("invalid OData product flags")
			}
			product, err := bindFields[Product](row, config.Products.Fields)
			if err != nil || !linkedGUID(product.ID) {
				return ProductPage{}, errors.New("invalid OData product")
			}
			next++
			page.Scanned++
			if string(row["IsFolder"]) == "false" && string(row["DeletionMark"]) == "false" {
				page.Items = append(page.Items, product)
			}
			if len(page.Items) == limit || page.Scanned == productBrowseScanLimit {
				if next-batchStart < len(response.Value) || len(response.Value) == top {
					page.NextOffset = &next
				}
				return page, nil
			}
		}
		if len(response.Value) < top {
			return page, nil
		}
	}
	return page, nil
}
