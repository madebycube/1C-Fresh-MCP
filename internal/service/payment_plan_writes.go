package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

const (
	paymentPlanResourceName  = "Document_СчетНаОплатуПоставщика"
	paymentPlanOperation     = "СчетНаОплатуПоставщика"
	paymentPlanMoneyType     = "Безналичные"
	organizationOwnerType    = "StandardODATA.Catalog_Организации"
	counterpartyOwnerType    = "StandardODATA.Catalog_Контрагенты"
	bankAccountsResourceName = "Catalog_БанковскиеСчета"
)

// PaymentPlanDraft asks for a supplier invoice that plans payment of one goods receipt.
// The invoice is the tenant's payment request: it is saved unposted and never approved by this tool.
type PaymentPlanDraft struct {
	ReceiptID         string `json:"receipt_id"`
	AccountID         string `json:"account_id"`
	SupplierAccountID string `json:"supplier_account_id,omitempty"`
	Date              string `json:"date"`
	PaymentDate       string `json:"payment_date"`
	Amount            string `json:"amount,omitempty"`
	Comment           string `json:"comment,omitempty"`
	AllowDuplicate    bool   `json:"allow_duplicate,omitempty"`
	Apply             bool   `json:"apply,omitempty"`
}

type PaymentPlanWrite struct {
	Applied           bool                `json:"applied"`
	ID                string              `json:"id,omitempty"`
	DataVersion       string              `json:"data_version,omitempty"`
	Posted            bool                `json:"posted"`
	ReceiptID         string              `json:"receipt_id,omitempty"`
	ReceiptNumber     string              `json:"receipt_number,omitempty"`
	ReceiptDate       string              `json:"receipt_date,omitempty"`
	BasisOrderID      string              `json:"basis_order_id,omitempty"`
	OrganizationID    string              `json:"organization_id"`
	SupplierID        string              `json:"supplier_id"`
	ContractID        string              `json:"contract_id"`
	CurrencyID        string              `json:"currency_id"`
	DepartmentID      string              `json:"department_id,omitempty"`
	AccountID         string              `json:"account_id"`
	SupplierAccountID string              `json:"supplier_account_id"`
	PaymentArticleID  string              `json:"payment_article_id,omitempty"`
	Date              string              `json:"date"`
	PaymentDate       string              `json:"payment_date"`
	IncomingNumber    string              `json:"incoming_number"`
	IncomingDate      string              `json:"incoming_date"`
	Comment           string              `json:"comment"`
	DocumentAmount    Decimal             `json:"document_amount"`
	Amount            Decimal             `json:"amount"`
	VAT               Decimal             `json:"vat"`
	Percent           Decimal             `json:"percent"`
	Before            *PaymentPlanTotals  `json:"before,omitempty"`
	Changes           []string            `json:"changes,omitempty"`
	ReceiptStatus     string              `json:"receipt_status,omitempty"`
	Outstanding       Decimal             `json:"outstanding,omitempty"`
	ExistingPlans     []TracedPaymentPlan `json:"existing_plans"`
	Payments          []TracedPayment     `json:"payments"`
	Candidates        []TraceCandidate    `json:"candidates"`
	Duplicates        []TracedPaymentPlan `json:"duplicates"`
	Incomplete        []string            `json:"incomplete"`
	Approval          string              `json:"approval"`
	Impact            []string            `json:"impact"`
}

type PaymentPlanTotals struct {
	PaymentDate string  `json:"payment_date"`
	Amount      Decimal `json:"amount"`
	AccountID   string  `json:"account_id"`
}

type planLine struct {
	ProductID        string
	CharacteristicID string
	UnitID           string
	UnitType         string
	Quantity         Decimal
	Price            Decimal
	Amount           Decimal
	VATRateID        string
	VAT              Decimal
	Total            Decimal
}

type planReceipt struct {
	ID             string
	Number         string
	Date           string
	Posted         bool
	OrganizationID string
	SupplierID     string
	ContractID     string
	CurrencyID     string
	DepartmentID   string
	OrderID        string
	IncomingNumber string
	IncomingDate   string
	IncludesVAT    bool
	Taxation       string
	Amount         *big.Rat
	VAT            *big.Rat
	Lines          []planLine
}

const notApprovedYet = "Saved unposted, so the payment calendar has no record yet. This tool never approves payments; after posting, 1C sets the approval state by its own settings and get payment-plan shows it."

// CreatePaymentPlanDraft previews a supplier invoice for a receipt and, with Apply, saves it unposted.
// It refuses a settled receipt, blocks duplicates unless allowed, and never retries an uncertain POST.
func (s Service) CreatePaymentPlanDraft(ctx context.Context, input PaymentPlanDraft) (PaymentPlanWrite, error) {
	date, err := receiptDate(input.Date, false)
	if err != nil {
		return PaymentPlanWrite{}, errors.New("date must be YYYY-MM-DD")
	}
	paymentDate, err := receiptDate(input.PaymentDate, false)
	if err != nil {
		return PaymentPlanWrite{}, errors.New("payment date must be YYYY-MM-DD")
	}
	if len([]rune(input.Comment)) > 500 {
		return PaymentPlanWrite{}, errors.New("comment must be at most 500 characters")
	}
	receipt, err := s.readPlanReceipt(ctx, input.ReceiptID)
	if err != nil {
		return PaymentPlanWrite{}, err
	}
	if !receipt.Posted {
		return PaymentPlanWrite{}, errors.New("receipt is not posted, so it records no supplier debt to plan; post it first")
	}
	trace, err := s.TraceSupplierReceipt(ctx, receipt.ID)
	if err != nil {
		return PaymentPlanWrite{}, err
	}
	result := PaymentPlanWrite{
		ReceiptID: receipt.ID, ReceiptNumber: receipt.Number, ReceiptDate: receipt.Date[:10], BasisOrderID: receipt.OrderID,
		OrganizationID: receipt.OrganizationID, SupplierID: receipt.SupplierID, ContractID: receipt.ContractID, CurrencyID: receipt.CurrencyID,
		DepartmentID: receipt.DepartmentID, Date: date, PaymentDate: paymentDate, IncomingNumber: receipt.IncomingNumber, IncomingDate: receipt.IncomingDate,
		Comment: planComment(receipt, input.Comment), DocumentAmount: ratDecimal(receipt.Amount, 2), ReceiptStatus: trace.Summary.Status,
		ExistingPlans: trace.PaymentPlans, Payments: trace.Payments, Candidates: trace.Candidates, Duplicates: []TracedPaymentPlan{}, Incomplete: []string{},
		Approval: notApprovedYet,
	}
	for _, scan := range trace.Scans {
		if scan.Error != "" {
			result.Incomplete = append(result.Incomplete, scan.Resource+": "+scan.Error)
		}
	}
	if !trace.Settlements.Available {
		result.Incomplete = append(result.Incomplete, "supplier-settlement register: "+trace.Settlements.Error)
	}
	outstanding := new(big.Rat)
	if trace.Settlements.Available {
		result.Outstanding = trace.Settlements.Outstanding
		outstanding, _ = new(big.Rat).SetString(string(trace.Settlements.Outstanding))
		if outstanding == nil {
			return PaymentPlanWrite{}, errors.New("invalid outstanding amount")
		}
	}
	for _, plan := range trace.PaymentPlans {
		if !plan.Deleted {
			result.Duplicates = append(result.Duplicates, plan)
		}
	}
	amount := outstanding
	if input.Amount != "" {
		if amount, err = exactDecimal(input.Amount, 2, false); err != nil {
			return PaymentPlanWrite{}, errors.New("amount must be a positive number with at most 2 decimals")
		}
	}
	if err := s.fillPlanAccounts(ctx, &result, input.AccountID, input.SupplierAccountID); err != nil {
		return PaymentPlanWrite{}, err
	}
	if amount.Sign() > 0 {
		if err := setPlanAmount(&result, receipt, amount); err != nil {
			return PaymentPlanWrite{}, err
		}
	}
	result.Impact = []string{
		"creates an unposted supplier invoice for " + string(result.DocumentAmount) + " with the receipt's lines and one payment-calendar line of " + string(result.Amount) + " on " + paymentDate[:10],
		"does not post or approve it, and never creates a payment order or bank transfer",
	}
	if !input.Apply {
		return result, nil
	}
	switch {
	case len(result.Incomplete) > 0:
		return PaymentPlanWrite{}, errors.New("payment evidence is incomplete, so the outstanding amount is unknown; retry when every source is readable")
	case outstanding.Sign() <= 0:
		return PaymentPlanWrite{}, errors.New("receipt is " + trace.Summary.Status + " with nothing outstanding; a new payment plan would double-plan the outflow")
	case amount.Cmp(outstanding) > 0:
		return PaymentPlanWrite{}, errors.New("amount " + string(result.Amount) + " exceeds the outstanding " + string(result.Outstanding))
	case len(result.Duplicates) > 0 && !input.AllowDuplicate:
		return PaymentPlanWrite{}, errors.New("a payment plan for this receipt or its supplier order exists; review the duplicates or allow them explicitly")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PaymentPlanWrite{}, errors.New("OData client does not support writes")
	}
	operation, err := s.predefinedOperation(ctx, paymentPlanOperation)
	if err != nil {
		return PaymentPlanWrite{}, err
	}
	fields := planHeaderFields(result)
	basis := emptyGUID
	if result.BasisOrderID != "" {
		basis = result.BasisOrderID
	}
	for key, value := range map[string]any{
		"Ref_Key": emptyGUID, "ДокументОснование_Key": basis, "ТипДенежныхСредств": paymentPlanMoneyType, "Касса_Key": emptyGUID,
		"Курс": 1, "Кратность": 1, "НалогообложениеНДС": receipt.Taxation, "СуммаВключаетНДС": receipt.IncludesVAT,
		"СуммаДокумента": json.Number(result.DocumentAmount), "ЗапланироватьОплату": true, "РезервироватьДенежныеСредства": false,
		"РегистрироватьЦеныПоставщика": false, "ХозяйственнаяОперация_Key": operation, "Подразделение_Key": orEmptyGUID(result.DepartmentID),
	} {
		fields[key] = value
	}
	rows := make([]map[string]any, 0, len(receipt.Lines))
	for index, line := range receipt.Lines {
		rows = append(rows, map[string]any{
			"LineNumber": fmt.Sprint(index + 1), "Номенклатура_Key": line.ProductID, "Характеристика_Key": orEmptyGUID(line.CharacteristicID),
			"ЕдиницаИзмерения": line.UnitID, "ЕдиницаИзмерения_Type": line.UnitType, "Количество": json.Number(line.Quantity),
			"Цена": json.Number(line.Price), "Сумма": json.Number(line.Amount), "СтавкаНДС_Key": line.VATRateID,
			"СуммаНДС": json.Number(line.VAT), "Всего": json.Number(line.Total),
		})
	}
	fields["Запасы"] = rows
	body, err := json.Marshal(fields)
	if err != nil {
		return PaymentPlanWrite{}, errors.New("could not encode payment plan")
	}
	response, err := writer.Write(ctx, http.MethodPost, paymentPlanResourceName, body, "")
	if err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("could not confirm payment plan creation; run trace receipt-payments for this receipt before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return PaymentPlanWrite{}, errors.New("1C may have created the payment plan but did not return its ID; run trace receipt-payments before retrying")
	}
	saved, err := s.readEditablePlan(ctx, created.ID)
	if err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("payment plan %s was created but could not be read back: %w", created.ID, err)
	}
	if err := verifyPlanSaved(saved, result); err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("payment plan %s was created but differs from the request: %w", created.ID, err)
	}
	result.Applied, result.ID, result.DataVersion, result.Posted = true, saved.ID, saved.DataVersion, saved.Posted
	return result, nil
}

type PaymentPlanAmendment struct {
	ReceiptID           string  `json:"receipt_id,omitempty"`
	PaymentDate         *string `json:"payment_date,omitempty"`
	Amount              *string `json:"amount,omitempty"`
	AccountID           *string `json:"account_id,omitempty"`
	SupplierAccountID   *string `json:"supplier_account_id,omitempty"`
	Comment             *string `json:"comment,omitempty"`
	ExpectedDataVersion string  `json:"expected_data_version,omitempty"`
	Apply               bool    `json:"apply,omitempty"`
}

// AmendPaymentPlan previews and, with Apply, changes an unposted single-line payment plan, keeping every other field.
// A new amount is checked against the receipt's outstanding debt, net of this plan, so receipt_id is required for it.
func (s Service) AmendPaymentPlan(ctx context.Context, id string, change PaymentPlanAmendment) (PaymentPlanWrite, error) {
	current, err := s.readEditablePlan(ctx, id)
	if err != nil {
		return PaymentPlanWrite{}, err
	}
	if current.Posted {
		return PaymentPlanWrite{}, errors.New("payment plan is posted; unpost it explicitly first, amend, then post it again")
	}
	if len(current.Schedule) != 1 {
		return PaymentPlanWrite{}, fmt.Errorf("payment plan has %d payment-calendar lines; only single-line plans can be amended here", len(current.Schedule))
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != current.DataVersion {
		return PaymentPlanWrite{}, errors.New("payment plan changed since the preview; preview again")
	}
	line := current.Schedule[0]
	result := PaymentPlanWrite{
		ID: current.ID, DataVersion: current.DataVersion, BasisOrderID: current.BasisID, OrganizationID: current.OrganizationID,
		SupplierID: current.SupplierID, ContractID: current.ContractID, CurrencyID: current.CurrencyID, AccountID: current.AccountID,
		SupplierAccountID: current.SupplierAccountID, Date: current.Date, PaymentDate: line.Date, IncomingNumber: current.IncomingNumber,
		IncomingDate: current.IncomingDate, Comment: current.Comment, DocumentAmount: current.Amount, Amount: line.Amount, VAT: line.VAT,
		Percent: line.Percent, Before: &PaymentPlanTotals{PaymentDate: line.Date, Amount: line.Amount, AccountID: current.AccountID},
		ExistingPlans: []TracedPaymentPlan{}, Payments: []TracedPayment{}, Candidates: []TraceCandidate{}, Duplicates: []TracedPaymentPlan{},
		Incomplete: []string{}, Changes: []string{}, Approval: notApprovedYet,
	}
	if change.PaymentDate != nil {
		paymentDate, err := receiptDate(*change.PaymentDate, false)
		if err != nil {
			return PaymentPlanWrite{}, errors.New("payment date must be YYYY-MM-DD")
		}
		if paymentDate != result.PaymentDate {
			result.PaymentDate = paymentDate
			result.Changes = append(result.Changes, "payment date")
		}
	}
	if change.Comment != nil && *change.Comment != result.Comment {
		if len([]rune(*change.Comment)) > 500 {
			return PaymentPlanWrite{}, errors.New("comment must be at most 500 characters")
		}
		result.Comment = *change.Comment
		result.Changes = append(result.Changes, "comment")
	}
	account, supplierAccount := result.AccountID, result.SupplierAccountID
	if change.AccountID != nil {
		account = *change.AccountID
	}
	if change.SupplierAccountID != nil {
		supplierAccount = *change.SupplierAccountID
	}
	if !strings.EqualFold(account, result.AccountID) || !strings.EqualFold(supplierAccount, result.SupplierAccountID) {
		if err := s.fillPlanAccounts(ctx, &result, account, supplierAccount); err != nil {
			return PaymentPlanWrite{}, err
		}
		if !strings.EqualFold(account, current.AccountID) {
			result.Changes = append(result.Changes, "paying account")
		}
		if !strings.EqualFold(supplierAccount, current.SupplierAccountID) {
			result.Changes = append(result.Changes, "supplier account")
		}
	}
	if change.Amount != nil {
		amount, err := exactDecimal(*change.Amount, 2, false)
		if err != nil {
			return PaymentPlanWrite{}, errors.New("amount must be a positive number with at most 2 decimals")
		}
		if !decimalEqual(ratDecimal(amount, 2), line.Amount) {
			if err := s.checkAmendedAmount(ctx, &result, change.ReceiptID, amount); err != nil {
				return PaymentPlanWrite{}, err
			}
			result.Changes = append(result.Changes, "amount")
		}
	}
	result.Impact = []string{"changes only the listed fields of the unposted payment plan; it is not posted or approved"}
	if !change.Apply || len(result.Changes) == 0 {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PaymentPlanWrite{}, errors.New("OData client does not support writes")
	}
	fields := map[string]any{"БанковскийСчет_Key": result.AccountID, "БанковскийСчетПоставщика_Key": result.SupplierAccountID, "Комментарий": result.Comment}
	calendar := make(map[string]any, len(line.Raw)+4)
	for key, value := range line.Raw {
		calendar[key] = value
	}
	calendar["ДатаОплаты"] = result.PaymentDate
	calendar["СуммаОплаты"] = json.Number(result.Amount)
	calendar["СуммаНДСОплаты"] = json.Number(result.VAT)
	calendar["ПроцентОплаты"] = json.Number(result.Percent)
	fields["ПлатежныйКалендарь"] = []map[string]any{calendar}
	body, err := json.Marshal(fields)
	if err != nil {
		return PaymentPlanWrite{}, errors.New("could not encode payment plan")
	}
	if _, err := writer.Write(ctx, http.MethodPatch, planResource(current.ID), body, current.DataVersion); err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("could not confirm the payment plan update; read it before retrying: %w", err)
	}
	saved, err := s.readEditablePlan(ctx, current.ID)
	if err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("payment plan %s was updated but could not be read back: %w", current.ID, err)
	}
	if err := verifyPlanSaved(saved, result); err != nil {
		return PaymentPlanWrite{}, fmt.Errorf("payment plan %s was updated but differs from the request: %w", current.ID, err)
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	return result, nil
}

func (s Service) checkAmendedAmount(ctx context.Context, result *PaymentPlanWrite, receiptID string, amount *big.Rat) error {
	if !linkedGUID(receiptID) {
		return errors.New("changing the amount needs receipt_id, so it can be checked against what is still outstanding")
	}
	receipt, err := s.readPlanReceipt(ctx, receiptID)
	if err != nil {
		return err
	}
	trace, err := s.TraceSupplierReceipt(ctx, receipt.ID)
	if err != nil {
		return err
	}
	linked := false
	for _, plan := range trace.PaymentPlans {
		if strings.EqualFold(plan.ID, result.ID) {
			linked = true
		} else if !plan.Deleted {
			result.Duplicates = append(result.Duplicates, plan)
		}
	}
	if !linked {
		return errors.New("this payment plan is not linked to that receipt by supplier order or incoming document")
	}
	result.ReceiptID, result.ReceiptNumber, result.ReceiptDate = receipt.ID, receipt.Number, receipt.Date[:10]
	result.ReceiptStatus, result.ExistingPlans, result.Payments, result.Candidates = trace.Summary.Status, trace.PaymentPlans, trace.Payments, trace.Candidates
	if !trace.Settlements.Available {
		return errors.New("supplier-settlement register is unavailable, so the outstanding amount is unknown: " + trace.Settlements.Error)
	}
	result.Outstanding = trace.Settlements.Outstanding
	outstanding, ok := new(big.Rat).SetString(string(trace.Settlements.Outstanding))
	if !ok || amount.Cmp(outstanding) > 0 {
		return errors.New("amount " + string(ratDecimal(amount, 2)) + " exceeds the outstanding " + string(trace.Settlements.Outstanding))
	}
	documentAmount, ok := new(big.Rat).SetString(string(result.DocumentAmount))
	if !ok || documentAmount.Sign() <= 0 || amount.Cmp(documentAmount) > 0 {
		return errors.New("amount exceeds the invoice total " + string(result.DocumentAmount))
	}
	return setPlanAmount(result, receipt, amount)
}

// setPlanAmount sets the planned amount and splits VAT and percent in proportion to the receipt totals.
func setPlanAmount(result *PaymentPlanWrite, receipt planReceipt, amount *big.Rat) error {
	if amount.Sign() <= 0 {
		return errors.New("amount must be positive")
	}
	if receipt.Amount.Sign() <= 0 {
		return errors.New("receipt total must be positive")
	}
	share := new(big.Rat).Quo(amount, receipt.Amount)
	result.Amount = ratDecimal(amount, 2)
	result.VAT = ratDecimal(roundHalfAway(new(big.Rat).Mul(receipt.VAT, share), 2), 2)
	result.Percent = ratDecimal(roundHalfAway(new(big.Rat).Mul(share, big.NewRat(100, 1)), 2), 2)
	return nil
}

func (s Service) fillPlanAccounts(ctx context.Context, result *PaymentPlanWrite, accountID, supplierAccountID string) error {
	if !linkedGUID(accountID) {
		return errors.New("choose the organization's paying bank account explicitly (account_id)")
	}
	account, err := s.activeRecord(ctx, bankAccountsResourceName, accountID, "Ref_Key,DeletionMark,Owner,Owner_Type")
	if err != nil {
		return fmt.Errorf("paying account: %w", err)
	}
	if rawString(account["Owner_Type"]) != organizationOwnerType || !strings.EqualFold(rawString(account["Owner"]), result.OrganizationID) {
		return errors.New("paying account must belong to the receipt's organization")
	}
	result.AccountID = strings.ToLower(accountID)
	if supplierAccountID == "" {
		accounts, err := s.scanRows(ctx, bankAccountsResourceName, "Ref_Key,DeletionMark,Owner,Owner_Type", "", maxDocumentScan)
		if err != nil {
			return err
		}
		for _, row := range accounts {
			if rawString(row["Owner_Type"]) == counterpartyOwnerType && strings.EqualFold(rawString(row["Owner"]), result.SupplierID) && string(row["DeletionMark"]) == "false" {
				if supplierAccountID != "" {
					return errors.New("the supplier has several bank accounts; choose one (supplier_account_id)")
				}
				supplierAccountID = rawString(row["Ref_Key"])
			}
		}
		if supplierAccountID == "" {
			return errors.New("the supplier has no bank account in 1C; add one before planning a bank payment")
		}
	}
	supplierAccount, err := s.activeRecord(ctx, bankAccountsResourceName, supplierAccountID, "Ref_Key,DeletionMark,Owner,Owner_Type")
	if err != nil {
		return fmt.Errorf("supplier account: %w", err)
	}
	if rawString(supplierAccount["Owner_Type"]) != counterpartyOwnerType || !strings.EqualFold(rawString(supplierAccount["Owner"]), result.SupplierID) {
		return errors.New("supplier account must belong to the receipt's supplier")
	}
	result.SupplierAccountID = strings.ToLower(supplierAccountID)
	contract, err := s.activeRecord(ctx, "Catalog_ДоговорыКонтрагентов", result.ContractID, "Ref_Key,DeletionMark,СтатьяДДСПоУмолчанию_Key")
	if err != nil {
		return fmt.Errorf("contract: %w", err)
	}
	if article := rawString(contract["СтатьяДДСПоУмолчанию_Key"]); linkedGUID(article) {
		result.PaymentArticleID = article
	}
	return nil
}

func (s Service) predefinedOperation(ctx context.Context, name string) (string, error) {
	rows, err := s.scanRows(ctx, "Catalog_ХозяйственныеОперации", "Ref_Key,DeletionMark,PredefinedDataName", "", 2000)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if rawString(row["PredefinedDataName"]) == name && string(row["DeletionMark"]) != "true" {
			return strings.ToLower(rawString(row["Ref_Key"])), nil
		}
	}
	return "", errors.New("predefined business operation " + name + " was not found")
}

// readPlanReceipt reads every goods and expense line of a supplier receipt, refusing receipts whose lines do not add up.
func (s Service) readPlanReceipt(ctx context.Context, id string) (planReceipt, error) {
	if !linkedGUID(id) {
		return planReceipt{}, errors.New("receipt ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, receiptResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return planReceipt{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return planReceipt{}, errors.New("invalid OData receipt response")
	}
	if rawString(row["ВидОперации"]) != receiptOperation || string(row["DeletionMark"]) != "false" {
		return planReceipt{}, errors.New("ID must identify an active goods receipt from a supplier")
	}
	receipt := planReceipt{
		ID: strings.ToLower(id), Number: strings.TrimSpace(rawString(row["Number"])), Date: rawString(row["Date"]), Posted: string(row["Posted"]) == "true",
		OrganizationID: rawString(row["Организация_Key"]), SupplierID: rawString(row["Контрагент_Key"]), ContractID: rawString(row["Договор_Key"]),
		CurrencyID: rawString(row["ВалютаДокумента_Key"]), DepartmentID: rawString(row["Подразделение_Key"]),
		IncomingNumber: strings.TrimSpace(rawString(row["НомерВходящегоДокумента"])), IncludesVAT: string(row["СуммаВключаетНДС"]) == "true",
		Taxation: rawString(row["НалогообложениеНДС"]), Amount: new(big.Rat), VAT: new(big.Rat),
	}
	if len(receipt.Date) < 10 || !linkedGUID(receipt.SupplierID) || !linkedGUID(receipt.OrganizationID) || !linkedGUID(receipt.ContractID) || !linkedGUID(receipt.CurrencyID) {
		return planReceipt{}, errors.New("receipt needs an organization, supplier, contract and currency")
	}
	if !linkedGUID(receipt.DepartmentID) {
		receipt.DepartmentID = ""
	}
	if rawString(row["Заказ_Type"]) == supplierOrderType && linkedGUID(rawString(row["Заказ"])) {
		receipt.OrderID = strings.ToLower(rawString(row["Заказ"]))
	}
	if incoming := rawString(row["ДатаВходящегоДокумента"]); !strings.HasPrefix(incoming, "0001-01-01") && len(incoming) >= 10 {
		receipt.IncomingDate = incoming[:10]
	}
	total, ok := new(big.Rat).SetString(string(rawDecimal(row["СуммаДокумента"])))
	if !ok || total.Sign() <= 0 {
		return planReceipt{}, errors.New("receipt total must be positive")
	}
	sum := new(big.Rat)
	for _, tab := range []string{"Запасы", "Расходы"} {
		var lines []map[string]any
		if raw, ok := row[tab]; ok && string(raw) != "null" {
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if err := decoder.Decode(&lines); err != nil {
				return planReceipt{}, errors.New("invalid OData receipt lines")
			}
		}
		for _, line := range lines {
			parsed := planLine{
				ProductID: anyString(line["Номенклатура_Key"]), CharacteristicID: anyString(line["Характеристика_Key"]),
				UnitID: anyString(line["ЕдиницаИзмерения"]), UnitType: anyString(line["ЕдиницаИзмерения_Type"]),
				Quantity: anyDecimal(line["Количество"]), Price: anyDecimal(line["Цена"]), Amount: anyDecimal(line["Сумма"]),
				VATRateID: anyString(line["СтавкаНДС_Key"]), VAT: anyDecimal(line["СуммаНДС"]), Total: anyDecimal(line["Всего"]),
			}
			if parsed.VAT == "" {
				parsed.VAT = "0"
			}
			lineTotal, okTotal := new(big.Rat).SetString(string(parsed.Total))
			lineVAT, okVAT := new(big.Rat).SetString(string(parsed.VAT))
			if !linkedGUID(parsed.ProductID) || !linkedGUID(parsed.UnitID) || parsed.UnitType == "" || !okTotal || !okVAT || parsed.Quantity == "" || parsed.Price == "" || parsed.Amount == "" {
				return planReceipt{}, errors.New("receipt has a " + tab + " line without product, unit or amounts; plan it in 1C")
			}
			if !linkedGUID(parsed.VATRateID) {
				parsed.VATRateID = emptyGUID
			}
			sum.Add(sum, lineTotal)
			receipt.VAT.Add(receipt.VAT, lineVAT)
			receipt.Lines = append(receipt.Lines, parsed)
		}
	}
	if len(receipt.Lines) == 0 || sum.Cmp(total) != 0 {
		return planReceipt{}, errors.New("receipt lines do not add up to its total, so they cannot be copied to a payment plan; plan it in 1C")
	}
	receipt.Amount = total
	return receipt, nil
}

type editableSchedule struct {
	Date    string
	Percent Decimal
	Amount  Decimal
	VAT     Decimal
	Raw     map[string]any
}

type editablePlan struct {
	ID                string
	DataVersion       string
	Posted            bool
	OrganizationID    string
	SupplierID        string
	ContractID        string
	CurrencyID        string
	AccountID         string
	SupplierAccountID string
	BasisID           string
	Date              string
	IncomingNumber    string
	IncomingDate      string
	Comment           string
	PlanPayment       bool
	Amount            Decimal
	Lines             int
	Schedule          []editableSchedule
}

func (s Service) readEditablePlan(ctx context.Context, id string) (editablePlan, error) {
	if !linkedGUID(id) {
		return editablePlan{}, errors.New("payment plan ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, planResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return editablePlan{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return editablePlan{}, errors.New("invalid OData payment plan response")
	}
	if string(row["DeletionMark"]) != "false" {
		return editablePlan{}, errors.New("payment plan is marked for deletion")
	}
	if rawString(row["DataVersion"]) == "" {
		return editablePlan{}, errors.New("payment plan has no data version for a safe update")
	}
	plan := editablePlan{
		ID: strings.ToLower(id), DataVersion: rawString(row["DataVersion"]), Posted: string(row["Posted"]) == "true",
		OrganizationID: rawString(row["Организация_Key"]), SupplierID: rawString(row["Контрагент_Key"]), ContractID: rawString(row["Договор_Key"]),
		CurrencyID: rawString(row["ВалютаДокумента_Key"]), AccountID: rawString(row["БанковскийСчет_Key"]), SupplierAccountID: rawString(row["БанковскийСчетПоставщика_Key"]),
		BasisID: rawString(row["ДокументОснование_Key"]), Date: rawString(row["Date"]), IncomingNumber: strings.TrimSpace(rawString(row["НомерВходящегоДокумента"])),
		Comment: rawString(row["Комментарий"]), PlanPayment: string(row["ЗапланироватьОплату"]) == "true", Amount: rawDecimal(row["СуммаДокумента"]),
	}
	clearUnlinked(&plan.AccountID, &plan.SupplierAccountID, &plan.BasisID)
	if incoming := rawString(row["ДатаВходящегоДокумента"]); !strings.HasPrefix(incoming, "0001-01-01") && len(incoming) >= 10 {
		plan.IncomingDate = incoming[:10]
	}
	var goods []json.RawMessage
	if raw, ok := row["Запасы"]; ok && string(raw) != "null" {
		if err := json.Unmarshal(raw, &goods); err != nil {
			return editablePlan{}, errors.New("invalid OData payment plan lines")
		}
	}
	plan.Lines = len(goods)
	var schedule []map[string]any
	if raw, ok := row["ПлатежныйКалендарь"]; ok && string(raw) != "null" {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&schedule); err != nil {
			return editablePlan{}, errors.New("invalid OData payment schedule")
		}
	}
	for _, line := range schedule {
		for key := range line {
			if strings.Contains(key, "@") {
				delete(line, key)
			}
		}
		plan.Schedule = append(plan.Schedule, editableSchedule{
			Date: anyString(line["ДатаОплаты"]), Percent: anyDecimal(line["ПроцентОплаты"]), Amount: anyDecimal(line["СуммаОплаты"]),
			VAT: anyDecimal(line["СуммаНДСОплаты"]), Raw: line,
		})
	}
	return plan, nil
}

func verifyPlanSaved(saved editablePlan, want PaymentPlanWrite) error {
	if saved.Posted {
		return errors.New("1C saved the payment plan as posted")
	}
	if !strings.EqualFold(saved.SupplierID, want.SupplierID) || !strings.EqualFold(saved.ContractID, want.ContractID) || !strings.EqualFold(saved.AccountID, want.AccountID) || !strings.EqualFold(saved.SupplierAccountID, want.SupplierAccountID) || !strings.EqualFold(saved.BasisID, want.BasisOrderID) || saved.IncomingNumber != want.IncomingNumber || saved.IncomingDate != want.IncomingDate || saved.Comment != want.Comment {
		return errors.New("header fields do not match")
	}
	if !saved.PlanPayment || len(saved.Schedule) != 1 {
		return errors.New("payment calendar was not saved as one planned payment")
	}
	line := saved.Schedule[0]
	if !decimalEqual(line.Amount, want.Amount) || !decimalEqual(line.VAT, want.VAT) || !strings.HasPrefix(line.Date, want.PaymentDate[:10]) {
		return errors.New("planned payment " + string(line.Amount) + " on " + line.Date + " differs from " + string(want.Amount) + " on " + want.PaymentDate)
	}
	if !decimalEqual(saved.Amount, want.DocumentAmount) {
		return errors.New("document total " + string(saved.Amount) + " differs from " + string(want.DocumentAmount))
	}
	return nil
}

func planHeaderFields(result PaymentPlanWrite) map[string]any {
	fields := map[string]any{
		"Date": result.Date, "Организация_Key": result.OrganizationID, "Контрагент_Key": result.SupplierID, "Договор_Key": result.ContractID,
		"ВалютаДокумента_Key": result.CurrencyID, "БанковскийСчет_Key": result.AccountID, "БанковскийСчетПоставщика_Key": result.SupplierAccountID,
		"НомерВходящегоДокумента": result.IncomingNumber, "Комментарий": result.Comment,
		"ПлатежныйКалендарь": []map[string]any{{
			"LineNumber": "1", "ДатаОплаты": result.PaymentDate, "ПроцентОплаты": json.Number(result.Percent),
			"СуммаОплаты": json.Number(result.Amount), "СуммаНДСОплаты": json.Number(result.VAT),
		}},
	}
	fields["ДатаВходящегоДокумента"] = "0001-01-01T00:00:00"
	if result.IncomingDate != "" {
		fields["ДатаВходящегоДокумента"] = result.IncomingDate + "T00:00:00"
	}
	return fields
}

// planComment names the receipt in the invoice, because the invoice's basis field can only hold a supplier order.
func planComment(receipt planReceipt, comment string) string {
	basis := "Приходная накладная № " + receipt.Number + " от " + receipt.Date[8:10] + "." + receipt.Date[5:7] + "." + receipt.Date[:4]
	if comment = strings.TrimSpace(comment); comment != "" {
		return basis + ". " + comment
	}
	return basis
}

func planResource(id string) string {
	return paymentPlanResourceName + "(guid'" + strings.ToLower(id) + "')"
}

func orEmptyGUID(id string) string {
	if id == "" {
		return emptyGUID
	}
	return id
}

type PaymentPlanPosting struct {
	ID             string                   `json:"id"`
	Action         string                   `json:"action"`
	Applied        bool                     `json:"applied"`
	PostedFrom     bool                     `json:"posted_before"`
	Posted         bool                     `json:"posted"`
	DataVersion    string                   `json:"data_version"`
	Amount         Decimal                  `json:"amount"`
	PlannedAmounts []Decimal                `json:"planned_amounts"`
	Impact         []string                 `json:"impact"`
	Calendar       *PaymentCalendarEvidence `json:"calendar,omitempty"`
}

// SetPaymentPlanPosting previews and, with Apply, posts or unposts a payment plan. Posting is not approval:
// the approval state is whatever 1C records, and is reported from the payment-calendar register afterwards.
func (s Service) SetPaymentPlanPosting(ctx context.Context, id, action, expectedDataVersion string, apply bool) (PaymentPlanPosting, error) {
	if action != "post" && action != "unpost" {
		return PaymentPlanPosting{}, errors.New("action must be post or unpost")
	}
	current, err := s.readEditablePlan(ctx, id)
	if err != nil {
		return PaymentPlanPosting{}, err
	}
	result := PaymentPlanPosting{ID: current.ID, Action: action, PostedFrom: current.Posted, Posted: current.Posted, DataVersion: current.DataVersion, Amount: current.Amount, PlannedAmounts: []Decimal{}}
	for _, line := range current.Schedule {
		result.PlannedAmounts = append(result.PlannedAmounts, line.Amount)
	}
	if action == "post" {
		result.Impact = []string{
			fmt.Sprintf("records %d planned supplier payments in the payment calendar", len(current.Schedule)),
			"does not approve them, create a payment order, or send a bank transfer; 1C sets the approval state by its settings",
		}
		if !current.PlanPayment || len(current.Schedule) == 0 {
			result.Impact = append(result.Impact, "this invoice does not plan payment, so posting adds nothing to the payment calendar")
		}
	} else {
		result.Impact = []string{"removes this invoice's planned payments and their approval state from the payment calendar until it is posted again"}
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return PaymentPlanPosting{}, errors.New("payment plan changed since the preview; preview again")
	}
	if !apply || current.Posted == (action == "post") {
		return result, nil
	}
	actor, ok := s.OData.(documentActor)
	if !ok {
		return PaymentPlanPosting{}, errors.New("OData client does not support document actions")
	}
	name := "Post"
	if action == "unpost" {
		name = "Unpost"
	}
	if _, err := actor.Action(ctx, planResource(current.ID), name); err != nil {
		return PaymentPlanPosting{}, fmt.Errorf("1C rejected %s; the payment plan was not changed by this request or its state is uncertain, so read it before retrying: %w", action, err)
	}
	saved, err := s.readEditablePlan(ctx, current.ID)
	if err != nil {
		return PaymentPlanPosting{}, fmt.Errorf("%s was sent but the payment plan could not be read back: %w", action, err)
	}
	if saved.Posted != (action == "post") {
		return PaymentPlanPosting{}, fmt.Errorf("%s was accepted but the payment plan posting flag did not change", action)
	}
	result.Applied, result.Posted, result.DataVersion = true, saved.Posted, saved.DataVersion
	result.Calendar = s.paymentCalendar(ctx, current.ID)
	return result, nil
}
