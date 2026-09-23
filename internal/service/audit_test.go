package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

type auditReader struct {
	rows  map[string][]map[string]any
	calls atomic.Int64
}

func (r *auditReader) Check(context.Context) (int, error) { return 1, nil }

func (r *auditReader) Get(_ context.Context, resource string, params url.Values, _ int64) ([]byte, error) {
	r.calls.Add(1)
	kind := "sale"
	if strings.Contains(resource, "Возврат") {
		kind = "refund"
	}
	rows := r.rows[kind]
	if params.Get("$inlinecount") != "" {
		data, _ := json.Marshal(map[string]any{"odata.count": strconv.Itoa(len(rows)), "value": []any{}})
		return data, nil
	}
	skip, _ := strconv.Atoi(params.Get("$skip"))
	top, _ := strconv.Atoi(params.Get("$top"))
	page := make([]map[string]any, 0)
	for index := skip; index < skip+top && index < len(rows); index++ {
		row := rows[index]
		if params.Get("$select") == "Date" {
			row = map[string]any{"Date": row["Date"]}
		}
		page = append(page, row)
	}
	data, _ := json.Marshal(map[string]any{"value": page})
	return data, nil
}

func auditRow(index int, date string, posted any) map[string]any {
	return map[string]any{
		"Ref_Key": fmt.Sprintf("00000000-0000-0000-0000-%012x", index),
		"Number":  strconv.Itoa(index), "Date": date, "Posted": posted, "DeletionMark": false, "СуммаДокумента": 10,
	}
}

func TestAuditUnpostedReceipts(t *testing.T) {
	rows := make([]map[string]any, 0, 105)
	rows = append(rows, auditRow(0, "2026-09-01T10:00:00", false))
	for index := 1; index < 104; index++ {
		rows = append(rows, auditRow(index, "2026-09-02T10:00:00", true))
	}
	rows = append(rows, auditRow(104, "2026-09-03T00:00:00", false))
	reader := &auditReader{rows: map[string][]map[string]any{
		"sale":   rows,
		"refund": {auditRow(1, "2026-09-02T12:00:00", false)},
	}}
	report, err := (Service{OData: reader}).AuditUnpostedReceipts(context.Background(), "both", "2026-09-01", "2026-09-03")
	if err != nil {
		t.Fatal(err)
	}
	if report.Inspected != 105 || len(report.Findings) != 2 || report.Findings[0].Kind != "sale" || report.Findings[1].Kind != "refund" || report.Findings[0].Posted || report.Findings[1].Posted {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestAuditRejectsBadInputBeforeOData(t *testing.T) {
	reader := &auditReader{}
	svc := Service{OData: reader}
	for _, test := range []struct{ kind, from, before string }{
		{"other", "2026-09-01", "2026-09-02"},
		{"sale", "bad", "2026-09-02"},
		{"sale", "2026-09-01", "bad"},
		{"sale", "2026-09-02", "2026-09-02"},
		{"sale", "2026-09-02", "2026-09-01"},
		{"sale", "2026-09-01", "2026-11-01"},
	} {
		if _, err := svc.AuditUnpostedReceipts(context.Background(), test.kind, test.from, test.before); err == nil {
			t.Errorf("accepted %+v", test)
		}
	}
	if reader.calls.Load() != 0 {
		t.Fatal("invalid input reached OData")
	}
}

func TestAuditRejectsMissingPostedStatusAndOversizedScan(t *testing.T) {
	reader := &auditReader{rows: map[string][]map[string]any{
		"sale": {auditRow(1, "2026-09-01T10:00:00", nil)},
	}}
	if _, err := (Service{OData: reader}).AuditUnpostedReceipts(context.Background(), "sale", "2026-09-01", "2026-09-02"); err == nil {
		t.Fatal("missing posted status accepted")
	}
	rows := make([]map[string]any, maxAuditReceipts+1)
	for index := range rows {
		rows[index] = auditRow(index, "2026-09-01T10:00:00", true)
	}
	reader.rows["sale"] = rows
	if _, err := (Service{OData: reader}).AuditUnpostedReceipts(context.Background(), "sale", "2026-09-01", "2026-09-02"); err == nil {
		t.Fatal("oversized scan accepted")
	}
}

func TestAuditEmptyRange(t *testing.T) {
	reader := &auditReader{rows: map[string][]map[string]any{}}
	report, err := (Service{OData: reader}).AuditUnpostedReceipts(context.Background(), "both", "2026-09-01", "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if report.Inspected != 0 || report.Findings == nil || len(report.Findings) != 0 {
		t.Fatalf("unexpected empty report: %+v", report)
	}
}
