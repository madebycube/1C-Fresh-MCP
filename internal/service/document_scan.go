package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"golang.org/x/sync/errgroup"
)

const documentScanPageSize = 100
const maxDocumentScan = 20000

func (s Service) allDocumentRows(ctx context.Context, plan config.DocumentResource) ([]map[string]json.RawMessage, error) {
	fields := sourceFields(plan.Fields)
	if !strings.Contains(","+fields+",", ","+plan.DeletedField+",") {
		fields += "," + plan.DeletedField
	}
	countParams := url.Values{
		"$format": {"json"}, "$inlinecount": {"allpages"},
		"$select": {"Ref_Key"}, "$top": {"1"},
	}
	countData, err := s.OData.Get(ctx, plan.Name, countParams, 1<<20)
	if err != nil {
		return nil, err
	}
	var countResponse struct {
		Count string `json:"odata.count"`
	}
	if err := json.Unmarshal(countData, &countResponse); err != nil {
		return nil, errors.New("invalid OData document count")
	}
	count, err := strconv.Atoi(countResponse.Count)
	if err != nil || count < 0 || count > maxDocumentScan {
		return nil, errors.New("invalid or excessive OData document count")
	}
	pageCount := (count + documentScanPageSize - 1) / documentScanPageSize
	pages := make([][]map[string]json.RawMessage, pageCount)
	group, requestContext := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index := range pages {
		index := index
		offset := index * documentScanPageSize
		limit := min(documentScanPageSize, count-offset)
		group.Go(func() error {
			params := url.Values{
				"$format": {"json"}, "$select": {fields},
				"$orderby": {"Ref_Key asc"}, "$skip": {strconv.Itoa(offset)}, "$top": {strconv.Itoa(limit)},
			}
			data, err := s.OData.Get(requestContext, plan.Name, params, 4<<20)
			if err != nil {
				return err
			}
			var response struct {
				Value []map[string]json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(data, &response); err != nil || len(response.Value) != limit {
				return errors.New("document history changed during lookup")
			}
			pages[index] = response.Value
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	rows := make([]map[string]json.RawMessage, 0, count)
	seen := make(map[string]bool, count)
	for _, page := range pages {
		for _, row := range page {
			var id, date string
			if err := json.Unmarshal(row["Ref_Key"], &id); err != nil || !guidPattern.MatchString(id) || seen[id] {
				return nil, errors.New("invalid or repeated OData document ID")
			}
			seen[id] = true
			if err := json.Unmarshal(row[plan.DateField], &date); err != nil || len(date) < 19 {
				return nil, errors.New("invalid OData document date")
			}
			if _, err := time.Parse("2006-01-02T15:04:05", date[:19]); err != nil {
				return nil, errors.New("invalid OData document date")
			}
			if !catalogBoolean(row[plan.DeletedField]) {
				return nil, errors.New("invalid OData deletion mark")
			}
			if string(row[plan.DeletedField]) == "false" {
				rows = append(rows, row)
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		var firstDate, secondDate, firstID, secondID string
		_ = json.Unmarshal(rows[i][plan.DateField], &firstDate)
		_ = json.Unmarshal(rows[j][plan.DateField], &secondDate)
		if firstDate != secondDate {
			return firstDate < secondDate
		}
		_ = json.Unmarshal(rows[i]["Ref_Key"], &firstID)
		_ = json.Unmarshal(rows[j]["Ref_Key"], &secondID)
		return firstID < secondID
	})
	return rows, nil
}

func documentDateRange(from, to string) (string, string, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return "", "", errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Format("2006-01-02") != to {
		return "", "", errors.New("to must be YYYY-MM-DD")
	}
	if end.Before(start) || end.Sub(start) > 30*24*time.Hour {
		return "", "", errors.New("date range must be ordered and at most 31 calendar days")
	}
	return from + "T00:00:00", end.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00", nil
}

func selectedDocumentRows(rows []map[string]json.RawMessage, dateField, start, end string) []map[string]json.RawMessage {
	selected := make([]map[string]json.RawMessage, 0)
	for _, row := range rows {
		var date string
		_ = json.Unmarshal(row[dateField], &date)
		if date >= start && date < end {
			selected = append(selected, row)
		}
	}
	return selected
}
