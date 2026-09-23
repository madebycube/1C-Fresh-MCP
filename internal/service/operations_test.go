package service

import (
	"context"
	"testing"
)

func TestPurchaseDocumentsFilterOperationAndTypedOrderLink(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Number": "R1", "Date": "2026-09-01T12:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "ПоступлениеОтПоставщика", "Контрагент_Key": salesID(9), "СтруктурнаяЕдиница_Key": salesID(8), "СуммаДокумента": 12.50, "Заказ": salesID(7), "Заказ_Type": "StandardODATA.Document_ЗаказПоставщику", "Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": salesID(10), "Количество": 2}}},
		{"Ref_Key": salesID(2), "Number": "RETURN", "Date": "2026-09-02T12:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "ВозвратОтПокупателя", "Контрагент_Key": salesID(9)},
		{"Ref_Key": salesID(3), "Number": "R2", "Date": "2026-09-03T12:00:00", "Posted": false, "DeletionMark": false, "ВидОперации": "ПоступлениеОтПоставщика", "Контрагент_Key": salesID(9), "Заказ": salesID(6), "Заказ_Type": "StandardODATA.Document_ЗаказПокупателя"},
		{"Ref_Key": salesID(4), "Number": "DELETED", "Date": "2026-09-04T12:00:00", "Posted": true, "DeletionMark": true, "ВидОперации": "ПоступлениеОтПоставщика"},
	}
	svc := Service{OData: salesReader{rows: map[string][]map[string]any{"Document_ПриходнаяНакладная": rows}}}
	page, err := svc.ListOperationalDocuments(context.Background(), "purchase", "receipt", salesID(9), "", "", "", 1, 0)
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].Number != "R2" || page.NextOffset == nil {
		t.Fatalf("purchase page: %+v, %v", page, err)
	}
	next, err := svc.ListOperationalDocuments(context.Background(), "purchase", "receipt", salesID(9), "", "", "", 1, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].SupplierOrderID != salesID(7) || next.NextOffset != nil {
		t.Fatalf("purchase next page: %+v, %v", next, err)
	}
	detail, err := svc.GetOperationalDocument(context.Background(), "purchase", "receipt", salesID(1))
	if err != nil || len(detail.Lines) != 1 || detail.Lines[0].ProductID != salesID(10) {
		t.Fatalf("receipt detail: %+v, %v", detail, err)
	}
	if _, err := svc.GetOperationalDocument(context.Background(), "purchase", "receipt", salesID(2)); err == nil {
		t.Fatal("customer return accepted as supplier goods receipt")
	}
	if _, err := svc.ListOperationalDocuments(context.Background(), "purchase", "receipt", "", "bad", "", "", 1, 0); err == nil {
		t.Fatal("invalid warehouse ID accepted")
	}
}

func TestWarehouseTransferFiltersExpenseOperationAndBothWarehouses(t *testing.T) {
	rows := []map[string]any{
		{"Ref_Key": salesID(1), "Number": "T1", "Date": "2026-09-01T12:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Перемещение", "СтруктурнаяЕдиница_Key": salesID(8), "СтруктурнаяЕдиницаПолучатель_Key": salesID(9), "ЗаказНаПеремещение_Key": emptyGUID},
		{"Ref_Key": salesID(2), "Number": "EXPENSE", "Date": "2026-09-02T12:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "СписаниеНаРасходы", "СтруктурнаяЕдиница_Key": salesID(8)},
		{"Ref_Key": salesID(3), "Number": "T2", "Date": "2026-09-03T12:00:00", "Posted": true, "DeletionMark": false, "ВидОперации": "Перемещение", "СтруктурнаяЕдиница_Key": salesID(9), "СтруктурнаяЕдиницаПолучатель_Key": salesID(8), "ЗаказНаПеремещение_Key": salesID(7)},
	}
	svc := Service{OData: salesReader{rows: map[string][]map[string]any{"Document_ПеремещениеЗапасов": rows}}}
	page, err := svc.ListOperationalDocuments(context.Background(), "warehouse", "transfer", "", salesID(8), "", "", 20, 0)
	if err != nil || page.Total != 2 || page.Items[0].Number != "T2" || page.Items[0].TransferOrderID != salesID(7) || page.Items[1].TransferOrderID != "" {
		t.Fatalf("warehouse transfers: %+v, %v", page, err)
	}
	if _, err := svc.GetOperationalDocument(context.Background(), "warehouse", "transfer", salesID(2)); err == nil {
		t.Fatal("expense operation accepted as transfer")
	}
	if _, err := svc.ListOperationalDocuments(context.Background(), "warehouse", "transfer", salesID(1), "", "", "", 20, 0); err == nil {
		t.Fatal("supplier filter accepted for warehouse documents")
	}
}
