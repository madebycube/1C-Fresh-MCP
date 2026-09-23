package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

func (s Service) documentCount(ctx context.Context, plan config.DocumentResource) (int, error) {
	params := url.Values{
		"$format":      {"json"},
		"$inlinecount": {"allpages"},
		"$filter":      {documentFilter(plan)},
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

func documentFilter(plan config.DocumentResource) string {
	if plan.Filter != "" {
		return plan.Filter
	}
	return plan.DeletedField + " eq false"
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
