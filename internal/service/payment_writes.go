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
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	paymentLinesField   = "РасшифровкаПлатежа"
	receiptDocumentType = "StandardODATA.Document_ПриходнаяНакладная"
	maxPaymentLines     = 100
)

// editablePaymentKinds are the outgoing supplier payments whose allocation can be amended.
var editablePaymentKinds = map[string]bool{"bank-out": true, "cash-out": true}

type PaymentAllocationInput struct {
	ContractID string `json:"contract_id"`
	DocumentID string `json:"document_id,omitempty"`
	Amount     string `json:"amount"`
	VATRate    string `json:"vat_rate"`
}

type PaymentAllocationChange struct {
	LineNumber int     `json:"line_number"`
	ContractID *string `json:"contract_id,omitempty"`
	DocumentID *string `json:"document_id,omitempty"`
	Amount     *string `json:"amount,omitempty"`
	VATRate    *string `json:"vat_rate,omitempty"`
}

type PaymentAmendment struct {
	Date                *string                   `json:"date,omitempty"`
	Comment             *string                   `json:"comment,omitempty"`
	Amount              *string                   `json:"amount,omitempty"`
	ChangeLines         []PaymentAllocationChange `json:"change_lines,omitempty"`
	AddLines            []PaymentAllocationInput  `json:"add_lines,omitempty"`
	RemoveLines         []int                     `json:"remove_lines,omitempty"`
	Repost              bool                      `json:"repost,omitempty"`
	ExpectedDataVersion string                    `json:"expected_data_version,omitempty"`
	Apply               bool                      `json:"apply,omitempty"`
}

type PaymentAllocationPreview struct {
	LineNumber   int     `json:"line_number"`
	ContractID   string  `json:"contract_id"`
	DocumentID   string  `json:"document_id,omitempty"`
	DocumentType string  `json:"document_type,omitempty"`
	Advance      bool    `json:"advance"`
	Amount       Decimal `json:"amount"`
	VATRate      string  `json:"vat_rate,omitempty"`
	VATRateID    string  `json:"vat_rate_id,omitempty"`
	VAT          Decimal `json:"vat"`
	Preserved    bool    `json:"preserved,omitempty"`
	// source is the stored line this preview came from, or -1 for a new line.
	source int
}

type PaymentTotals struct {
	Amount Decimal `json:"amount"`
	Lines  int     `json:"lines"`
}

// PaymentWrite is both the preview and the result of a supplier payment amendment.
type PaymentWrite struct {
	Applied        bool                       `json:"applied"`
	Kind           string                     `json:"kind"`
	ID             string                     `json:"id"`
	Number         string                     `json:"number"`
	DataVersion    string                     `json:"data_version"`
	Posted         bool                       `json:"posted"`
	OrganizationID string                     `json:"organization_id"`
	SupplierID     string                     `json:"supplier_id"`
	Date           string                     `json:"date"`
	Comment        string                     `json:"comment"`
	Amount         Decimal                    `json:"amount"`
	Allocations    []PaymentAllocationPreview `json:"allocations"`
	Before         PaymentTotals              `json:"before"`
	Changes        []string                   `json:"changes"`
	Impact         []string                   `json:"impact"`
}

type editablePayment struct {
	Kind           string
	ID             string
	Number         string
	DataVersion    string
	Posted         bool
	OrganizationID string
	SupplierID     string
	CurrencyID     string
	Date           string
	Comment        string
	Amount         Decimal
	Lines          []map[string]any
}

// AmendSupplierPayment previews and, with Apply, changes the date, comment, amount, or allocation of an outgoing supplier payment.
// A posted payment needs Repost: it is unposted, changed and posted again.
func (s Service) AmendSupplierPayment(ctx context.Context, kind, id string, change PaymentAmendment) (PaymentWrite, error) {
	current, err := s.readEditablePayment(ctx, kind, id)
	if err != nil {
		return PaymentWrite{}, err
	}
	if current.Posted && !change.Repost {
		return PaymentWrite{}, errors.New("payment is posted; pass repost to unpost, amend and post it again")
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != current.DataVersion {
		return PaymentWrite{}, errors.New("payment changed since the preview; preview again")
	}
	currency, err := s.activeRecord(ctx, config.Currencies.Name, current.CurrencyID, "Ref_Key,DeletionMark,Code")
	if err != nil || rawString(currency["Code"]) != receiptBaseCurrency {
		return PaymentWrite{}, unsupported("only ruble payments can be amended; other currencies need exchange rates this tool does not set")
	}
	result := PaymentWrite{
		Kind: kind, ID: current.ID, Number: current.Number, DataVersion: current.DataVersion, Posted: current.Posted,
		OrganizationID: current.OrganizationID, SupplierID: current.SupplierID, Date: current.Date, Comment: current.Comment, Amount: current.Amount,
		Before: PaymentTotals{Amount: current.Amount, Lines: len(current.Lines)}, Changes: []string{},
	}
	if change.Date != nil {
		date, err := receiptDate(*change.Date, false)
		if err != nil {
			return PaymentWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
		}
		if date != current.Date {
			result.Date = date
			result.Changes = append(result.Changes, "date")
		}
	}
	if change.Comment != nil && *change.Comment != current.Comment {
		comment := strings.TrimSpace(*change.Comment)
		if utf8.RuneCountInString(comment) > 1024 || strings.IndexFunc(comment, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) >= 0 {
			return PaymentWrite{}, errors.New("comment must be at most 1024 characters without control characters other than line breaks and tabs")
		}
		result.Comment = comment
		result.Changes = append(result.Changes, "comment")
	}
	if change.Amount != nil {
		amount, err := exactDecimal(*change.Amount, receiptAmountScale, false)
		if err != nil {
			return PaymentWrite{}, errors.New("amount must be positive with at most 2 decimals")
		}
		if !decimalEqual(ratDecimal(amount, receiptAmountScale), current.Amount) {
			result.Amount = ratDecimal(amount, receiptAmountScale)
			result.Changes = append(result.Changes, "amount")
		}
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return PaymentWrite{}, err
	}
	rateNames := make(map[string]string, len(rates))
	for name, rate := range rates {
		rateNames[rate.ID] = name
	}
	byNumber := make(map[int]int, len(current.Lines))
	for index, line := range current.Lines {
		number := 0
		if _, err := fmt.Sscan(anyString(line["LineNumber"]), &number); err != nil || number < 1 {
			return PaymentWrite{}, errors.New("invalid OData payment line number")
		}
		byNumber[number] = index
		rateID := strings.ToLower(anyString(line["СтавкаНДС_Key"]))
		preview := PaymentAllocationPreview{
			LineNumber: number, ContractID: strings.ToLower(anyString(line["Договор_Key"])), Advance: line["ПризнакАванса"] == true,
			Amount: anyDecimal(line["СуммаПлатежа"]), VATRateID: rateID, VATRate: rateNames[rateID], VAT: anyDecimal(line["СуммаНДС"]), Preserved: true, source: index,
		}
		if documentType := anyString(line["Документ_Type"]); documentType != "StandardODATA.Undefined" && linkedGUID(anyString(line["Документ"])) {
			preview.DocumentID, preview.DocumentType = strings.ToLower(anyString(line["Документ"])), documentType
		}
		result.Allocations = append(result.Allocations, preview)
	}
	removed := make(map[int]bool)
	for _, number := range change.RemoveLines {
		if _, ok := byNumber[number]; !ok || removed[number] {
			return PaymentWrite{}, fmt.Errorf("line %d does not exist or is removed twice", number)
		}
		removed[number] = true
		result.Changes = append(result.Changes, fmt.Sprintf("remove line %d", number))
	}
	changed := make(map[int]bool)
	for _, lineChange := range change.ChangeLines {
		index, ok := byNumber[lineChange.LineNumber]
		if !ok || changed[lineChange.LineNumber] || removed[lineChange.LineNumber] {
			return PaymentWrite{}, fmt.Errorf("line %d does not exist, is removed, or is changed twice", lineChange.LineNumber)
		}
		changed[lineChange.LineNumber] = true
		line := &result.Allocations[index]
		input := PaymentAllocationInput{ContractID: line.ContractID, DocumentID: line.DocumentID, Amount: string(line.Amount), VATRate: line.VATRate}
		if lineChange.ContractID != nil {
			input.ContractID = *lineChange.ContractID
		}
		if lineChange.DocumentID != nil {
			if *lineChange.DocumentID == "" && line.DocumentID != "" {
				return PaymentWrite{}, fmt.Errorf("line %d: to make it an advance, remove the line and add one without a document", line.LineNumber)
			}
			input.DocumentID = *lineChange.DocumentID
		}
		if lineChange.Amount != nil {
			input.Amount = *lineChange.Amount
		}
		if lineChange.VATRate != nil {
			input.VATRate = *lineChange.VATRate
		}
		if input.VATRate == "" {
			return PaymentWrite{}, fmt.Errorf("line %d uses a VAT rate without a rate code; pass a VAT rate", line.LineNumber)
		}
		updated, err := s.paymentAllocation(ctx, input, current, rates, *line)
		if err != nil {
			return PaymentWrite{}, fmt.Errorf("line %d: %w", line.LineNumber, err)
		}
		updated.LineNumber, updated.source = line.LineNumber, line.source
		*line = updated
		result.Changes = append(result.Changes, fmt.Sprintf("line %d", line.LineNumber))
	}
	kept := make([]PaymentAllocationPreview, 0, len(result.Allocations)+len(change.AddLines))
	for _, line := range result.Allocations {
		if !removed[line.LineNumber] {
			kept = append(kept, line)
		}
	}
	for _, input := range change.AddLines {
		line, err := s.paymentAllocation(ctx, input, current, rates, PaymentAllocationPreview{})
		if err != nil {
			return PaymentWrite{}, fmt.Errorf("new line: %w", err)
		}
		line.source = -1
		kept = append(kept, line)
		result.Changes = append(result.Changes, "new line")
	}
	if len(kept) == 0 || len(kept) > maxPaymentLines {
		return PaymentWrite{}, fmt.Errorf("a payment needs 1 to %d allocation lines", maxPaymentLines)
	}
	total := &stockSum{}
	for index := range kept {
		kept[index].LineNumber = index + 1
		_ = total.Add(kept[index].Amount)
	}
	result.Allocations = kept
	if !decimalEqual(total.Decimal(), result.Amount) {
		return PaymentWrite{}, fmt.Errorf("allocation lines add up to %s but the payment amount is %s; change the lines or pass the new amount", total.Decimal(), result.Amount)
	}
	result.Impact = []string{"changes how this payment settles supplier debts: receipts it no longer covers become unpaid again, and lines without a document become advances"}
	if !decimalEqual(result.Amount, current.Amount) {
		result.Impact = append(result.Impact, fmt.Sprintf("changes the money that left the account from %s to %s; it must match the bank statement or cash book", current.Amount, result.Amount))
	}
	if current.Posted {
		result.Impact = append(result.Impact, "unposts the payment, saves the changes, then posts it again; if a step fails the payment is left unposted, so read it before retrying")
	} else {
		result.Impact = append(result.Impact, "saved unposted: no money or settlement movements until it is posted")
	}
	if !change.Apply || len(result.Changes) == 0 {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PaymentWrite{}, errors.New("OData client does not support writes")
	}
	body, err := json.Marshal(paymentFields(result, current))
	if err != nil {
		return PaymentWrite{}, errors.New("could not encode payment")
	}
	resource := paymentResource(current)
	version := current.DataVersion
	var actor documentActor
	if current.Posted {
		if actor, ok = s.OData.(documentActor); !ok {
			return PaymentWrite{}, errors.New("OData client does not support document actions")
		}
		if _, err := actor.Action(ctx, resource, "Unpost"); err != nil {
			return PaymentWrite{}, fmt.Errorf("1C rejected unpost; nothing was changed or the state is uncertain, so read the payment before retrying: %w", err)
		}
		unposted, err := s.readEditablePayment(ctx, kind, current.ID)
		if err != nil {
			return PaymentWrite{}, fmt.Errorf("payment was unposted but could not be read back; no changes were saved: %w", err)
		}
		if unposted.Posted || !decimalEqual(unposted.Amount, current.Amount) || len(unposted.Lines) != len(current.Lines) {
			return PaymentWrite{}, errors.New("payment did not unpost cleanly; no changes were saved, check it in 1C")
		}
		version = unposted.DataVersion
	}
	if _, err := writer.Write(ctx, http.MethodPatch, resource, body, version); err != nil {
		if current.Posted {
			return PaymentWrite{}, fmt.Errorf("payment was unposted but the changes were not saved; it is left unposted: %w", err)
		}
		return PaymentWrite{}, err
	}
	saved, err := s.readEditablePayment(ctx, kind, current.ID)
	if err != nil {
		return PaymentWrite{}, fmt.Errorf("payment was updated but could not be read back: %w", err)
	}
	if err := verifyPaymentSaved(saved, result); err != nil {
		return PaymentWrite{}, fmt.Errorf("payment was updated but differs from the request; it is left unposted: %w", err)
	}
	if current.Posted {
		if _, err := actor.Action(ctx, resource, "Post"); err != nil {
			return PaymentWrite{}, fmt.Errorf("changes were saved but 1C rejected posting; the payment is left unposted: %w", err)
		}
		if saved, err = s.readEditablePayment(ctx, kind, current.ID); err != nil {
			return PaymentWrite{}, fmt.Errorf("changes were saved and posting was sent, but the payment could not be read back: %w", err)
		}
		if !saved.Posted {
			return PaymentWrite{}, errors.New("changes were saved but posting did not take effect; the payment is left unposted")
		}
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	return result, nil
}

// paymentAllocation checks one allocation line against the payment's supplier and organization and computes its VAT.
// keep is the stored line, so an existing line may keep a contract marked inactive since and its stored document link.
func (s Service) paymentAllocation(ctx context.Context, input PaymentAllocationInput, payment editablePayment, rates map[string]vatRate, keep PaymentAllocationPreview) (PaymentAllocationPreview, error) {
	if !linkedGUID(input.ContractID) {
		return PaymentAllocationPreview{}, errors.New("contract ID must be a nonzero GUID")
	}
	contractID := strings.ToLower(input.ContractID)
	contract, err := s.activeRecord(ctx, "Catalog_ДоговорыКонтрагентов", contractID, "Ref_Key,DeletionMark,Owner,Организация_Key,ВалютаРасчетов_Key,Недействителен,РасчетыВУсловныхЕдиницах")
	if err != nil {
		return PaymentAllocationPreview{}, fmt.Errorf("contract: %w", err)
	}
	if !strings.EqualFold(rawString(contract["Owner"]), payment.SupplierID) || !strings.EqualFold(rawString(contract["Организация_Key"]), payment.OrganizationID) {
		return PaymentAllocationPreview{}, errors.New("contract must belong to the payment's supplier and organization")
	}
	if string(contract["Недействителен"]) == "true" && contractID != keep.ContractID || string(contract["РасчетыВУсловныхЕдиницах"]) == "true" {
		return PaymentAllocationPreview{}, errors.New("contract is inactive or settles in conventional units")
	}
	if !strings.EqualFold(rawString(contract["ВалютаРасчетов_Key"]), payment.CurrencyID) {
		return PaymentAllocationPreview{}, unsupported("contract settles in another currency than the payment")
	}
	amount, err := exactDecimal(input.Amount, receiptAmountScale, false)
	if err != nil {
		return PaymentAllocationPreview{}, errors.New("amount must be positive with at most 2 decimals")
	}
	rate, ok := rates[input.VATRate]
	if !ok {
		return PaymentAllocationPreview{}, errors.New("VAT rate must be one of the 1C rate codes, for example НДС22, НДС5 or БезНДС")
	}
	vat := new(big.Rat)
	if !rate.NotTaxable {
		// Payments always include VAT.
		vat = roundHalfAway(new(big.Rat).Quo(new(big.Rat).Mul(amount, rate.Percent), new(big.Rat).Add(rate.Percent, big.NewRat(100, 1))), receiptAmountScale)
	}
	line := PaymentAllocationPreview{
		ContractID: contractID, Advance: true, Amount: ratDecimal(amount, receiptAmountScale),
		VATRate: rate.Name, VATRateID: rate.ID, VAT: ratDecimal(vat, receiptAmountScale),
	}
	if input.DocumentID == "" {
		return line, nil
	}
	if !linkedGUID(input.DocumentID) {
		return PaymentAllocationPreview{}, errors.New("document ID must be a nonzero GUID of a supplier goods receipt")
	}
	line.DocumentID, line.DocumentType, line.Advance = strings.ToLower(input.DocumentID), receiptDocumentType, false
	if line.DocumentID == keep.DocumentID {
		// The stored link is kept as is; only newly linked documents must be receipts of this supplier and contract.
		line.DocumentType, line.Advance = keep.DocumentType, keep.Advance
		return line, nil
	}
	receipt, err := s.readEditableReceipt(ctx, input.DocumentID)
	if err != nil {
		return PaymentAllocationPreview{}, fmt.Errorf("document: %w", err)
	}
	if !strings.EqualFold(receipt.SupplierID, payment.SupplierID) || !strings.EqualFold(receipt.ContractID, contractID) {
		return PaymentAllocationPreview{}, errors.New("receipt must belong to the same supplier and contract")
	}
	return line, nil
}

func (s Service) readEditablePayment(ctx context.Context, kind, id string) (editablePayment, error) {
	plan, ok := config.MoneyDocuments[kind]
	if !ok || !editablePaymentKinds[kind] {
		return editablePayment{}, errors.New("kind must be bank-out or cash-out")
	}
	if !linkedGUID(id) {
		return editablePayment{}, errors.New("payment ID must be a nonzero GUID")
	}
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	data, err := s.OData.Get(ctx, resource, url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return editablePayment{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return editablePayment{}, errors.New("invalid OData payment response")
	}
	if rawString(row["ВидОперации"]) != supplierPaymentAction || string(row["DeletionMark"]) != "false" {
		return editablePayment{}, errors.New("ID must identify an active payment to a supplier")
	}
	if rawString(row["DataVersion"]) == "" {
		return editablePayment{}, errors.New("payment has no data version for a safe update")
	}
	payment := editablePayment{
		Kind: kind, ID: strings.ToLower(id), Number: rawString(row["Number"]), DataVersion: rawString(row["DataVersion"]), Posted: string(row["Posted"]) == "true",
		OrganizationID: strings.ToLower(rawString(row["Организация_Key"])), SupplierID: strings.ToLower(rawString(row["Контрагент_Key"])),
		CurrencyID: strings.ToLower(rawString(row["ВалютаДенежныхСредств_Key"])), Date: rawString(row["Date"]), Comment: rawString(row["Комментарий"]),
		Amount: rawDecimal(row["СуммаДокумента"]),
	}
	if raw, ok := row[paymentLinesField]; ok {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&payment.Lines); err != nil {
			return editablePayment{}, errors.New("invalid OData payment breakdown")
		}
	}
	for _, line := range payment.Lines {
		for key := range line {
			if strings.Contains(key, "@") {
				delete(line, key)
			}
		}
	}
	return payment, nil
}

// paymentFields starts each kept line from its stored row, so fields this tool does not manage survive an amendment.
func paymentFields(result PaymentWrite, current editablePayment) map[string]any {
	rows := make([]map[string]any, 0, len(result.Allocations))
	var cashFlowItem any
	if len(current.Lines) > 0 {
		cashFlowItem = current.Lines[0]["СтатьяДвиженияДенежныхСредств_Key"]
	}
	for _, line := range result.Allocations {
		row := map[string]any{}
		if line.source >= 0 {
			for key, value := range current.Lines[line.source] {
				row[key] = value
			}
		} else {
			row["Курс"], row["Кратность"] = json.Number("1"), json.Number("1")
			if cashFlowItem != nil {
				row["СтатьяДвиженияДенежныхСредств_Key"] = cashFlowItem
			}
		}
		row["LineNumber"] = fmt.Sprint(line.LineNumber)
		if !line.Preserved {
			row["Договор_Key"] = line.ContractID
			row["ПризнакАванса"] = line.Advance
			if line.DocumentID != "" {
				row["Документ"], row["Документ_Type"] = line.DocumentID, line.DocumentType
			}
			row["СуммаПлатежа"], row["СуммаРасчетов"] = json.Number(line.Amount), json.Number(line.Amount)
			row["СтавкаНДС_Key"], row["СуммаНДС"] = line.VATRateID, json.Number(line.VAT)
		}
		rows = append(rows, row)
	}
	return map[string]any{"Date": result.Date, "Комментарий": result.Comment, "СуммаДокумента": json.Number(result.Amount), paymentLinesField: rows}
}

func verifyPaymentSaved(saved editablePayment, want PaymentWrite) error {
	if saved.Posted {
		return errors.New("1C saved the payment as posted")
	}
	if !decimalEqual(saved.Amount, want.Amount) {
		return errors.New("payment amount " + string(saved.Amount) + " differs from " + string(want.Amount))
	}
	if len(saved.Lines) != len(want.Allocations) {
		return errors.New("allocation line count does not match")
	}
	for index, line := range saved.Lines {
		expected := want.Allocations[index]
		if !strings.EqualFold(anyString(line["Договор_Key"]), expected.ContractID) || !decimalEqual(anyDecimal(line["СуммаПлатежа"]), expected.Amount) {
			return fmt.Errorf("allocation line %d does not match", index+1)
		}
		if expected.DocumentID != "" && !strings.EqualFold(anyString(line["Документ"]), expected.DocumentID) {
			return fmt.Errorf("allocation line %d document does not match", index+1)
		}
	}
	return nil
}

func paymentResource(payment editablePayment) string {
	return config.MoneyDocuments[payment.Kind].Name + "(guid'" + payment.ID + "')"
}
