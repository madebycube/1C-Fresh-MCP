package service

import (
	"context"
	"strings"
	"testing"
)

func reconciliationFixture() *traceReader {
	org, supplier, other := salesID(80), salesID(50), salesID(51)
	c1, c2 := salesID(60), salesID(61)
	r1, ob, p0, p1, k1, p2 := salesID(1), salesID(2), salesID(3), salesID(4), salesID(5), salesID(6)
	const (
		bankOut = "StandardODATA.Document_РасходСоСчета"
		cashOut = "StandardODATA.Document_РасходИзКассы"
		opening = "StandardODATA.Document_ВводНачальныхОстатков"
	)
	record := func(recorder, recorderType, line, period, kind, settlement, contract, document, documentType string, amount float64) map[string]any {
		return map[string]any{"Recorder": recorder, "Recorder_Type": recorderType, "LineNumber": line, "Period": period, "Active": true, "RecordType": kind, "ТипРасчетов": settlement,
			"Организация_Key": org, "Контрагент_Key": supplier, "Договор_Key": contract, "Документ": document, "Документ_Type": documentType, "Сумма": amount, "СуммаВал": amount}
	}
	offset := record(r1, supplierReceiptType, "2", "2026-09-05T10:00:00", "Expense", "Долг", c1, r1, supplierReceiptType, 100)
	offset["ДокументОплаты"], offset["ДокументОплаты_Type"] = p0, bankOut
	automatic := record(p1, bankOut, "1", "2026-09-10T09:00:00", "Expense", "Долг", c1, r1, supplierReceiptType, 600)
	automatic["СпособЗачета"] = "Автоматически"
	inactive := record(salesID(9), cashOut, "1", "2026-09-11T09:00:00", "Expense", "Долг", c1, r1, supplierReceiptType, 999)
	inactive["Active"] = false
	foreign := record(salesID(10), supplierReceiptType, "1", "2026-09-11T09:00:00", "Receipt", "Долг", c1, salesID(10), supplierReceiptType, 5)
	foreign["Контрагент_Key"] = other
	header := func(id, number, date string, posted, deleted bool, amount float64) map[string]any {
		return map[string]any{"Ref_Key": id, "Number": number, "Date": date, "Posted": posted, "DeletionMark": deleted, "Организация_Key": org, "Контрагент_Key": supplier, "СуммаДокумента": amount}
	}
	return &traceReader{rows: map[string][]map[string]any{
		"AccumulationRegister_РасчетыСПоставщиками_RecordType": {
			record(ob, opening, "1", "2026-08-10T00:00:00", "Receipt", "Долг", c1, ob, opening, 300),
			record(p0, bankOut, "1", "2026-08-20T09:00:00", "Expense", "Аванс", c1, p0, bankOut, 100),
			record(r1, supplierReceiptType, "1", "2026-09-05T10:00:00", "Receipt", "Долг", c1, r1, supplierReceiptType, 1000.10),
			offset,
			record(r1, supplierReceiptType, "3", "2026-09-05T10:00:00", "Receipt", "Аванс", c1, p0, bankOut, 100),
			automatic,
			record(k1, cashOut, "1", "2026-09-12T09:00:00", "Expense", "Долг", c1, ob, opening, 300),
			record(p2, bankOut, "1", "2026-09-20T09:00:00", "Expense", "Аванс", c1, p2, bankOut, 50),
			record(salesID(11), supplierReceiptType, "1", "2026-10-01T00:00:00", "Receipt", "Долг", c1, salesID(11), supplierReceiptType, 77),
			record(salesID(12), supplierReceiptType, "1", "2026-09-15T00:00:00", "Receipt", "Долг", c2, salesID(12), supplierReceiptType, 10),
			inactive,
			foreign,
		},
		"Catalog_ДоговорыКонтрагентов": {{"Ref_Key": c1, "ВалютаРасчетов_Key": salesID(70)}},
		"Document_ПриходнаяНакладная": {
			header(r1, "R1", "2026-09-05T10:00:00", true, false, 1000.10),
			header(salesID(13), "R-DRAFT", "2026-09-25T10:00:00", false, false, 40),
			header(salesID(14), "R-DEL", "2026-09-26T10:00:00", false, true, 41),
			header(salesID(15), "R-OLD", "2026-07-26T10:00:00", false, false, 42),
		},
		"Document_РасходСоСчета": {
			{"Ref_Key": p1, "Number": "P1", "Date": "2026-09-10T09:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": org, "Контрагент_Key": supplier, "СуммаДокумента": 600, "БанковскийСчет_Key": salesID(90)},
			{"Ref_Key": p2, "Number": "P2", "Date": "2026-09-20T09:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": org, "Контрагент_Key": supplier, "СуммаДокумента": 50, "БанковскийСчет_Key": salesID(91)},
		},
		"Document_РасходИзКассы":          {{"Ref_Key": k1, "Number": "K1", "Date": "2026-09-12T09:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": org, "Контрагент_Key": supplier, "СуммаДокумента": 300, "Касса_Key": salesID(92)}},
		"Document_ПлатежноеПоручение":     {header(salesID(16), "PO1", "2026-09-09T09:00:00", false, false, 600)},
		"Document_СчетНаОплатуПоставщика": {header(salesID(17), "INV1", "2026-09-04T09:00:00", true, false, 1000.10)},
	}}
}

func reconciliationQuery() SupplierReconciliationQuery {
	return SupplierReconciliationQuery{OrganizationID: salesID(80), SupplierID: salesID(50), From: "2026-09-01", To: "2026-09-30", Limit: 5}
}

func TestReconcileSupplierBalancesPerContractAndType(t *testing.T) {
	reader := reconciliationFixture()
	result, err := Service{OData: reader}.ReconcileSupplier(context.Background(), reconciliationQuery())
	if err != nil {
		t.Fatal(err)
	}
	for _, filter := range reader.filters {
		if filter != "" {
			t.Fatalf("reconciliation must not use $filter: %q", filter)
		}
	}
	got := make([]string, 0, len(result.Balances))
	for _, balance := range result.Balances {
		got = append(got, balance.ContractID[len(balance.ContractID)-2:]+"/"+balance.SettlementType+" "+string(balance.Opening)+" +"+string(balance.Increase)+" -"+string(balance.Decrease)+" = "+string(balance.Closing))
	}
	want := []string{
		"3c/Аванс -100 +100 -50 = -50",
		"3c/Долг 300 +1000.1 -1000 = 300.1",
		"3c/total 200 +1100.1 -1050 = 250.1",
		"3d/Долг 0 +10 -0 = 10",
		"3d/total 0 +10 -0 = 10",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("balances:\n%s", strings.Join(got, "\n"))
	}
	if result.Balances[0].CurrencyID != salesID(70) || result.Balances[3].CurrencyID != "" || result.Complete {
		t.Fatalf("an unreadable contract currency must be reported: %+v", result)
	}
	if !result.Derived || result.OfficialReportCompared || result.InactiveRecords != 1 {
		t.Fatalf("labels: %+v", result)
	}
	items := make([]string, 0, len(result.OpenItems))
	for _, item := range result.OpenItems {
		items = append(items, item.Kind+" "+item.DocumentID[len(item.DocumentID)-1:]+" "+string(item.Balance))
	}
	if strings.Join(items, ",") != "unallocated-advance 6 -50,outstanding-receipt 1 300.1,outstanding-receipt c 10" {
		t.Fatalf("open items: %v", items)
	}
	if len(result.Allocations) != 4 || result.Allocations[0].Source != "advance-offset" || result.Allocations[0].PaymentID != salesID(3) || result.Allocations[1].AllocationMethod != "Автоматически" {
		t.Fatalf("allocations: %+v", result.Allocations)
	}
	if result.MovementsTotal != 7 || len(result.Movements) != 5 || result.NextOffset == nil || *result.NextOffset != 5 {
		t.Fatalf("movement page: %d %d %v", result.MovementsTotal, len(result.Movements), result.NextOffset)
	}
	excluded := make([]string, 0)
	for _, item := range append(result.Exclusions, result.NotPayments...) {
		excluded = append(excluded, item.Number+":"+item.Reason)
	}
	if strings.Join(excluded, ",") != "R-DRAFT:not posted, so it has no settlement records,R-DEL:marked for deletion,PO1:a payment order is an instruction to the bank, not a recorded payment,INV1:a payment plan is not a payment" {
		t.Fatalf("exclusions: %v", excluded)
	}
}

func TestReconcileSupplierFiltersKeepTheirMeaning(t *testing.T) {
	query := reconciliationQuery()
	query.AccountID = salesID(90)
	result, err := Service{OData: reconciliationFixture()}.ReconcileSupplier(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Allocations) != 1 || result.Allocations[0].RecorderID != salesID(4) || len(result.Balances) != 5 || result.Balances[2].Closing != "250.1" {
		t.Fatalf("the account filter must narrow only allocations: %+v %+v", result.Allocations, result.Balances)
	}
	query = reconciliationQuery()
	query.ContractID = salesID(61)
	result, err = Service{OData: reconciliationFixture()}.ReconcileSupplier(context.Background(), query)
	if err != nil || len(result.Balances) != 2 || result.Balances[0].Closing != "10" || len(result.Allocations) != 0 {
		t.Fatalf("contract filter: %+v, %v", result.Balances, err)
	}
}

func TestReconcileSupplierRejectsBadInputAndRepeatedRecords(t *testing.T) {
	svc := Service{OData: reconciliationFixture()}
	for name, mutate := range map[string]func(*SupplierReconciliationQuery){
		"no supplier":   func(q *SupplierReconciliationQuery) { q.SupplierID = "" },
		"no period":     func(q *SupplierReconciliationQuery) { q.From = "" },
		"reversed":      func(q *SupplierReconciliationQuery) { q.From, q.To = "2026-10-01", "2026-09-01" },
		"too long":      func(q *SupplierReconciliationQuery) { q.From = "2020-01-01" },
		"bad contract":  func(q *SupplierReconciliationQuery) { q.ContractID = "x" },
		"no page limit": func(q *SupplierReconciliationQuery) { q.Limit = 0 },
		"negative skip": func(q *SupplierReconciliationQuery) { q.Offset = -1 },
	} {
		query := reconciliationQuery()
		mutate(&query)
		if _, err := svc.ReconcileSupplier(context.Background(), query); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
	reader := reconciliationFixture()
	register := "AccumulationRegister_РасчетыСПоставщиками_RecordType"
	reader.rows[register] = append(reader.rows[register], reader.rows[register][0])
	if _, err := (Service{OData: reader}).ReconcileSupplier(context.Background(), reconciliationQuery()); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("repeated records must fail: %v", err)
	}
	reader = reconciliationFixture()
	reader.fail = map[string]bool{"Document_ПлатежноеПоручение": true}
	result, err := Service{OData: reader}.ReconcileSupplier(context.Background(), reconciliationQuery())
	if err != nil || result.Complete {
		t.Fatalf("an unreadable source must mark the result incomplete: %v", err)
	}
}
