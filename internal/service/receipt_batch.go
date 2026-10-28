package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxReceiptBatch = 20

type ReceiptBatchItem struct {
	Index      int                `json:"index"`
	Status     string             `json:"status"`
	Reason     string             `json:"reason,omitempty"`
	Duplicates []ReceiptDuplicate `json:"duplicates,omitempty"`
	Preview    *ReceiptWrite      `json:"preview,omitempty"`
}

type ReceiptBatchPreview struct {
	Count       int                `json:"count"`
	Ready       int                `json:"ready"`
	NeedsReview int                `json:"needs_review"`
	Exists      int                `json:"exists"`
	Unsupported int                `json:"unsupported"`
	Items       []ReceiptBatchItem `json:"items"`
	Note        string             `json:"note"`
}

// PreviewReceiptBatch checks each draft exactly as create receipt would, without writing, and gives each one a status.
func (s Service) PreviewReceiptBatch(ctx context.Context, drafts []ReceiptDraft) (ReceiptBatchPreview, error) {
	if len(drafts) == 0 || len(drafts) > maxReceiptBatch {
		return ReceiptBatchPreview{}, fmt.Errorf("a batch holds 1–%d receipts", maxReceiptBatch)
	}
	result := ReceiptBatchPreview{Count: len(drafts), Items: make([]ReceiptBatchItem, 0, len(drafts)), Note: "preview only: nothing was written; save ready receipts one at a time with create receipt"}
	seen := map[string]int{}
	for index, draft := range drafts {
		draft.Apply = false
		item := ReceiptBatchItem{Index: index + 1}
		preview, err := s.CreateReceiptDraft(ctx, draft)
		var unsupportedErr unsupportedError
		switch {
		case errors.As(err, &unsupportedErr):
			item.Status, item.Reason = "unsupported", err.Error()
		case err != nil:
			item.Status, item.Reason = "needs_review", err.Error()
		case len(preview.Duplicates) > 0:
			item.Status, item.Duplicates = "exists", preview.Duplicates
			item.Reason = "a receipt with the same organization, supplier, incoming number and date exists"
		default:
			item.Status, item.Preview = "ready", &preview
		}
		if item.Status == "ready" && preview.IncomingNumber != "" {
			key := strings.ToLower(preview.OrganizationID + "|" + preview.SupplierID + "|" + preview.IncomingNumber + "|" + preview.IncomingDate)
			if first, ok := seen[key]; ok {
				item.Status, item.Preview = "needs_review", nil
				item.Reason = fmt.Sprintf("same supplier document as batch item %d", first)
			} else {
				seen[key] = index + 1
			}
		}
		switch item.Status {
		case "ready":
			result.Ready++
		case "exists":
			result.Exists++
		case "unsupported":
			result.Unsupported++
		default:
			result.NeedsReview++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}
