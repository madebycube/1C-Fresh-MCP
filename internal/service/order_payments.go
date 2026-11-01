package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"golang.org/x/sync/errgroup"
)

const customerOrderType = "StandardODATA.Document_ЗаказПокупателя"
const maxSettlementScan = 100000
const maxOrderPaymentDocuments = 500

type OrderPaymentsQuery struct {
	OrderID        string
	Number         string
	OrganizationID string
	CustomerID     string
	Date           string
	AsOf           string
}

// OrderPayments is derived from the customer-settlement register: only movements 1C allocated to the order count as its payments.
type OrderPayments struct {
	Order       OrderPaymentsOrder     `json:"order"`
	AsOf        string                 `json:"as_of"`
	Derived     bool                   `json:"derived"`
	Summary     OrderPaymentsSummary   `json:"summary"`
	Documents   []OrderPaymentDocument `json:"documents"`
	Unallocated OrderUnallocated       `json:"unallocated"`
	NotCounted  []OrderPaymentDocument `json:"not_counted"`
	Issues      []OrderPaymentIssue    `json:"issues"`
	Movements   OrderMovementCounts    `json:"movements"`
	Scans       []TraceScan            `json:"scans"`
	Notes       []string               `json:"notes"`
}

type OrderPaymentsOrder struct {
	ID             string  `json:"id"`
	Number         string  `json:"number"`
	Date           string  `json:"date"`
	Posted         bool    `json:"posted"`
	Deleted        bool    `json:"deleted"`
	Amount         Decimal `json:"amount"`
	CurrencyID     string  `json:"currency_id,omitempty"`
	OrganizationID string  `json:"organization_id,omitempty"`
	CustomerID     string  `json:"customer_id,omitempty"`
	ContractID     string  `json:"contract_id,omitempty"`
	Resource       string  `json:"resource"`
}

// OrderPaymentsSummary amounts are in settlement currency. NetPaid = Payments - Refunds + Offsets.
type OrderPaymentsSummary struct {
	Status          string  `json:"status"`
	NeedsReview     bool    `json:"needs_review"`
	Complete        bool    `json:"complete"`
	OrderAmount     Decimal `json:"order_amount"`
	Payments        Decimal `json:"payments"`
	Refunds         Decimal `json:"refunds"`
	Offsets         Decimal `json:"offsets"`
	NetPaid         Decimal `json:"net_paid"`
	Remaining       Decimal `json:"remaining"`
	Overpaid        Decimal `json:"overpaid"`
	Shipped         Decimal `json:"shipped"`
	RegisterBalance Decimal `json:"register_balance"`
}

// OrderPaymentDocument is one document with its net allocation to the order; Allocated is signed: positive pays, negative refunds.
type OrderPaymentDocument struct {
	Kind             string   `json:"kind"`
	Role             string   `json:"role"`
	ID               string   `json:"id"`
	Resource         string   `json:"resource"`
	Number           string   `json:"number,omitempty"`
	Date             string   `json:"date,omitempty"`
	Period           string   `json:"period,omitempty"`
	Posted           *bool    `json:"posted,omitempty"`
	Deleted          *bool    `json:"deleted,omitempty"`
	Operation        string   `json:"operation,omitempty"`
	DocumentAmount   Decimal  `json:"document_amount,omitempty"`
	Allocated        Decimal  `json:"allocated,omitempty"`
	AllocatedOutside Decimal  `json:"allocated_outside,omitempty"`
	SettlementTypes  []string `json:"settlement_types,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	Error            string   `json:"error,omitempty"`
}

type OrderUnallocated struct {
	AdvanceBalance Decimal                `json:"advance_balance"`
	Documents      []OrderPaymentDocument `json:"documents"`
}

type OrderPaymentIssue struct {
	Kind       string `json:"kind"`
	DocumentID string `json:"document_id,omitempty"`
	Reason     string `json:"reason"`
}

type OrderMovementCounts struct {
	Counted   int `json:"counted"`
	Inactive  int `json:"inactive"`
	AfterAsOf int `json:"after_as_of"`
}

type customerSettlement struct {
	RecorderID     string  `json:"recorder_id"`
	RecorderType   string  `json:"recorder_type"`
	LineNumber     string  `json:"line_number"`
	Period         string  `json:"period"`
	Active         bool    `json:"active"`
	RecordType     string  `json:"record_type"`
	SettlementType string  `json:"settlement_type"`
	OrganizationID string  `json:"organization_id"`
	CustomerID     string  `json:"customer_id"`
	ContractID     string  `json:"contract_id"`
	DocumentID     string  `json:"document_id"`
	DocumentType   string  `json:"document_type"`
	OrderID        string  `json:"order_id"`
	OrderType      string  `json:"order_type"`
	Amount         Decimal `json:"amount"`
	CurrencyAmount Decimal `json:"currency_amount"`
}

// orderPaymentSource says how a recorder's Expense (customer credit) and Receipt (customer debit) movements count.
// Roles: payment and refund change the amount paid, offset does both ways, debt is shipment or sale and never payment.
type orderPaymentSource struct {
	kind, expense, receipt string
	amount, operation      string
	reference, typeField   string
}

var orderPaymentSources = map[string]orderPaymentSource{
	"Document_ПоступлениеВКассу":         {kind: "cash-in", expense: "payment", receipt: "refund", amount: "СуммаДокумента", operation: "ВидОперации", reference: "ДокументОснование", typeField: "ДокументОснование_Type"},
	"Document_ПоступлениеНаСчет":         {kind: "bank-in", expense: "payment", receipt: "refund", amount: "СуммаДокумента", operation: "ВидОперации", reference: "ДокументОснование", typeField: "ДокументОснование_Type"},
	"Document_ОперацияПоПлатежнымКартам": {kind: "card-payment", expense: "payment", receipt: "refund", amount: "СуммаДокумента", operation: "ВидОперации", reference: "ДокументОснование", typeField: "ДокументОснование_Type"},
	"Document_РасходИзКассы":             {kind: "cash-out", expense: "payment", receipt: "refund", amount: "СуммаДокумента", operation: "ВидОперации", reference: "ДокументОснование", typeField: "ДокументОснование_Type"},
	"Document_РасходСоСчета":             {kind: "bank-out", expense: "payment", receipt: "refund", amount: "СуммаДокумента", operation: "ВидОперации", reference: "ДокументОснование", typeField: "ДокументОснование_Type"},
	"Document_ЧекККМ":                    {kind: "cash-register-receipt", expense: "payment", receipt: "debt", amount: "СуммаДокумента", reference: "Заказ_Key"},
	"Document_ЧекККМВозврат":             {kind: "cash-register-refund", expense: "debt", receipt: "refund", amount: "СуммаДокумента", reference: "Заказ_Key"},
	"Document_ОтчетОРозничныхПродажах":   {kind: "retail-report", expense: "payment", receipt: "debt"},
	"Document_ВводНачальныхОстатков":     {kind: "opening-balance", expense: "payment", receipt: "debt"},
	"Document_Взаимозачет":               {kind: "debt-offset", expense: "offset", receipt: "offset"},
	"Document_КорректировкаДолга":        {kind: "debt-adjustment", expense: "offset", receipt: "offset"},
	"Document_РасходнаяНакладная":        {kind: "shipment", expense: "debt", receipt: "debt"},
	"Document_АктВыполненныхРабот":       {kind: "works-act", expense: "debt", receipt: "debt"},
	"Document_ПриходнаяНакладная":        {kind: "customer-return", expense: "debt", receipt: "debt"},
	"Document_КорректировкаРеализации":   {kind: "sale-correction", expense: "debt", receipt: "debt"},
	"Document_ЗаказПокупателя":           {kind: "order", expense: "debt", receipt: "debt"},
}

// GetOrderPayments collects every payment, refund and offset 1C allocated to one customer order, as of a date.
func (s Service) GetOrderPayments(ctx context.Context, query OrderPaymentsQuery) (OrderPayments, error) {
	if (query.OrderID == "") == (strings.TrimSpace(query.Number) == "") {
		return OrderPayments{}, errors.New("give either an order ID or an order number")
	}
	if query.OrderID != "" && !linkedGUID(query.OrderID) {
		return OrderPayments{}, errors.New("order ID must be a nonzero GUID")
	}
	for _, id := range []string{query.OrganizationID, query.CustomerID} {
		if id != "" && !linkedGUID(id) {
			return OrderPayments{}, errors.New("organization and customer must be nonzero GUIDs")
		}
	}
	if query.Date != "" {
		if parsed, err := time.Parse("2006-01-02", query.Date); err != nil || parsed.Format("2006-01-02") != query.Date {
			return OrderPayments{}, errors.New("date must be YYYY-MM-DD")
		}
	}
	if query.AsOf == "" {
		query.AsOf = time.Now().Format("2006-01-02")
	}
	asOf, err := time.Parse("2006-01-02", query.AsOf)
	if err != nil || asOf.Format("2006-01-02") != query.AsOf {
		return OrderPayments{}, errors.New("as-of must be YYYY-MM-DD")
	}
	end := asOf.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00"
	order, err := s.findPaymentOrder(ctx, query)
	if err != nil {
		return OrderPayments{}, err
	}
	result := OrderPayments{
		Order: order, AsOf: query.AsOf, Derived: true, Documents: []OrderPaymentDocument{}, NotCounted: []OrderPaymentDocument{}, Issues: []OrderPaymentIssue{}, Scans: []TraceScan{},
		Unallocated: OrderUnallocated{AdvanceBalance: "0", Documents: []OrderPaymentDocument{}},
		Summary:     OrderPaymentsSummary{Complete: true, OrderAmount: order.Amount},
		Notes: []string{
			"Derived from AccumulationRegister_РасчетыСПокупателями over OData: a payment counts only when 1C allocated it to this order. Equal customer, amount or date never counts as a link.",
			"Amounts are in settlement currency. Acquirer commission is not deducted: the customer's payment is counted in full.",
			"An acquirer payout to the bank settles acquiring, not the customer, so it never adds to the order's payments.",
		},
	}
	if !order.Posted || order.Deleted {
		result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "order-not-posted", DocumentID: order.ID, Reason: "the order is not posted or is marked for deletion"})
	}
	register := config.CustomerSettlements
	rows, err := s.scanRows(ctx, register.Name, sourceFields(register.Fields), "", maxSettlementScan)
	scan := TraceScan{Resource: register.Name, Rows: len(rows)}
	if err != nil {
		scan.Error = err.Error()
		result.Scans = append(result.Scans, scan)
		return OrderPayments{}, errors.New("customer-settlement register is unavailable, so payments cannot be confirmed: " + err.Error())
	}
	result.Scans = append(result.Scans, scan)
	payments, refunds, offsets, shipped, balance, unallocated := &stockSum{}, &stockSum{}, &stockSum{}, &stockSum{}, &stockSum{}, &stockSum{}
	documents := map[string]*OrderPaymentDocument{}
	allocatedSums := map[string]*stockSum{}
	unallocatedDocs := map[string]*OrderPaymentDocument{}
	unallocatedSums := map[string]*stockSum{}
	seen := make(map[string]bool, len(rows))
	unknownSeen := map[string]bool{}
	for _, row := range rows {
		movement, err := bindFields[customerSettlement](row, register.Fields)
		if err != nil {
			return OrderPayments{}, errors.New("invalid OData customer-settlement record")
		}
		key := strings.ToLower(movement.RecorderType + "/" + movement.RecorderID + "/" + movement.LineNumber)
		if seen[key] {
			return OrderPayments{}, errors.New("settlement records repeated during lookup; retry")
		}
		seen[key] = true
		ownOrder := movement.OrderType == customerOrderType && strings.EqualFold(movement.OrderID, order.ID)
		noOrder := movement.OrderType == "" || movement.OrderType == "StandardODATA.Undefined" || !linkedGUID(movement.OrderID)
		sameCustomer := strings.EqualFold(movement.CustomerID, order.CustomerID) && (order.OrganizationID == "" || strings.EqualFold(movement.OrganizationID, order.OrganizationID)) && (order.ContractID == "" || strings.EqualFold(movement.ContractID, order.ContractID))
		if !ownOrder && !(noOrder && sameCustomer) {
			continue
		}
		if !movement.Active {
			if ownOrder {
				result.Movements.Inactive++
			}
			continue
		}
		if movement.Period >= end {
			if ownOrder {
				result.Movements.AfterAsOf++
			}
			continue
		}
		if movement.RecordType != "Receipt" && movement.RecordType != "Expense" {
			return OrderPayments{}, errors.New("unknown settlement record type " + movement.RecordType)
		}
		resource := strings.TrimPrefix(movement.RecorderType, "StandardODATA.")
		source, known := orderPaymentSources[resource]
		role := "debt"
		if known {
			role = source.receipt
			if movement.RecordType == "Expense" {
				role = source.expense
			}
		} else {
			source.kind = "other"
		}
		signed := movement.CurrencyAmount
		if movement.RecordType == "Expense" {
			signed = negate(signed)
		}
		if !ownOrder {
			// Unallocated advance: customer credit not tied to any order, which may belong to this one.
			if movement.SettlementType != "Аванс" {
				continue
			}
			if err := unallocated.Add(negate(signed)); err != nil {
				return OrderPayments{}, errors.New("invalid settlement amount")
			}
			if role == "payment" && movement.RecordType == "Expense" {
				id := strings.ToLower(movement.RecorderID)
				if unallocatedDocs[id] == nil {
					unallocatedDocs[id] = &OrderPaymentDocument{Kind: source.kind, Role: role, ID: id, Resource: resource, Period: movement.Period}
					unallocatedSums[id] = &stockSum{}
				}
				if err := unallocatedSums[id].Add(movement.CurrencyAmount); err != nil {
					return OrderPayments{}, errors.New("invalid settlement amount")
				}
			}
			continue
		}
		result.Movements.Counted++
		if order.CustomerID != "" && !strings.EqualFold(movement.CustomerID, order.CustomerID) {
			result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "customer-mismatch", DocumentID: strings.ToLower(movement.RecorderID), Reason: "a settlement movement for this order names a different customer"})
		}
		if err := balance.Add(signed); err != nil {
			return OrderPayments{}, errors.New("invalid settlement amount")
		}
		var target *stockSum
		credit := movement.CurrencyAmount
		switch role {
		case "payment":
			target = payments
		case "refund":
			target = refunds
		case "offset":
			target, credit = offsets, negate(signed)
		}
		if target == nil {
			if err := shipped.Add(signed); err != nil {
				return OrderPayments{}, errors.New("invalid settlement amount")
			}
			if !known && !unknownSeen[strings.ToLower(movement.RecorderID)] {
				unknownSeen[strings.ToLower(movement.RecorderID)] = true
				result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "unknown-recorder", DocumentID: strings.ToLower(movement.RecorderID), Reason: "a " + resource + " moves this order's settlements; it is counted as shipment or debt, not payment"})
			}
			continue
		}
		if err := target.Add(credit); err != nil {
			return OrderPayments{}, errors.New("invalid settlement amount")
		}
		id := strings.ToLower(movement.RecorderID)
		doc := documents[id]
		if doc == nil {
			doc = &OrderPaymentDocument{Kind: source.kind, Role: role, ID: id, Resource: resource, Period: movement.Period}
			documents[id] = doc
			allocatedSums[id] = &stockSum{}
		} else if doc.Role != role {
			doc.Role = "mixed"
		}
		if !containsString(doc.SettlementTypes, movement.SettlementType) {
			doc.SettlementTypes = append(doc.SettlementTypes, movement.SettlementType)
		}
		if err := allocatedSums[id].Add(negate(signed)); err != nil {
			return OrderPayments{}, errors.New("invalid settlement amount")
		}
	}
	result.Summary.Payments, result.Summary.Refunds, result.Summary.Offsets = payments.Decimal(), refunds.Decimal(), offsets.Decimal()
	result.Summary.Shipped, result.Summary.RegisterBalance = shipped.Decimal(), balance.Decimal()
	net := new(big.Rat).Sub(&payments.value, &refunds.value)
	net.Add(net, &offsets.value)
	scale := max(payments.scale, refunds.scale, offsets.scale, 2)
	result.Summary.NetPaid = ratDecimal(net, scale)
	for id, doc := range documents {
		doc.Allocated = allocatedSums[id].Decimal()
		result.Documents = append(result.Documents, *doc)
	}
	for id, doc := range unallocatedDocs {
		doc.Allocated = unallocatedSums[id].Decimal()
		doc.Reason = "customer payment recorded without an order"
		result.Unallocated.Documents = append(result.Unallocated.Documents, *doc)
	}
	result.Unallocated.AdvanceBalance = unallocated.Decimal()
	sortPaymentDocuments(result.Documents)
	sortPaymentDocuments(result.Unallocated.Documents)
	if len(result.Unallocated.Documents) > 50 {
		result.Unallocated.Documents = result.Unallocated.Documents[len(result.Unallocated.Documents)-50:]
		result.Notes = append(result.Notes, "Only the latest 50 payments without an order are listed.")
	}
	if len(result.Documents) > maxOrderPaymentDocuments {
		return OrderPayments{}, errors.New("more than " + strconv.Itoa(maxOrderPaymentDocuments) + " documents allocated to the order")
	}
	s.fillPaymentHeaders(ctx, result.Documents, &result)
	s.fillPaymentHeaders(ctx, result.Unallocated.Documents, &result)
	for index := range result.Documents {
		doc := &result.Documents[index]
		if doc.DocumentAmount == "" || doc.Role == "offset" {
			continue
		}
		outside := new(big.Rat)
		total, ok := new(big.Rat).SetString(string(doc.DocumentAmount))
		allocated, ok2 := new(big.Rat).SetString(string(doc.Allocated))
		if ok && ok2 {
			outside.Sub(total, new(big.Rat).Abs(allocated))
			if outside.Sign() > 0 {
				doc.AllocatedOutside = ratDecimal(outside, 2)
			}
		}
	}
	s.orderPaymentCandidates(ctx, order, end, documents, &result)
	orderPaymentDuplicates(&result)
	if positive(result.Unallocated.AdvanceBalance) {
		result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "unallocated-advance", Reason: "the customer has an advance of " + string(result.Unallocated.AdvanceBalance) + " not allocated to any order; part of it may belong to this order"})
	}
	result.Summary.NeedsReview = len(result.Issues) > 0
	orderPaymentStatus(&result.Summary, net)
	return result, nil
}

func (s Service) findPaymentOrder(ctx context.Context, query OrderPaymentsQuery) (OrderPaymentsOrder, error) {
	plan := config.CustomerOrderHeader
	var row map[string]json.RawMessage
	if query.OrderID != "" {
		data, err := s.OData.Get(ctx, plan.Name+"(guid'"+strings.ToLower(query.OrderID)+"')", url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields)}}, 1<<20)
		if err != nil {
			return OrderPaymentsOrder{}, err
		}
		if err := json.Unmarshal(data, &row); err != nil || row == nil {
			return OrderPaymentsOrder{}, errors.New("invalid OData customer order response")
		}
	} else {
		rows, err := s.scanRows(ctx, plan.Name, sourceFields(plan.Fields), "", maxDocumentScan)
		if err != nil {
			return OrderPaymentsOrder{}, err
		}
		number := strings.TrimSpace(query.Number)
		matches := make([]map[string]json.RawMessage, 0)
		for _, candidate := range rows {
			if !strings.EqualFold(strings.TrimSpace(rawString(candidate["Number"])), number) ||
				query.OrganizationID != "" && !strings.EqualFold(rawString(candidate["Организация_Key"]), query.OrganizationID) ||
				query.CustomerID != "" && !strings.EqualFold(rawString(candidate["Контрагент_Key"]), query.CustomerID) ||
				query.Date != "" && !strings.HasPrefix(rawString(candidate["Date"]), query.Date) {
				continue
			}
			matches = append(matches, candidate)
		}
		switch {
		case len(matches) == 0:
			return OrderPaymentsOrder{}, errors.New("no customer order matches that number and filters")
		case len(matches) > 1:
			found := make([]string, 0, len(matches))
			for _, match := range matches[:min(5, len(matches))] {
				found = append(found, strings.ToLower(rawString(match["Ref_Key"]))+" ("+rawString(match["Date"])[:min(10, len(rawString(match["Date"])))]+")")
			}
			return OrderPaymentsOrder{}, errors.New(strconv.Itoa(len(matches)) + " customer orders match that number; add --organization, --customer or --date, or use the ID: " + strings.Join(found, ", "))
		}
		row = matches[0]
	}
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return OrderPaymentsOrder{}, errors.New("invalid OData customer order flags")
	}
	order, err := bindFields[OrderPaymentsOrder](row, plan.Fields)
	if err != nil || !linkedGUID(order.ID) || query.OrderID != "" && !strings.EqualFold(order.ID, query.OrderID) {
		return OrderPaymentsOrder{}, errors.New("invalid OData customer order")
	}
	order.ID, order.Number = strings.ToLower(order.ID), strings.TrimSpace(order.Number)
	clearUnlinked(&order.CurrencyID, &order.OrganizationID, &order.CustomerID, &order.ContractID)
	order.Resource = plan.Name + "(guid'" + order.ID + "')"
	return order, nil
}

// fillPaymentHeaders reads each document's number, date, status and total; an unreadable header is reported, not fatal.
func (s Service) fillPaymentHeaders(ctx context.Context, docs []OrderPaymentDocument, result *OrderPayments) {
	errs := make([]string, len(docs))
	group, requestContext := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for index := range docs {
		index := index
		group.Go(func() error {
			doc := &docs[index]
			source, known := orderPaymentSources[doc.Resource]
			if !known {
				return nil
			}
			fields := "Ref_Key,Number,Date,Posted,DeletionMark"
			for _, field := range []string{source.amount, source.operation} {
				if field != "" {
					fields += "," + field
				}
			}
			data, err := s.OData.Get(requestContext, doc.Resource+"(guid'"+doc.ID+"')", url.Values{"$format": {"json"}, "$select": {fields}}, 1<<20)
			var row map[string]json.RawMessage
			if err == nil && (json.Unmarshal(data, &row) != nil || row == nil || !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"])) {
				err = errors.New("invalid OData document header")
			}
			if err != nil {
				doc.Error, errs[index] = err.Error(), err.Error()
				return nil
			}
			posted, deleted := string(row["Posted"]) == "true", string(row["DeletionMark"]) == "true"
			doc.Number, doc.Date, doc.Posted, doc.Deleted = strings.TrimSpace(rawString(row["Number"])), rawString(row["Date"]), &posted, &deleted
			if source.amount != "" {
				doc.DocumentAmount = rawDecimal(row[source.amount])
			}
			if source.operation != "" {
				doc.Operation = rawString(row[source.operation])
			}
			return nil
		})
	}
	_ = group.Wait()
	for index, message := range errs {
		if message != "" {
			result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "header-unavailable", DocumentID: docs[index].ID, Reason: "document header could not be read: " + message})
		}
	}
}

// orderPaymentCandidates lists documents that name the order in their header but have no counted allocation to it.
func (s Service) orderPaymentCandidates(ctx context.Context, order OrderPaymentsOrder, end string, counted map[string]*OrderPaymentDocument, result *OrderPayments) {
	resources := make([]string, 0)
	for resource, source := range orderPaymentSources {
		if source.reference != "" {
			resources = append(resources, resource)
		}
	}
	sort.Strings(resources)
	for _, resource := range resources {
		source := orderPaymentSources[resource]
		fields := "Ref_Key,Number,Date,Posted,DeletionMark," + source.amount + "," + source.reference
		if source.typeField != "" {
			fields += "," + source.typeField
		}
		rows, err := s.scanRows(ctx, resource, fields, "", maxDocumentScan)
		scan := TraceScan{Resource: resource, Rows: len(rows)}
		if err != nil {
			scan.Error, result.Summary.Complete = err.Error(), false
			result.Scans = append(result.Scans, scan)
			continue
		}
		result.Scans = append(result.Scans, scan)
		for _, row := range rows {
			if !strings.EqualFold(rawString(row[source.reference]), order.ID) || source.typeField != "" && rawString(row[source.typeField]) != customerOrderType || rawString(row["Date"]) >= end {
				continue
			}
			id := strings.ToLower(rawString(row["Ref_Key"]))
			posted, deleted := string(row["Posted"]) == "true", string(row["DeletionMark"]) == "true"
			doc := OrderPaymentDocument{Kind: source.kind, ID: id, Resource: resource, Number: strings.TrimSpace(rawString(row["Number"])), Date: rawString(row["Date"]), Posted: &posted, Deleted: &deleted, DocumentAmount: rawDecimal(row[source.amount])}
			switch {
			case counted[id] != nil:
				continue
			case deleted:
				doc.Reason = "marked for deletion, so it is not a payment"
				result.NotCounted = append(result.NotCounted, doc)
			case !posted:
				doc.Reason = "not posted, so it is not a confirmed payment"
				result.NotCounted = append(result.NotCounted, doc)
			default:
				doc.Reason = "posted and names the order, but 1C allocated none of it to the order"
				result.NotCounted = append(result.NotCounted, doc)
				result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "reference-without-allocation", DocumentID: id, Reason: "a posted " + source.kind + " names the order but its settlements go elsewhere or are unallocated"})
			}
		}
	}
	sortPaymentDocuments(result.NotCounted)
	result.Notes = append(result.Notes, "Unposted documents are found only when the order is in their header (basis or order field); links only in payment-breakdown rows are not scanned.")
}

// orderPaymentDuplicates flags pairs that may be one payment recorded twice: a fiscal receipt with a card or bank payment of the same amount and day,
// or a bank receipt that looks like an acquirer payout.
func orderPaymentDuplicates(result *OrderPayments) {
	group := map[string]string{"cash-register-receipt": "receipt", "card-payment": "card", "bank-in": "bank"}
	for i, first := range result.Documents {
		if first.Kind == "bank-in" && strings.Contains(strings.ToLower(first.Operation), "эквайр") {
			result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "acquirer-payout", DocumentID: first.ID, Reason: "a bank receipt with an acquiring operation is allocated to the order; check it is not the same card payment counted again"})
		}
		for _, second := range result.Documents[i+1:] {
			if group[first.Kind] == "" || group[second.Kind] == "" || first.Kind == second.Kind || first.Role != "payment" || second.Role != "payment" {
				continue
			}
			if decimalEqual(first.Allocated, second.Allocated) && len(first.Period) >= 10 && len(second.Period) >= 10 && first.Period[:10] == second.Period[:10] {
				result.Issues = append(result.Issues, OrderPaymentIssue{Kind: "possible-duplicate", DocumentID: second.ID, Reason: first.Kind + " " + first.ID + " and " + second.Kind + " " + second.ID + " pay the same amount on the same day; check they are not one payment"})
			}
		}
	}
}

func orderPaymentStatus(summary *OrderPaymentsSummary, net *big.Rat) {
	summary.Remaining, summary.Overpaid = "0", "0"
	amount, ok := new(big.Rat).SetString(string(summary.OrderAmount))
	switch {
	case !summary.Complete:
		summary.Status = "incomplete"
	case !ok:
		summary.Status = "unknown"
		return
	}
	difference := new(big.Rat).Sub(amount, net)
	if difference.Sign() > 0 {
		summary.Remaining = ratDecimal(difference, 2)
	} else if difference.Sign() < 0 {
		summary.Overpaid = ratDecimal(new(big.Rat).Neg(difference), 2)
	}
	if summary.Status != "" {
		return
	}
	switch {
	case net.Sign() < 0:
		summary.Status = "refunded_more_than_paid"
	case net.Sign() == 0:
		summary.Status = "no_allocated_payments"
	case difference.Sign() > 0:
		summary.Status = "partly_paid"
	case difference.Sign() == 0:
		summary.Status = "paid"
	default:
		summary.Status = "overpaid"
	}
}

func sortPaymentDocuments(docs []OrderPaymentDocument) {
	sort.Slice(docs, func(i, j int) bool {
		a, b := docs[i].Period+docs[i].Date, docs[j].Period+docs[j].Date
		if a != b {
			return a < b
		}
		return docs[i].ID < docs[j].ID
	})
}

func positive(value Decimal) bool {
	number, ok := new(big.Rat).SetString(string(value))
	return ok && number.Sign() > 0
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
