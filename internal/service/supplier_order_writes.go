package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	supplierOrderResourceName = "Document_ЗаказПоставщику"
	supplierOrderPurchase     = "ЗаказНаПокупку"
	maxOrderLines             = 200
	maxScheduleLines          = 24
	paymentTypeBank           = "Безналичные"
	paymentTypeCash           = "Наличные"
	cashBoxesResourceName     = "Catalog_Кассы"
)

type SupplierOrderLineInput struct {
	ProductID        string `json:"product_id"`
	CharacteristicID string `json:"characteristic_id,omitempty"`
	Quantity         string `json:"quantity"`
	Price            string `json:"price"`
	VATRate          string `json:"vat_rate"`
	DiscountPercent  string `json:"discount_percent,omitempty"`
	UnitID           string `json:"unit_id,omitempty"`
	Amount           string `json:"amount,omitempty"`
	VAT              string `json:"vat,omitempty"`
	DeliveryDate     string `json:"delivery_date,omitempty"`
}

// SupplierOrderPaymentInput is one planned payment; give either a percent of the order total or an exact amount.
type SupplierOrderPaymentInput struct {
	Date    string `json:"date"`
	Percent string `json:"percent,omitempty"`
	Amount  string `json:"amount,omitempty"`
}

type SupplierOrderDraft struct {
	OrganizationID string                      `json:"organization_id"`
	SupplierID     string                      `json:"supplier_id"`
	ContractID     string                      `json:"contract_id"`
	WarehouseID    string                      `json:"warehouse_id"`
	DepartmentID   string                      `json:"department_id"`
	Date           string                      `json:"date"`
	InvoiceNumber  string                      `json:"invoice_number,omitempty"`
	InvoiceDate    string                      `json:"invoice_date,omitempty"`
	DeliveryDate   string                      `json:"delivery_date,omitempty"`
	StateID        string                      `json:"state_id,omitempty"`
	Comment        string                      `json:"comment,omitempty"`
	VATMode        string                      `json:"vat_mode"`
	Lines          []SupplierOrderLineInput    `json:"lines"`
	Payments       []SupplierOrderPaymentInput `json:"payments,omitempty"`
	PaymentType    string                      `json:"payment_type,omitempty"`
	BankAccountID  string                      `json:"bank_account_id,omitempty"`
	CashBoxID      string                      `json:"cash_box_id,omitempty"`
	AllowDuplicate bool                        `json:"allow_duplicate,omitempty"`
	Apply          bool                        `json:"apply,omitempty"`
}

type SupplierOrderPreviewLine struct {
	LineNumber       int     `json:"line_number"`
	ProductID        string  `json:"product_id"`
	CharacteristicID string  `json:"characteristic_id,omitempty"`
	UnitID           string  `json:"unit_id"`
	UnitType         string  `json:"unit_type"`
	Quantity         Decimal `json:"quantity"`
	Price            Decimal `json:"price"`
	DiscountPercent  Decimal `json:"discount_percent"`
	Discount         Decimal `json:"discount"`
	VATRate          string  `json:"vat_rate"`
	VATRateID        string  `json:"vat_rate_id"`
	Amount           Decimal `json:"amount"`
	VAT              Decimal `json:"vat"`
	Total            Decimal `json:"total"`
	DeliveryDate     string  `json:"delivery_date,omitempty"`
	Preserved        bool    `json:"preserved,omitempty"`
}

type SupplierOrderPayment struct {
	Date    string  `json:"date"`
	Percent Decimal `json:"percent"`
	Amount  Decimal `json:"amount"`
	VAT     Decimal `json:"vat"`
}

type SupplierOrderDuplicate struct {
	ID            string  `json:"id"`
	Number        string  `json:"number"`
	Date          string  `json:"date"`
	Posted        bool    `json:"posted"`
	InvoiceNumber string  `json:"invoice_number"`
	Amount        Decimal `json:"amount"`
	Reason        string  `json:"reason"`
}

// SupplierOrderWrite is both the preview and the result of a supplier order create or amend.
type SupplierOrderWrite struct {
	Applied        bool                       `json:"applied"`
	ID             string                     `json:"id,omitempty"`
	Number         string                     `json:"number,omitempty"`
	DataVersion    string                     `json:"data_version,omitempty"`
	Posted         bool                       `json:"posted"`
	OrganizationID string                     `json:"organization_id"`
	SupplierID     string                     `json:"supplier_id"`
	ContractID     string                     `json:"contract_id"`
	CurrencyID     string                     `json:"currency_id"`
	WarehouseID    string                     `json:"warehouse_id"`
	DepartmentID   string                     `json:"department_id"`
	Date           string                     `json:"date"`
	InvoiceNumber  string                     `json:"invoice_number"`
	InvoiceDate    string                     `json:"invoice_date"`
	DeliveryDate   string                     `json:"delivery_date"`
	StateID        string                     `json:"state_id"`
	StateSource    string                     `json:"state_source,omitempty"`
	Comment        string                     `json:"comment"`
	VATMode        string                     `json:"vat_mode"`
	Taxation       string                     `json:"taxation"`
	Lines          []SupplierOrderPreviewLine `json:"lines"`
	Amount         Decimal                    `json:"amount"`
	VAT            Decimal                    `json:"vat"`
	PaymentType    string                     `json:"payment_type,omitempty"`
	BankAccountID  string                     `json:"bank_account_id,omitempty"`
	CashBoxID      string                     `json:"cash_box_id,omitempty"`
	Payments       []SupplierOrderPayment     `json:"payments"`
	Before         *ReceiptTotals             `json:"before,omitempty"`
	Changes        []string                   `json:"changes,omitempty"`
	Duplicates     []SupplierOrderDuplicate   `json:"duplicates"`
	Unsupported    []string                   `json:"unsupported"`
	Impact         []string                   `json:"impact"`
}

// orderTemplate holds what this tool copies from the tenant's own supplier orders instead of guessing it.
type orderTemplate struct {
	Operation      string
	BusinessOpID   string
	StateID        string
	StateFrom      string
	IncludeVATCost bool
}

// CreateSupplierOrderDraft previews a supplier order and, with Apply, saves it unposted. It never retries an uncertain POST.
func (s Service) CreateSupplierOrderDraft(ctx context.Context, input SupplierOrderDraft) (SupplierOrderWrite, error) {
	result, err := s.validateSupplierOrderHeader(ctx, input)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maxOrderLines {
		return SupplierOrderWrite{}, fmt.Errorf("a supplier order needs 1–%d lines", maxOrderLines)
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	for index, line := range input.Lines {
		unit, err := s.stockLineUnit(ctx, line.ProductID, line.CharacteristicID, line.UnitID)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("line %d: %w", index+1, err)
		}
		preview, err := computeOrderLine(line, index+1, unit, result.VATMode, rates)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("line %d: %w", index+1, err)
		}
		result.Lines = append(result.Lines, preview)
	}
	finishOrderTotals(&result)
	if err := s.setOrderPayments(ctx, &result, input.Payments, input.PaymentType, input.BankAccountID, input.CashBoxID); err != nil {
		return SupplierOrderWrite{}, err
	}
	rows, err := s.supplierOrderHeaders(ctx)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	template, err := supplierOrderTemplate(rows)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	if result.StateID == "" {
		result.StateID, result.StateSource = template.StateID, template.StateFrom
	} else if _, err := s.activeRecord(ctx, "Catalog_СостоянияЗаказовПоставщикам", result.StateID, "Ref_Key,DeletionMark"); err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("state: %w", err)
	}
	result.Duplicates = supplierOrderDuplicates(rows, result, "")
	if !input.Apply {
		return result, nil
	}
	if len(result.Duplicates) > 0 && !input.AllowDuplicate {
		return SupplierOrderWrite{}, errors.New("a supplier order that looks the same already exists; review the duplicates or allow them explicitly")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return SupplierOrderWrite{}, errors.New("OData client does not support writes")
	}
	fields := orderHeaderFields(result)
	fields["Ref_Key"] = emptyGUID
	fields["ВидОперации"] = template.Operation
	fields["ХозяйственнаяОперация_Key"] = template.BusinessOpID
	fields["Курс"] = 1
	fields["Кратность"] = 1
	fields["ПоложениеСклада"] = "ВШапке"
	fields["НДСВключатьВСтоимость"] = template.IncludeVATCost
	lines := make([]map[string]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		lines = append(lines, orderLineFields(line, result, map[string]any{"КлючСвязи": line.LineNumber}))
	}
	fields["Запасы"] = lines
	body, err := json.Marshal(fields)
	if err != nil {
		return SupplierOrderWrite{}, errors.New("could not encode supplier order")
	}
	response, err := writer.Write(ctx, http.MethodPost, supplierOrderResourceName, body, "")
	if err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("could not confirm supplier order creation; run the same preview again, which lists a saved order as a duplicate, before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return SupplierOrderWrite{}, errors.New("1C may have created the supplier order but did not return its ID; preview again to find it before retrying")
	}
	saved, err := s.readEditableOrder(ctx, created.ID)
	if err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("supplier order %s was created but could not be read back: %w", created.ID, err)
	}
	if err := verifyOrderSaved(saved, result); err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("supplier order %s was created but differs from the request: %w", created.ID, err)
	}
	result.Applied, result.ID, result.Number, result.DataVersion, result.Posted = true, saved.ID, saved.Number, saved.DataVersion, saved.Posted
	return result, nil
}

type SupplierOrderLineChange struct {
	LineNumber      int     `json:"line_number"`
	Quantity        *string `json:"quantity,omitempty"`
	Price           *string `json:"price,omitempty"`
	VATRate         *string `json:"vat_rate,omitempty"`
	DiscountPercent *string `json:"discount_percent,omitempty"`
	Amount          *string `json:"amount,omitempty"`
	VAT             *string `json:"vat,omitempty"`
	DeliveryDate    *string `json:"delivery_date,omitempty"`
}

type SupplierOrderAmendment struct {
	Date                *string                     `json:"date,omitempty"`
	WarehouseID         *string                     `json:"warehouse_id,omitempty"`
	DepartmentID        *string                     `json:"department_id,omitempty"`
	InvoiceNumber       *string                     `json:"invoice_number,omitempty"`
	InvoiceDate         *string                     `json:"invoice_date,omitempty"`
	DeliveryDate        *string                     `json:"delivery_date,omitempty"`
	StateID             *string                     `json:"state_id,omitempty"`
	Comment             *string                     `json:"comment,omitempty"`
	ChangeLines         []SupplierOrderLineChange   `json:"change_lines,omitempty"`
	AddLines            []SupplierOrderLineInput    `json:"add_lines,omitempty"`
	Payments            []SupplierOrderPaymentInput `json:"payments,omitempty"`
	ClearPayments       bool                        `json:"clear_payments,omitempty"`
	PaymentType         *string                     `json:"payment_type,omitempty"`
	BankAccountID       *string                     `json:"bank_account_id,omitempty"`
	CashBoxID           *string                     `json:"cash_box_id,omitempty"`
	Repost              bool                        `json:"repost,omitempty"`
	ExpectedDataVersion string                      `json:"expected_data_version,omitempty"`
	Apply               bool                        `json:"apply,omitempty"`
}

// AmendSupplierOrder previews and, with Apply, saves changes to a supplier order, keeping every other field and line as read.
// With Repost a posted order is unposted, changed and posted again.
func (s Service) AmendSupplierOrder(ctx context.Context, id string, change SupplierOrderAmendment) (SupplierOrderWrite, error) {
	current, err := s.readEditableOrder(ctx, id)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	if current.Operation != supplierOrderPurchase {
		return SupplierOrderWrite{}, unsupported("only purchase orders can be changed; this order has operation " + current.Operation)
	}
	if current.Posted && !change.Repost {
		return SupplierOrderWrite{}, errors.New("supplier order is posted; pass repost to unpost, amend and post it again, or unpost it explicitly first")
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != current.DataVersion {
		return SupplierOrderWrite{}, errors.New("supplier order changed since the preview; preview again")
	}
	draft := current.draft()
	changes := make([]string, 0)
	setText := func(target *string, value *string, label string) {
		if value != nil && *value != *target {
			*target = *value
			changes = append(changes, label)
		}
	}
	setText(&draft.Date, change.Date, "date")
	setText(&draft.WarehouseID, change.WarehouseID, "warehouse")
	setText(&draft.DepartmentID, change.DepartmentID, "department")
	setText(&draft.InvoiceNumber, change.InvoiceNumber, "supplier invoice number")
	setText(&draft.InvoiceDate, change.InvoiceDate, "supplier invoice date")
	setText(&draft.DeliveryDate, change.DeliveryDate, "delivery date")
	setText(&draft.StateID, change.StateID, "state")
	setText(&draft.Comment, change.Comment, "comment")
	result, err := s.validateSupplierOrderHeader(ctx, draft)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	if change.StateID != nil && *change.StateID != current.StateID {
		if _, err := s.activeRecord(ctx, "Catalog_СостоянияЗаказовПоставщикам", result.StateID, "Ref_Key,DeletionMark"); err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("state: %w", err)
		}
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	byNumber := make(map[int]int, len(current.Lines))
	for index, line := range current.Lines {
		preview, err := existingOrderLine(line, rates)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("existing line %d: %w", line.LineNumber, err)
		}
		byNumber[line.LineNumber] = index
		result.Lines = append(result.Lines, preview)
	}
	before := ReceiptTotals{Amount: current.Amount, VAT: current.VAT, Lines: len(current.Lines)}
	seen := make(map[int]bool)
	for _, lineChange := range change.ChangeLines {
		index, ok := byNumber[lineChange.LineNumber]
		if !ok || seen[lineChange.LineNumber] {
			return SupplierOrderWrite{}, fmt.Errorf("line %d does not exist or is changed twice", lineChange.LineNumber)
		}
		seen[lineChange.LineNumber] = true
		line := &result.Lines[index]
		input := SupplierOrderLineInput{ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, Quantity: string(line.Quantity), Price: string(line.Price), VATRate: line.VATRate, DiscountPercent: string(line.DiscountPercent), DeliveryDate: line.DeliveryDate}
		for target, value := range map[*string]*string{&input.Quantity: lineChange.Quantity, &input.Price: lineChange.Price, &input.VATRate: lineChange.VATRate, &input.DiscountPercent: lineChange.DiscountPercent, &input.Amount: lineChange.Amount, &input.VAT: lineChange.VAT, &input.DeliveryDate: lineChange.DeliveryDate} {
			if value != nil {
				*target = *value
			}
		}
		updated, err := computeOrderLine(input, line.LineNumber, lineUnit{ID: line.UnitID, Type: line.UnitType}, result.VATMode, rates)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("line %d: %w", line.LineNumber, err)
		}
		*line = updated
		changes = append(changes, fmt.Sprintf("line %d", line.LineNumber))
	}
	if len(result.Lines)+len(change.AddLines) > maxOrderLines {
		return SupplierOrderWrite{}, fmt.Errorf("a supplier order holds at most %d lines", maxOrderLines)
	}
	next := 0
	for _, line := range current.Lines {
		next = max(next, line.LineNumber)
	}
	for _, input := range change.AddLines {
		next++
		unit, err := s.stockLineUnit(ctx, input.ProductID, input.CharacteristicID, input.UnitID)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("new line: %w", err)
		}
		preview, err := computeOrderLine(input, next, unit, result.VATMode, rates)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("new line: %w", err)
		}
		result.Lines = append(result.Lines, preview)
		changes = append(changes, fmt.Sprintf("new line %d", next))
	}
	finishOrderTotals(&result)
	if current.Taxation != "" {
		result.Taxation = current.Taxation
	}
	paymentType, bankAccount, cashBox := current.paymentTypeName(), current.BankAccountID, current.CashBoxID
	for target, value := range map[*string]*string{&paymentType: change.PaymentType, &bankAccount: change.BankAccountID, &cashBox: change.CashBoxID} {
		if value != nil {
			*target = *value
		}
	}
	scheduleChanged := len(change.Payments) > 0 || change.ClearPayments || change.PaymentType != nil || change.BankAccountID != nil || change.CashBoxID != nil
	if change.ClearPayments && len(change.Payments) > 0 {
		return SupplierOrderWrite{}, errors.New("clear payments cannot be combined with new payments")
	}
	switch {
	case change.ClearPayments:
		result.Payments = []SupplierOrderPayment{}
		changes = append(changes, "payment schedule cleared")
	case scheduleChanged:
		inputs := change.Payments
		if len(inputs) == 0 {
			inputs = current.paymentInputs()
		}
		if err := s.setOrderPayments(ctx, &result, inputs, paymentType, bankAccount, cashBox); err != nil {
			return SupplierOrderWrite{}, err
		}
		changes = append(changes, "payment schedule")
	default:
		result.Payments, result.PaymentType, result.BankAccountID, result.CashBoxID = current.Schedule, paymentType, current.BankAccountID, current.CashBoxID
		if len(current.Schedule) > 0 && !decimalEqual(result.Amount, current.Amount) {
			return SupplierOrderWrite{}, errors.New("the order total changes but its payment schedule does not; pass new payments or clear them")
		}
	}
	result.ID, result.Number, result.DataVersion, result.Before, result.Changes, result.Posted = current.ID, current.Number, current.DataVersion, &before, changes, current.Posted
	if current.Posted {
		result.Impact = []string{
			"unposts the order, removing its ordered-goods and planned-payment movements, saves the changes, then posts it again",
			"if a step fails the order is left unposted; read it before retrying",
		}
	}
	rows, err := s.supplierOrderHeaders(ctx)
	if err != nil {
		return SupplierOrderWrite{}, err
	}
	result.Duplicates = supplierOrderDuplicates(rows, result, current.ID)
	if !change.Apply || len(changes) == 0 {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return SupplierOrderWrite{}, errors.New("OData client does not support writes")
	}
	fields := orderHeaderFields(result)
	if !scheduleChanged {
		delete(fields, "ПлатежныйКалендарь")
		delete(fields, "ЗапланироватьОплату")
		delete(fields, "ТипДенежныхСредств")
		delete(fields, "БанковскийСчет_Key")
		delete(fields, "Касса_Key")
	}
	lines := make([]map[string]any, 0, len(result.Lines))
	for index, line := range result.Lines {
		var original map[string]any
		if index < len(current.Lines) {
			original = current.Lines[index].Raw
		} else {
			original = map[string]any{"КлючСвязи": line.LineNumber}
		}
		lines = append(lines, orderLineFields(line, result, original))
	}
	fields["Запасы"] = lines
	body, err := json.Marshal(fields)
	if err != nil {
		return SupplierOrderWrite{}, errors.New("could not encode supplier order")
	}
	version := current.DataVersion
	var actor documentActor
	if current.Posted {
		if actor, ok = s.OData.(documentActor); !ok {
			return SupplierOrderWrite{}, errors.New("OData client does not support document actions")
		}
		if _, err := actor.Action(ctx, supplierOrderResource(current.ID), "Unpost"); err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("1C rejected unpost; nothing was changed or the state is uncertain, so read the order before retrying: %w", err)
		}
		unposted, err := s.readEditableOrder(ctx, current.ID)
		if err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("supplier order was unposted but could not be read back; no changes were saved: %w", err)
		}
		if unposted.Posted || !decimalEqual(unposted.Amount, current.Amount) || len(unposted.Lines) != len(current.Lines) {
			return SupplierOrderWrite{}, errors.New("supplier order did not unpost cleanly; no changes were saved, check it in 1C")
		}
		version = unposted.DataVersion
	}
	if _, err := writer.Write(ctx, http.MethodPatch, supplierOrderResource(current.ID), body, version); err != nil {
		if current.Posted {
			return SupplierOrderWrite{}, fmt.Errorf("supplier order was unposted but the changes were not saved; it is left unposted: %w", err)
		}
		return SupplierOrderWrite{}, err
	}
	saved, err := s.readEditableOrder(ctx, current.ID)
	if err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("supplier order was updated but could not be read back: %w", err)
	}
	if err := verifyOrderSaved(saved, result); err != nil {
		return SupplierOrderWrite{}, fmt.Errorf("supplier order was updated but differs from the request; it is left unposted: %w", err)
	}
	if current.Posted {
		if _, err := actor.Action(ctx, supplierOrderResource(current.ID), "Post"); err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("changes were saved but 1C rejected posting; the order is left unposted: %w", err)
		}
		if saved, err = s.readEditableOrder(ctx, current.ID); err != nil {
			return SupplierOrderWrite{}, fmt.Errorf("changes were saved and posting was sent, but the order could not be read back: %w", err)
		}
		if !saved.Posted {
			return SupplierOrderWrite{}, errors.New("changes were saved but posting did not take effect; the order is left unposted")
		}
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	return result, nil
}

// SetSupplierOrderPosting previews and, with Apply, posts or unposts a supplier order through the 1C document actions.
func (s Service) SetSupplierOrderPosting(ctx context.Context, id, action, expectedDataVersion string, apply bool) (ReceiptPosting, error) {
	if action != "post" && action != "unpost" {
		return ReceiptPosting{}, errors.New("action must be post or unpost")
	}
	current, err := s.readEditableOrder(ctx, id)
	if err != nil {
		return ReceiptPosting{}, err
	}
	result := ReceiptPosting{ID: current.ID, Action: action, PostedFrom: current.Posted, Posted: current.Posted, DataVersion: current.DataVersion, Amount: current.Amount, Lines: len(current.Lines)}
	if action == "post" {
		result.Impact = []string{
			fmt.Sprintf("records %d lines as ordered from the supplier and expected at warehouse %s", len(current.Lines), current.WarehouseID),
			"records the payment schedule as planned payments; it does not pay, approve or send anything to a bank",
		}
	} else {
		result.Impact = []string{"removes the order's ordered-goods and planned-payment movements until it is posted again; linked receipts and payments stay as they are"}
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return ReceiptPosting{}, errors.New("supplier order changed since the preview; preview again")
	}
	if !apply || current.Posted == (action == "post") {
		return result, nil
	}
	actor, ok := s.OData.(documentActor)
	if !ok {
		return ReceiptPosting{}, errors.New("OData client does not support document actions")
	}
	name := "Post"
	if action == "unpost" {
		name = "Unpost"
	}
	if _, err := actor.Action(ctx, supplierOrderResource(current.ID), name); err != nil {
		return ReceiptPosting{}, fmt.Errorf("1C rejected %s; the order was not changed by this request or its state is uncertain, so read it before retrying: %w", action, err)
	}
	saved, err := s.readEditableOrder(ctx, current.ID)
	if err != nil {
		return ReceiptPosting{}, fmt.Errorf("%s was sent but the order could not be read back: %w", action, err)
	}
	if saved.Posted != (action == "post") {
		return ReceiptPosting{}, fmt.Errorf("%s was accepted but the order posting flag did not change", action)
	}
	result.Applied, result.Posted, result.DataVersion = true, saved.Posted, saved.DataVersion
	return result, nil
}

func (s Service) validateSupplierOrderHeader(ctx context.Context, input SupplierOrderDraft) (SupplierOrderWrite, error) {
	header, err := s.validateReceiptHeader(ctx, ReceiptDraft{
		OrganizationID: input.OrganizationID, SupplierID: input.SupplierID, ContractID: input.ContractID, WarehouseID: input.WarehouseID, DepartmentID: input.DepartmentID,
		Date: input.Date, IncomingNumber: input.InvoiceNumber, IncomingDate: input.InvoiceDate, Comment: input.Comment, VATMode: input.VATMode,
	})
	if err != nil {
		if isUnsupported(err) {
			return SupplierOrderWrite{}, err
		}
		return SupplierOrderWrite{}, errors.New(strings.NewReplacer("incoming number", "supplier invoice number", "incoming date", "supplier invoice date", "receipt", "order").Replace(err.Error()))
	}
	deliveryDate, err := receiptDate(input.DeliveryDate, true)
	if err != nil {
		return SupplierOrderWrite{}, errors.New("delivery date must be YYYY-MM-DD")
	}
	if deliveryDate != "" && deliveryDate < header.Date[:10] {
		return SupplierOrderWrite{}, errors.New("delivery date cannot be before the order date")
	}
	if input.StateID != "" && !linkedGUID(input.StateID) {
		return SupplierOrderWrite{}, errors.New("state ID must be a nonzero GUID")
	}
	return SupplierOrderWrite{
		OrganizationID: header.OrganizationID, SupplierID: header.SupplierID, ContractID: header.ContractID, CurrencyID: header.CurrencyID,
		WarehouseID: header.WarehouseID, DepartmentID: header.DepartmentID, Date: header.Date, InvoiceNumber: header.IncomingNumber, InvoiceDate: header.IncomingDate,
		DeliveryDate: deliveryDate, StateID: strings.ToLower(input.StateID), Comment: header.Comment, VATMode: header.VATMode,
		Lines: []SupplierOrderPreviewLine{}, Payments: []SupplierOrderPayment{}, Duplicates: []SupplierOrderDuplicate{},
		Unsupported: []string{"business line (направление деятельности): the supplier order in this configuration has no such field; it is set on payments and receipts"},
	}, nil
}

func isUnsupported(err error) bool {
	var target unsupportedError
	return errors.As(err, &target)
}
