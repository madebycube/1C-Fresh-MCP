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
	selectFields := make([]string, 0, len(plan.Fields))
	for _, field := range plan.Fields {
		selectFields = append(selectFields, field.Source)
	}
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
			results[index].rows, results[index].err = s.searchProductField(ctx, plan, field, query, limit, selectFields)
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

func (s Service) searchProductField(ctx context.Context, plan config.SearchResource, field, query string, limit int, selectFields []string) ([]Product, error) {
	quoted := "'" + strings.ReplaceAll(query, "'", "''") + "'"
	params := url.Values{
		"$format":     {"json"},
		"$filter":     {"substringof(" + quoted + "," + field + ") and " + plan.FolderField + " eq false and " + plan.DeletedField + " eq false"},
		"$select":     {strings.Join(selectFields, ",")},
		"$top":        {strconv.Itoa(limit)},
		"allowedOnly": {"true"},
	}
	data, err := s.OData.Get(ctx, plan.Name, params, 2<<20)
	if err != nil {
		return nil, err
	}
	var response struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
		return nil, errors.New("invalid OData product response")
	}
	products := make([]Product, 0, len(response.Value))
	for _, row := range response.Value {
		mapped := make(map[string]string, len(plan.Fields))
		for _, field := range plan.Fields {
			if value, ok := row[field.Source].(string); ok {
				mapped[field.Output] = value
			}
		}
		products = append(products, Product{
			ID: mapped["id"], Code: mapped["code"], Name: mapped["name"],
			FullName: mapped["full_name"], Article: mapped["article"], ParentID: mapped["parent_id"],
		})
	}
	return products, nil
}
