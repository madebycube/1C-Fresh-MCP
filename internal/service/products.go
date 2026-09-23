package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const MaxProducts = 50
const productSearchBatch = 100
const productSearchScanLimit = 500

type Reader interface {
	Get(context.Context, string, url.Values, int64) ([]byte, error)
	Check(context.Context) (int, error)
}

type Product struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Article  string `json:"article"`
	ParentID string `json:"parent_id"`
}

type Service struct {
	OData Reader
}

func (s Service) Check(ctx context.Context) (int, error) {
	return s.OData.Check(ctx)
}

func (s Service) SearchProducts(ctx context.Context, query string, limit int) ([]Product, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 120 || strings.IndexFunc(query, unicode.IsControl) >= 0 {
		return nil, errors.New("query must be 1–120 characters without control characters")
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > MaxProducts {
		return nil, fmt.Errorf("limit must be between 1 and %d", MaxProducts)
	}
	plan := config.Products
	type result struct {
		rows []Product
		err  error
	}
	results := make([]result, len(plan.SearchFields))
	var workers sync.WaitGroup
	for index, field := range plan.SearchFields {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results[index].rows, results[index].err = s.searchProductField(ctx, plan, field, query, limit)
		}()
	}
	workers.Wait()
	products := make([]Product, 0, limit)
	seen := make(map[string]bool)
	for _, result := range results {
		if result.err != nil {
			return nil, result.err
		}
		for _, product := range result.rows {
			if product.ID == "" || seen[product.ID] {
				continue
			}
			seen[product.ID] = true
			products = append(products, product)
			if len(products) == limit {
				return products, nil
			}
		}
	}
	return products, nil
}

func (s Service) searchProductField(ctx context.Context, plan config.SearchResource, field, query string, limit int) ([]Product, error) {
	quoted := "'" + strings.ReplaceAll(query, "'", "''") + "'"
	needle := strings.ToLower(query)
	params := url.Values{
		"$format":     {"json"},
		"$filter":     {"substringof(" + quoted + "," + field + ")"},
		"$select":     {sourceFields(plan.Fields) + "," + plan.FolderField + "," + plan.DeletedField},
		"$orderby":    {"Ref_Key asc"},
		"allowedOnly": {"true"},
	}
	products := make([]Product, 0, limit)
	for scanned := 0; scanned < productSearchScanLimit && len(products) < limit; {
		top := min(productSearchBatch, productSearchScanLimit-scanned)
		params.Set("$top", strconv.Itoa(top))
		params.Set("$skip", strconv.Itoa(scanned))
		data, err := s.OData.Get(ctx, plan.Name, params, 4<<20)
		if err != nil {
			return nil, err
		}
		var response struct {
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil || len(response.Value) > top {
			return nil, errors.New("invalid OData product response")
		}
		if len(response.Value) == 0 {
			break
		}
		for _, row := range response.Value {
			if !catalogBoolean(row[plan.FolderField]) || !catalogBoolean(row[plan.DeletedField]) {
				return nil, errors.New("invalid OData product flags")
			}
			if string(row[plan.FolderField]) != "false" || string(row[plan.DeletedField]) != "false" {
				continue
			}
			var value string
			if len(row[field]) == 0 {
				continue
			}
			if err := json.Unmarshal(row[field], &value); err != nil || !strings.Contains(strings.ToLower(value), needle) {
				continue
			}
			product, err := bindFields[Product](row, plan.Fields)
			if err != nil || !linkedGUID(product.ID) {
				return nil, errors.New("invalid OData product")
			}
			products = append(products, product)
			if len(products) == limit {
				return products, nil
			}
		}
		scanned += len(response.Value)
		if len(response.Value) < top {
			break
		}
	}
	return products, nil
}
