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

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	cashMoneyType          = "Наличные"
	cashDesksResourceName  = "Catalog_Кассы"
	approvedPaymentState   = "Утвержден"
	purposePrefix          = "Назначение платежа: "
	maxPlanCommentRunes    = 500
	departmentUnitType     = "Подразделение"
	paymentRequestNotStore = "the supplier invoice has no field for payment article, business line or payment purpose: the article comes from the contract default when 1C posts it, and the purpose is kept in the comment"
)

// PrepaymentRequest plans payment of an existing supplier invoice before any goods receipt exists.
// The tenant has no payment-request document, so the planned invoice is the request.
type PrepaymentRequest struct {
	InvoiceID           string  `json:"invoice_id"`
	Source              string  `json:"source"`
	AccountID           string  `json:"account_id,omitempty"`
	CashID              string  `json:"cash_id,omitempty"`
	SupplierAccountID   string  `json:"supplier_account_id,omitempty"`
	PaymentDate         string  `json:"payment_date"`
	Amount              string  `json:"amount,omitempty"`
	DepartmentID        string  `json:"department_id,omitempty"`
	Purpose             string  `json:"purpose,omitempty"`
	Comment             *string `json:"comment,omitempty"`
	ExpectedDataVersion string  `json:"expected_data_version,omitempty"`
	Apply               bool    `json:"apply,omitempty"`
}

type PrepaymentWrite struct {
	Applied           bool                 `json:"applied"`
	AlreadyPlanned    bool                 `json:"already_planned"`
	ID                string               `json:"id"`
	Number            string               `json:"number"`
	Date              string               `json:"date"`
	DataVersion       string               `json:"data_version"`
	Posted            bool                 `json:"posted"`
	Status            string               `json:"status"`
	OrganizationID    string               `json:"organization_id"`
	SupplierID        string               `json:"supplier_id"`
	ContractID        string               `json:"contract_id"`
	CurrencyID        string               `json:"currency_id"`
	BasisID           string               `json:"basis_id,omitempty"`
	IncomingNumber    string               `json:"incoming_number"`
	IncomingDate      string               `json:"incoming_date"`
	Source            string               `json:"source"`
	AccountID         string               `json:"account_id,omitempty"`
	CashID            string               `json:"cash_id,omitempty"`
	SupplierAccountID string               `json:"supplier_account_id,omitempty"`
	DepartmentID      string               `json:"department_id,omitempty"`
	PaymentArticleID  string               `json:"payment_article_id,omitempty"`
	PaymentDate       string               `json:"payment_date"`
	DocumentAmount    Decimal              `json:"document_amount"`
	Paid              Decimal              `json:"paid"`
	Remaining         Decimal              `json:"remaining"`
	Amount            Decimal              `json:"amount"`
	VAT               Decimal              `json:"vat"`
	Percent           Decimal              `json:"percent"`
	Comment           string               `json:"comment"`
	Changes           []string             `json:"changes"`
	Payments          InvoicePaymentStatus `json:"payments"`
	Approval          string               `json:"approval"`
	Impact            []string             `json:"impact"`
	NotStored         []string             `json:"not_stored"`
}

// InvoicePaymentStatus counts only posted bank and cash expenses whose breakdown names the invoice.
// Payment orders, approval and equal amounts are listed or ignored, never counted as paid.
type InvoicePaymentStatus struct {
	Complete   bool            `json:"complete"`
	Paid       Decimal         `json:"paid"`
	Payments   []TracedPayment `json:"linked"`
	Incomplete []string        `json:"incomplete"`
	Notes      []string        `json:"notes"`
}

type plannableInvoice struct {
	editablePlan
	Number       string
	DepartmentID string
	CashID       string
	MoneyType    string
	VAT          *big.Rat
	Total        *big.Rat
}

// PlanInvoicePrepayment previews and, with Apply, turns an unposted supplier invoice into a planned prepayment.
// It never posts, approves or pays; a rerun after a lost response finds the saved plan and writes nothing.
func (s Service) PlanInvoicePrepayment(ctx context.Context, input PrepaymentRequest) (PrepaymentWrite, error) {
	if input.Source != "bank" && input.Source != "cash" {
		return PrepaymentWrite{}, errors.New("source must be bank (organization bank account) or cash (cash desk)")
	}
	paymentDate, err := receiptDate(input.PaymentDate, false)
	if err != nil {
		return PrepaymentWrite{}, errors.New("payment date must be YYYY-MM-DD")
	}
	invoice, err := s.readPlannableInvoice(ctx, input.InvoiceID)
	if err != nil {
		return PrepaymentWrite{}, err
	}
	if input.ExpectedDataVersion != "" && input.ExpectedDataVersion != invoice.DataVersion {
		return PrepaymentWrite{}, errors.New("supplier invoice changed since the preview; preview again")
	}
	result := PrepaymentWrite{
		ID: invoice.ID, Number: invoice.Number, Date: invoice.Date, DataVersion: invoice.DataVersion, Posted: invoice.Posted,
		OrganizationID: invoice.OrganizationID, SupplierID: invoice.SupplierID, ContractID: invoice.ContractID, CurrencyID: invoice.CurrencyID,
		BasisID: invoice.BasisID, IncomingNumber: invoice.IncomingNumber, IncomingDate: invoice.IncomingDate, Source: input.Source,
		DepartmentID: invoice.DepartmentID, PaymentDate: paymentDate, DocumentAmount: invoice.Amount, Comment: invoice.Comment,
		Changes: []string{}, Approval: notApprovedYet, NotStored: []string{paymentRequestNotStore},
	}
	if invoice.PlanPayment && len(invoice.Schedule) > 0 {
		return s.existingPrepayment(ctx, result, invoice, input, paymentDate)
	}
	if invoice.Posted {
		return PrepaymentWrite{}, errors.New("supplier invoice is posted; unpost it explicitly first, plan the payment, then post it again")
	}
	if err := s.fillPrepaymentSource(ctx, &result, input); err != nil {
		return PrepaymentWrite{}, err
	}
	if input.DepartmentID != "" && !strings.EqualFold(input.DepartmentID, invoice.DepartmentID) {
		unit, err := s.activeRecord(ctx, config.Warehouses.Name, input.DepartmentID, "Ref_Key,DeletionMark,ТипСтруктурнойЕдиницы,Недействителен")
		if err != nil {
			return PrepaymentWrite{}, fmt.Errorf("department: %w", err)
		}
		if rawString(unit["ТипСтруктурнойЕдиницы"]) != departmentUnitType || string(unit["Недействителен"]) == "true" {
			return PrepaymentWrite{}, errors.New("department ID must identify an active department")
		}
		result.DepartmentID = strings.ToLower(input.DepartmentID)
		result.Changes = append(result.Changes, "department")
	}
	comment, err := prepaymentComment(invoice.Comment, input.Comment, input.Purpose)
	if err != nil {
		return PrepaymentWrite{}, err
	}
	if comment != invoice.Comment {
		result.Comment = comment
		result.Changes = append(result.Changes, "comment")
	}
	result.Payments = s.invoicePayments(ctx, invoice.SupplierID, invoice.ID)
	paid, _ := new(big.Rat).SetString(string(result.Payments.Paid))
	remaining := new(big.Rat).Sub(invoice.Total, paid)
	result.Paid, result.Remaining = result.Payments.Paid, ratDecimal(remaining, 2)
	amount := remaining
	if input.Amount != "" {
		if amount, err = exactDecimal(input.Amount, 2, false); err != nil {
			return PrepaymentWrite{}, errors.New("amount must be a positive number with at most 2 decimals")
		}
	}
	if amount.Sign() > 0 {
		share := new(big.Rat).Quo(amount, invoice.Total)
		result.Amount = ratDecimal(amount, 2)
		result.VAT = ratDecimal(roundHalfAway(new(big.Rat).Mul(invoice.VAT, share), 2), 2)
		result.Percent = ratDecimal(roundHalfAway(new(big.Rat).Mul(share, big.NewRat(100, 1)), 2), 2)
	}
	result.Changes = append(result.Changes, "planned payment")
	result.Status = paymentRequestStatus(false, false, nil, result.Payments, invoice.Total)
	from := "bank account " + result.AccountID + " to supplier account " + result.SupplierAccountID
	if input.Source == "cash" {
		from = "cash desk " + result.CashID
	}
	result.Impact = []string{
		"plans a prepayment of " + string(result.Amount) + " on " + paymentDate[:10] + " from " + from + " on this unposted supplier invoice; no goods receipt is needed",
		"does not post or approve it, and never creates a payment order, cash voucher or bank transfer",
	}
	if !input.Apply {
		return result, nil
	}
	switch {
	case !result.Payments.Complete:
		return PrepaymentWrite{}, errors.New("payment evidence is incomplete, so the amount still to pay is unknown; retry when every source is readable")
	case remaining.Sign() <= 0:
		return PrepaymentWrite{}, errors.New("invoice is already paid by linked payments; a prepayment plan would double-plan the outflow")
	case amount.Cmp(remaining) > 0:
		return PrepaymentWrite{}, errors.New("amount " + string(result.Amount) + " exceeds what is left to pay, " + string(result.Remaining))
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PrepaymentWrite{}, errors.New("OData client does not support writes")
	}
	fields := map[string]any{
		"ЗапланироватьОплату": true, "Комментарий": result.Comment, "Подразделение_Key": orEmptyGUID(result.DepartmentID),
		"ПлатежныйКалендарь": []map[string]any{{
			"LineNumber": "1", "ДатаОплаты": paymentDate, "ПроцентОплаты": json.Number(result.Percent),
			"СуммаОплаты": json.Number(result.Amount), "СуммаНДСОплаты": json.Number(result.VAT),
		}},
	}
	if input.Source == "cash" {
		fields["ТипДенежныхСредств"], fields["Касса_Key"] = cashMoneyType, result.CashID
	} else {
		fields["ТипДенежныхСредств"], fields["БанковскийСчет_Key"], fields["БанковскийСчетПоставщика_Key"] = paymentPlanMoneyType, result.AccountID, result.SupplierAccountID
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return PrepaymentWrite{}, errors.New("could not encode payment plan")
	}
	if _, err := writer.Write(ctx, http.MethodPatch, planResource(invoice.ID), body, invoice.DataVersion); err != nil {
		return PrepaymentWrite{}, fmt.Errorf("could not confirm the prepayment plan; run this command again without apply to see what 1C saved: %w", err)
	}
	saved, err := s.readPlannableInvoice(ctx, invoice.ID)
	if err != nil {
		return PrepaymentWrite{}, fmt.Errorf("supplier invoice %s was updated but could not be read back: %w", invoice.ID, err)
	}
	if err := verifyPrepaymentSaved(saved, result); err != nil {
		return PrepaymentWrite{}, fmt.Errorf("supplier invoice %s was updated but differs from the request: %w", invoice.ID, err)
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	result.Status = paymentRequestStatus(saved.Posted, true, nil, result.Payments, invoice.Total)
	return result, nil
}

// existingPrepayment answers a rerun: the same plan reports as already saved, a different one is refused.
func (s Service) existingPrepayment(ctx context.Context, result PrepaymentWrite, invoice plannableInvoice, input PrepaymentRequest, paymentDate string) (PrepaymentWrite, error) {
	if len(invoice.Schedule) != 1 {
		return PrepaymentWrite{}, fmt.Errorf("supplier invoice already plans %d payments; review it with get payment-plan", len(invoice.Schedule))
	}
	line := invoice.Schedule[0]
	same := strings.HasPrefix(line.Date, paymentDate[:10])
	if input.Amount != "" {
		amount, err := exactDecimal(input.Amount, 2, false)
		same = same && err == nil && decimalEqual(ratDecimal(amount, 2), line.Amount)
	}
	switch input.Source {
	case "cash":
		same = same && invoice.MoneyType == cashMoneyType && strings.EqualFold(invoice.CashID, input.CashID)
	default:
		same = same && invoice.MoneyType == paymentPlanMoneyType && strings.EqualFold(invoice.AccountID, input.AccountID) && (input.SupplierAccountID == "" || strings.EqualFold(invoice.SupplierAccountID, input.SupplierAccountID))
	}
	if !same {
		return PrepaymentWrite{}, errors.New("supplier invoice already plans a different payment; change it with update payment-plan instead of planning it again")
	}
	result.AlreadyPlanned = true
	result.AccountID, result.SupplierAccountID, result.CashID = invoice.AccountID, invoice.SupplierAccountID, invoice.CashID
	result.Amount, result.VAT, result.Percent, result.PaymentDate = line.Amount, line.VAT, line.Percent, line.Date
	result.Payments = s.invoicePayments(ctx, invoice.SupplierID, invoice.ID)
	paid, _ := new(big.Rat).SetString(string(result.Payments.Paid))
	result.Paid, result.Remaining = result.Payments.Paid, ratDecimal(new(big.Rat).Sub(invoice.Total, paid), 2)
	var calendar *PaymentCalendarEvidence
	if invoice.Posted {
		calendar = s.paymentCalendar(ctx, invoice.ID)
	}
	result.Status = paymentRequestStatus(invoice.Posted, true, calendar, result.Payments, invoice.Total)
	result.Impact = []string{"this payment is already planned on the invoice; nothing was written"}
	return result, nil
}

func (s Service) fillPrepaymentSource(ctx context.Context, result *PrepaymentWrite, input PrepaymentRequest) error {
	accounts := PaymentPlanWrite{OrganizationID: result.OrganizationID, SupplierID: result.SupplierID, ContractID: result.ContractID, CurrencyID: result.CurrencyID, PaymentDate: result.PaymentDate}
	if input.Source == "cash" {
		if input.AccountID != "" || input.SupplierAccountID != "" {
			return errors.New("a cash payment takes cash_id only, not bank accounts")
		}
		desk, err := s.activeRecord(ctx, cashDesksResourceName, input.CashID, "Ref_Key,DeletionMark,Недействителен")
		if err != nil {
			return fmt.Errorf("cash desk: %w", err)
		}
		if string(desk["Недействителен"]) == "true" {
			return errors.New("cash desk is inactive")
		}
		result.CashID = strings.ToLower(input.CashID)
		contract, err := s.activeRecord(ctx, "Catalog_ДоговорыКонтрагентов", result.ContractID, "Ref_Key,DeletionMark,СтатьяДДСПоУмолчанию_Key")
		if err != nil {
			return fmt.Errorf("contract: %w", err)
		}
		if article := rawString(contract["СтатьяДДСПоУмолчанию_Key"]); linkedGUID(article) {
			result.PaymentArticleID = article
		}
		return nil
	}
	if input.CashID != "" {
		return errors.New("a bank payment takes account_id, not cash_id")
	}
	if err := s.fillPlanAccounts(ctx, &accounts, input.AccountID, input.SupplierAccountID); err != nil {
		return err
	}
	result.AccountID, result.SupplierAccountID, result.PaymentArticleID = accounts.AccountID, accounts.SupplierAccountID, accounts.PaymentArticleID
	return nil
}

// prepaymentComment keeps the current comment unless a new one is given, and keeps the purpose on its own line.
func prepaymentComment(current string, replacement *string, purpose string) (string, error) {
	comment := current
	if replacement != nil {
		comment = strings.TrimSpace(*replacement)
	}
	if purpose = strings.TrimSpace(purpose); purpose != "" {
		lines := make([]string, 0)
		for _, line := range strings.Split(comment, "\n") {
			if !strings.HasPrefix(line, purposePrefix) && strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
		}
		comment = strings.Join(append(lines, purposePrefix+strings.Join(strings.Fields(purpose), " ")), "\n")
	}
	if len([]rune(comment)) > maxPlanCommentRunes {
		return "", fmt.Errorf("comment with the payment purpose must be at most %d characters", maxPlanCommentRunes)
	}
	return comment, nil
}

func verifyPrepaymentSaved(saved plannableInvoice, want PrepaymentWrite) error {
	if saved.Posted {
		return errors.New("1C saved the invoice as posted")
	}
	if !saved.PlanPayment || len(saved.Schedule) != 1 {
		return errors.New("payment calendar was not saved as one planned payment")
	}
	line := saved.Schedule[0]
	if !decimalEqual(line.Amount, want.Amount) || !decimalEqual(line.VAT, want.VAT) || !strings.HasPrefix(line.Date, want.PaymentDate[:10]) {
		return errors.New("planned payment " + string(line.Amount) + " on " + line.Date + " differs from " + string(want.Amount) + " on " + want.PaymentDate)
	}
	if want.Source == "cash" {
		if saved.MoneyType != cashMoneyType || !strings.EqualFold(saved.CashID, want.CashID) {
			return errors.New("cash desk was not saved")
		}
	} else if saved.MoneyType != paymentPlanMoneyType || !strings.EqualFold(saved.AccountID, want.AccountID) || !strings.EqualFold(saved.SupplierAccountID, want.SupplierAccountID) {
		return errors.New("bank accounts were not saved as requested")
	}
	if saved.Comment != want.Comment || !strings.EqualFold(saved.DepartmentID, want.DepartmentID) || !decimalEqual(saved.Amount, want.DocumentAmount) {
		return errors.New("comment, department or invoice total changed")
	}
	return nil
}

// readPlannableInvoice reads a supplier invoice with the totals needed to split VAT over a planned payment.
func (s Service) readPlannableInvoice(ctx context.Context, id string) (plannableInvoice, error) {
	plan, err := s.readEditablePlan(ctx, id)
	if err != nil {
		return plannableInvoice{}, err
	}
	data, err := s.OData.Get(ctx, planResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return plannableInvoice{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil {
		return plannableInvoice{}, errors.New("invalid OData payment plan response")
	}
	invoice := plannableInvoice{
		editablePlan: plan, Number: strings.TrimSpace(rawString(row["Number"])), DepartmentID: rawString(row["Подразделение_Key"]),
		CashID: rawString(row["Касса_Key"]), MoneyType: rawString(row["ТипДенежныхСредств"]), VAT: new(big.Rat),
	}
	clearUnlinked(&invoice.DepartmentID, &invoice.CashID)
	if !linkedGUID(plan.OrganizationID) || !linkedGUID(plan.SupplierID) || !linkedGUID(plan.ContractID) || !linkedGUID(plan.CurrencyID) || len(plan.Date) < 10 {
		return plannableInvoice{}, errors.New("supplier invoice needs an organization, supplier, contract and currency")
	}
	total, ok := new(big.Rat).SetString(string(plan.Amount))
	if !ok || total.Sign() <= 0 {
		return plannableInvoice{}, errors.New("supplier invoice total must be positive")
	}
	invoice.Total = total
	var lines []map[string]any
	if raw, ok := row["Запасы"]; ok && string(raw) != "null" {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&lines); err != nil {
			return plannableInvoice{}, errors.New("invalid OData payment plan lines")
		}
	}
	sum := new(big.Rat)
	for _, line := range lines {
		lineTotal, okTotal := new(big.Rat).SetString(string(anyDecimal(line["Всего"])))
		vat := anyDecimal(line["СуммаНДС"])
		if vat == "" {
			vat = "0"
		}
		lineVAT, okVAT := new(big.Rat).SetString(string(vat))
		if !okTotal || !okVAT {
			return plannableInvoice{}, errors.New("supplier invoice has a line without amounts; plan it in 1C")
		}
		sum.Add(sum, lineTotal)
		invoice.VAT.Add(invoice.VAT, lineVAT)
	}
	if len(lines) == 0 || sum.Cmp(total) != 0 {
		return plannableInvoice{}, errors.New("supplier invoice lines do not add up to its total, so VAT cannot be split over the payment; plan it in 1C")
	}
	return invoice, nil
}

// invoicePayments never fails the caller: unreadable sources make the result incomplete instead.
func (s Service) invoicePayments(ctx context.Context, supplierID, invoiceID string) InvoicePaymentStatus {
	status := InvoicePaymentStatus{Complete: true, Paid: "0", Payments: []TracedPayment{}, Incomplete: []string{}, Notes: []string{
		"Only posted bank and cash expenses whose breakdown names this invoice are counted; payment orders, approval and equal amounts are not payment.",
	}}
	paid := &stockSum{}
	for _, kind := range []string{"payment-order", "bank-out", "cash-out"} {
		plan := config.SupplierPaymentDocuments[kind]
		rows, err := s.supplierDocumentsWithLines(ctx, plan, supplierID)
		if err != nil {
			status.Complete = false
			status.Incomplete = append(status.Incomplete, plan.Name+": "+err.Error())
			continue
		}
		for _, row := range rows {
			payment, err := tracedPayment(row, plan, kind)
			if err != nil {
				status.Complete = false
				status.Incomplete = append(status.Incomplete, plan.Name+": "+err.Error())
				continue
			}
			if kind != "payment-order" && payment.Operation != supplierPaymentAction {
				continue
			}
			linked := make([]TracedAllocation, 0)
			for _, allocation := range payment.Allocations {
				if allocation.InvoiceType == supplierInvoiceType && strings.EqualFold(allocation.InvoiceID, invoiceID) || allocation.PlanType == supplierInvoiceType && strings.EqualFold(allocation.PlanID, invoiceID) {
					allocation.Link = "invoice"
					linked = append(linked, allocation)
				}
			}
			basis := payment.BasisType == supplierInvoiceType && strings.EqualFold(payment.BasisID, invoiceID)
			if len(linked) == 0 && !basis {
				continue
			}
			payment.Allocations, payment.Link = linked, "allocation"
			if len(linked) == 0 {
				payment.Link = "basis"
				if kind != "payment-order" && payment.Posted && !payment.Deleted {
					status.Notes = append(status.Notes, "Payment "+payment.Number+" names the invoice only as its basis, without an allocation line, so it is listed but not counted.")
				}
			}
			if kind != "payment-order" && payment.Posted && !payment.Deleted {
				for _, allocation := range linked {
					if err := paid.Add(allocation.SettlementAmount); err != nil {
						status.Complete = false
						status.Incomplete = append(status.Incomplete, plan.Name+": invalid allocation amount")
					}
				}
			}
			status.Payments = append(status.Payments, payment)
		}
	}
	status.Paid = paid.Decimal()
	if status.Paid == "" {
		status.Paid = "0"
	}
	return status
}

// paymentRequestStatus reports the request's real state. Posting is not approval, and approval is not payment.
func paymentRequestStatus(posted, planned bool, calendar *PaymentCalendarEvidence, payments InvoicePaymentStatus, total *big.Rat) string {
	paid, _ := new(big.Rat).SetString(string(payments.Paid))
	switch {
	case paid != nil && paid.Sign() > 0 && total != nil && paid.Cmp(total) >= 0:
		return "paid"
	case paid != nil && paid.Sign() > 0:
		return "partly_paid"
	case !planned:
		return "no_planned_payment"
	case !posted:
		return "draft_unposted"
	case calendar == nil || !calendar.Available:
		return "posted_approval_unknown"
	}
	for _, state := range calendar.ApprovalStates {
		if state == approvedPaymentState {
			return "approved_not_paid"
		}
	}
	return "posted_not_approved"
}
