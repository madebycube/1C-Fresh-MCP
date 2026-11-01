package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
)

type OrderReceiptLine struct {
	LineNumber int    `json:"line_number"`
	Quantity   string `json:"quantity"`
}

// SupplierOrderReceiptDraft asks for a goods receipt of what actually arrived against a supplier order.
type SupplierOrderReceiptDraft struct {
	OrderID        string             `json:"order_id"`
	Date           string             `json:"date"`
	WarehouseID    string             `json:"warehouse_id,omitempty"`
	IncomingNumber string             `json:"incoming_number,omitempty"`
	IncomingDate   string             `json:"incoming_date,omitempty"`
	Comment        string             `json:"comment,omitempty"`
	Lines          []OrderReceiptLine `json:"lines,omitempty"`
	AllRemaining   bool               `json:"all_remaining,omitempty"`
	AllowDuplicate bool               `json:"allow_duplicate,omitempty"`
	Apply          bool               `json:"apply,omitempty"`
}

// CreateReceiptFromOrder previews and, with Apply, saves an unposted receipt linked to a posted supplier order.
// Only the given quantities are received; the rest stays expected on the order. Quantities are checked against
// what is still expected, counting unposted linked receipts, so a retried request cannot receive the same goods twice.
func (s Service) CreateReceiptFromOrder(ctx context.Context, input SupplierOrderReceiptDraft) (ReceiptWrite, error) {
	order, err := s.readEditableOrder(ctx, input.OrderID)
	if err != nil {
		return ReceiptWrite{}, err
	}
	if order.Operation != supplierOrderPurchase {
		return ReceiptWrite{}, unsupported("only purchase orders can be received; this order has operation " + order.Operation)
	}
	if !order.Posted {
		return ReceiptWrite{}, errors.New("supplier order is not posted; post it before receiving goods against it")
	}
	if input.AllRemaining == (len(input.Lines) > 0) {
		return ReceiptWrite{}, errors.New("give the received lines, or all remaining, but not both")
	}
	status, expected, err := s.orderStatus(ctx, order, "")
	if err != nil {
		return ReceiptWrite{}, err
	}
	requested := input.Lines
	if input.AllRemaining {
		for _, line := range order.Lines {
			if left := expected[line.LineNumber]; left.Sign() > 0 {
				requested = append(requested, OrderReceiptLine{LineNumber: line.LineNumber, Quantity: ratDecimal(left, maxQuantityScale).String()})
			}
		}
		if len(requested) == 0 {
			return ReceiptWrite{}, fmt.Errorf("nothing is left to receive on order %s; %d receipts are already linked to it", order.Number, len(status.Receipts))
		}
	}
	if len(requested) > maxReceiptLines {
		return ReceiptWrite{}, fmt.Errorf("a receipt holds at most %d lines", maxReceiptLines)
	}
	warehouse := firstNonEmpty(input.WarehouseID, order.WarehouseID)
	mode := receiptVATExcluded
	if order.IncludesVAT {
		mode = receiptVATIncluded
	}
	result, err := s.validateReceiptHeader(ctx, ReceiptDraft{
		OrganizationID: order.OrganizationID, SupplierID: order.SupplierID, ContractID: order.ContractID, WarehouseID: warehouse, DepartmentID: order.DepartmentID,
		Date: input.Date, IncomingNumber: input.IncomingNumber, IncomingDate: input.IncomingDate, Comment: input.Comment, VATMode: mode,
	})
	if err != nil {
		return ReceiptWrite{}, err
	}
	result.OrderID = order.ID
	if result.Date < order.Date[:min(len(order.Date), 19)] {
		return ReceiptWrite{}, errors.New("receipt date cannot be before the order date")
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return ReceiptWrite{}, err
	}
	byNumber := make(map[int]editableOrderLine, len(order.Lines))
	for _, line := range order.Lines {
		byNumber[line.LineNumber] = line
	}
	seen := make(map[int]bool)
	remaining := make([]string, 0)
	for index, request := range requested {
		line, ok := byNumber[request.LineNumber]
		if !ok || seen[request.LineNumber] {
			return ReceiptWrite{}, fmt.Errorf("order line %d does not exist or is given twice", request.LineNumber)
		}
		seen[request.LineNumber] = true
		quantity, err := exactDecimal(request.Quantity, maxQuantityScale, false)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("order line %d: quantity must be positive with at most %d decimals", request.LineNumber, maxQuantityScale)
		}
		if left := expected[line.LineNumber]; quantity.Cmp(left) > 0 {
			return ReceiptWrite{}, fmt.Errorf("order line %d: %s exceeds the %s still expected; check the receipts already linked to the order", request.LineNumber, ratDecimal(quantity, maxQuantityScale), ratDecimal(left, maxQuantityScale))
		}
		unit, err := s.stockLineUnit(ctx, line.ProductID, line.CharacteristicID, line.UnitID)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("order line %d: %w", request.LineNumber, err)
		}
		if unit.ID != line.UnitID || unit.Type != line.UnitType {
			return ReceiptWrite{}, unsupported(fmt.Sprintf("order line %d uses a unit this tool cannot carry over", request.LineNumber))
		}
		rateName := ""
		for name, rate := range rates {
			if strings.EqualFold(rate.ID, line.VATRateID) {
				rateName = name
			}
		}
		ordered, okOrdered := new(big.Rat).SetString(string(line.Quantity))
		orderAmount, okAmount := new(big.Rat).SetString(string(line.Amount))
		if rateName == "" || !okOrdered || !okAmount || ordered.Sign() <= 0 {
			return ReceiptWrite{}, fmt.Errorf("order line %d has no usable VAT rate, quantity or amount", request.LineNumber)
		}
		// The receipt takes the order's discounted amount in proportion; the full quantity gets exactly the ordered amount.
		amount := roundHalfAway(new(big.Rat).Quo(new(big.Rat).Mul(orderAmount, quantity), ordered), receiptAmountScale)
		price := roundHalfAway(new(big.Rat).Quo(orderAmount, ordered), maxPriceScale)
		preview, err := computeReceiptLine(ReceiptLineInput{ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, Quantity: ratDecimal(quantity, maxQuantityScale).String(), Price: ratDecimal(price, maxPriceScale).String(), VATRate: rateName, Amount: ratDecimal(amount, receiptAmountScale).String()}, index+1, unit, mode, rates)
		if err != nil {
			return ReceiptWrite{}, fmt.Errorf("order line %d: %w", request.LineNumber, err)
		}
		result.Lines = append(result.Lines, preview)
		left := new(big.Rat).Sub(expected[line.LineNumber], quantity)
		if left.Sign() > 0 {
			remaining = append(remaining, fmt.Sprintf("line %d: %s", line.LineNumber, ratDecimal(left, maxQuantityScale)))
		}
	}
	for _, line := range order.Lines {
		if !seen[line.LineNumber] && expected[line.LineNumber].Sign() > 0 {
			remaining = append(remaining, fmt.Sprintf("line %d: %s", line.LineNumber, ratDecimal(expected[line.LineNumber], maxQuantityScale)))
		}
	}
	finishReceiptTotals(&result)
	result.Impact = append(result.Impact, "linked to supplier order "+order.Number+" in the header and on every line")
	if len(remaining) > 0 {
		result.Impact = append(result.Impact, "still expected on the order after this receipt: "+strings.Join(remaining, ", "))
	} else {
		result.Impact = append(result.Impact, "nothing stays expected on the order after this receipt")
	}
	return s.saveReceiptDraft(ctx, result, input.AllowDuplicate, input.Apply)
}

type ReceiptOrderLinkLine struct {
	ReceiptLine int     `json:"receipt_line"`
	OrderLine   int     `json:"order_line,omitempty"`
	ProductID   string  `json:"product_id"`
	Quantity    Decimal `json:"quantity"`
	Expected    Decimal `json:"expected_before"`
	Problem     string  `json:"problem,omitempty"`
}

type ReceiptOrderLink struct {
	Applied       bool                   `json:"applied"`
	Unchanged     bool                   `json:"unchanged"`
	ReceiptID     string                 `json:"receipt_id"`
	ReceiptNumber string                 `json:"receipt_number"`
	OrderID       string                 `json:"order_id"`
	OrderNumber   string                 `json:"order_number"`
	Posted        bool                   `json:"posted"`
	DataVersion   string                 `json:"data_version"`
	Lines         []ReceiptOrderLinkLine `json:"lines"`
	Problems      []string               `json:"problems"`
	Impact        []string               `json:"impact"`
}

type ReceiptOrderLinkChange struct {
	Repost              bool   `json:"repost,omitempty"`
	ExpectedDataVersion string `json:"expected_data_version,omitempty"`
	Apply               bool   `json:"apply,omitempty"`
}

// LinkReceiptToOrder previews and, with Apply, links an existing receipt to a supplier order without creating a document.
// Every receipt line must match an order line by product, characteristic and unit and fit in what is still expected.
func (s Service) LinkReceiptToOrder(ctx context.Context, receiptID, orderID string, change ReceiptOrderLinkChange) (ReceiptOrderLink, error) {
	receipt, err := s.readEditableReceipt(ctx, receiptID)
	if err != nil {
		return ReceiptOrderLink{}, err
	}
	order, err := s.readEditableOrder(ctx, orderID)
	if err != nil {
		return ReceiptOrderLink{}, err
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != receipt.DataVersion {
		return ReceiptOrderLink{}, errors.New("receipt changed since the preview; preview again")
	}
	result := ReceiptOrderLink{ReceiptID: receipt.ID, ReceiptNumber: receipt.Number, OrderID: order.ID, OrderNumber: order.Number, Posted: receipt.Posted, DataVersion: receipt.DataVersion, Lines: []ReceiptOrderLinkLine{}, Problems: []string{}}
	problem := func(text string) { result.Problems = append(result.Problems, text) }
	if order.Operation != supplierOrderPurchase {
		problem("the order is not a purchase order")
	}
	if !order.Posted {
		problem("the order is not posted")
	}
	if !strings.EqualFold(receipt.OrganizationID, order.OrganizationID) || !strings.EqualFold(receipt.SupplierID, order.SupplierID) || !strings.EqualFold(receipt.ContractID, order.ContractID) {
		problem("the receipt's organization, supplier or contract differs from the order's")
	}
	if receipt.OrderID != "" && receipt.OrderID != order.ID {
		problem("the receipt is already linked to another supplier order " + receipt.OrderID)
	}
	linkedAlready := receipt.OrderID == order.ID
	for _, line := range receipt.Lines {
		lineOrder := anyString(line.Raw["Заказ"])
		if linkedGUID(lineOrder) && !strings.EqualFold(lineOrder, order.ID) {
			problem(fmt.Sprintf("receipt line %d is linked to another order", line.LineNumber))
		}
		if !strings.EqualFold(lineOrder, order.ID) {
			linkedAlready = false
		}
	}
	if linkedAlready {
		result.Unchanged = true
		result.Impact = []string{"the receipt is already linked to this order; nothing to change"}
		return result, nil
	}
	_, expected, err := s.orderStatus(ctx, order, receipt.ID)
	if err != nil {
		return ReceiptOrderLink{}, err
	}
	left := make(map[int]*big.Rat, len(expected))
	for number, value := range expected {
		left[number] = new(big.Rat).Set(value)
	}
	for _, line := range receipt.Lines {
		entry := ReceiptOrderLinkLine{ReceiptLine: line.LineNumber, ProductID: line.ProductID, Quantity: line.Quantity}
		quantity, ok := new(big.Rat).SetString(string(line.Quantity))
		if !ok {
			return ReceiptOrderLink{}, errors.New("invalid OData receipt quantity")
		}
		for _, orderLine := range order.Lines {
			if orderLine.ProductID != strings.ToLower(line.ProductID) || orderLine.CharacteristicID != strings.ToLower(line.CharacteristicID) || orderLine.UnitID != strings.ToLower(line.UnitID) {
				continue
			}
			if entry.OrderLine == 0 {
				entry.OrderLine, entry.Expected = orderLine.LineNumber, ratDecimal(left[orderLine.LineNumber], maxQuantityScale)
			}
			if left[orderLine.LineNumber].Cmp(quantity) >= 0 {
				entry.OrderLine, entry.Expected = orderLine.LineNumber, ratDecimal(left[orderLine.LineNumber], maxQuantityScale)
				break
			}
		}
		switch {
		case entry.OrderLine == 0:
			entry.Problem = "the order has no line with this product, characteristic and unit"
		case left[entry.OrderLine].Cmp(quantity) < 0:
			entry.Problem = "more than the order still expects"
		default:
			left[entry.OrderLine].Sub(left[entry.OrderLine], quantity)
		}
		if entry.Problem != "" {
			problem(fmt.Sprintf("receipt line %d: %s", line.LineNumber, entry.Problem))
		}
		result.Lines = append(result.Lines, entry)
	}
	result.Impact = []string{"sets the supplier order on the receipt header and every line; quantities, prices and other fields stay as read", "creates no document"}
	if receipt.Posted {
		result.Impact = append(result.Impact, "the receipt is posted: it is unposted, linked and posted again, and left unposted if a step fails")
	}
	if !change.Apply {
		return result, nil
	}
	if len(result.Problems) > 0 {
		return ReceiptOrderLink{}, errors.New("the receipt does not match the order: " + strings.Join(result.Problems, "; "))
	}
	if receipt.Posted && !change.Repost {
		return ReceiptOrderLink{}, errors.New("receipt is posted; pass repost to unpost, link and post it again")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ReceiptOrderLink{}, errors.New("OData client does not support writes")
	}
	rows := make([]map[string]any, 0, len(receipt.Lines))
	for _, line := range receipt.Lines {
		row := make(map[string]any, len(line.Raw)+2)
		for key, value := range line.Raw {
			row[key] = value
		}
		row["Заказ"], row["Заказ_Type"] = order.ID, supplierOrderType
		rows = append(rows, row)
	}
	body, err := json.Marshal(map[string]any{"Заказ": order.ID, "Заказ_Type": supplierOrderType, "Запасы": rows})
	if err != nil {
		return ReceiptOrderLink{}, errors.New("could not encode receipt")
	}
	version := receipt.DataVersion
	var actor documentActor
	if receipt.Posted {
		if actor, ok = s.OData.(documentActor); !ok {
			return ReceiptOrderLink{}, errors.New("OData client does not support document actions")
		}
		if _, err := actor.Action(ctx, receiptResource(receipt.ID), "Unpost"); err != nil {
			return ReceiptOrderLink{}, fmt.Errorf("1C rejected unpost; read the receipt before retrying: %w", err)
		}
		unposted, err := s.readEditableReceipt(ctx, receipt.ID)
		if err != nil || unposted.Posted {
			return ReceiptOrderLink{}, errors.New("receipt did not unpost cleanly; nothing was linked, check it in 1C")
		}
		version = unposted.DataVersion
	}
	if _, err := writer.Write(ctx, http.MethodPatch, receiptResource(receipt.ID), body, version); err != nil {
		if receipt.Posted {
			return ReceiptOrderLink{}, fmt.Errorf("receipt was unposted but the link was not saved; it is left unposted: %w", err)
		}
		return ReceiptOrderLink{}, err
	}
	saved, err := s.readEditableReceipt(ctx, receipt.ID)
	if err != nil {
		return ReceiptOrderLink{}, fmt.Errorf("the link was saved but the receipt could not be read back: %w", err)
	}
	if saved.OrderID != order.ID || len(saved.Lines) != len(receipt.Lines) || !decimalEqual(saved.Amount, receipt.Amount) {
		return ReceiptOrderLink{}, errors.New("the link was saved but the receipt differs on read-back; check it in 1C")
	}
	for _, line := range saved.Lines {
		if !strings.EqualFold(anyString(line.Raw["Заказ"]), order.ID) {
			return ReceiptOrderLink{}, fmt.Errorf("the link was saved but receipt line %d is not linked on read-back", line.LineNumber)
		}
	}
	if receipt.Posted {
		if _, err := actor.Action(ctx, receiptResource(receipt.ID), "Post"); err != nil {
			return ReceiptOrderLink{}, fmt.Errorf("the link was saved but 1C rejected posting; the receipt is left unposted: %w", err)
		}
		if saved, err = s.readEditableReceipt(ctx, receipt.ID); err != nil || !saved.Posted {
			return ReceiptOrderLink{}, errors.New("the link was saved but posting did not take effect; the receipt is left unposted")
		}
	}
	result.Applied, result.Posted, result.DataVersion = true, saved.Posted, saved.DataVersion
	return result, nil
}
