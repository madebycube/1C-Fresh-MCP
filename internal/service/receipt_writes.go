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
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	receiptResourceName  = "Document_ПриходнаяНакладная"
	receiptOperation     = "ПоступлениеОтПоставщика"
	unitClassifierType   = "StandardODATA.Catalog_КлассификаторЕдиницИзмерения"
	productUnitType      = "StandardODATA.Catalog_ЕдиницыИзмерения"
	maxReceiptLines      = 200
	maxQuantityScale     = 3
	maxPriceScale        = 2
	receiptAmountScale   = 2
	receiptBaseCurrency  = "643"
	receiptVATIncluded   = "inclusive"
	receiptVATExcluded   = "exclusive"
	receiptTaxed         = "ОблагаетсяНДС"
	receiptNotTaxed      = "НеОблагаетсяНДС"
	receiptStockProducts = "Запас"
)

type documentActor interface {
	Action(context.Context, string, string) ([]byte, error)
}

type ReceiptLineInput struct {
	ProductID        string `json:"product_id"`
	CharacteristicID string `json:"characteristic_id,omitempty"`
	Quantity         string `json:"quantity"`
	Price            string `json:"price"`
	VATRate          string `json:"vat_rate"`
	UnitID           string `json:"unit_id,omitempty"`
	Amount           string `json:"amount,omitempty"`
	VAT              string `json:"vat,omitempty"`
}

type ReceiptDraft struct {
	OrganizationID string             `json:"organization_id"`
	SupplierID     string             `json:"supplier_id"`
	ContractID     string             `json:"contract_id"`
	WarehouseID    string             `json:"warehouse_id"`
	DepartmentID   string             `json:"department_id"`
	Date           string             `json:"date"`
	IncomingNumber string             `json:"incoming_number,omitempty"`
	IncomingDate   string             `json:"incoming_date,omitempty"`
	Comment        string             `json:"comment,omitempty"`
	VATMode        string             `json:"vat_mode"`
	Lines          []ReceiptLineInput `json:"lines"`
	AllowDuplicate bool               `json:"allow_duplicate,omitempty"`
	Apply          bool               `json:"apply,omitempty"`
}

type ReceiptPreviewLine struct {
	LineNumber       int     `json:"line_number"`
	ProductID        string  `json:"product_id"`
	CharacteristicID string  `json:"characteristic_id,omitempty"`
	UnitID           string  `json:"unit_id"`
	UnitType         string  `json:"unit_type"`
	UnitFactor       Decimal `json:"unit_factor,omitempty"`
	BaseQuantity     Decimal `json:"base_quantity,omitempty"`
	Quantity         Decimal `json:"quantity"`
	Price            Decimal `json:"price"`
	VATRate          string  `json:"vat_rate"`
	VATRateID        string  `json:"vat_rate_id"`
	Amount           Decimal `json:"amount"`
	VAT              Decimal `json:"vat"`
	Total            Decimal `json:"total"`
	ComputedAmount   Decimal `json:"computed_amount,omitempty"`
	ComputedVAT      Decimal `json:"computed_vat,omitempty"`
	Preserved        bool    `json:"preserved,omitempty"`
}

// unsupportedError marks a receipt this tool cannot write at all, as opposed to one with fixable input.
type unsupportedError struct{ message string }

func (e unsupportedError) Error() string { return e.message }

func unsupported(message string) error { return unsupportedError{message} }

// lineUnit is the unit a receipt line is entered in; Factor converts it to the product's base unit when known.
type lineUnit struct {
	ID     string
	Type   string
	Factor Decimal
}

type ReceiptDuplicate struct {
	ID             string `json:"id"`
	Number         string `json:"number"`
	Date           string `json:"date"`
	Posted         bool   `json:"posted"`
	IncomingNumber string `json:"incoming_number"`
	IncomingDate   string `json:"incoming_date"`
}

// ReceiptWrite is both the preview and the result of a receipt create or amend.
type ReceiptWrite struct {
	Applied        bool                 `json:"applied"`
	ID             string               `json:"id,omitempty"`
	DataVersion    string               `json:"data_version,omitempty"`
	Posted         bool                 `json:"posted"`
	OrganizationID string               `json:"organization_id"`
	SupplierID     string               `json:"supplier_id"`
	ContractID     string               `json:"contract_id"`
	CurrencyID     string               `json:"currency_id"`
	WarehouseID    string               `json:"warehouse_id"`
	DepartmentID   string               `json:"department_id"`
	Date           string               `json:"date"`
	IncomingNumber string               `json:"incoming_number"`
	IncomingDate   string               `json:"incoming_date"`
	Comment        string               `json:"comment"`
	VATMode        string               `json:"vat_mode"`
	Taxation       string               `json:"taxation"`
	Lines          []ReceiptPreviewLine `json:"lines"`
	Amount         Decimal              `json:"amount"`
	VAT            Decimal              `json:"vat"`
	Before         *ReceiptTotals       `json:"before,omitempty"`
	Changes        []string             `json:"changes,omitempty"`
	Duplicates     []ReceiptDuplicate   `json:"duplicates"`
	Impact         []string             `json:"impact"`
}

type ReceiptTotals struct {
	Amount Decimal `json:"amount"`
	VAT    Decimal `json:"vat"`
	Lines  int     `json:"lines"`
}

type vatRate struct {
	ID         string
	Name       string
	Percent    *big.Rat
	NotTaxable bool
}

// CreateReceiptDraft previews a supplier receipt and, with Apply, saves it unposted. It never retries an uncertain POST.
func (s Service) CreateReceiptDraft(ctx context.Context, input ReceiptDraft) (ReceiptWrite, error) {
	result, err := s.validateReceiptHeader(ctx, input)
	if err != nil {
		return ReceiptWrite{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maxReceiptLines {
		return ReceiptWrite{}, fmt.Errorf("a receipt needs 1–%d lines", maxReceiptLines)
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return ReceiptWrite{}, err
	}
	for index, line := range input.Lines {
		preview, err := s.receiptLine(ctx, line, index+1, result.VATMode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("line %d: %w", index+1, err)
		}
		result.Lines = append(result.Lines, preview)
	}
	finishReceiptTotals(&result)
	duplicates, err := s.receiptDuplicates(ctx, result, "")
	if err != nil {
		return ReceiptWrite{}, err
	}
	result.Duplicates = duplicates
	if !input.Apply {
		return result, nil
	}
	if len(duplicates) > 0 && !input.AllowDuplicate {
		return ReceiptWrite{}, errors.New("a receipt with the same organization, supplier, incoming number and incoming date exists; review the duplicates or allow them explicitly")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ReceiptWrite{}, errors.New("OData client does not support writes")
	}
	fields := receiptHeaderFields(result)
	fields["Ref_Key"] = emptyGUID
	fields["ВидОперации"] = receiptOperation
	fields["Курс"] = 1
	fields["Кратность"] = 1
	fields["ПоложениеСклада"] = "ВШапке"
	fields["НДСВключатьВСтоимость"] = true
	operation, err := s.receiptBusinessOperation(ctx)
	if err != nil {
		return ReceiptWrite{}, err
	}
	fields["ХозяйственнаяОперация_Key"] = operation
	rows := make([]map[string]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		rows = append(rows, receiptLineFields(line, result.WarehouseID, map[string]any{"КлючСвязи": line.LineNumber}))
	}
	fields["Запасы"] = rows
	body, err := json.Marshal(fields)
	if err != nil {
		return ReceiptWrite{}, errors.New("could not encode receipt")
	}
	response, err := writer.Write(ctx, http.MethodPost, receiptResourceName, body, "")
	if err != nil {
		return ReceiptWrite{}, fmt.Errorf("could not confirm receipt creation; check list purchases for incoming number %q before retrying: %w", result.IncomingNumber, err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return ReceiptWrite{}, errors.New("1C may have created the receipt but did not return its ID; check list purchases before retrying")
	}
	saved, err := s.readEditableReceipt(ctx, created.ID)
	if err != nil {
		return ReceiptWrite{}, fmt.Errorf("receipt %s was created but could not be read back: %w", created.ID, err)
	}
	if err := verifyReceiptSaved(saved, result); err != nil {
		return ReceiptWrite{}, fmt.Errorf("receipt %s was created but differs from the request: %w", created.ID, err)
	}
	result.Applied, result.ID, result.DataVersion, result.Posted = true, created.ID, saved.DataVersion, saved.Posted
	return result, nil
}

type ReceiptLineChange struct {
	LineNumber int     `json:"line_number"`
	Quantity   *string `json:"quantity,omitempty"`
	Price      *string `json:"price,omitempty"`
	VATRate    *string `json:"vat_rate,omitempty"`
	Amount     *string `json:"amount,omitempty"`
	VAT        *string `json:"vat,omitempty"`
}

type ReceiptAmendment struct {
	Date                *string             `json:"date,omitempty"`
	WarehouseID         *string             `json:"warehouse_id,omitempty"`
	DepartmentID        *string             `json:"department_id,omitempty"`
	IncomingNumber      *string             `json:"incoming_number,omitempty"`
	IncomingDate        *string             `json:"incoming_date,omitempty"`
	Comment             *string             `json:"comment,omitempty"`
	VATMode             *string             `json:"vat_mode,omitempty"`
	VATRate             *string             `json:"vat_rate,omitempty"`
	ChangeLines         []ReceiptLineChange `json:"change_lines,omitempty"`
	AddLines            []ReceiptLineInput  `json:"add_lines,omitempty"`
	ExpectedDataVersion string              `json:"expected_data_version,omitempty"`
	Apply               bool                `json:"apply,omitempty"`
}

// AmendReceipt previews and, with Apply, saves changes to an unposted receipt, keeping every other field and line as read.
func (s Service) AmendReceipt(ctx context.Context, id string, change ReceiptAmendment) (ReceiptWrite, error) {
	current, err := s.readEditableReceipt(ctx, id)
	if err != nil {
		return ReceiptWrite{}, err
	}
	if current.Posted {
		return ReceiptWrite{}, errors.New("receipt is posted; unpost it explicitly first, amend, then post it again")
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != current.DataVersion {
		return ReceiptWrite{}, errors.New("receipt changed since the preview; preview again")
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
	setText(&draft.IncomingNumber, change.IncomingNumber, "incoming number")
	setText(&draft.IncomingDate, change.IncomingDate, "incoming date")
	setText(&draft.Comment, change.Comment, "comment")
	setText(&draft.VATMode, change.VATMode, "VAT mode")
	result, err := s.validateReceiptHeader(ctx, draft)
	if err != nil {
		return ReceiptWrite{}, err
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return ReceiptWrite{}, err
	}
	byNumber := make(map[int]int, len(current.Lines))
	for index, line := range current.Lines {
		preview, err := existingReceiptLine(line, result.VATMode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("existing line %d: %w", line.LineNumber, err)
		}
		byNumber[line.LineNumber] = index
		result.Lines = append(result.Lines, preview)
	}
	// A new VAT mode or a rate for every line keeps each line's stored amount and recomputes its VAT and total.
	modeChanged := result.VATMode != current.draft().VATMode
	for index := range result.Lines {
		line := &result.Lines[index]
		rate := line.VATRate
		if change.VATRate != nil {
			rate = *change.VATRate
		}
		if !modeChanged && rate == line.VATRate {
			continue
		}
		input := ReceiptLineInput{ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, Quantity: string(line.Quantity), Price: string(line.Price), VATRate: rate, Amount: string(line.Amount)}
		updated, err := computeReceiptLine(input, line.LineNumber, lineUnit{ID: line.UnitID, Type: line.UnitType}, result.VATMode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("line %d: %w", line.LineNumber, err)
		}
		*line = updated
		changes = append(changes, fmt.Sprintf("line %d VAT", line.LineNumber))
	}
	before := ReceiptTotals{Amount: current.Amount, VAT: current.VAT, Lines: len(current.Lines)}
	seen := make(map[int]bool)
	for _, lineChange := range change.ChangeLines {
		index, ok := byNumber[lineChange.LineNumber]
		if !ok || seen[lineChange.LineNumber] {
			return ReceiptWrite{}, fmt.Errorf("line %d does not exist or is changed twice", lineChange.LineNumber)
		}
		seen[lineChange.LineNumber] = true
		line := &result.Lines[index]
		input := ReceiptLineInput{ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, Quantity: string(line.Quantity), Price: string(line.Price), VATRate: line.VATRate}
		if lineChange.Amount != nil {
			input.Amount = *lineChange.Amount
		}
		if lineChange.VAT != nil {
			input.VAT = *lineChange.VAT
		}
		if lineChange.Quantity != nil {
			input.Quantity = *lineChange.Quantity
		}
		if lineChange.Price != nil {
			input.Price = *lineChange.Price
		}
		if lineChange.VATRate != nil {
			input.VATRate = *lineChange.VATRate
		}
		updated, err := computeReceiptLine(input, line.LineNumber, lineUnit{ID: line.UnitID, Type: line.UnitType, Factor: line.UnitFactor}, result.VATMode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("line %d: %w", line.LineNumber, err)
		}
		updated.Preserved = false
		*line = updated
		changes = append(changes, fmt.Sprintf("line %d", line.LineNumber))
	}
	next := 0
	for _, line := range current.Lines {
		next = max(next, line.LineNumber)
	}
	if len(result.Lines)+len(change.AddLines) > maxReceiptLines {
		return ReceiptWrite{}, fmt.Errorf("a receipt holds at most %d lines", maxReceiptLines)
	}
	for _, input := range change.AddLines {
		next++
		preview, err := s.receiptLine(ctx, input, next, result.VATMode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("new line: %w", err)
		}
		result.Lines = append(result.Lines, preview)
		changes = append(changes, fmt.Sprintf("new line %d", next))
	}
	finishReceiptTotals(&result)
	if current.Taxation != "" && change.VATRate == nil {
		// An amendment keeps the receipt's taxation unless every line gets a new rate; a new draft derives it from its rates.
		result.Taxation = current.Taxation
	}
	result.ID, result.DataVersion, result.Before, result.Changes = current.ID, current.DataVersion, &before, changes
	duplicates, err := s.receiptDuplicates(ctx, result, current.ID)
	if err != nil {
		return ReceiptWrite{}, err
	}
	result.Duplicates = duplicates
	if !change.Apply || len(changes) == 0 {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ReceiptWrite{}, errors.New("OData client does not support writes")
	}
	fields := receiptHeaderFields(result)
	rows := make([]map[string]any, 0, len(result.Lines))
	for index, line := range result.Lines {
		var original map[string]any
		if index < len(current.Lines) {
			original = current.Lines[index].Raw
		}
		rows = append(rows, receiptLineFields(line, result.WarehouseID, original))
	}
	fields["Запасы"] = rows
	body, err := json.Marshal(fields)
	if err != nil {
		return ReceiptWrite{}, errors.New("could not encode receipt")
	}
	if _, err := writer.Write(ctx, http.MethodPatch, receiptResource(current.ID), body, current.DataVersion); err != nil {
		return ReceiptWrite{}, err
	}
	saved, err := s.readEditableReceipt(ctx, current.ID)
	if err != nil {
		return ReceiptWrite{}, fmt.Errorf("receipt was updated but could not be read back: %w", err)
	}
	if err := verifyReceiptSaved(saved, result); err != nil {
		return ReceiptWrite{}, fmt.Errorf("receipt was updated but differs from the request: %w", err)
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	return result, nil
}

type ReceiptPosting struct {
	ID          string   `json:"id"`
	Action      string   `json:"action"`
	Applied     bool     `json:"applied"`
	PostedFrom  bool     `json:"posted_before"`
	Posted      bool     `json:"posted"`
	DataVersion string   `json:"data_version"`
	Amount      Decimal  `json:"amount"`
	Lines       int      `json:"lines"`
	Impact      []string `json:"impact"`
}

// SetReceiptPosting previews and, with Apply, posts or unposts a supplier receipt through the 1C document actions.
func (s Service) SetReceiptPosting(ctx context.Context, id, action, expectedDataVersion string, apply bool) (ReceiptPosting, error) {
	if action != "post" && action != "unpost" {
		return ReceiptPosting{}, errors.New("action must be post or unpost")
	}
	current, err := s.readEditableReceipt(ctx, id)
	if err != nil {
		return ReceiptPosting{}, err
	}
	result := ReceiptPosting{ID: current.ID, Action: action, PostedFrom: current.Posted, Posted: current.Posted, DataVersion: current.DataVersion, Amount: current.Amount, Lines: len(current.Lines)}
	if action == "post" {
		result.Impact = []string{
			fmt.Sprintf("adds the quantities of %d lines to the warehouse stock", len(current.Lines)),
			"records a supplier debt of " + string(current.Amount) + " on the contract; 1C may offset open advances when it posts",
		}
	} else {
		result.Impact = []string{"removes this receipt's stock and supplier-settlement movements until it is posted again, including any advance offset"}
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return ReceiptPosting{}, errors.New("receipt changed since the preview; preview again")
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
	if _, err := actor.Action(ctx, receiptResource(current.ID), name); err != nil {
		return ReceiptPosting{}, fmt.Errorf("1C rejected %s; the receipt was not changed by this request or its state is uncertain, so read it before retrying: %w", action, err)
	}
	saved, err := s.readEditableReceipt(ctx, current.ID)
	if err != nil {
		return ReceiptPosting{}, fmt.Errorf("%s was sent but the receipt could not be read back: %w", action, err)
	}
	if saved.Posted != (action == "post") {
		return ReceiptPosting{}, fmt.Errorf("%s was accepted but the receipt posting flag did not change", action)
	}
	result.Applied, result.Posted, result.DataVersion = true, saved.Posted, saved.DataVersion
	return result, nil
}

func (s Service) validateReceiptHeader(ctx context.Context, input ReceiptDraft) (ReceiptWrite, error) {
	for label, id := range map[string]string{"organization": input.OrganizationID, "supplier": input.SupplierID, "contract": input.ContractID, "warehouse": input.WarehouseID, "department": input.DepartmentID} {
		if !linkedGUID(id) {
			return ReceiptWrite{}, fmt.Errorf("%s ID must be a nonzero GUID", label)
		}
	}
	date, err := receiptDate(input.Date, false)
	if err != nil {
		return ReceiptWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
	}
	incomingDate, err := receiptDate(input.IncomingDate, true)
	if err != nil {
		return ReceiptWrite{}, errors.New("incoming date must be YYYY-MM-DD")
	}
	incomingNumber, err := counterpartyText(input.IncomingNumber, true, 30)
	if err != nil {
		return ReceiptWrite{}, errors.New("incoming number must be at most 30 characters without control characters")
	}
	comment := strings.TrimSpace(input.Comment)
	if utf8.RuneCountInString(comment) > 1024 || strings.IndexFunc(comment, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		return ReceiptWrite{}, errors.New("comment must be at most 1024 characters")
	}
	if input.VATMode != receiptVATIncluded && input.VATMode != receiptVATExcluded {
		return ReceiptWrite{}, errors.New("VAT mode must be inclusive or exclusive")
	}
	if _, err := s.activeRecord(ctx, "Catalog_Организации", input.OrganizationID, "Ref_Key,DeletionMark"); err != nil {
		return ReceiptWrite{}, fmt.Errorf("organization: %w", err)
	}
	supplier, err := s.readCounterpartyRecord(ctx, input.SupplierID)
	if err != nil {
		return ReceiptWrite{}, fmt.Errorf("supplier: %w", err)
	}
	if supplier.IsFolder == nil || *supplier.IsFolder || supplier.Deleted == nil || *supplier.Deleted || supplier.Supplier == nil || !*supplier.Supplier {
		return ReceiptWrite{}, errors.New("supplier ID must identify an active supplier")
	}
	contract, err := s.activeRecord(ctx, "Catalog_ДоговорыКонтрагентов", input.ContractID, "Ref_Key,DeletionMark,Owner,Организация_Key,ВалютаРасчетов_Key,Недействителен,РасчетыВУсловныхЕдиницах")
	if err != nil {
		return ReceiptWrite{}, fmt.Errorf("contract: %w", err)
	}
	if !strings.EqualFold(rawString(contract["Owner"]), input.SupplierID) || !strings.EqualFold(rawString(contract["Организация_Key"]), input.OrganizationID) {
		return ReceiptWrite{}, errors.New("contract must belong to the supplier and the organization")
	}
	if string(contract["Недействителен"]) == "true" || string(contract["РасчетыВУсловныхЕдиницах"]) == "true" {
		return ReceiptWrite{}, errors.New("contract is inactive or settles in conventional units")
	}
	currencyID := rawString(contract["ВалютаРасчетов_Key"])
	currency, err := s.activeRecord(ctx, config.Currencies.Name, currencyID, "Ref_Key,DeletionMark,Code")
	if err != nil || rawString(currency["Code"]) != receiptBaseCurrency {
		return ReceiptWrite{}, unsupported("only contracts settled in rubles are supported; foreign-currency receipts need an exchange rate this tool does not set")
	}
	for label, pair := range map[string][2]string{"warehouse": {input.WarehouseID, "Склад"}, "department": {input.DepartmentID, "Подразделение"}} {
		unit, err := s.activeRecord(ctx, config.Warehouses.Name, pair[0], "Ref_Key,DeletionMark,ТипСтруктурнойЕдиницы,Недействителен")
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("%s: %w", label, err)
		}
		if rawString(unit["ТипСтруктурнойЕдиницы"]) != pair[1] || string(unit["Недействителен"]) == "true" {
			return ReceiptWrite{}, fmt.Errorf("%s ID must identify an active %s", label, pair[1])
		}
	}
	return ReceiptWrite{
		OrganizationID: strings.ToLower(input.OrganizationID), SupplierID: strings.ToLower(input.SupplierID), ContractID: strings.ToLower(input.ContractID),
		CurrencyID: strings.ToLower(currencyID), WarehouseID: strings.ToLower(input.WarehouseID), DepartmentID: strings.ToLower(input.DepartmentID),
		Date: date, IncomingNumber: incomingNumber, IncomingDate: incomingDate, Comment: comment, VATMode: input.VATMode,
		Lines: []ReceiptPreviewLine{}, Duplicates: []ReceiptDuplicate{},
	}, nil
}

func (s Service) receiptLine(ctx context.Context, input ReceiptLineInput, number int, vatMode string, rates map[string]vatRate) (ReceiptPreviewLine, error) {
	product, err := s.activeRecord(ctx, config.Products.Name, input.ProductID, "Ref_Key,DeletionMark,IsFolder,ТипНоменклатуры,ЕдиницаИзмерения_Key")
	if err != nil {
		return ReceiptPreviewLine{}, fmt.Errorf("product: %w", err)
	}
	if string(product["IsFolder"]) != "false" || rawString(product["ТипНоменклатуры"]) != receiptStockProducts {
		return ReceiptPreviewLine{}, unsupported("product must be an active stock item; services belong to the receipt's expenses")
	}
	unitID := rawString(product["ЕдиницаИзмерения_Key"])
	if !linkedGUID(unitID) {
		return ReceiptPreviewLine{}, unsupported("product has no base unit; unit conversion is not supported")
	}
	unit := lineUnit{ID: strings.ToLower(unitID), Type: unitClassifierType}
	if input.UnitID != "" && !strings.EqualFold(input.UnitID, unitID) {
		if unit, err = s.productPackagingUnit(ctx, input.ProductID, input.UnitID); err != nil {
			return ReceiptPreviewLine{}, err
		}
	}
	if input.CharacteristicID != "" {
		characteristic, err := s.activeRecord(ctx, config.ProductCharacteristics.Name, input.CharacteristicID, "Ref_Key,DeletionMark,Owner,Недействителен")
		if err != nil {
			return ReceiptPreviewLine{}, fmt.Errorf("characteristic: %w", err)
		}
		if !strings.EqualFold(rawString(characteristic["Owner"]), input.ProductID) || string(characteristic["Недействителен"]) == "true" {
			return ReceiptPreviewLine{}, errors.New("characteristic must be active and belong to the product")
		}
	}
	return computeReceiptLine(input, number, unit, vatMode, rates)
}

// productPackagingUnit accepts a product's own unit only when 1C stores a positive conversion factor for it.
func (s Service) productPackagingUnit(ctx context.Context, productID, unitID string) (lineUnit, error) {
	row, err := s.activeRecord(ctx, config.ProductUnits.Name, unitID, "Ref_Key,DeletionMark,Owner,Owner_Type,Коэффициент")
	if err != nil {
		return lineUnit{}, fmt.Errorf("unit: %w", err)
	}
	if !strings.EqualFold(rawString(row["Owner"]), productID) || rawString(row["Owner_Type"]) != productOwnerType {
		return lineUnit{}, errors.New("unit must be the product's base unit or one of its own units from list product-units")
	}
	factor, ok := new(big.Rat).SetString(string(row["Коэффициент"]))
	if !ok || factor.Sign() <= 0 {
		return lineUnit{}, errors.New("unit has no confirmed conversion factor to the base unit")
	}
	return lineUnit{ID: strings.ToLower(unitID), Type: productUnitType, Factor: Decimal(string(row["Коэффициент"]))}, nil
}

func computeReceiptLine(input ReceiptLineInput, number int, unit lineUnit, vatMode string, rates map[string]vatRate) (ReceiptPreviewLine, error) {
	quantity, err := exactDecimal(input.Quantity, maxQuantityScale, false)
	if err != nil {
		return ReceiptPreviewLine{}, fmt.Errorf("quantity must be positive with at most %d decimals", maxQuantityScale)
	}
	price, err := exactDecimal(input.Price, maxPriceScale, true)
	if err != nil {
		return ReceiptPreviewLine{}, fmt.Errorf("price must be nonnegative with at most %d decimals", maxPriceScale)
	}
	rate, ok := rates[input.VATRate]
	if !ok {
		return ReceiptPreviewLine{}, errors.New("VAT rate must be one of the 1C rate codes, for example НДС22, НДС5 or БезНДС")
	}
	computedAmount := roundHalfAway(new(big.Rat).Mul(quantity, price), receiptAmountScale)
	amount := computedAmount
	if input.Amount != "" {
		if amount, err = exactDecimal(input.Amount, receiptAmountScale, true); err != nil {
			return ReceiptPreviewLine{}, fmt.Errorf("amount must be nonnegative with at most %d decimals", receiptAmountScale)
		}
		// The price is rounded to kopecks, so the document's amount may differ from quantity × price by up to half a kopeck per unit.
		tolerance := new(big.Rat).Add(new(big.Rat).Mul(quantity, big.NewRat(1, 200)), big.NewRat(1, 200))
		if new(big.Rat).Abs(new(big.Rat).Sub(amount, new(big.Rat).Mul(quantity, price))).Cmp(tolerance) > 0 {
			return ReceiptPreviewLine{}, fmt.Errorf("amount %s does not match quantity × price %s within price rounding; check the price", ratDecimal(amount, receiptAmountScale), ratDecimal(computedAmount, receiptAmountScale))
		}
	}
	computedVAT := new(big.Rat)
	if !rate.NotTaxable {
		if vatMode == receiptVATIncluded {
			computedVAT = new(big.Rat).Quo(new(big.Rat).Mul(amount, rate.Percent), new(big.Rat).Add(rate.Percent, big.NewRat(100, 1)))
		} else {
			computedVAT = new(big.Rat).Quo(new(big.Rat).Mul(amount, rate.Percent), big.NewRat(100, 1))
		}
		computedVAT = roundHalfAway(computedVAT, receiptAmountScale)
	}
	vat := computedVAT
	if input.VAT != "" {
		if vat, err = exactDecimal(input.VAT, receiptAmountScale, true); err != nil {
			return ReceiptPreviewLine{}, fmt.Errorf("VAT must be nonnegative with at most %d decimals", receiptAmountScale)
		}
		if rate.NotTaxable && vat.Sign() != 0 || new(big.Rat).Abs(new(big.Rat).Sub(vat, computedVAT)).Cmp(big.NewRat(1, 100)) > 0 {
			return ReceiptPreviewLine{}, fmt.Errorf("VAT %s differs from the %s rate's %s by more than one kopeck", ratDecimal(vat, receiptAmountScale), rate.Name, ratDecimal(computedVAT, receiptAmountScale))
		}
	}
	total := new(big.Rat).Set(amount)
	if vatMode == receiptVATExcluded {
		total.Add(total, vat)
	}
	characteristic := strings.ToLower(input.CharacteristicID)
	if characteristic == emptyGUID {
		characteristic = ""
	}
	line := ReceiptPreviewLine{
		LineNumber: number, ProductID: strings.ToLower(input.ProductID), CharacteristicID: characteristic, UnitID: unit.ID, UnitType: unit.Type, UnitFactor: unit.Factor,
		Quantity: ratDecimal(quantity, maxQuantityScale), Price: ratDecimal(price, maxPriceScale), VATRate: rate.Name, VATRateID: rate.ID,
		Amount: ratDecimal(amount, receiptAmountScale), VAT: ratDecimal(vat, receiptAmountScale), Total: ratDecimal(total, receiptAmountScale),
	}
	if factor, ok := new(big.Rat).SetString(string(unit.Factor)); ok {
		line.BaseQuantity = Decimal(new(big.Rat).Mul(quantity, factor).FloatString(maxQuantityScale + 3))
		line.BaseQuantity = Decimal(strings.TrimRight(strings.TrimRight(string(line.BaseQuantity), "0"), "."))
	}
	if amount.Cmp(computedAmount) != 0 {
		line.ComputedAmount = ratDecimal(computedAmount, receiptAmountScale)
	}
	if vat.Cmp(computedVAT) != 0 {
		line.ComputedVAT = ratDecimal(computedVAT, receiptAmountScale)
	}
	return line, nil
}

// existingReceiptLine keeps the amounts 1C stored and only names the line's rate; it is recomputed only when changed.
func existingReceiptLine(line editableReceiptLine, vatMode string, rates map[string]vatRate) (ReceiptPreviewLine, error) {
	rateName := ""
	for name, rate := range rates {
		if strings.EqualFold(rate.ID, line.VATRateID) {
			rateName = name
		}
	}
	if rateName == "" {
		return ReceiptPreviewLine{}, errors.New("line uses a VAT rate without a rate code")
	}
	if line.UnitType != unitClassifierType && line.UnitType != productUnitType {
		return ReceiptPreviewLine{}, errors.New("line uses an unsupported unit type")
	}
	return ReceiptPreviewLine{
		LineNumber: line.LineNumber, ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, UnitID: line.UnitID, UnitType: line.UnitType,
		Quantity: line.Quantity, Price: line.Price, VATRate: rateName, VATRateID: line.VATRateID,
		Amount: line.Amount, VAT: line.VAT, Total: line.Total, Preserved: true,
	}, nil
}

func finishReceiptTotals(result *ReceiptWrite) {
	total, vat := &stockSum{}, &stockSum{}
	taxed := false
	for _, line := range result.Lines {
		_ = total.Add(line.Total)
		_ = vat.Add(line.VAT)
		if line.VATRate != "БезНДС" {
			taxed = true
		}
	}
	result.Amount, result.VAT = total.Decimal(), vat.Decimal()
	result.Taxation = receiptNotTaxed
	if taxed {
		result.Taxation = receiptTaxed
	}
	result.Impact = []string{
		"saved unposted: no stock, cost or supplier-debt movements until it is posted explicitly",
		fmt.Sprintf("posting would add %d lines to warehouse %s and a supplier debt of %s on contract %s", len(result.Lines), result.WarehouseID, result.Amount, result.ContractID),
	}
}

func (s Service) receiptDuplicates(ctx context.Context, draft ReceiptWrite, exceptID string) ([]ReceiptDuplicate, error) {
	duplicates := []ReceiptDuplicate{}
	if draft.IncomingNumber == "" {
		return duplicates, nil
	}
	plan := config.DocumentResource{Name: receiptResourceName, DateField: "Date", DeletedField: "DeletionMark", Fields: []config.FieldBinding{
		{Output: "id", Source: "Ref_Key"}, {Output: "number", Source: "Number"}, {Output: "date", Source: "Date"}, {Output: "posted", Source: "Posted"},
		{Output: "organization_id", Source: "Организация_Key"}, {Output: "supplier_id", Source: "Контрагент_Key"},
		{Output: "incoming_number", Source: "НомерВходящегоДокумента"}, {Output: "incoming_date", Source: "ДатаВходящегоДокумента"},
	}}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("could not check for duplicate receipts: %w", err)
	}
	for _, row := range rows {
		candidate, err := bindFields[struct {
			ReceiptDuplicate
			OrganizationID string `json:"organization_id"`
			SupplierID     string `json:"supplier_id"`
		}](row, plan.Fields)
		if err != nil {
			return nil, errors.New("invalid OData receipt fields")
		}
		if strings.EqualFold(candidate.ID, exceptID) || !strings.EqualFold(candidate.OrganizationID, draft.OrganizationID) || !strings.EqualFold(candidate.SupplierID, draft.SupplierID) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(candidate.IncomingNumber), draft.IncomingNumber) {
			continue
		}
		if draft.IncomingDate != "" && !strings.HasPrefix(candidate.IncomingDate, draft.IncomingDate[:10]) {
			continue
		}
		duplicates = append(duplicates, candidate.ReceiptDuplicate)
	}
	return duplicates, nil
}

func (s Service) vatRates(ctx context.Context) (map[string]vatRate, error) {
	rows, err := s.scanRows(ctx, "Catalog_СтавкиНДС", "Ref_Key,DeletionMark,Ставка,НеОблагается,Расчетная,ПеречислениеСтавкаНДС", "", 500)
	if err != nil {
		return nil, err
	}
	rates := make(map[string]vatRate)
	ambiguous := make(map[string]bool)
	for _, row := range rows {
		name := rawString(row["ПеречислениеСтавкаНДС"])
		if name == "" || string(row["DeletionMark"]) == "true" || string(row["Расчетная"]) == "true" {
			continue
		}
		percent, ok := new(big.Rat).SetString(string(row["Ставка"]))
		if !ok {
			return nil, errors.New("invalid OData VAT rate")
		}
		if _, exists := rates[name]; exists {
			ambiguous[name] = true
		}
		rates[name] = vatRate{ID: strings.ToLower(rawString(row["Ref_Key"])), Name: name, Percent: percent, NotTaxable: string(row["НеОблагается"]) == "true"}
	}
	for name := range ambiguous {
		delete(rates, name)
	}
	return rates, nil
}

func (s Service) receiptBusinessOperation(ctx context.Context) (string, error) {
	return s.predefinedOperation(ctx, receiptOperation)
}

func (s Service) activeRecord(ctx context.Context, resource, id, fields string) (map[string]json.RawMessage, error) {
	if !linkedGUID(id) {
		return nil, errors.New("ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, resource+"(guid'"+strings.ToLower(id)+"')", url.Values{"$format": {"json"}, "$select": {fields}}, 1<<20)
	if err != nil {
		return nil, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return nil, errors.New("invalid OData response")
	}
	if string(row["DeletionMark"]) != "false" {
		return nil, errors.New("record is marked for deletion")
	}
	return row, nil
}

type editableReceiptLine struct {
	LineNumber       int
	ProductID        string
	CharacteristicID string
	UnitID           string
	UnitType         string
	Quantity         Decimal
	Price            Decimal
	VATRateID        string
	Amount           Decimal
	VAT              Decimal
	Total            Decimal
	Raw              map[string]any
}

type editableReceipt struct {
	ID             string
	DataVersion    string
	Posted         bool
	OrganizationID string
	SupplierID     string
	ContractID     string
	WarehouseID    string
	DepartmentID   string
	Date           string
	IncomingNumber string
	IncomingDate   string
	Comment        string
	IncludesVAT    bool
	Taxation       string
	Amount         Decimal
	VAT            Decimal
	Lines          []editableReceiptLine
}

func (r editableReceipt) draft() ReceiptDraft {
	mode := receiptVATExcluded
	if r.IncludesVAT {
		mode = receiptVATIncluded
	}
	return ReceiptDraft{OrganizationID: r.OrganizationID, SupplierID: r.SupplierID, ContractID: r.ContractID, WarehouseID: r.WarehouseID, DepartmentID: r.DepartmentID, Date: r.Date, IncomingNumber: r.IncomingNumber, IncomingDate: r.IncomingDate, Comment: r.Comment, VATMode: mode}
}

func (s Service) readEditableReceipt(ctx context.Context, id string) (editableReceipt, error) {
	if !linkedGUID(id) {
		return editableReceipt{}, errors.New("receipt ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, receiptResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return editableReceipt{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return editableReceipt{}, errors.New("invalid OData receipt response")
	}
	if rawString(row["ВидОперации"]) != receiptOperation || string(row["DeletionMark"]) != "false" {
		return editableReceipt{}, errors.New("ID must identify an active goods receipt from a supplier")
	}
	if rawString(row["DataVersion"]) == "" {
		return editableReceipt{}, errors.New("receipt has no data version for a safe update")
	}
	receipt := editableReceipt{
		ID: strings.ToLower(id), DataVersion: rawString(row["DataVersion"]), Posted: string(row["Posted"]) == "true",
		OrganizationID: rawString(row["Организация_Key"]), SupplierID: rawString(row["Контрагент_Key"]), ContractID: rawString(row["Договор_Key"]),
		WarehouseID: rawString(row["СтруктурнаяЕдиница_Key"]), DepartmentID: rawString(row["Подразделение_Key"]),
		Date: rawString(row["Date"]), IncomingNumber: strings.TrimSpace(rawString(row["НомерВходящегоДокумента"])), Comment: rawString(row["Комментарий"]),
		IncludesVAT: string(row["СуммаВключаетНДС"]) == "true", Taxation: rawString(row["НалогообложениеНДС"]), Amount: rawDecimal(row["СуммаДокумента"]),
	}
	if incoming := rawString(row["ДатаВходящегоДокумента"]); !strings.HasPrefix(incoming, "0001-01-01") && len(incoming) >= 10 {
		receipt.IncomingDate = incoming[:10]
	}
	var lines []map[string]any
	if raw, ok := row["Запасы"]; ok {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&lines); err != nil {
			return editableReceipt{}, errors.New("invalid OData receipt lines")
		}
	}
	vat := &stockSum{}
	for _, line := range lines {
		for key := range line {
			if strings.Contains(key, "@") {
				delete(line, key)
			}
		}
		number, _ := line["LineNumber"].(string)
		var lineNumber int
		if _, err := fmt.Sscan(number, &lineNumber); err != nil || lineNumber < 1 {
			return editableReceipt{}, errors.New("invalid OData receipt line number")
		}
		parsed := editableReceiptLine{
			LineNumber: lineNumber, ProductID: anyString(line["Номенклатура_Key"]), CharacteristicID: anyString(line["Характеристика_Key"]),
			UnitID: anyString(line["ЕдиницаИзмерения"]), UnitType: anyString(line["ЕдиницаИзмерения_Type"]),
			Quantity: anyDecimal(line["Количество"]), Price: anyDecimal(line["Цена"]), VATRateID: anyString(line["СтавкаНДС_Key"]),
			Amount: anyDecimal(line["Сумма"]), VAT: anyDecimal(line["СуммаНДС"]), Total: anyDecimal(line["Всего"]), Raw: line,
		}
		if parsed.CharacteristicID == emptyGUID {
			parsed.CharacteristicID = ""
		}
		_ = vat.Add(parsed.VAT)
		receipt.Lines = append(receipt.Lines, parsed)
	}
	receipt.VAT = vat.Decimal()
	return receipt, nil
}

func verifyReceiptSaved(saved editableReceipt, want ReceiptWrite) error {
	if saved.Posted {
		return errors.New("1C saved the receipt as posted")
	}
	if !strings.EqualFold(saved.SupplierID, want.SupplierID) || !strings.EqualFold(saved.ContractID, want.ContractID) || !strings.EqualFold(saved.WarehouseID, want.WarehouseID) || !strings.EqualFold(saved.DepartmentID, want.DepartmentID) || saved.IncomingNumber != want.IncomingNumber {
		return errors.New("header fields do not match")
	}
	if len(saved.Lines) != len(want.Lines) {
		return errors.New("line count does not match")
	}
	for index, line := range saved.Lines {
		expected := want.Lines[index]
		if !strings.EqualFold(line.ProductID, expected.ProductID) || !decimalEqual(line.Quantity, expected.Quantity) || !decimalEqual(line.Price, expected.Price) || !decimalEqual(line.Total, expected.Total) {
			return fmt.Errorf("line %d does not match", line.LineNumber)
		}
	}
	if !decimalEqual(saved.Amount, want.Amount) {
		return errors.New("document total " + string(saved.Amount) + " differs from " + string(want.Amount))
	}
	return nil
}

func receiptHeaderFields(result ReceiptWrite) map[string]any {
	fields := map[string]any{
		"Date": result.Date, "Организация_Key": result.OrganizationID, "Контрагент_Key": result.SupplierID, "Договор_Key": result.ContractID,
		"ВалютаДокумента_Key": result.CurrencyID, "СтруктурнаяЕдиница_Key": result.WarehouseID, "Подразделение_Key": result.DepartmentID,
		"НомерВходящегоДокумента": result.IncomingNumber, "Комментарий": result.Comment, "СуммаВключаетНДС": result.VATMode == receiptVATIncluded,
		"НалогообложениеНДС": result.Taxation, "СуммаДокумента": json.Number(result.Amount),
	}
	fields["ДатаВходящегоДокумента"] = "0001-01-01T00:00:00"
	if result.IncomingDate != "" {
		fields["ДатаВходящегоДокумента"] = result.IncomingDate + "T00:00:00"
	}
	return fields
}

// receiptLineFields starts from the stored row, so fields this tool does not manage survive an amendment.
func receiptLineFields(line ReceiptPreviewLine, warehouseID string, original map[string]any) map[string]any {
	row := make(map[string]any, len(original)+12)
	for key, value := range original {
		row[key] = value
	}
	if line.Preserved {
		row["СтруктурнаяЕдиница_Key"] = warehouseID
		return row
	}
	characteristic := line.CharacteristicID
	if characteristic == "" {
		characteristic = emptyGUID
	}
	row["LineNumber"] = fmt.Sprint(line.LineNumber)
	row["Номенклатура_Key"] = line.ProductID
	row["Характеристика_Key"] = characteristic
	row["ЕдиницаИзмерения"] = line.UnitID
	row["ЕдиницаИзмерения_Type"] = line.UnitType
	row["Количество"] = json.Number(line.Quantity)
	row["Цена"] = json.Number(line.Price)
	row["Сумма"] = json.Number(line.Amount)
	row["СтавкаНДС_Key"] = line.VATRateID
	row["СуммаНДС"] = json.Number(line.VAT)
	row["Всего"] = json.Number(line.Total)
	row["СтруктурнаяЕдиница_Key"] = warehouseID
	return row
}

func receiptResource(id string) string {
	return receiptResourceName + "(guid'" + strings.ToLower(id) + "')"
}

func receiptDate(value string, dateOnly bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if dateOnly {
			return "", nil
		}
		return "", errors.New("date is required")
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil && parsed.Format("2006-01-02") == value {
		if dateOnly {
			return value, nil
		}
		return value + "T00:00:00", nil
	}
	if dateOnly {
		return "", errors.New("invalid date")
	}
	if parsed, err := time.Parse("2006-01-02T15:04:05", value); err == nil && parsed.Format("2006-01-02T15:04:05") == value {
		return value, nil
	}
	return "", errors.New("invalid date")
}

// exactDecimal parses a plain decimal string with a bounded scale, rejecting exponents and rounding.
func exactDecimal(value string, scale int, allowZero bool) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	whole, fraction, _ := strings.Cut(value, ".")
	if whole == "" || len(fraction) > scale || strings.Trim(whole, "0123456789") != "" || strings.Trim(fraction, "0123456789") != "" || len(whole) > 12 {
		return nil, errors.New("invalid decimal")
	}
	number, ok := new(big.Rat).SetString(value)
	if !ok || number.Sign() < 0 || !allowZero && number.Sign() == 0 {
		return nil, errors.New("invalid decimal")
	}
	return number, nil
}

func roundHalfAway(value *big.Rat, scale int) *big.Rat {
	factor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(value, new(big.Rat).SetInt(factor))
	quotient, remainder := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(remainder), big.NewInt(2)).Cmp(scaled.Denom()) >= 0 {
		if scaled.Sign() < 0 {
			quotient.Sub(quotient, big.NewInt(1))
		} else {
			quotient.Add(quotient, big.NewInt(1))
		}
	}
	return new(big.Rat).SetFrac(quotient, factor)
}

func ratDecimal(value *big.Rat, scale int) Decimal {
	text := value.FloatString(scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return Decimal(text)
}

func rawString(value json.RawMessage) string {
	var text string
	_ = json.Unmarshal(value, &text)
	return text
}

func rawDecimal(value json.RawMessage) Decimal {
	var number Decimal
	_ = json.Unmarshal(value, &number)
	return number
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func anyDecimal(value any) Decimal {
	if number, ok := value.(json.Number); ok {
		return Decimal(number)
	}
	return ""
}
