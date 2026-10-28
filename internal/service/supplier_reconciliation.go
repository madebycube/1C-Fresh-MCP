package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const maxReconciliationDays = 5 * 366

type SupplierReconciliationQuery struct {
	OrganizationID string
	SupplierID     string
	ContractID     string
	AccountID      string
	From           string
	To             string
	Limit          int
	Offset         int
}

// SupplierReconciliation is derived from the supplier-settlement register over OData; it is not the official UNF report.
// Amounts are positive when the organization owes the supplier and negative for advances and supplier credits.
type SupplierReconciliation struct {
	OrganizationID         string                  `json:"organization_id"`
	SupplierID             string                  `json:"supplier_id"`
	ContractID             string                  `json:"contract_id,omitempty"`
	AccountID              string                  `json:"account_id,omitempty"`
	From                   string                  `json:"from"`
	To                     string                  `json:"to"`
	Derived                bool                    `json:"derived"`
	OfficialReportCompared bool                    `json:"official_report_compared"`
	SignConvention         string                  `json:"sign_convention"`
	Balances               []ContractBalance       `json:"balances"`
	OpenItems              []ReconciliationItem    `json:"open_items"`
	Allocations            []PaymentAllocation     `json:"allocations"`
	Movements              []SettlementMovement    `json:"movements"`
	MovementsTotal         int                     `json:"movements_total"`
	Offset                 int                     `json:"offset"`
	NextOffset             *int                    `json:"next_offset,omitempty"`
	Exclusions             []ReconciliationExclude `json:"exclusions"`
	NotPayments            []ReconciliationExclude `json:"not_payments"`
	InactiveRecords        int                     `json:"inactive_records"`
	Complete               bool                    `json:"complete"`
	Scans                  []TraceScan             `json:"scans"`
	Notes                  []string                `json:"notes"`
}

// ContractBalance is one contract and settlement type; SettlementType "total" sums the types of that contract.
// Currency amounts are in the contract's settlement currency and accounting amounts in the base currency.
type ContractBalance struct {
	ContractID          string  `json:"contract_id"`
	CurrencyID          string  `json:"currency_id,omitempty"`
	SettlementType      string  `json:"settlement_type"`
	Opening             Decimal `json:"opening"`
	Increase            Decimal `json:"increase"`
	Decrease            Decimal `json:"decrease"`
	Closing             Decimal `json:"closing"`
	OpeningAccounting   Decimal `json:"opening_accounting"`
	ClosingAccounting   Decimal `json:"closing_accounting"`
	PeriodMovementCount int     `json:"period_movements"`
}

// ReconciliationItem is a settlement document with a nonzero balance at the end of the period.
type ReconciliationItem struct {
	ContractID     string  `json:"contract_id"`
	SettlementType string  `json:"settlement_type"`
	Kind           string  `json:"kind"`
	DocumentID     string  `json:"document_id"`
	DocumentType   string  `json:"document_type"`
	Balance        Decimal `json:"balance"`
}

// PaymentAllocation is a recorded payment or offset closing a settlement document within the period.
type PaymentAllocation struct {
	ContractID       string  `json:"contract_id"`
	Source           string  `json:"source"`
	RecorderID       string  `json:"recorder_id"`
	RecorderType     string  `json:"recorder_type"`
	Period           string  `json:"period"`
	DocumentID       string  `json:"document_id"`
	DocumentType     string  `json:"document_type"`
	PaymentID        string  `json:"payment_document_id,omitempty"`
	PaymentType      string  `json:"payment_document_type,omitempty"`
	Amount           Decimal `json:"amount"`
	AllocationMethod string  `json:"allocation_method,omitempty"`
}

type ReconciliationExclude struct {
	Kind    string  `json:"kind"`
	ID      string  `json:"id"`
	Number  string  `json:"number"`
	Date    string  `json:"date"`
	Posted  bool    `json:"posted"`
	Deleted bool    `json:"deleted"`
	Amount  Decimal `json:"amount"`
	Reason  string  `json:"reason"`
}

type reconciliationBucket struct {
	opening, increase, decrease, openingAccounting, periodAccounting stockSum
	movements                                                        int
}

// ReconcileSupplier reads the supplier-settlement register without $filter and totals it per contract and settlement type.
func (s Service) ReconcileSupplier(ctx context.Context, query SupplierReconciliationQuery) (SupplierReconciliation, error) {
	if !linkedGUID(query.OrganizationID) || !linkedGUID(query.SupplierID) {
		return SupplierReconciliation{}, errors.New("organization and supplier must be nonzero GUIDs")
	}
	for _, id := range []string{query.ContractID, query.AccountID} {
		if id != "" && !linkedGUID(id) {
			return SupplierReconciliation{}, errors.New("contract and account filters must be nonzero GUIDs")
		}
	}
	if err := validateListPage(query.Limit, query.Offset); err != nil {
		return SupplierReconciliation{}, err
	}
	start, end, err := reconciliationRange(query.From, query.To)
	if err != nil {
		return SupplierReconciliation{}, err
	}
	result := SupplierReconciliation{
		OrganizationID: strings.ToLower(query.OrganizationID), SupplierID: strings.ToLower(query.SupplierID), ContractID: strings.ToLower(query.ContractID),
		AccountID: strings.ToLower(query.AccountID), From: query.From, To: query.To, Derived: true, Offset: query.Offset, Complete: true,
		SignConvention: "positive = the organization owes the supplier; negative = advance paid or supplier credit",
		Balances:       []ContractBalance{}, OpenItems: []ReconciliationItem{}, Allocations: []PaymentAllocation{}, Movements: []SettlementMovement{},
		Exclusions: []ReconciliationExclude{}, NotPayments: []ReconciliationExclude{}, Scans: []TraceScan{},
		Notes: []string{
			"Derived from AccumulationRegister_РасчетыСПоставщиками over OData; not yet compared with the UNF supplier-settlement report.",
			"Only posted documents write register records, so balances include posted receipts, payments, offsets, corrections and opening balances only.",
			"Contracts and settlement types are never netted together; compare a contract's total row, not the sum of contracts, unless their currencies match.",
		},
	}
	register := config.SupplierSettlements
	rows, err := s.scanRows(ctx, register.Name, sourceFields(register.Fields), "", maxDocumentScan)
	if err != nil {
		return SupplierReconciliation{}, err
	}
	result.Scans = append(result.Scans, TraceScan{Resource: register.Name, Rows: len(rows)})
	buckets := make(map[[2]string]*reconciliationBucket)
	documents := make(map[[4]string]*stockSum)
	seen := make(map[string]bool, len(rows))
	periodMovements := make([]SettlementMovement, 0)
	for _, row := range rows {
		movement, err := bindFields[SettlementMovement](row, register.Fields)
		if err != nil {
			return SupplierReconciliation{}, errors.New("invalid OData settlement record")
		}
		key := strings.ToLower(movement.RecorderType + "/" + movement.RecorderID + "/" + movement.LineNumber)
		if seen[key] {
			return SupplierReconciliation{}, errors.New("settlement records repeated during lookup; retry")
		}
		seen[key] = true
		if !strings.EqualFold(movement.OrganizationID, query.OrganizationID) || !strings.EqualFold(movement.SupplierID, query.SupplierID) || !idMatches(query.ContractID, movement.ContractID) {
			continue
		}
		if !movement.Active {
			result.InactiveRecords++
			continue
		}
		if movement.Period >= end {
			continue
		}
		if movement.RecordType != "Receipt" && movement.RecordType != "Expense" {
			return SupplierReconciliation{}, errors.New("unknown settlement record type " + movement.RecordType)
		}
		movement.Source = settlementSources[movement.RecorderType]
		if movement.Source == "" {
			movement.Source = "other"
		} else if movement.Source == "receipt" && movement.RecordType == "Expense" {
			movement.Source = "advance-offset"
		}
		movement.Counted = true
		clearUnlinked(&movement.OrganizationID, &movement.SupplierID, &movement.ContractID)
		if movement.PaymentDocumentType == "StandardODATA.Undefined" || !linkedGUID(movement.PaymentDocumentID) {
			movement.PaymentDocumentID, movement.PaymentDocumentType = "", ""
		}
		contract := strings.ToLower(movement.ContractID)
		signedCurrency, signedAccounting := movement.CurrencyAmount, movement.Amount
		if movement.RecordType == "Expense" {
			signedCurrency, signedAccounting = negate(signedCurrency), negate(signedAccounting)
		}
		for _, settlementType := range []string{movement.SettlementType, "total"} {
			bucketKey := [2]string{contract, settlementType}
			bucket := buckets[bucketKey]
			if bucket == nil {
				bucket = &reconciliationBucket{}
				buckets[bucketKey] = bucket
			}
			var err error
			if movement.Period < start {
				err = errors.Join(bucket.opening.Add(signedCurrency), bucket.openingAccounting.Add(signedAccounting))
			} else {
				target := &bucket.increase
				if movement.RecordType == "Expense" {
					target = &bucket.decrease
				}
				err = errors.Join(target.Add(movement.CurrencyAmount), bucket.periodAccounting.Add(signedAccounting))
				bucket.movements++
			}
			if err != nil {
				return SupplierReconciliation{}, errors.New("invalid settlement amount")
			}
		}
		documentKey := [4]string{contract, movement.SettlementType, movement.DocumentType, strings.ToLower(movement.DocumentID)}
		if documents[documentKey] == nil {
			documents[documentKey] = &stockSum{}
		}
		if err := documents[documentKey].Add(signedCurrency); err != nil {
			return SupplierReconciliation{}, errors.New("invalid settlement amount")
		}
		if movement.Period >= start {
			periodMovements = append(periodMovements, movement)
		}
	}
	currencies := s.contractCurrencies(ctx, buckets, &result)
	keys := make([][2]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] != "total" && (keys[j][1] == "total" || keys[i][1] < keys[j][1])
	})
	for _, key := range keys {
		bucket := buckets[key]
		closing, closingAccounting := &stockSum{}, &stockSum{}
		_ = closing.Add(bucket.opening.Decimal())
		_ = closing.Add(bucket.increase.Decimal())
		_ = closing.Add(negate(bucket.decrease.Decimal()))
		_ = closingAccounting.Add(bucket.openingAccounting.Decimal())
		_ = closingAccounting.Add(bucket.periodAccounting.Decimal())
		result.Balances = append(result.Balances, ContractBalance{
			ContractID: key[0], CurrencyID: currencies[key[0]], SettlementType: key[1],
			Opening: bucket.opening.Decimal(), Increase: bucket.increase.Decimal(), Decrease: bucket.decrease.Decimal(), Closing: closing.Decimal(),
			OpeningAccounting: bucket.openingAccounting.Decimal(), ClosingAccounting: closingAccounting.Decimal(), PeriodMovementCount: bucket.movements,
		})
	}
	for key, sum := range documents {
		balance := sum.Decimal()
		if decimalEqual(balance, "0") {
			continue
		}
		item := ReconciliationItem{ContractID: key[0], SettlementType: key[1], DocumentType: key[2], DocumentID: key[3], Balance: balance, Kind: "other"}
		positive := !strings.HasPrefix(string(balance), "-")
		switch {
		case positive && strings.EqualFold(key[2], supplierReceiptType):
			item.Kind = "outstanding-receipt"
		case positive:
			item.Kind = "outstanding-debt"
		case settlementSources[item.DocumentType] == "bank-out" || settlementSources[item.DocumentType] == "cash-out":
			item.Kind = "unallocated-advance"
		default:
			item.Kind = "supplier-credit"
		}
		result.OpenItems = append(result.OpenItems, item)
	}
	sort.Slice(result.OpenItems, func(i, j int) bool {
		a, b := result.OpenItems[i], result.OpenItems[j]
		return a.ContractID+a.SettlementType+a.Kind+a.DocumentID < b.ContractID+b.SettlementType+b.Kind+b.DocumentID
	})
	accounts, err := s.paymentAccounts(ctx, query.AccountID, &result)
	if err != nil {
		return SupplierReconciliation{}, err
	}
	sort.Slice(periodMovements, func(i, j int) bool {
		a, b := periodMovements[i], periodMovements[j]
		return a.Period+a.RecorderID+a.LineNumber < b.Period+b.RecorderID+b.LineNumber
	})
	for _, movement := range periodMovements {
		if movement.RecordType != "Expense" || movement.Source == "receipt" {
			continue
		}
		if query.AccountID != "" && !strings.EqualFold(accounts[strings.ToLower(movement.RecorderID)], query.AccountID) {
			continue
		}
		result.Allocations = append(result.Allocations, PaymentAllocation{
			ContractID: movement.ContractID, Source: movement.Source, RecorderID: movement.RecorderID, RecorderType: movement.RecorderType, Period: movement.Period,
			DocumentID: movement.DocumentID, DocumentType: movement.DocumentType, PaymentID: movement.PaymentDocumentID, PaymentType: movement.PaymentDocumentType,
			Amount: movement.CurrencyAmount, AllocationMethod: movement.AllocationMethod,
		})
	}
	result.MovementsTotal = len(periodMovements)
	first := min(query.Offset, len(periodMovements))
	last := min(first+query.Limit, len(periodMovements))
	result.Movements = append(result.Movements, periodMovements[first:last]...)
	if last < len(periodMovements) {
		result.NextOffset = &last
	}
	s.reconciliationExclusions(ctx, query, start, end, &result)
	return result, nil
}

func reconciliationRange(from, to string) (string, string, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return "", "", errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Format("2006-01-02") != to || end.Before(start) {
		return "", "", errors.New("to must be YYYY-MM-DD and no earlier than from")
	}
	if end.Sub(start) > maxReconciliationDays*24*time.Hour {
		return "", "", errors.New("period must be at most five years")
	}
	return from + "T00:00:00", end.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00", nil
}

func negate(value Decimal) Decimal {
	text := string(value)
	if trimmed, ok := strings.CutPrefix(text, "-"); ok {
		return Decimal(trimmed)
	}
	return Decimal("-" + text)
}

// contractCurrencies reads each contract's settlement currency; an unreadable contract leaves its currency empty and marks the result incomplete.
func (s Service) contractCurrencies(ctx context.Context, buckets map[[2]string]*reconciliationBucket, result *SupplierReconciliation) map[string]string {
	currencies := make(map[string]string)
	for key := range buckets {
		contract := key[0]
		if _, done := currencies[contract]; done || !linkedGUID(contract) {
			continue
		}
		currencies[contract] = ""
		data, err := s.OData.Get(ctx, "Catalog_ДоговорыКонтрагентов(guid'"+contract+"')", url.Values{"$format": {"json"}, "$select": {"Ref_Key,ВалютаРасчетов_Key"}}, 1<<20)
		var row map[string]json.RawMessage
		if err == nil && json.Unmarshal(data, &row) == nil && linkedGUID(rawString(row["ВалютаРасчетов_Key"])) {
			currencies[contract] = strings.ToLower(rawString(row["ВалютаРасчетов_Key"]))
			continue
		}
		result.Complete = false
		result.Notes = append(result.Notes, "Settlement currency of contract "+contract+" could not be read; do not compare its amounts with other contracts.")
	}
	return currencies
}

// paymentAccounts maps bank and cash expenses to their account only when an account filter is given.
// The filter narrows the allocation list; balances always cover every account.
func (s Service) paymentAccounts(ctx context.Context, accountID string, result *SupplierReconciliation) (map[string]string, error) {
	accounts := make(map[string]string)
	if accountID == "" {
		return accounts, nil
	}
	result.Notes = append(result.Notes, "The account filter narrows only the allocation list to bank and cash expenses from that account; balances and open items still cover every account.")
	for resource, field := range map[string]string{"Document_РасходСоСчета": "БанковскийСчет_Key", "Document_РасходИзКассы": "Касса_Key"} {
		rows, err := s.scanRows(ctx, resource, "Ref_Key,"+field, "", maxDocumentScan)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			accounts[strings.ToLower(rawString(row["Ref_Key"]))] = strings.ToLower(rawString(row[field]))
		}
	}
	return accounts, nil
}

var reconciliationDocuments = []struct {
	kind, resource, amount string
	counted                bool
}{
	{"receipt", "Document_ПриходнаяНакладная", "СуммаДокумента", true},
	{"bank-out", "Document_РасходСоСчета", "СуммаДокумента", true},
	{"cash-out", "Document_РасходИзКассы", "СуммаДокумента", true},
	{"payment-order", "Document_ПлатежноеПоручение", "СуммаДокумента", false},
	{"payment-plan", "Document_СчетНаОплатуПоставщика", "СуммаДокумента", false},
}

// reconciliationExclusions lists the supplier's documents in the period that the balance leaves out:
// unposted or deletion-marked settlement documents, and payment orders and plans, which are never payments.
func (s Service) reconciliationExclusions(ctx context.Context, query SupplierReconciliationQuery, start, end string, result *SupplierReconciliation) {
	for _, source := range reconciliationDocuments {
		rows, err := s.scanRows(ctx, source.resource, "Ref_Key,Number,Date,Posted,DeletionMark,Организация_Key,Контрагент_Key,"+source.amount, "", maxDocumentScan)
		scan := TraceScan{Resource: source.resource, Rows: len(rows)}
		if err != nil {
			scan.Error, result.Complete = err.Error(), false
		}
		for _, row := range rows {
			date := rawString(row["Date"])
			if !strings.EqualFold(rawString(row["Организация_Key"]), query.OrganizationID) || !strings.EqualFold(rawString(row["Контрагент_Key"]), query.SupplierID) || date < start || date >= end {
				continue
			}
			item := ReconciliationExclude{
				Kind: source.kind, ID: strings.ToLower(rawString(row["Ref_Key"])), Number: strings.TrimSpace(rawString(row["Number"])), Date: date,
				Posted: string(row["Posted"]) == "true", Deleted: string(row["DeletionMark"]) == "true", Amount: rawDecimal(row[source.amount]),
			}
			switch {
			case !source.counted && item.Kind == "payment-order":
				item.Reason = "a payment order is an instruction to the bank, not a recorded payment"
				result.NotPayments = append(result.NotPayments, item)
			case !source.counted:
				item.Reason = "a payment plan is not a payment"
				result.NotPayments = append(result.NotPayments, item)
			case item.Deleted:
				item.Reason = "marked for deletion"
				result.Exclusions = append(result.Exclusions, item)
			case !item.Posted:
				item.Reason = "not posted, so it has no settlement records"
				result.Exclusions = append(result.Exclusions, item)
			}
		}
		result.Scans = append(result.Scans, scan)
	}
	result.Notes = append(result.Notes, "Debt offsets (Взаимозачет) and receipt corrections are counted from the register; unposted ones are not listed as exclusions. No source confirms bank payments independently of the recorded expense documents.")
}
