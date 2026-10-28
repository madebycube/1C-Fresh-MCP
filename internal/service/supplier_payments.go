package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"golang.org/x/sync/errgroup"
)

const maxTraceScan = 5000
const registerScanPageSize = 500

type SupplierPaymentPlan struct {
	ID                string                   `json:"id"`
	Number            string                   `json:"number"`
	Date              string                   `json:"date"`
	Posted            bool                     `json:"posted"`
	Deleted           bool                     `json:"deleted"`
	Amount            Decimal                  `json:"amount"`
	CurrencyID        string                   `json:"currency_id,omitempty"`
	OrganizationID    string                   `json:"organization_id,omitempty"`
	SupplierID        string                   `json:"supplier_id,omitempty"`
	ContractID        string                   `json:"contract_id,omitempty"`
	AccountID         string                   `json:"account_id,omitempty"`
	CashAccountID     string                   `json:"cash_account_id,omitempty"`
	SupplierAccountID string                   `json:"supplier_account_id,omitempty"`
	MoneyType         string                   `json:"money_type,omitempty"`
	PlanPayment       bool                     `json:"plan_payment"`
	IncomingNumber    string                   `json:"incoming_number,omitempty"`
	IncomingDate      string                   `json:"incoming_date,omitempty"`
	BasisID           string                   `json:"basis_id,omitempty"`
	Schedule          []PaymentScheduleLine    `json:"schedule,omitempty"`
	Calendar          *PaymentCalendarEvidence `json:"calendar,omitempty"`
}

type PaymentScheduleLine struct {
	LineNumber string  `json:"line_number"`
	Date       string  `json:"date"`
	Percent    Decimal `json:"percent"`
	Amount     Decimal `json:"amount"`
	VAT        Decimal `json:"vat"`
}

// PaymentCalendarEvidence reports payment-calendar register records that reference an invoice.
// Approval states are returned as stored; approval is separate from posting and from payment.
type PaymentCalendarEvidence struct {
	Available      bool                    `json:"available"`
	Error          string                  `json:"error,omitempty"`
	ApprovalStates []string                `json:"approval_states"`
	Records        []PaymentCalendarRecord `json:"records"`
}

type PaymentCalendarRecord struct {
	InvoiceID     string  `json:"invoice_id"`
	InvoiceType   string  `json:"invoice_type"`
	RecorderID    string  `json:"recorder_id"`
	RecorderType  string  `json:"recorder_type"`
	Period        string  `json:"period"`
	Active        bool    `json:"active"`
	ApprovalState string  `json:"approval_state"`
	Amount        Decimal `json:"amount"`
	PaymentAmount Decimal `json:"payment_amount"`
}

type SupplierPaymentPlanFilter struct {
	OrganizationID string
	SupplierID     string
	ContractID     string
	AccountID      string
	From           string
	To             string
}

type SupplierPaymentPlanPage struct {
	OrganizationID string                `json:"organization_id,omitempty"`
	SupplierID     string                `json:"supplier_id,omitempty"`
	ContractID     string                `json:"contract_id,omitempty"`
	AccountID      string                `json:"account_id,omitempty"`
	From           string                `json:"from,omitempty"`
	To             string                `json:"to,omitempty"`
	Total          int                   `json:"total"`
	Offset         int                   `json:"offset"`
	NextOffset     *int                  `json:"next_offset,omitempty"`
	Items          []SupplierPaymentPlan `json:"items"`
}

func (s Service) ListSupplierPaymentPlans(ctx context.Context, filter SupplierPaymentPlanFilter, limit, offset int) (SupplierPaymentPlanPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return SupplierPaymentPlanPage{}, err
	}
	for _, id := range []string{filter.OrganizationID, filter.SupplierID, filter.ContractID, filter.AccountID} {
		if id != "" && !linkedGUID(id) {
			return SupplierPaymentPlanPage{}, errors.New("filter IDs must be nonzero GUIDs")
		}
	}
	startDate, endDate, err := optionalDocumentDateRange(filter.From, filter.To)
	if err != nil {
		return SupplierPaymentPlanPage{}, err
	}
	plan := config.SupplierPaymentPlans
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return SupplierPaymentPlanPage{}, err
	}
	matched := make([]SupplierPaymentPlan, 0)
	for _, row := range rows {
		item, err := supplierPaymentPlan(row)
		if err != nil {
			return SupplierPaymentPlanPage{}, err
		}
		if !idMatches(filter.OrganizationID, item.OrganizationID) || !idMatches(filter.SupplierID, item.SupplierID) || !idMatches(filter.ContractID, item.ContractID) || !idMatches(filter.AccountID, item.AccountID) || startDate != "" && (item.Date < startDate || item.Date >= endDate) {
			continue
		}
		matched = append(matched, item)
	}
	page := SupplierPaymentPlanPage{OrganizationID: filter.OrganizationID, SupplierID: filter.SupplierID, ContractID: filter.ContractID, AccountID: filter.AccountID, From: filter.From, To: filter.To, Total: len(matched), Offset: offset, Items: make([]SupplierPaymentPlan, 0)}
	start := min(offset, len(matched))
	end := start + min(limit, len(matched)-start)
	for index := len(matched) - start - 1; index >= len(matched)-end; index-- {
		page.Items = append(page.Items, matched[index])
	}
	if end < len(matched) {
		page.NextOffset = &end
	}
	return page, nil
}

func (s Service) GetSupplierPaymentPlan(ctx context.Context, id string) (SupplierPaymentPlan, error) {
	if !linkedGUID(id) {
		return SupplierPaymentPlan{}, errors.New("payment plan ID must be a nonzero GUID")
	}
	plan := config.SupplierPaymentPlans
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return SupplierPaymentPlan{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return SupplierPaymentPlan{}, errors.New("invalid OData payment plan response")
	}
	item, err := supplierPaymentPlan(row)
	if err != nil || !strings.EqualFold(item.ID, id) {
		return SupplierPaymentPlan{}, errors.New("invalid OData payment plan")
	}
	var lines []map[string]json.RawMessage
	if raw, ok := row[plan.LinesField]; ok {
		if err := json.Unmarshal(raw, &lines); err != nil {
			return SupplierPaymentPlan{}, errors.New("invalid OData payment schedule")
		}
	}
	item.Schedule = make([]PaymentScheduleLine, 0, len(lines))
	for _, lineRow := range lines {
		line, err := bindFields[PaymentScheduleLine](lineRow, plan.LineFields)
		if err != nil {
			return SupplierPaymentPlan{}, errors.New("invalid OData payment schedule line")
		}
		item.Schedule = append(item.Schedule, line)
	}
	item.Calendar = s.paymentCalendar(ctx, item.ID)
	return item, nil
}

// paymentCalendar never fails the caller: an unreadable register is reported as unavailable evidence.
func (s Service) paymentCalendar(ctx context.Context, invoiceID string) *PaymentCalendarEvidence {
	register := config.PaymentCalendar
	evidence := &PaymentCalendarEvidence{ApprovalStates: []string{}, Records: []PaymentCalendarRecord{}}
	rows, err := s.scanRows(ctx, register.Name, sourceFields(register.Fields), "", maxDocumentScan)
	if err != nil {
		evidence.Error = err.Error()
		return evidence
	}
	seen := make(map[string]bool)
	for _, row := range rows {
		record, err := bindFields[PaymentCalendarRecord](row, register.Fields)
		if err != nil {
			evidence.Error = "invalid OData payment calendar record"
			evidence.Records = []PaymentCalendarRecord{}
			evidence.ApprovalStates = []string{}
			return evidence
		}
		if record.InvoiceType != supplierInvoiceType || !strings.EqualFold(record.InvoiceID, invoiceID) {
			continue
		}
		evidence.Records = append(evidence.Records, record)
		if record.ApprovalState != "" && !seen[record.ApprovalState] {
			seen[record.ApprovalState] = true
			evidence.ApprovalStates = append(evidence.ApprovalStates, record.ApprovalState)
		}
	}
	evidence.Available = true
	return evidence
}

func supplierPaymentPlan(row map[string]json.RawMessage) (SupplierPaymentPlan, error) {
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return SupplierPaymentPlan{}, errors.New("invalid OData payment plan flags")
	}
	item, err := bindFields[SupplierPaymentPlan](row, config.SupplierPaymentPlans.Fields)
	if err != nil || !linkedGUID(item.ID) || len(item.Date) < 19 {
		return SupplierPaymentPlan{}, errors.New("invalid OData payment plan fields")
	}
	clearUnlinked(&item.CurrencyID, &item.OrganizationID, &item.SupplierID, &item.ContractID, &item.AccountID, &item.CashAccountID, &item.SupplierAccountID, &item.BasisID)
	if strings.HasPrefix(item.IncomingDate, "0001-01-01") {
		item.IncomingDate = ""
	}
	return item, nil
}

type SupplierReceiptTrace struct {
	Receipt      TracedReceipt         `json:"receipt"`
	PaymentPlans []TracedPaymentPlan   `json:"payment_plans"`
	Payments     []TracedPayment       `json:"payments"`
	Prepayments  []TracedPrepayment    `json:"prepayments"`
	Candidates   []TraceCandidate      `json:"candidates"`
	Settlements  SettlementEvidence    `json:"settlements"`
	Summary      SupplierReceiptStatus `json:"summary"`
	Scans        []TraceScan           `json:"scans"`
	Unsupported  []string              `json:"unsupported"`
}

type TracedReceipt struct {
	ID             string  `json:"id"`
	Number         string  `json:"number"`
	Date           string  `json:"date"`
	Posted         bool    `json:"posted"`
	Deleted        bool    `json:"deleted"`
	Operation      string  `json:"operation"`
	Amount         Decimal `json:"amount"`
	CurrencyID     string  `json:"currency_id,omitempty"`
	OrganizationID string  `json:"organization_id,omitempty"`
	SupplierID     string  `json:"supplier_id"`
	ContractID     string  `json:"contract_id,omitempty"`
	OrderID        string  `json:"order_id,omitempty"`
	OrderType      string  `json:"order_type,omitempty"`
	IncomingNumber string  `json:"incoming_number,omitempty"`
	IncomingDate   string  `json:"incoming_date,omitempty"`
}

type TracedPaymentPlan struct {
	SupplierPaymentPlan
	Link string `json:"link"`
}

type TracedPayment struct {
	Kind           string             `json:"kind"`
	ID             string             `json:"id"`
	Number         string             `json:"number"`
	Date           string             `json:"date"`
	Posted         bool               `json:"posted"`
	Deleted        bool               `json:"deleted"`
	Operation      string             `json:"operation"`
	Amount         Decimal            `json:"amount"`
	CurrencyID     string             `json:"currency_id,omitempty"`
	AccountID      string             `json:"account_id,omitempty"`
	SupplierID     string             `json:"supplier_id,omitempty"`
	ContractID     string             `json:"contract_id,omitempty"`
	Paid           *bool              `json:"paid,omitempty"`
	BasisID        string             `json:"basis_id,omitempty"`
	BasisType      string             `json:"basis_type,omitempty"`
	PaymentOrderID string             `json:"payment_order_id,omitempty"`
	Link           string             `json:"link"`
	Allocations    []TracedAllocation `json:"allocations"`
}

type TracedAllocation struct {
	LineNumber       string  `json:"line_number"`
	Link             string  `json:"link"`
	ContractID       string  `json:"contract_id,omitempty"`
	Advance          bool    `json:"advance"`
	DocumentID       string  `json:"document_id,omitempty"`
	DocumentType     string  `json:"document_type,omitempty"`
	OrderID          string  `json:"order_id,omitempty"`
	OrderType        string  `json:"order_type,omitempty"`
	InvoiceID        string  `json:"invoice_id,omitempty"`
	InvoiceType      string  `json:"invoice_type,omitempty"`
	PlanID           string  `json:"plan_id,omitempty"`
	PlanType         string  `json:"plan_type,omitempty"`
	SettlementAmount Decimal `json:"settlement_amount"`
	PaymentAmount    Decimal `json:"payment_amount"`
}

type TracedPrepayment struct {
	LineNumber       string  `json:"line_number"`
	DocumentID       string  `json:"document_id"`
	DocumentType     string  `json:"document_type"`
	SettlementAmount Decimal `json:"settlement_amount"`
	PaymentAmount    Decimal `json:"payment_amount"`
}

type TraceCandidate struct {
	Kind   string  `json:"kind"`
	ID     string  `json:"id"`
	Number string  `json:"number"`
	Date   string  `json:"date"`
	Amount Decimal `json:"amount"`
	Reason string  `json:"reason"`
}

type TraceScan struct {
	Resource string `json:"resource"`
	Rows     int    `json:"rows"`
	Excluded int    `json:"excluded"`
	Error    string `json:"error,omitempty"`
}

// SupplierReceiptStatus counts only recorded supplier payments linked to this receipt.
// Payment orders, plans, approval and equal amounts are never treated as payment.
type SupplierReceiptStatus struct {
	Status             string   `json:"status"`
	LinkStatus         string   `json:"link_status"`
	Complete           bool     `json:"complete"`
	LinkedSettlement   Decimal  `json:"linked_settlement"`
	OutstandingByLinks Decimal  `json:"outstanding_by_links"`
	Notes              []string `json:"notes"`
}

const (
	supplierReceiptType   = "StandardODATA.Document_ПриходнаяНакладная"
	supplierOrderType     = "StandardODATA.Document_ЗаказПоставщику"
	supplierInvoiceType   = "StandardODATA.Document_СчетНаОплатуПоставщика"
	paymentOrderType      = "StandardODATA.Document_ПлатежноеПоручение"
	supplierPaymentAction = "Поставщику"
)

func (s Service) TraceSupplierReceipt(ctx context.Context, id string) (SupplierReceiptTrace, error) {
	if !linkedGUID(id) {
		return SupplierReceiptTrace{}, errors.New("receipt ID must be a nonzero GUID")
	}
	receipt, prepayments, err := s.tracedReceipt(ctx, id)
	if err != nil {
		return SupplierReceiptTrace{}, err
	}
	trace := SupplierReceiptTrace{Receipt: receipt, PaymentPlans: []TracedPaymentPlan{}, Payments: []TracedPayment{}, Prepayments: prepayments, Candidates: []TraceCandidate{}, Scans: []TraceScan{}, Unsupported: []string{
		"supplier payment documents other than payment orders, bank expenses and cash expenses",
		"opening balances and contract-level advances not allocated to this receipt (see supplier reconciliation)",
	}}
	complete := true

	planIDs := make(map[string]bool)
	planRows, err := s.allDocumentRows(ctx, config.SupplierPaymentPlans)
	scan := TraceScan{Resource: config.SupplierPaymentPlans.Name}
	if err != nil {
		scan.Error, complete = err.Error(), false
	}
	for _, row := range planRows {
		item, err := supplierPaymentPlan(row)
		if err != nil {
			return SupplierReceiptTrace{}, err
		}
		if !strings.EqualFold(item.SupplierID, receipt.SupplierID) {
			continue
		}
		scan.Rows++
		link := ""
		switch {
		case strings.EqualFold(item.BasisID, receipt.ID):
			link = "basis-receipt"
		case receipt.OrderID != "" && strings.EqualFold(item.BasisID, receipt.OrderID):
			link = "basis-supplier-order"
		}
		if link == "" {
			continue
		}
		planIDs[strings.ToLower(item.ID)] = true
		trace.PaymentPlans = append(trace.PaymentPlans, TracedPaymentPlan{SupplierPaymentPlan: item, Link: link})
	}
	trace.Scans = append(trace.Scans, scan)

	settled := &stockSum{}
	directPayments := make(map[string]bool)
	paymentOrders := make(map[string]bool)
	unlinked := make([]TracedPayment, 0)
	for _, kind := range []string{"payment-order", "bank-out", "cash-out"} {
		plan := config.SupplierPaymentDocuments[kind]
		rows, err := s.supplierDocumentsWithLines(ctx, plan, receipt.SupplierID)
		scan := TraceScan{Resource: plan.Name, Rows: len(rows)}
		if err != nil {
			scan.Error, complete = err.Error(), false
		}
		for _, row := range rows {
			payment, err := tracedPayment(row, plan, kind)
			if err != nil {
				return SupplierReceiptTrace{}, err
			}
			if kind != "payment-order" && payment.Operation != supplierPaymentAction {
				scan.Excluded++
				continue
			}
			linked := make([]TracedAllocation, 0)
			for _, allocation := range payment.Allocations {
				switch {
				case allocation.DocumentType == supplierReceiptType && strings.EqualFold(allocation.DocumentID, receipt.ID):
					allocation.Link = "receipt"
				case allocation.InvoiceType == supplierInvoiceType && planIDs[strings.ToLower(allocation.InvoiceID)] || allocation.PlanType == supplierInvoiceType && planIDs[strings.ToLower(allocation.PlanID)]:
					allocation.Link = "payment-plan"
				case receipt.OrderID != "" && allocation.OrderType == supplierOrderType && strings.EqualFold(allocation.OrderID, receipt.OrderID):
					allocation.Link = "supplier-order"
				default:
					continue
				}
				linked = append(linked, allocation)
			}
			basisLinked := payment.BasisType == supplierInvoiceType && planIDs[strings.ToLower(payment.BasisID)] || payment.BasisType == supplierReceiptType && strings.EqualFold(payment.BasisID, receipt.ID)
			if len(linked) == 0 && !basisLinked {
				payment.Allocations = nil
				unlinked = append(unlinked, payment)
				continue
			}
			payment.Allocations = linked
			payment.Link = "allocation"
			if len(linked) == 0 {
				payment.Link = "basis"
			}
			if kind == "payment-order" {
				paymentOrders[strings.ToLower(payment.ID)] = true
			}
			if kind != "payment-order" && payment.Posted && !payment.Deleted {
				for _, allocation := range linked {
					if allocation.Link == "receipt" {
						if err := settled.Add(allocation.SettlementAmount); err != nil {
							return SupplierReceiptTrace{}, err
						}
						directPayments[strings.ToLower(payment.ID)] = true
					}
				}
			}
			trace.Payments = append(trace.Payments, payment)
		}
		trace.Scans = append(trace.Scans, scan)
	}
	// Bank expenses created from a linked payment order are linked through that order.
	for _, payment := range unlinked {
		if payment.Kind == "bank-out" && payment.BasisType == paymentOrderType && paymentOrders[strings.ToLower(payment.BasisID)] {
			payment.PaymentOrderID = payment.BasisID
			payment.Link = "payment-order"
			payment.Allocations = []TracedAllocation{}
			trace.Payments = append(trace.Payments, payment)
			continue
		}
		if payment.Kind != "payment-order" && !payment.Deleted && payment.Amount != "" && decimalEqual(payment.Amount, receipt.Amount) {
			trace.Candidates = append(trace.Candidates, TraceCandidate{Kind: payment.Kind, ID: payment.ID, Number: payment.Number, Date: payment.Date, Amount: payment.Amount, Reason: "same supplier and equal amount without a document link; not proof of payment"})
		}
	}
	for index := range trace.Payments {
		payment := &trace.Payments[index]
		if payment.Kind == "bank-out" && payment.BasisType == paymentOrderType && paymentOrders[strings.ToLower(payment.BasisID)] {
			payment.PaymentOrderID = payment.BasisID
		}
	}
	notes := []string{"Only posted bank and cash expenses allocated to this receipt are counted; payment orders, plans and approval are not payment."}
	for _, prepayment := range trace.Prepayments {
		if directPayments[strings.ToLower(prepayment.DocumentID)] {
			notes = append(notes, "Prepayment line "+prepayment.LineNumber+" references a payment already counted and was not added again.")
			continue
		}
		if err := settled.Add(prepayment.SettlementAmount); err != nil {
			return SupplierReceiptTrace{}, err
		}
	}
	trace.Summary = receiptStatus(receipt.Amount, settled, complete, notes)
	trace.Settlements = s.receiptSettlements(ctx, receipt.ID)
	applySettlementStatus(&trace.Summary, trace.Settlements)
	return trace, nil
}

// SettlementEvidence is the receipt's own movements in the supplier-settlement register.
// Debt is recorded as Receipt movements and closing payments or advance offsets as Expense movements.
type SettlementEvidence struct {
	Available   bool                 `json:"available"`
	Error       string               `json:"error,omitempty"`
	Debt        Decimal              `json:"debt"`
	Settled     Decimal              `json:"settled"`
	Outstanding Decimal              `json:"outstanding"`
	Movements   []SettlementMovement `json:"movements"`
}

type SettlementMovement struct {
	Source              string  `json:"source"`
	RecorderID          string  `json:"recorder_id"`
	RecorderType        string  `json:"recorder_type"`
	LineNumber          string  `json:"line_number"`
	Period              string  `json:"period"`
	Active              bool    `json:"active"`
	RecordType          string  `json:"record_type"`
	SettlementType      string  `json:"settlement_type"`
	OrganizationID      string  `json:"organization_id,omitempty"`
	SupplierID          string  `json:"supplier_id,omitempty"`
	ContractID          string  `json:"contract_id,omitempty"`
	DocumentID          string  `json:"document_id"`
	DocumentType        string  `json:"document_type"`
	PaymentDocumentID   string  `json:"payment_document_id,omitempty"`
	PaymentDocumentType string  `json:"payment_document_type,omitempty"`
	Amount              Decimal `json:"amount"`
	CurrencyAmount      Decimal `json:"currency_amount"`
	AllocationMethod    string  `json:"allocation_method,omitempty"`
	Counted             bool    `json:"counted"`
}

var settlementSources = map[string]string{
	supplierReceiptType: "receipt",
	"StandardODATA.Document_РасходСоСчета":            "bank-out",
	"StandardODATA.Document_РасходИзКассы":            "cash-out",
	"StandardODATA.Document_Взаимозачет":              "debt-offset",
	"StandardODATA.Document_КорректировкаПоступления": "receipt-correction",
	"StandardODATA.Document_ВводНачальныхОстатков":    "opening-balance",
}

// receiptSettlements never fails the caller: an unreadable register is reported as unavailable evidence.
func (s Service) receiptSettlements(ctx context.Context, receiptID string) SettlementEvidence {
	register := config.SupplierSettlements
	evidence := SettlementEvidence{Movements: []SettlementMovement{}}
	rows, err := s.scanRows(ctx, register.Name, sourceFields(register.Fields), "", maxDocumentScan)
	if err != nil {
		evidence.Error = err.Error()
		return evidence
	}
	debt, settled := &stockSum{}, &stockSum{}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		movement, err := bindFields[SettlementMovement](row, register.Fields)
		if err != nil {
			return SettlementEvidence{Error: "invalid OData settlement record", Movements: []SettlementMovement{}}
		}
		key := strings.ToLower(movement.RecorderType + "/" + movement.RecorderID + "/" + movement.LineNumber)
		if seen[key] {
			return SettlementEvidence{Error: "settlement records repeated during lookup", Movements: []SettlementMovement{}}
		}
		seen[key] = true
		ownDocument := movement.DocumentType == supplierReceiptType && strings.EqualFold(movement.DocumentID, receiptID)
		if !ownDocument {
			// The receipt's posting also moves the advance it consumes; list it to name that payment, without counting it.
			if movement.RecorderType == supplierReceiptType && strings.EqualFold(movement.RecorderID, receiptID) {
				movement.Source = "advance-consumed"
				clearUnlinked(&movement.OrganizationID, &movement.SupplierID, &movement.ContractID)
				evidence.Movements = append(evidence.Movements, movement)
			}
			continue
		}
		movement.Source = settlementSources[movement.RecorderType]
		if movement.Source == "" {
			movement.Source = "other"
		} else if movement.Source == "receipt" && movement.RecordType == "Expense" {
			movement.Source = "advance-offset"
		}
		clearUnlinked(&movement.OrganizationID, &movement.SupplierID, &movement.ContractID)
		if movement.PaymentDocumentType == "StandardODATA.Undefined" || !linkedGUID(movement.PaymentDocumentID) {
			movement.PaymentDocumentID, movement.PaymentDocumentType = "", ""
		}
		movement.Counted = movement.Active
		if movement.Active {
			target := debt
			if movement.RecordType == "Expense" {
				target = settled
			} else if movement.RecordType != "Receipt" {
				return SettlementEvidence{Error: "unknown settlement record type", Movements: []SettlementMovement{}}
			}
			if err := target.Add(movement.CurrencyAmount); err != nil {
				return SettlementEvidence{Error: err.Error(), Movements: []SettlementMovement{}}
			}
		}
		evidence.Movements = append(evidence.Movements, movement)
	}
	evidence.Available = true
	evidence.Debt, evidence.Settled = debt.Decimal(), settled.Decimal()
	evidence.Outstanding = decimalDifference(debt, settled)
	return evidence
}

func applySettlementStatus(summary *SupplierReceiptStatus, evidence SettlementEvidence) {
	summary.LinkStatus = summary.Status
	if !evidence.Available {
		summary.Notes = append(summary.Notes, "Supplier-settlement register unavailable, so status falls back to document links: "+evidence.Error)
		return
	}
	summary.Notes = append(summary.Notes, "Status comes from the receipt's movements in the supplier-settlement register, in settlement currency. Document links are a separate view and can overlap with register advance offsets.")
	outstanding, _ := new(big.Rat).SetString(string(evidence.Outstanding))
	debt, _ := new(big.Rat).SetString(string(evidence.Debt))
	switch {
	case debt == nil || debt.Sign() == 0:
		summary.Status = "not_in_settlements"
		summary.Notes = append(summary.Notes, "The receipt has no settlement debt; it may be unposted or recorded elsewhere.")
	case outstanding.Sign() < 0:
		summary.Status = "overpaid_in_settlements"
		summary.Notes = append(summary.Notes, "More was closed against this receipt than its debt; 1C shows the excess as a supplier credit.")
	case outstanding.Sign() == 0:
		summary.Status = "closed_in_settlements"
	case outstanding.Cmp(debt) < 0:
		summary.Status = "partly_closed_in_settlements"
	default:
		summary.Status = "open_in_settlements"
	}
}

func decimalDifference(first, second *stockSum) Decimal {
	difference := new(big.Rat).Sub(&first.value, &second.value)
	text := difference.FloatString(max(first.scale, second.scale))
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return Decimal(text)
}

func (s Service) tracedReceipt(ctx context.Context, id string) (TracedReceipt, []TracedPrepayment, error) {
	plan := config.SupplierReceiptTrace
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.PaymentsField}}
	data, err := s.OData.Get(ctx, resource, params, 4<<20)
	if err != nil {
		return TracedReceipt{}, nil, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil || !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return TracedReceipt{}, nil, errors.New("invalid OData goods receipt response")
	}
	receipt, err := bindFields[TracedReceipt](row, plan.Fields)
	if err != nil || !strings.EqualFold(receipt.ID, id) {
		return TracedReceipt{}, nil, errors.New("invalid OData goods receipt")
	}
	if receipt.Operation != "ПоступлениеОтПоставщика" {
		return TracedReceipt{}, nil, errors.New("document is not a goods receipt from a supplier")
	}
	if !linkedGUID(receipt.SupplierID) {
		return TracedReceipt{}, nil, errors.New("goods receipt has no supplier")
	}
	clearUnlinked(&receipt.CurrencyID, &receipt.OrganizationID, &receipt.ContractID)
	if receipt.OrderType != supplierOrderType || !linkedGUID(receipt.OrderID) {
		receipt.OrderID, receipt.OrderType = "", ""
	}
	if strings.HasPrefix(receipt.IncomingDate, "0001-01-01") {
		receipt.IncomingDate = ""
	}
	var lines []map[string]json.RawMessage
	if raw, ok := row[plan.PaymentsField]; ok {
		if err := json.Unmarshal(raw, &lines); err != nil {
			return TracedReceipt{}, nil, errors.New("invalid OData goods receipt prepayments")
		}
	}
	prepayments := make([]TracedPrepayment, 0, len(lines))
	for _, line := range lines {
		prepayment, err := bindFields[TracedPrepayment](line, plan.PaymentFields)
		if err != nil {
			return TracedReceipt{}, nil, errors.New("invalid OData goods receipt prepayment")
		}
		prepayments = append(prepayments, prepayment)
	}
	return receipt, prepayments, nil
}

func tracedPayment(row map[string]json.RawMessage, plan config.DocumentResource, kind string) (TracedPayment, error) {
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return TracedPayment{}, errors.New("invalid OData payment flags")
	}
	payment, err := bindFields[TracedPayment](row, plan.Fields)
	if err != nil || !linkedGUID(payment.ID) || len(payment.Date) < 19 {
		return TracedPayment{}, errors.New("invalid OData payment fields")
	}
	payment.Kind = kind
	clearUnlinked(&payment.CurrencyID, &payment.AccountID, &payment.SupplierID, &payment.ContractID)
	if payment.BasisType == "StandardODATA.Undefined" || !linkedGUID(payment.BasisID) {
		payment.BasisID, payment.BasisType = "", ""
	}
	var lines []map[string]json.RawMessage
	if raw, ok := row[plan.LinesField]; ok {
		if err := json.Unmarshal(raw, &lines); err != nil {
			return TracedPayment{}, errors.New("invalid OData payment breakdown")
		}
	}
	payment.Allocations = make([]TracedAllocation, 0, len(lines))
	for _, line := range lines {
		allocation, err := bindFields[TracedAllocation](line, plan.LineFields)
		if err != nil {
			return TracedPayment{}, errors.New("invalid OData payment breakdown line")
		}
		clearUnlinked(&allocation.ContractID)
		payment.Allocations = append(payment.Allocations, allocation)
	}
	return payment, nil
}

func receiptStatus(amount Decimal, settled *stockSum, complete bool, notes []string) SupplierReceiptStatus {
	status := SupplierReceiptStatus{Complete: complete, LinkedSettlement: settled.Decimal(), Notes: notes}
	total, ok := new(big.Rat).SetString(string(amount))
	if !ok {
		status.Status = "unknown"
		status.Notes = append(status.Notes, "Receipt amount is not a decimal.")
		return status
	}
	remaining := new(big.Rat).Sub(total, &settled.value)
	scale := settled.scale
	if dot := strings.IndexByte(string(amount), '.'); dot >= 0 {
		scale = max(scale, len(amount)-dot-1)
	}
	outstanding := remaining.FloatString(scale)
	if strings.Contains(outstanding, ".") {
		outstanding = strings.TrimRight(strings.TrimRight(outstanding, "0"), ".")
	}
	status.OutstandingByLinks = Decimal(outstanding)
	switch {
	case remaining.Sign() <= 0:
		status.Status = "settled_by_linked_payments"
	case settled.value.Sign() > 0:
		status.Status = "partly_settled_by_linked_payments"
	default:
		status.Status = "no_linked_payment_found"
		status.Notes = append(status.Notes, "No linked payment does not mean unpaid; payments can be allocated through offsets or reconciliation.")
	}
	if !complete {
		status.Notes = append(status.Notes, "Some sources could not be scanned; the result is incomplete.")
	}
	return status
}

func decimalEqual(first, second Decimal) bool {
	a, ok := new(big.Rat).SetString(string(first))
	if !ok {
		return false
	}
	b, ok := new(big.Rat).SetString(string(second))
	return ok && a.Cmp(b) == 0
}

func idMatches(filter, value string) bool {
	return filter == "" || strings.EqualFold(filter, value)
}

func clearUnlinked(fields ...*string) {
	for _, field := range fields {
		if !linkedGUID(*field) {
			*field = ""
		}
	}
}

// supplierDocumentsWithLines scans document headers, keeps the supplier's documents and reads each one's lines by ID.
// The tested tenant answers HTTP 500 to reference filters, so matching happens locally.
func (s Service) supplierDocumentsWithLines(ctx context.Context, plan config.DocumentResource, supplierID string) ([]map[string]json.RawMessage, error) {
	headers, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, row := range headers {
		var supplier, id string
		_ = json.Unmarshal(row["Контрагент_Key"], &supplier)
		_ = json.Unmarshal(row["Ref_Key"], &id)
		if strings.EqualFold(supplier, supplierID) {
			ids = append(ids, id)
		}
	}
	if len(ids) > maxTraceScan {
		return nil, errors.New("more than " + strconv.Itoa(maxTraceScan) + " supplier documents")
	}
	rows := make([]map[string]json.RawMessage, len(ids))
	group, requestContext := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index, id := range ids {
		index, id := index, id
		group.Go(func() error {
			params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
			data, err := s.OData.Get(requestContext, plan.Name+"(guid'"+strings.ToLower(id)+"')", params, 4<<20)
			if err != nil {
				return err
			}
			var row map[string]json.RawMessage
			if err := json.Unmarshal(data, &row); err != nil || row == nil {
				return errors.New("invalid OData payment document response")
			}
			rows[index] = row
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return rows, nil
}

// scanRows reads a whole resource page by page, failing rather than truncating when it holds more than maxRows.
func (s Service) scanRows(ctx context.Context, resource, fields, orderBy string, maxRows int) ([]map[string]json.RawMessage, error) {
	countParams := url.Values{"$format": {"json"}, "$inlinecount": {"allpages"}, "$select": {fields}, "$top": {"1"}}
	data, err := s.OData.Get(ctx, resource, countParams, 4<<20)
	if err != nil {
		return nil, err
	}
	var countResponse struct {
		Count string `json:"odata.count"`
	}
	if err := json.Unmarshal(data, &countResponse); err != nil {
		return nil, errors.New("invalid OData count")
	}
	count, err := strconv.Atoi(countResponse.Count)
	if err != nil || count < 0 {
		return nil, errors.New("invalid OData count")
	}
	if count > maxRows {
		return nil, errors.New("more than " + strconv.Itoa(maxRows) + " records")
	}
	rows := make([]map[string]json.RawMessage, 0, count)
	for offset := 0; offset < count; offset += registerScanPageSize {
		params := url.Values{
			"$format": {"json"}, "$select": {fields},
			"$skip": {strconv.Itoa(offset)}, "$top": {strconv.Itoa(min(registerScanPageSize, count-offset))},
		}
		if orderBy != "" {
			params.Set("$orderby", orderBy)
		}
		data, err := s.OData.Get(ctx, resource, params, 8<<20)
		if err != nil {
			return nil, err
		}
		var response struct {
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
			return nil, errors.New("invalid OData page")
		}
		rows = append(rows, response.Value...)
	}
	if len(rows) != count {
		return nil, errors.New("records changed during lookup")
	}
	return rows, nil
}
