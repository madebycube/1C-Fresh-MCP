package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type BankStatementCandidate struct {
	ID        string `json:"id"`
	Number    string `json:"number"`
	Operation string `json:"operation"`
	Posted    bool   `json:"posted"`
	PartnerID string `json:"counterparty_id,omitempty"`
	BasisID   string `json:"basis_id,omitempty"`
	BasisType string `json:"basis_type,omitempty"`
}

type BankStatementReviewRow struct {
	EntryID        string                   `json:"entry_id"`
	Status         string                   `json:"status"`
	CandidateCount int                      `json:"candidate_count"`
	Candidates     []BankStatementCandidate `json:"candidates"`
}

type BankStatementReview struct {
	Statement BankStatement            `json:"statement"`
	AccountID string                   `json:"account_id"`
	Rows      []BankStatementReviewRow `json:"rows"`
	Counts    map[string]int           `json:"counts"`
}

const bankAccountOwnerType = "StandardODATA.Catalog_Организации"

func (s Service) statementAccount(ctx context.Context, id, number string) (string, error) {
	account, err := s.activeRecord(ctx, config.MoneyAccounts["bank"].Name, id, "Ref_Key,DeletionMark,Недействителен,Owner,Owner_Type,НомерСчета")
	if err != nil {
		return "", fmt.Errorf("read statement account: %w", err)
	}
	owner := rawString(account["Owner"])
	if rawString(account["НомерСчета"]) != number || rawString(account["Owner_Type"]) != bankAccountOwnerType ||
		!linkedGUID(owner) || string(account["Недействителен"]) != "false" {
		return "", errors.New("the 1C bank account is inactive, does not belong to an organization, or has a different number")
	}
	return strings.ToLower(owner), nil
}

func (s Service) ReviewBankStatement(ctx context.Context, input io.Reader, scope BankStatementScope, accountID string) (BankStatementReview, error) {
	statement, err := ParseBankStatement(input, scope)
	if err != nil {
		return BankStatementReview{}, err
	}
	if _, err := s.statementAccount(ctx, accountID, scope.AccountNumber); err != nil {
		return BankStatementReview{}, err
	}
	type key struct {
		kind, date string
		amount     Decimal
	}
	index := map[key][]BankStatementCandidate{}
	expected := map[key]int{}
	kinds := map[string]bool{}
	for _, entry := range statement.Entries {
		kinds[entry.Kind] = true
		expected[key{entry.Kind, entry.BookedDate, entry.Amount}]++
	}
	for _, kind := range []string{"bank-in", "bank-out"} {
		if !kinds[kind] {
			continue
		}
		plan := config.MoneyDocuments[kind]
		rows, err := s.allDocumentRows(ctx, plan)
		if err != nil {
			return BankStatementReview{}, fmt.Errorf("scan %s documents: %w", kind, err)
		}
		for _, row := range rows {
			if !strings.EqualFold(rawString(row["БанковскийСчет_Key"]), accountID) {
				continue
			}
			date := rawString(row["Date"])[:10] // allDocumentRows validates the date first.
			if date < scope.From || date > scope.To {
				continue
			}
			document, err := moneyDocument(row, plan, kind)
			if err != nil {
				return BankStatementReview{}, err
			}
			if !moneyOperationAllowed(document) {
				continue
			}
			amount, err := exactDecimal(string(document.Amount), 2, false)
			if err != nil {
				return BankStatementReview{}, errors.New("invalid amount on a scoped 1C bank document")
			}
			match := key{kind, date, ratDecimal(amount, 2)}
			if expected[match] > 0 {
				index[match] = append(index[match], BankStatementCandidate{
					ID: document.ID, Number: document.Number, Operation: document.Operation, Posted: document.Posted,
					PartnerID: document.CounterpartyID, BasisID: document.BasisID, BasisType: document.BasisType,
				})
			}
		}
	}
	for match := range index {
		sort.Slice(index[match], func(i, j int) bool { return index[match][i].ID < index[match][j].ID })
	}
	result := BankStatementReview{
		Statement: statement, AccountID: strings.ToLower(accountID),
		Rows: make([]BankStatementReviewRow, 0, len(statement.Entries)), Counts: map[string]int{},
	}
	for _, entry := range statement.Entries {
		match := key{entry.Kind, entry.BookedDate, entry.Amount}
		candidates := index[match]
		status := "candidate"
		switch {
		case len(candidates) == 0:
			status = "no_candidate"
		case len(candidates) > 1 || expected[match] > 1:
			status = "ambiguous"
		}
		result.Rows = append(result.Rows, BankStatementReviewRow{
			EntryID: entry.ID, Status: status, CandidateCount: len(candidates),
			Candidates: append([]BankStatementCandidate{}, candidates[:min(len(candidates), 10)]...),
		})
		result.Counts[status]++
	}
	return result, nil
}

type BankStatementPostingRequest struct {
	BankStatementScope
	AccountID           string `json:"account_id"`
	EntryID             string `json:"entry_id"`
	DocumentID          string `json:"document_id"`
	ExpectedDataVersion string `json:"expect_version,omitempty"`
	Apply               bool   `json:"apply,omitempty"`
}

type BankStatementPosting struct {
	Entry       BankStatementEntry `json:"entry"`
	Document    MoneyDocument      `json:"document"`
	DataVersion string             `json:"data_version"`
	Status      string             `json:"status"`
	Movements   *DocumentMovements `json:"movements,omitempty"`
	Error       string             `json:"error,omitempty"`
}

// PostBankStatementPayment validates the selected row and document again before
// invoking the existing OData posting action. A posted document is verified on
// reruns, including when the successful action's response was lost.
func (s Service) PostBankStatementPayment(ctx context.Context, input io.Reader, request BankStatementPostingRequest) (BankStatementPosting, error) {
	statement, err := ParseBankStatement(input, request.BankStatementScope)
	if err != nil {
		return BankStatementPosting{}, err
	}
	var entry BankStatementEntry
	for _, candidate := range statement.Entries {
		if candidate.ID == request.EntryID {
			entry = candidate
			break
		}
	}
	if entry.ID == "" || !linkedGUID(request.DocumentID) {
		return BankStatementPosting{}, errors.New("select a statement entry ID and a valid 1C document GUID")
	}
	if request.Apply && !statement.hasIndependentTotals() {
		return BankStatementPosting{}, errors.New("posting requires both turnover control totals or a closing balance in the source statement")
	}
	organization, err := s.statementAccount(ctx, request.AccountID, request.AccountNumber)
	if err != nil {
		return BankStatementPosting{}, err
	}
	result, err := s.readStatementPosting(ctx, request, entry, organization)
	if err != nil {
		return BankStatementPosting{}, err
	}
	if result.Document.Posted {
		return s.verifyStatementPosting(ctx, result), nil
	}
	if request.ExpectedDataVersion != "" && request.ExpectedDataVersion != result.DataVersion {
		return BankStatementPosting{}, errors.New("bank document changed since the preview; preview again")
	}
	result.Status = "ready"
	if !request.Apply {
		return result, nil
	}
	if request.ExpectedDataVersion == "" {
		return BankStatementPosting{}, errors.New("posting requires the data version from the preview")
	}
	actor, ok := s.OData.(documentActor)
	if !ok {
		return BankStatementPosting{}, errors.New("OData client does not support document actions")
	}
	if _, ok := s.OData.(recordSetReader); !ok {
		return BankStatementPosting{}, errors.New("OData client cannot verify the resulting register movements")
	}
	resource := config.MoneyDocuments[entry.Kind].Name + "(guid'" + strings.ToLower(request.DocumentID) + "')"
	_, actionErr := actor.Action(ctx, resource, "Post")
	saved, readErr := s.readStatementPosting(ctx, request, entry, organization)
	if readErr == nil && saved.Document.Posted {
		return s.verifyStatementPosting(ctx, saved), nil
	}
	result.Status = "posting_uncertain"
	result.Error = errors.Join(errors.New("1C did not confirm the posting"), actionErr, readErr).Error()
	return result, nil
}

func (s BankStatement) hasIndependentTotals() bool {
	incoming, outgoing := false, false
	for _, total := range s.CheckedTotals {
		switch total {
		case "closing":
			return true
		case "incoming":
			incoming = true
		case "outgoing":
			outgoing = true
		}
	}
	return incoming && outgoing
}

func (s Service) readStatementPosting(ctx context.Context, request BankStatementPostingRequest, entry BankStatementEntry, organization string) (BankStatementPosting, error) {
	document, row, err := s.readMoneyDocument(ctx, entry.Kind, request.DocumentID, "DataVersion,Организация_Key")
	if err != nil {
		return BankStatementPosting{}, err
	}
	version := rawString(row["DataVersion"])
	if version == "" || !strings.EqualFold(rawString(row["Организация_Key"]), organization) ||
		!strings.EqualFold(document.AccountID, request.AccountID) ||
		!strings.HasPrefix(document.Date, entry.BookedDate+"T") ||
		!decimalEqual(document.Amount, entry.Amount) || document.Deleted {
		return BankStatementPosting{}, errors.New("1C bank document does not match the statement account, organization, date or amount, or has no data version")
	}
	result := BankStatementPosting{Entry: entry, Document: document, DataVersion: version}
	if err := s.validateStatementCounterparty(ctx, document, entry); err != nil {
		return BankStatementPosting{}, err
	}
	if document.Posted {
		return result, nil
	}
	if entry.Kind == "bank-in" && document.Operation != "ОтПокупателя" ||
		entry.Kind == "bank-out" && document.Operation != "Поставщику" {
		return BankStatementPosting{}, unsupported("statement posting currently supports customer receipts and supplier disbursements")
	}
	if err := s.validateStatementAllocation(ctx, document, entry, organization); err != nil {
		return BankStatementPosting{}, err
	}
	return result, nil
}

func (s Service) validateStatementCounterparty(ctx context.Context, document MoneyDocument, entry BankStatementEntry) error {
	currency, err := s.activeRecord(ctx, config.Currencies.Name, document.CurrencyID, "Ref_Key,DeletionMark,Code")
	if err != nil || rawString(currency["Code"]) != receiptBaseCurrency {
		return errors.New("statement posting requires a ruble bank document")
	}
	partner, err := s.activeRecord(ctx, config.Customers.Name, document.CounterpartyID, "Ref_Key,DeletionMark,IsFolder,Недействителен,ИНН")
	if err != nil || string(partner["IsFolder"]) != "false" || string(partner["Недействителен"]) != "false" ||
		entry.CounterpartyINN == "" || entry.CounterpartyINN != rawString(partner["ИНН"]) {
		return errors.New("the active counterparty INN must match the statement")
	}
	return nil
}

func (s Service) validateStatementAllocation(ctx context.Context, document MoneyDocument, entry BankStatementEntry, organization string) error {
	var total stockSum
	contracts := map[string]bool{}
	bases := map[string]bool{}
	for _, line := range document.Allocations {
		if !linkedGUID(line.ContractID) || !linkedGUID(line.CashFlowItemID) ||
			(!line.Advance && !linkedGUID(line.DocumentID) && !linkedGUID(line.OrderID) && !linkedGUID(line.InvoiceID) && !linkedGUID(line.PlanID)) {
			return errors.New("each allocation needs a contract, cash-flow item, and settlement basis or advance flag")
		}
		if _, err := exactDecimal(string(line.PaymentAmount), 2, false); err != nil {
			return errors.New("each allocation must have a positive amount in rubles")
		}
		if err := total.Add(line.PaymentAmount); err != nil {
			return err
		}
		for _, base := range []struct{ id, kind string }{
			{line.DocumentID, line.DocumentType}, {line.OrderID, line.OrderType},
			{line.InvoiceID, line.InvoiceType}, {line.PlanID, line.PlanType},
		} {
			if base.id == "" {
				continue
			}
			key := base.kind + "/" + base.id + "/" + line.ContractID
			if bases[key] {
				continue
			}
			if err := s.validateStatementBasis(ctx, document, organization, line.ContractID, base.id, base.kind); err != nil {
				return err
			}
			bases[key] = true
		}
		if contracts[line.ContractID] {
			continue
		}
		contract, err := s.activeRecord(ctx, "Catalog_ДоговорыКонтрагентов", line.ContractID, "Ref_Key,DeletionMark,Owner,Организация_Key,ВалютаРасчетов_Key,Недействителен,РасчетыВУсловныхЕдиницах")
		if err != nil || !strings.EqualFold(rawString(contract["Owner"]), document.CounterpartyID) ||
			!strings.EqualFold(rawString(contract["Организация_Key"]), organization) ||
			!strings.EqualFold(rawString(contract["ВалютаРасчетов_Key"]), document.CurrencyID) ||
			string(contract["Недействителен"]) != "false" || string(contract["РасчетыВУсловныхЕдиницах"]) != "false" {
			return errors.New("each contract must be active and match the document organization, counterparty and currency")
		}
		contracts[line.ContractID] = true
	}
	if !decimalEqual(total.Decimal(), entry.Amount) {
		return errors.New("payment allocations do not add up to the statement amount")
	}
	return nil
}

func (s Service) validateStatementBasis(ctx context.Context, doc MoneyDocument, organization, contract, id, kind string) error {
	resource, ok := strings.CutPrefix(kind, "StandardODATA.")
	if !ok {
		return errors.New("payment basis has no supported 1C document type")
	}
	var postedRequired bool
	switch doc.Kind {
	case "bank-in":
		switch resource {
		case config.SalesDocuments["shipment"].Name:
			postedRequired = true
		case config.SalesDocuments["invoice"].Name, config.CustomerOrders.Name:
		default:
			return unsupported("unsupported customer payment basis: " + kind)
		}
	case "bank-out":
		switch resource {
		case config.OperationalDocuments["goods-receipt"].Name:
			postedRequired = true
		case paymentPlanResourceName, supplierOrderResourceName:
		default:
			return unsupported("unsupported supplier payment basis: " + kind)
		}
	}
	row, err := s.activeRecord(ctx, resource, id, "Ref_Key,DeletionMark,Posted,Организация_Key,Контрагент_Key,Договор_Key")
	if err != nil {
		return fmt.Errorf("read payment basis: %w", err)
	}
	if !strings.EqualFold(rawString(row["Организация_Key"]), organization) ||
		!strings.EqualFold(rawString(row["Контрагент_Key"]), doc.CounterpartyID) ||
		!strings.EqualFold(rawString(row["Договор_Key"]), contract) ||
		!catalogBoolean(row["Posted"]) || postedRequired && string(row["Posted"]) != "true" {
		return errors.New("payment basis does not match the organization, counterparty, contract or required posting status")
	}
	return nil
}

func (s Service) verifyStatementPosting(ctx context.Context, result BankStatementPosting) BankStatementPosting {
	result.Status = "verification_failed"
	movements, err := s.GetDocumentMovements(ctx, result.Document.Kind, result.Document.ID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Movements = &movements
	if err := checkBankMovement(result.Document, result.Entry, movements.Registers); err != nil {
		result.Error = err.Error()
		return result
	}
	result.Status = "verified"
	return result
}

func checkBankMovement(doc MoneyDocument, entry BankStatementEntry, registers []RegisterMovements) error {
	var total stockSum
	matched := false
	direction := "Receipt"
	if doc.Kind == "bank-out" {
		direction = "Expense"
	}
	for _, register := range registers {
		if register.Register != "AccumulationRegister_ДенежныеСредства" {
			continue
		}
		for _, row := range register.Rows {
			if anyString(row["БанковскийСчетКасса_Type"]) != "StandardODATA.Catalog_БанковскиеСчета" ||
				!strings.EqualFold(anyString(row["БанковскийСчетКасса"]), doc.AccountID) {
				continue
			}
			amount := anyDecimal(row["Сумма"])
			if amount == "" {
				// Some OData responses encode decimals as JSON strings.
				amount = Decimal(anyString(row["Сумма"]))
			}
			if active, ok := row["Active"].(bool); !ok || !active ||
				anyString(row["RecordType"]) != direction ||
				!strings.HasPrefix(anyString(row["Period"]), entry.BookedDate+"T") ||
				!strings.EqualFold(anyString(row["Валюта_Key"]), doc.CurrencyID) {
				return errors.New("cash movement activity, direction, date or currency does not match the statement")
			}
			if _, err := exactDecimal(string(amount), 2, false); err != nil || total.Add(amount) != nil {
				return errors.New("invalid cash movement amount")
			}
			matched = true
		}
	}
	if !matched || !decimalEqual(total.Decimal(), entry.Amount) {
		return errors.New("active cash movements for the statement account do not add up to the statement amount")
	}
	return nil
}
