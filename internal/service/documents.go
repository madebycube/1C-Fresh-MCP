package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

func (s Service) documentCount(ctx context.Context, plan config.DocumentResource) (int, error) {
	params := url.Values{
		"$format":      {"json"},
		"$inlinecount": {"allpages"},
		"$filter":      {plan.DeletedField + " eq false"},
		"$select":      {plan.DateField},
		"$top":         {"1"},
	}
	data, err := s.OData.Get(ctx, plan.Name, params, 1<<20)
	if err != nil {
		return 0, err
	}
	var response struct {
		Count string `json:"odata.count"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return 0, errors.New("invalid OData document count")
	}
	count, err := strconv.Atoi(response.Count)
	if err != nil || count < 0 {
		return 0, errors.New("invalid OData document count")
	}
	return count, nil
}

func (s Service) documentPage(ctx context.Context, plan config.DocumentResource, skip, top int) ([]map[string]json.RawMessage, error) {
	if top == 0 {
		return []map[string]json.RawMessage{}, nil
	}
	params := url.Values{
		"$format":  {"json"},
		"$filter":  {plan.DeletedField + " eq false"},
		"$select":  {sourceFields(plan.Fields)},
		"$orderby": {plan.DateField + " asc,Ref_Key asc"},
		"$skip":    {strconv.Itoa(skip)},
		"$top":     {strconv.Itoa(top)},
	}
	data, err := s.OData.Get(ctx, plan.Name, params, 4<<20)
	if err != nil {
		return nil, err
	}
	var response struct {
		Value []map[string]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
		return nil, errors.New("invalid OData document response")
	}
	return response.Value, nil
}

func (s Service) documentDateAt(ctx context.Context, plan config.DocumentResource, index int) (string, error) {
	params := url.Values{
		"$format":  {"json"},
		"$filter":  {plan.DeletedField + " eq false"},
		"$select":  {plan.DateField},
		"$orderby": {plan.DateField + " asc,Ref_Key asc"},
		"$skip":    {strconv.Itoa(index)},
		"$top":     {"1"},
	}
	data, err := s.OData.Get(ctx, plan.Name, params, 1<<20)
	if err != nil {
		return "", err
	}
	var response struct {
		Value []map[string]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &response); err != nil || len(response.Value) != 1 {
		return "", errors.New("invalid OData document date")
	}
	var date string
	if err := json.Unmarshal(response.Value[0][plan.DateField], &date); err != nil || len(date) < 19 {
		return "", errors.New("invalid OData document date")
	}
	return date, nil
}

func (s Service) lowerBoundDate(ctx context.Context, plan config.DocumentResource, count int, target string) (int, error) {
	low, high := 0, count
	for low < high {
		mid := low + (high-low)/2
		date, err := s.documentDateAt(ctx, plan, mid)
		if err != nil {
			return 0, err
		}
		if date >= target {
			high = mid
		} else {
			low = mid + 1
		}
	}
	return low, nil
}

func (s Service) dateRangePage(ctx context.Context, plan config.DocumentResource, from, to string, limit, offset int) ([]map[string]json.RawMessage, int, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return nil, 0, errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil {
		return nil, 0, errors.New("to must be YYYY-MM-DD")
	}
	if end.Before(start) || end.Sub(start) > 30*24*time.Hour {
		return nil, 0, errors.New("date range must be ordered and at most 31 calendar days")
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return nil, 0, errors.New("limit must be 1–100 and offset must be nonnegative")
	}
	count, err := s.documentCount(ctx, plan)
	if err != nil {
		return nil, 0, err
	}
	first, err := s.lowerBoundDate(ctx, plan, count, start.Format("2006-01-02")+"T00:00:00")
	if err != nil {
		return nil, 0, err
	}
	last, err := s.lowerBoundDate(ctx, plan, count, end.AddDate(0, 0, 1).Format("2006-01-02")+"T00:00:00")
	if err != nil {
		return nil, 0, err
	}
	total := last - first
	if total < 0 {
		return nil, 0, errors.New("OData document dates are not ordered")
	}
	if offset >= total {
		return []map[string]json.RawMessage{}, total, nil
	}
	if limit > total-offset {
		limit = total - offset
	}
	rows, err := s.documentPage(ctx, plan, first+offset, limit)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) != limit {
		return nil, 0, fmt.Errorf("OData returned %d documents; expected %d", len(rows), limit)
	}
	startText := start.Format("2006-01-02") + "T00:00:00"
	endText := end.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00"
	previous := ""
	for _, row := range rows {
		var date string
		if err := json.Unmarshal(row[plan.DateField], &date); err != nil || date < startText || date >= endText || date < previous {
			return nil, 0, errors.New("OData returned documents outside the requested date range or order")
		}
		previous = date
	}
	return rows, total, nil
}

func sourceFields(bindings []config.FieldBinding) string {
	fields := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		fields = append(fields, binding.Source)
	}
	return strings.Join(fields, ",")
}

func bindFields[T any](row map[string]json.RawMessage, bindings []config.FieldBinding) (T, error) {
	selected := make(map[string]json.RawMessage, len(bindings))
	for _, binding := range bindings {
		if value, ok := row[binding.Source]; ok {
			selected[binding.Output] = value
		}
	}
	data, err := json.Marshal(selected)
	if err != nil {
		var zero T
		return zero, err
	}
	var value T
	err = json.Unmarshal(data, &value)
	return value, err
}
