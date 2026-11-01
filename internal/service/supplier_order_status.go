package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type SupplierOrderLineStatus struct {
	LineNumber       int     `json:"line_number"`
	ProductID        string  `json:"product_id"`
	CharacteristicID string  `json:"characteristic_id,omitempty"`
	UnitID           string  `json:"unit_id"`
	Price            Decimal `json:"price"`
	DiscountPercent  Decimal `json:"discount_percent"`
	Amount           Decimal `json:"amount"`
	VATRateID        string  `json:"vat_rate_id"`
	Ordered          Decimal `json:"ordered"`
	Cancelled        Decimal `json:"cancelled"`
	Received         Decimal `json:"received"`
	Pending          Decimal `json:"pending_unposted"`
	Expected         Decimal `json:"expected"`
	DeliveryDate     string  `json:"delivery_date,omitempty"`
}

type SupplierOrderReceipt struct {
	ID     string  `json:"id"`
	Number string  `json:"number"`
	Date   string  `json:"date"`
	Posted bool    `json:"posted"`
	Amount Decimal `json:"amount"`
	Link   string  `json:"link"`
}

type SupplierOrderPaymentMovement struct {
	Source       string  `json:"source"`
	RecorderID   string  `json:"recorder_id"`
	RecorderType string  `json:"recorder_type"`
	Period       string  `json:"period"`
	RecordType   string  `json:"record_type"`
	Amount       Decimal `json:"amount"`
	Counted      bool    `json:"counted"`
}

type SupplierOrderPaymentDocument struct {
	Kind    string  `json:"kind"`
	ID      string  `json:"id"`
	Number  string  `json:"number"`
	Date    string  `json:"date"`
	Posted  bool    `json:"posted"`
	Amount  Decimal `json:"amount_for_order"`
	Counted bool    `json:"counted"`
}

type SupplierOrderPayments struct {
	Status      string                         `json:"status"`
	Available   bool                           `json:"available"`
	Paid        Decimal                        `json:"paid"`
	Returned    Decimal                        `json:"returned"`
	Outstanding Decimal                        `json:"outstanding"`
	Movements   []SupplierOrderPaymentMovement `json:"movements"`
	Documents   []SupplierOrderPaymentDocument `json:"documents"`
	Notes       []string                       `json:"notes"`
}

// SupplierOrderStatus shows what was ordered, received and paid, from posted documents and register movements only.
type SupplierOrderStatus struct {
	ID             string                    `json:"id"`
	Number         string                    `json:"number"`
	Date           string                    `json:"date"`
	Posted         bool                      `json:"posted"`
	DataVersion    string                    `json:"data_version"`
	Operation      string                    `json:"operation"`
	StateID        string                    `json:"state_id,omitempty"`
	OrganizationID string                    `json:"organization_id"`
	SupplierID     string                    `json:"supplier_id"`
	ContractID     string                    `json:"contract_id"`
	WarehouseID    string                    `json:"warehouse_id,omitempty"`
	DepartmentID   string                    `json:"department_id,omitempty"`
	InvoiceNumber  string                    `json:"invoice_number"`
	InvoiceDate    string                    `json:"invoice_date"`
	DeliveryDate   string                    `json:"delivery_date"`
	Comment        string                    `json:"comment"`
	VATMode        string                    `json:"vat_mode"`
	Amount         Decimal                   `json:"amount"`
	VAT            Decimal                   `json:"vat"`
	DeliveryStatus string                    `json:"delivery_status"`
	Lines          []SupplierOrderLineStatus `json:"lines"`
	Receipts       []SupplierOrderReceipt    `json:"receipts"`
	Schedule       []SupplierOrderPayment    `json:"payment_schedule"`
	Payments       SupplierOrderPayments     `json:"payments"`
	Notes          []string                  `json:"notes"`
}

// GetSupplierOrderStatus reads one supplier order with its delivery and payment progress.
func (s Service) GetSupplierOrderStatus(ctx context.Context, id string) (SupplierOrderStatus, error) {
	order, err := s.readEditableOrder(ctx, id)
	if err != nil {
		return SupplierOrderStatus{}, err
	}
	status, _, err := s.orderStatus(ctx, order, "")
	if err != nil {
		return SupplierOrderStatus{}, err
	}
	status.Payments = s.orderPayments(ctx, order)
	return status, nil
}

type orderReceiptLine struct {
	ReceiptID string
	Posted    bool
	Line      editableReceiptLine
}

// orderStatus matches receipt lines linked to the order with its lines; exceptReceipt leaves one receipt out.
func (s Service) orderStatus(ctx context.Context, order editableOrder, exceptReceipt string) (SupplierOrderStatus, map[int]*big.Rat, error) {
	mode := receiptVATExcluded
	if order.IncludesVAT {
		mode = receiptVATIncluded
	}
	status := SupplierOrderStatus{
		ID: order.ID, Number: order.Number, Date: order.Date, Posted: order.Posted, DataVersion: order.DataVersion, Operation: order.Operation, StateID: order.StateID,
		OrganizationID: order.OrganizationID, SupplierID: order.SupplierID, ContractID: order.ContractID, WarehouseID: order.WarehouseID, DepartmentID: order.DepartmentID,
		InvoiceNumber: order.InvoiceNumber, InvoiceDate: order.InvoiceDate, DeliveryDate: order.DeliveryDate, Comment: order.Comment, VATMode: mode,
		Amount: order.Amount, VAT: order.VAT, Lines: []SupplierOrderLineStatus{}, Receipts: []SupplierOrderReceipt{}, Schedule: order.Schedule, Notes: []string{},
	}
	if !order.Posted {
		status.Notes = append(status.Notes, "The order is not posted, so 1C does not count it as ordered yet.")
	}
	plan := config.DocumentResource{Name: receiptResourceName, DateField: "Date", DeletedField: "DeletionMark", Fields: []config.FieldBinding{
		{Output: "id", Source: "Ref_Key"}, {Output: "number", Source: "Number"}, {Output: "date", Source: "Date"}, {Output: "posted", Source: "Posted"}, {Output: "deleted", Source: "DeletionMark"},
		{Output: "operation", Source: "ВидОперации"}, {Output: "supplier_id", Source: "Контрагент_Key"}, {Output: "amount", Source: "СуммаДокумента"},
		{Output: "order_id", Source: "Заказ"}, {Output: "order_type", Source: "Заказ_Type"},
	}, LinesField: "Запасы"}
	rows, err := s.supplierDocumentsWithLines(ctx, plan, order.SupplierID)
	if err != nil {
		return SupplierOrderStatus{}, nil, fmt.Errorf("could not read the supplier's receipts: %w", err)
	}
	linked := make([]orderReceiptLine, 0)
	for _, row := range rows {
		if string(row["DeletionMark"]) == "true" || rawString(row["ВидОперации"]) != receiptOperation || strings.EqualFold(rawString(row["Ref_Key"]), exceptReceipt) {
			continue
		}
		headerOrder := rawString(row["Заказ_Type"]) == supplierOrderType && strings.EqualFold(rawString(row["Заказ"]), order.ID)
		var lines []map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(row["Запасы"])))
		decoder.UseNumber()
		if raw := row["Запасы"]; len(raw) > 0 && string(raw) != "null" {
			if err := decoder.Decode(&lines); err != nil {
				return SupplierOrderStatus{}, nil, errors.New("invalid OData receipt lines")
			}
		}
		posted := string(row["Posted"]) == "true"
		count, linkKind := 0, "lines"
		for _, line := range lines {
			lineOrder, lineType := anyString(line["Заказ"]), anyString(line["Заказ_Type"])
			lineLinked := lineType == supplierOrderType && strings.EqualFold(lineOrder, order.ID)
			if !lineLinked && !(headerOrder && !linkedGUID(lineOrder)) {
				continue
			}
			if !lineLinked {
				linkKind = "header"
			}
			var number int
			_, _ = fmt.Sscan(anyString(line["LineNumber"]), &number)
			characteristic := strings.ToLower(anyString(line["Характеристика_Key"]))
			if characteristic == emptyGUID {
				characteristic = ""
			}
			linked = append(linked, orderReceiptLine{ReceiptID: rawString(row["Ref_Key"]), Posted: posted, Line: editableReceiptLine{
				LineNumber: number, ProductID: strings.ToLower(anyString(line["Номенклатура_Key"])), CharacteristicID: characteristic,
				UnitID: strings.ToLower(anyString(line["ЕдиницаИзмерения"])), Quantity: anyDecimal(line["Количество"]),
			}})
			count++
		}
		if count > 0 {
			status.Receipts = append(status.Receipts, SupplierOrderReceipt{ID: strings.ToLower(rawString(row["Ref_Key"])), Number: strings.TrimSpace(rawString(row["Number"])), Date: rawString(row["Date"]), Posted: posted, Amount: rawDecimal(row["СуммаДокумента"]), Link: linkKind})
		}
	}
	sort.SliceStable(status.Receipts, func(i, j int) bool { return status.Receipts[i].Date < status.Receipts[j].Date })
	type tally struct{ ordered, cancelled, received, pending *big.Rat }
	tallies := make([]tally, len(order.Lines))
	for index, line := range order.Lines {
		ordered, ok := new(big.Rat).SetString(string(line.Quantity))
		if !ok {
			return SupplierOrderStatus{}, nil, errors.New("invalid OData supplier order quantity")
		}
		tallies[index] = tally{ordered: ordered, cancelled: new(big.Rat), received: new(big.Rat), pending: new(big.Rat)}
	}
	key := func(product, characteristic, unit string) string { return product + "|" + characteristic + "|" + unit }
	// Cancelled quantities come off the first matching lines.
	for _, cancelled := range order.Cancelled {
		left, ok := new(big.Rat).SetString(string(cancelled.Quantity))
		if !ok {
			continue
		}
		for index, line := range order.Lines {
			if left.Sign() <= 0 || key(line.ProductID, line.CharacteristicID, line.UnitID) != key(cancelled.ProductID, cancelled.CharacteristicID, cancelled.UnitID) {
				continue
			}
			room := new(big.Rat).Sub(tallies[index].ordered, tallies[index].cancelled)
			take := minRat(left, room)
			tallies[index].cancelled.Add(tallies[index].cancelled, take)
			left.Sub(left, take)
		}
	}
	unmatched := 0
	// Received goods fill order lines in line order; posted receipts first, so pending ones never hide what has arrived.
	sort.SliceStable(linked, func(i, j int) bool { return linked[i].Posted && !linked[j].Posted })
	for _, received := range linked {
		left, ok := new(big.Rat).SetString(string(received.Line.Quantity))
		if !ok {
			return SupplierOrderStatus{}, nil, errors.New("invalid OData receipt quantity")
		}
		matchedAny := false
		for index, line := range order.Lines {
			if key(line.ProductID, line.CharacteristicID, line.UnitID) != key(received.Line.ProductID, received.Line.CharacteristicID, received.Line.UnitID) {
				continue
			}
			matchedAny = true
			t := tallies[index]
			room := new(big.Rat).Sub(new(big.Rat).Sub(new(big.Rat).Sub(t.ordered, t.cancelled), t.received), t.pending)
			if room.Sign() <= 0 {
				continue
			}
			take := minRat(left, room)
			if received.Posted {
				t.received.Add(t.received, take)
			} else {
				t.pending.Add(t.pending, take)
			}
			left.Sub(left, take)
			if left.Sign() == 0 {
				break
			}
		}
		if !matchedAny || left.Sign() > 0 {
			unmatched++
		}
	}
	if unmatched > 0 {
		status.Notes = append(status.Notes, fmt.Sprintf("%d linked receipt lines do not fit the order lines (other product, unit or more than ordered); they are not counted.", unmatched))
	}
	expected := make(map[int]*big.Rat, len(order.Lines))
	open, done := false, true
	for index, line := range order.Lines {
		t := tallies[index]
		left := new(big.Rat).Sub(new(big.Rat).Sub(new(big.Rat).Sub(t.ordered, t.cancelled), t.received), t.pending)
		expected[line.LineNumber] = left
		if left.Sign() > 0 || t.pending.Sign() > 0 {
			done = false
		}
		if t.received.Sign() > 0 {
			open = true
		}
		status.Lines = append(status.Lines, SupplierOrderLineStatus{
			LineNumber: line.LineNumber, ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, UnitID: line.UnitID, Price: line.Price,
			DiscountPercent: orZero(line.DiscountPercent), Amount: line.Amount, VATRateID: line.VATRateID, DeliveryDate: firstNonEmpty(line.DeliveryDate, order.DeliveryDate),
			Ordered: line.Quantity, Cancelled: ratDecimal(t.cancelled, maxQuantityScale), Received: ratDecimal(t.received, maxQuantityScale),
			Pending: ratDecimal(t.pending, maxQuantityScale), Expected: ratDecimal(left, maxQuantityScale),
		})
	}
	switch {
	case done:
		status.DeliveryStatus = "received"
	case open:
		status.DeliveryStatus = "partly_received"
	default:
		status.DeliveryStatus = "awaiting"
	}
	return status, expected, nil
}

func minRat(first, second *big.Rat) *big.Rat {
	if first.Cmp(second) < 0 {
		return new(big.Rat).Set(first)
	}
	return new(big.Rat).Set(second)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var supplierOrderPaymentSources = map[string]string{
	"StandardODATA.Document_РасходСоСчета":         "bank-out",
	"StandardODATA.Document_РасходИзКассы":         "cash-out",
	"StandardODATA.Document_ПоступлениеНаСчет":     "bank-in",
	"StandardODATA.Document_ПоступлениеВКассу":     "cash-in",
	"StandardODATA.Document_Взаимозачет":           "debt-offset",
	"StandardODATA.Document_ВводНачальныхОстатков": "opening-balance",
	supplierReceiptType: "receipt",
	"StandardODATA.Document_КорректировкаПоступления": "receipt-correction",
}

// orderPayments counts the order's payments from supplier-settlement movements carrying the order;
// payment documents are listed as evidence and cross-checked, never added on top.
func (s Service) orderPayments(ctx context.Context, order editableOrder) SupplierOrderPayments {
	payments := SupplierOrderPayments{Movements: []SupplierOrderPaymentMovement{}, Documents: []SupplierOrderPaymentDocument{}, Notes: []string{
		"Paid comes from active supplier-settlement movements that carry this order: bank and cash payments, refunds, debt offsets and opening balances. Receipts that consume an advance are listed but do not change what was paid.",
		"Payment orders, payment plans and approvals are not payments; unposted documents are not counted.",
	}}
	register := config.SupplierSettlements
	rows, err := s.scanRows(ctx, register.Name, sourceFields(register.Fields)+",Заказ_Key", "", maxDocumentScan)
	paid, returned := &stockSum{}, &stockSum{}
	if err != nil {
		payments.Status = "unknown"
		payments.Notes = append(payments.Notes, "Supplier-settlement register unavailable: "+err.Error())
	} else {
		payments.Available = true
		for _, row := range rows {
			if !strings.EqualFold(rawString(row["Заказ_Key"]), order.ID) || string(row["Active"]) != "true" {
				continue
			}
			movement := SupplierOrderPaymentMovement{RecorderID: strings.ToLower(rawString(row["Recorder"])), RecorderType: rawString(row["Recorder_Type"]), Period: rawString(row["Period"]), RecordType: rawString(row["RecordType"]), Amount: rawDecimal(row["СуммаВал"])}
			movement.Source = supplierOrderPaymentSources[movement.RecorderType]
			switch movement.Source {
			case "receipt", "receipt-correction":
			case "":
				movement.Source = "other"
				payments.Notes = append(payments.Notes, "A settlement movement from "+movement.RecorderType+" carries this order; it is listed but not counted, check it in 1C.")
			default:
				movement.Counted = true
				target := paid
				if movement.RecordType == "Receipt" {
					target = returned
				} else if movement.RecordType != "Expense" {
					movement.Counted = false
				}
				if movement.Counted {
					if err := target.Add(movement.Amount); err != nil {
						movement.Counted = false
					}
				}
			}
			payments.Movements = append(payments.Movements, movement)
		}
	}
	documented := &stockSum{}
	for _, kind := range []string{"bank-out", "cash-out"} {
		plan := config.SupplierPaymentDocuments[kind]
		docs, err := s.supplierDocumentsWithLines(ctx, plan, order.SupplierID)
		if err != nil {
			payments.Notes = append(payments.Notes, "Could not read "+plan.Name+": "+err.Error())
			continue
		}
		for _, row := range docs {
			if string(row["DeletionMark"]) == "true" {
				continue
			}
			var lines []map[string]json.RawMessage
			_ = json.Unmarshal(row[plan.LinesField], &lines)
			amount := &stockSum{}
			for _, line := range lines {
				if rawString(line["Заказ_Type"]) == supplierOrderType && strings.EqualFold(rawString(line["Заказ"]), order.ID) {
					_ = amount.Add(rawDecimal(line["СуммаРасчетов"]))
				}
			}
			if amount.value.Sign() == 0 {
				continue
			}
			posted := string(row["Posted"]) == "true"
			payments.Documents = append(payments.Documents, SupplierOrderPaymentDocument{Kind: kind, ID: strings.ToLower(rawString(row["Ref_Key"])), Number: strings.TrimSpace(rawString(row["Number"])), Date: rawString(row["Date"]), Posted: posted, Amount: amount.Decimal(), Counted: posted && payments.Available})
			if posted {
				_ = documented.Add(amount.Decimal())
			}
		}
	}
	net := new(big.Rat).Sub(&paid.value, &returned.value)
	scale := max(paid.scale, returned.scale, 2)
	payments.Paid, payments.Returned = paid.Decimal(), returned.Decimal()
	total, ok := new(big.Rat).SetString(string(order.Amount))
	if !payments.Available || !ok {
		payments.Status = "unknown"
		payments.Outstanding = ""
		return payments
	}
	outstanding := new(big.Rat).Sub(total, net)
	payments.Outstanding = ratDecimal(outstanding, scale)
	switch {
	case net.Sign() <= 0:
		payments.Status = "unpaid"
	case outstanding.Sign() > 0:
		payments.Status = "partly_paid"
	case outstanding.Sign() == 0:
		payments.Status = "paid"
	default:
		payments.Status = "overpaid"
	}
	if paidOut := documented.value; paidOut.Cmp(&paid.value) != 0 {
		payments.Notes = append(payments.Notes, fmt.Sprintf("Posted bank and cash payments allocate %s to this order but the register shows %s paid; the difference may be an opening balance, an offset, or an allocation 1C moved. Check before relying on it.", documented.Decimal(), paid.Decimal()))
		if payments.Status == "paid" || payments.Status == "unpaid" {
			payments.Status += "_needs_review"
		}
	}
	return payments
}
