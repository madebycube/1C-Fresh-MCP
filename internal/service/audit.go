package service

import (
	"context"
	"errors"
	"time"
)

const maxAuditReceipts = 1000

type ReceiptAudit struct {
	Rule      string    `json:"rule"`
	Kind      string    `json:"kind"`
	From      string    `json:"from"`
	Before    string    `json:"before"`
	Inspected int       `json:"inspected"`
	Findings  []Receipt `json:"findings"`
}

func (s Service) AuditUnpostedReceipts(ctx context.Context, kind, from, before string) (ReceiptAudit, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil {
		return ReceiptAudit{}, errors.New("from must be YYYY-MM-DD")
	}
	cutoff, err := time.Parse("2006-01-02", before)
	if err != nil {
		return ReceiptAudit{}, errors.New("before must be YYYY-MM-DD")
	}
	if !cutoff.After(start) || cutoff.Sub(start) > 31*24*time.Hour {
		return ReceiptAudit{}, errors.New("audit range must be ordered and at most 31 calendar days")
	}
	if kind == "" {
		kind = "both"
	}
	kinds := []string{kind}
	if kind == "both" {
		kinds = []string{"sale", "refund"}
	} else if _, err := receiptPlan(kind); err != nil {
		return ReceiptAudit{}, errors.New("kind must be sale, refund, or both")
	}
	report := ReceiptAudit{
		Rule: "unposted_receipts_before_date", Kind: kind, From: from, Before: before,
		Findings: make([]Receipt, 0),
	}
	to := cutoff.AddDate(0, 0, -1).Format("2006-01-02")
	startText, endText, err := documentDateRange(from, to)
	if err != nil {
		return ReceiptAudit{}, err
	}
	for _, receiptKind := range kinds {
		plan, _ := receiptPlan(receiptKind)
		all, err := s.allDocumentRows(ctx, plan)
		if err != nil {
			return ReceiptAudit{}, err
		}
		rows := selectedDocumentRows(all, plan.DateField, startText, endText)
		total := len(rows)
		if total > maxAuditReceipts-report.Inspected {
			return ReceiptAudit{}, errors.New("audit range exceeds 1000 receipts; narrow the dates")
		}
		for _, row := range rows {
			if posted := string(row["Posted"]); posted != "true" && posted != "false" {
				return ReceiptAudit{}, errors.New("invalid OData receipt posted status")
			}
			receipt, err := bindFields[Receipt](row, plan.Fields)
			if err != nil {
				return ReceiptAudit{}, errors.New("invalid OData receipt fields")
			}
			if receipt.ID == "" || receipt.Date == "" {
				return ReceiptAudit{}, errors.New("invalid OData receipt identity or date")
			}
			if !receipt.Posted {
				receipt.Kind = receiptKind
				report.Findings = append(report.Findings, receipt)
			}
		}
		report.Inspected += total
	}
	return report, nil
}
