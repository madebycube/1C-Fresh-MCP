package service

import (
	"context"
	"testing"
)

func TestPreviewReceiptBatchClassifiesEachDraftWithoutWriting(t *testing.T) {
	fake := receiptFixture()
	ready := receiptDraftInput()
	ready.Apply = true
	exists := receiptDraftInput()
	exists.IncomingNumber, exists.IncomingDate = "ВХ-1", "2026-09-01"
	service := receiptDraftInput()
	service.Lines = []ReceiptLineInput{{ProductID: salesID(66), Quantity: "1", Price: "10", VATRate: "НДС22"}}
	badRate := receiptDraftInput()
	badRate.Lines[0].VATRate = "НДС99"
	preview, err := Service{OData: fake}.PreviewReceiptBatch(context.Background(), []ReceiptDraft{ready, exists, service, badRate, ready})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ready", "exists", "unsupported", "needs_review", "needs_review"}
	for i, item := range preview.Items {
		if item.Status != want[i] {
			t.Fatalf("item %d: %+v", i+1, item)
		}
	}
	if preview.Ready != 1 || preview.Exists != 1 || preview.Unsupported != 1 || preview.NeedsReview != 2 || preview.Items[0].Preview == nil || preview.Items[4].Reason != "same supplier document as batch item 1" {
		t.Fatalf("summary: %+v", preview)
	}
	if len(fake.writes) != 0 || len(fake.actions) != 0 {
		t.Fatalf("preview wrote: %v %v", fake.writes, fake.actions)
	}
	if _, err := (Service{OData: fake}).PreviewReceiptBatch(context.Background(), nil); err == nil {
		t.Fatal("empty batch accepted")
	}
}
