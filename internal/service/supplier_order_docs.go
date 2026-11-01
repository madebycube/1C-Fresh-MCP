package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

func computeOrderLine(input SupplierOrderLineInput, number int, unit lineUnit, vatMode string, rates map[string]vatRate) (SupplierOrderPreviewLine, error) {
	quantity, err := exactDecimal(input.Quantity, maxQuantityScale, false)
	if err != nil {
		return SupplierOrderPreviewLine{}, fmt.Errorf("quantity must be positive with at most %d decimals", maxQuantityScale)
	}
	price, err := exactDecimal(input.Price, maxPriceScale, true)
	if err != nil {
		return SupplierOrderPreviewLine{}, fmt.Errorf("price must be nonnegative with at most %d decimals", maxPriceScale)
	}
	percent := new(big.Rat)
	if input.DiscountPercent != "" {
		if percent, err = exactDecimal(input.DiscountPercent, 2, true); err != nil || percent.Cmp(big.NewRat(100, 1)) > 0 {
			return SupplierOrderPreviewLine{}, errors.New("discount percent must be 0–100 with at most 2 decimals")
		}
	}
	rate, ok := rates[input.VATRate]
	if !ok {
		return SupplierOrderPreviewLine{}, errors.New("VAT rate must be one of the 1C rate codes, for example НДС22, НДС5 or БезНДС")
	}
	deliveryDate, err := receiptDate(input.DeliveryDate, true)
	if err != nil {
		return SupplierOrderPreviewLine{}, errors.New("line delivery date must be YYYY-MM-DD")
	}
	gross := roundHalfAway(new(big.Rat).Mul(quantity, price), receiptAmountScale)
	discount := roundHalfAway(new(big.Rat).Quo(new(big.Rat).Mul(gross, percent), big.NewRat(100, 1)), receiptAmountScale)
	amount := new(big.Rat).Sub(gross, discount)
	if input.Amount != "" {
		given, err := exactDecimal(input.Amount, receiptAmountScale, true)
		if err != nil {
			return SupplierOrderPreviewLine{}, fmt.Errorf("amount must be nonnegative with at most %d decimals", receiptAmountScale)
		}
		exact := new(big.Rat).Mul(new(big.Rat).Mul(quantity, price), new(big.Rat).Sub(big.NewRat(1, 1), new(big.Rat).Quo(percent, big.NewRat(100, 1))))
		tolerance := new(big.Rat).Add(new(big.Rat).Mul(quantity, big.NewRat(1, 200)), big.NewRat(1, 100))
		if new(big.Rat).Abs(new(big.Rat).Sub(given, exact)).Cmp(tolerance) > 0 {
			return SupplierOrderPreviewLine{}, fmt.Errorf("amount %s does not match quantity × price less discount %s within rounding", ratDecimal(given, receiptAmountScale), ratDecimal(amount, receiptAmountScale))
		}
		amount = given
		discount = new(big.Rat).Sub(gross, amount)
		if discount.Sign() < 0 {
			discount = new(big.Rat)
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
			return SupplierOrderPreviewLine{}, fmt.Errorf("VAT must be nonnegative with at most %d decimals", receiptAmountScale)
		}
		if rate.NotTaxable && vat.Sign() != 0 || new(big.Rat).Abs(new(big.Rat).Sub(vat, computedVAT)).Cmp(big.NewRat(1, 100)) > 0 {
			return SupplierOrderPreviewLine{}, fmt.Errorf("VAT %s differs from the %s rate's %s by more than one kopeck", ratDecimal(vat, receiptAmountScale), rate.Name, ratDecimal(computedVAT, receiptAmountScale))
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
	return SupplierOrderPreviewLine{
		LineNumber: number, ProductID: strings.ToLower(input.ProductID), CharacteristicID: characteristic, UnitID: unit.ID, UnitType: unit.Type,
		Quantity: ratDecimal(quantity, maxQuantityScale), Price: ratDecimal(price, maxPriceScale), DiscountPercent: ratDecimal(percent, 2), Discount: ratDecimal(discount, receiptAmountScale),
		VATRate: rate.Name, VATRateID: rate.ID, Amount: ratDecimal(amount, receiptAmountScale), VAT: ratDecimal(vat, receiptAmountScale), Total: ratDecimal(total, receiptAmountScale),
		DeliveryDate: deliveryDate,
	}, nil
}

// existingOrderLine keeps the amounts 1C stored; they are recomputed only when the line is changed.
func existingOrderLine(line editableOrderLine, rates map[string]vatRate) (SupplierOrderPreviewLine, error) {
	rateName := ""
	for name, rate := range rates {
		if strings.EqualFold(rate.ID, line.VATRateID) {
			rateName = name
		}
	}
	if rateName == "" {
		return SupplierOrderPreviewLine{}, errors.New("line uses a VAT rate without a rate code")
	}
	if line.UnitType != unitClassifierType && line.UnitType != productUnitType {
		return SupplierOrderPreviewLine{}, errors.New("line uses an unsupported unit type")
	}
	return SupplierOrderPreviewLine{
		LineNumber: line.LineNumber, ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, UnitID: line.UnitID, UnitType: line.UnitType,
		Quantity: line.Quantity, Price: line.Price, DiscountPercent: orZero(line.DiscountPercent), Discount: orZero(line.Discount), VATRate: rateName, VATRateID: line.VATRateID,
		Amount: line.Amount, VAT: line.VAT, Total: line.Total, DeliveryDate: line.DeliveryDate, Preserved: true,
	}, nil
}

func orZero(value Decimal) Decimal {
	if value == "" {
		return "0"
	}
	return value
}

func finishOrderTotals(result *SupplierOrderWrite) {
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
		"saved unposted: nothing is recorded as ordered or planned for payment until it is posted explicitly",
		"does not create a receipt, a payment, a payment approval or a bank transfer",
	}
}

// setOrderPayments turns percents or amounts into a schedule whose amounts add up exactly to the order total.
func (s Service) setOrderPayments(ctx context.Context, result *SupplierOrderWrite, inputs []SupplierOrderPaymentInput, paymentType, bankAccountID, cashBoxID string) error {
	result.Payments = []SupplierOrderPayment{}
	if len(inputs) == 0 {
		if paymentType != "" || bankAccountID != "" || cashBoxID != "" {
			return errors.New("payment type, bank account and cash box need a payment schedule")
		}
		return nil
	}
	if len(inputs) > maxScheduleLines {
		return fmt.Errorf("a payment schedule holds at most %d payments", maxScheduleLines)
	}
	switch paymentType {
	case "", "bank":
		result.PaymentType = "bank"
		if cashBoxID != "" {
			return errors.New("a cash box needs payment type cash")
		}
	case "cash":
		result.PaymentType = "cash"
		if bankAccountID != "" {
			return errors.New("a bank account needs payment type bank")
		}
	default:
		return errors.New("payment type must be bank or cash")
	}
	total, ok := new(big.Rat).SetString(string(result.Amount))
	totalVAT, okVAT := new(big.Rat).SetString(string(result.VAT))
	if !ok || !okVAT || total.Sign() <= 0 {
		return errors.New("a payment schedule needs a positive order total")
	}
	amounts := make([]*big.Rat, len(inputs))
	sum := new(big.Rat)
	byPercent := 0
	for index, input := range inputs {
		if (input.Percent == "") == (input.Amount == "") {
			return fmt.Errorf("payment %d needs either a percent or an amount", index+1)
		}
		if input.Percent != "" {
			percent, err := exactDecimal(input.Percent, 2, false)
			if err != nil || percent.Cmp(big.NewRat(100, 1)) > 0 {
				return fmt.Errorf("payment %d percent must be above 0 and at most 100", index+1)
			}
			amounts[index] = roundHalfAway(new(big.Rat).Quo(new(big.Rat).Mul(total, percent), big.NewRat(100, 1)), receiptAmountScale)
			byPercent++
		} else {
			amount, err := exactDecimal(input.Amount, receiptAmountScale, false)
			if err != nil {
				return fmt.Errorf("payment %d amount must be positive with at most %d decimals", index+1, receiptAmountScale)
			}
			amounts[index] = amount
		}
		sum.Add(sum, amounts[index])
	}
	// Percents that add up to 100 can miss the total by rounding; the last payment absorbs at most one kopeck per payment.
	difference := new(big.Rat).Sub(total, sum)
	if byPercent > 0 && difference.Sign() != 0 && new(big.Rat).Abs(difference).Cmp(big.NewRat(int64(len(inputs)), 100)) <= 0 {
		amounts[len(amounts)-1].Add(amounts[len(amounts)-1], difference)
		sum.Set(total)
	}
	if sum.Cmp(total) != 0 {
		return fmt.Errorf("payments add up to %s but the order total is %s", ratDecimal(sum, receiptAmountScale), result.Amount)
	}
	vatLeft := new(big.Rat).Set(totalVAT)
	previous := ""
	for index, input := range inputs {
		date, err := receiptDate(input.Date, true)
		if err != nil || date == "" {
			return fmt.Errorf("payment %d date must be YYYY-MM-DD", index+1)
		}
		if date < previous {
			return errors.New("payments must be in date order")
		}
		previous = date
		vat := roundHalfAway(new(big.Rat).Quo(new(big.Rat).Mul(totalVAT, amounts[index]), total), receiptAmountScale)
		if index == len(inputs)-1 {
			vat = vatLeft
		}
		vatLeft = new(big.Rat).Sub(vatLeft, vat)
		percent := new(big.Rat).Quo(new(big.Rat).Mul(amounts[index], big.NewRat(100, 1)), total)
		result.Payments = append(result.Payments, SupplierOrderPayment{Date: date, Percent: ratDecimal(roundHalfAway(percent, 2), 2), Amount: ratDecimal(amounts[index], receiptAmountScale), VAT: ratDecimal(vat, receiptAmountScale)})
	}
	if bankAccountID != "" {
		account, err := s.activeRecord(ctx, bankAccountsResourceName, bankAccountID, "Ref_Key,DeletionMark,Owner,Owner_Type,ВалютаДенежныхСредств_Key")
		if err != nil {
			return fmt.Errorf("bank account: %w", err)
		}
		if rawString(account["Owner_Type"]) != organizationOwnerType || !strings.EqualFold(rawString(account["Owner"]), result.OrganizationID) {
			return errors.New("bank account must be one of the organization's own accounts, not the supplier's")
		}
		if !strings.EqualFold(rawString(account["ВалютаДенежныхСредств_Key"]), result.CurrencyID) {
			return errors.New("bank account currency differs from the contract currency")
		}
		result.BankAccountID = strings.ToLower(bankAccountID)
	}
	if cashBoxID != "" {
		if _, err := s.activeRecord(ctx, cashBoxesResourceName, cashBoxID, "Ref_Key,DeletionMark"); err != nil {
			return fmt.Errorf("cash box: %w", err)
		}
		result.CashBoxID = strings.ToLower(cashBoxID)
	}
	return nil
}

var supplierOrderHeaderPlan = config.DocumentResource{Name: supplierOrderResourceName, DateField: "Date", DeletedField: "DeletionMark", Fields: []config.FieldBinding{
	{Output: "id", Source: "Ref_Key"}, {Output: "number", Source: "Number"}, {Output: "date", Source: "Date"}, {Output: "posted", Source: "Posted"},
	{Output: "operation", Source: "ВидОперации"}, {Output: "business_operation_id", Source: "ХозяйственнаяОперация_Key"}, {Output: "state_id", Source: "СостояниеЗаказа_Key"},
	{Output: "include_vat_cost", Source: "НДСВключатьВСтоимость"}, {Output: "organization_id", Source: "Организация_Key"}, {Output: "supplier_id", Source: "Контрагент_Key"},
	{Output: "invoice_number", Source: "НомерВходящегоДокумента"}, {Output: "invoice_date", Source: "ДатаВходящегоДокумента"}, {Output: "amount", Source: "СуммаДокумента"},
}}

type supplierOrderHeader struct {
	ID             string  `json:"id"`
	Number         string  `json:"number"`
	Date           string  `json:"date"`
	Posted         bool    `json:"posted"`
	Operation      string  `json:"operation"`
	BusinessOpID   string  `json:"business_operation_id"`
	StateID        string  `json:"state_id"`
	IncludeVATCost bool    `json:"include_vat_cost"`
	OrganizationID string  `json:"organization_id"`
	SupplierID     string  `json:"supplier_id"`
	InvoiceNumber  string  `json:"invoice_number"`
	InvoiceDate    string  `json:"invoice_date"`
	Amount         Decimal `json:"amount"`
}

func (s Service) supplierOrderHeaders(ctx context.Context) ([]supplierOrderHeader, error) {
	rows, err := s.allDocumentRows(ctx, supplierOrderHeaderPlan)
	if err != nil {
		return nil, fmt.Errorf("could not read supplier orders: %w", err)
	}
	headers := make([]supplierOrderHeader, 0, len(rows))
	for _, row := range rows {
		header, err := bindFields[supplierOrderHeader](row, supplierOrderHeaderPlan.Fields)
		if err != nil {
			return nil, errors.New("invalid OData supplier order fields")
		}
		headers = append(headers, header)
	}
	return headers, nil
}

// supplierOrderTemplate copies the operation, business operation and a starting state from the newest purchase order,
// because their values are configuration data this tool has not been told.
func supplierOrderTemplate(headers []supplierOrderHeader) (orderTemplate, error) {
	candidates := make([]supplierOrderHeader, 0)
	for _, header := range headers {
		if header.Operation == supplierOrderPurchase && linkedGUID(header.BusinessOpID) {
			candidates = append(candidates, header)
		}
	}
	if len(candidates) == 0 {
		return orderTemplate{}, unsupported("no existing purchase order to copy the business operation and state from; create the first supplier order in 1C")
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Date > candidates[j].Date })
	operations := map[string]bool{}
	for _, candidate := range candidates {
		operations[strings.ToLower(candidate.BusinessOpID)] = true
	}
	if len(operations) > 1 {
		return orderTemplate{}, unsupported("existing purchase orders use more than one business operation; this tool cannot choose between them")
	}
	newest := candidates[0]
	template := orderTemplate{Operation: supplierOrderPurchase, BusinessOpID: strings.ToLower(newest.BusinessOpID), IncludeVATCost: newest.IncludeVATCost}
	if linkedGUID(newest.StateID) {
		template.StateID, template.StateFrom = strings.ToLower(newest.StateID), "copied from the newest supplier order "+strings.TrimSpace(newest.Number)+"; pass a state to choose another"
	}
	return template, nil
}

// supplierOrderDuplicates flags orders for the same supplier invoice, or without one the same supplier, day and total.
func supplierOrderDuplicates(headers []supplierOrderHeader, draft SupplierOrderWrite, exceptID string) []SupplierOrderDuplicate {
	duplicates := []SupplierOrderDuplicate{}
	for _, header := range headers {
		if strings.EqualFold(header.ID, exceptID) || !strings.EqualFold(header.OrganizationID, draft.OrganizationID) || !strings.EqualFold(header.SupplierID, draft.SupplierID) {
			continue
		}
		reason := ""
		number := strings.TrimSpace(header.InvoiceNumber)
		switch {
		case draft.InvoiceNumber != "" && strings.EqualFold(number, draft.InvoiceNumber) && (draft.InvoiceDate == "" || strings.HasPrefix(header.InvoiceDate, draft.InvoiceDate)):
			reason = "same supplier invoice"
		case draft.InvoiceNumber == "" && len(header.Date) >= 10 && header.Date[:10] == draft.Date[:10] && decimalEqual(header.Amount, draft.Amount):
			reason = "same supplier, day and total"
		}
		if reason != "" {
			duplicates = append(duplicates, SupplierOrderDuplicate{ID: header.ID, Number: strings.TrimSpace(header.Number), Date: header.Date, Posted: header.Posted, InvoiceNumber: number, Amount: header.Amount, Reason: reason})
		}
	}
	return duplicates
}

func orderHeaderFields(result SupplierOrderWrite) map[string]any {
	fields := map[string]any{
		"Date": result.Date, "Организация_Key": result.OrganizationID, "Контрагент_Key": result.SupplierID, "Договор_Key": result.ContractID,
		"ВалютаДокумента_Key": result.CurrencyID, "СтруктурнаяЕдиницаРезерв_Key": result.WarehouseID, "СтруктурнаяЕдиница_Key": result.DepartmentID,
		"НомерВходящегоДокумента": result.InvoiceNumber, "Комментарий": result.Comment, "СуммаВключаетНДС": result.VATMode == receiptVATIncluded,
		"НалогообложениеНДС": result.Taxation, "СуммаДокумента": json.Number(result.Amount), "СостояниеЗаказа_Key": orEmptyGUID(result.StateID),
	}
	fields["ДатаВходящегоДокумента"] = "0001-01-01T00:00:00"
	if result.InvoiceDate != "" {
		fields["ДатаВходящегоДокумента"] = result.InvoiceDate + "T00:00:00"
	}
	fields["ДатаПоступления"] = "0001-01-01T00:00:00"
	fields["ПоложениеДатыПоступления"] = "ВШапке"
	if result.DeliveryDate != "" {
		fields["ДатаПоступления"] = result.DeliveryDate + "T00:00:00"
	}
	for _, line := range result.Lines {
		if line.DeliveryDate != "" && line.DeliveryDate != result.DeliveryDate {
			fields["ПоложениеДатыПоступления"] = "ВСтроках"
		}
	}
	schedule := make([]map[string]any, 0, len(result.Payments))
	for index, payment := range result.Payments {
		schedule = append(schedule, map[string]any{"LineNumber": fmt.Sprint(index + 1), "ДатаОплаты": payment.Date + "T00:00:00", "ПроцентОплаты": json.Number(payment.Percent), "СуммаОплаты": json.Number(payment.Amount), "СуммаНДСОплаты": json.Number(payment.VAT)})
	}
	fields["ПлатежныйКалендарь"] = schedule
	fields["ЗапланироватьОплату"] = len(schedule) > 0
	fields["ТипДенежныхСредств"] = paymentTypeBank
	if result.PaymentType == "cash" {
		fields["ТипДенежныхСредств"] = paymentTypeCash
	}
	fields["БанковскийСчет_Key"] = orEmptyGUID(result.BankAccountID)
	fields["Касса_Key"] = orEmptyGUID(result.CashBoxID)
	return fields
}

// orderLineFields starts from the stored row, so fields this tool does not manage survive an amendment.
func orderLineFields(line SupplierOrderPreviewLine, result SupplierOrderWrite, original map[string]any) map[string]any {
	row := make(map[string]any, len(original)+14)
	for key, value := range original {
		row[key] = value
	}
	if line.Preserved {
		return row
	}
	characteristic := line.CharacteristicID
	if characteristic == "" {
		characteristic = emptyGUID
	}
	delivery := line.DeliveryDate
	if delivery == "" {
		delivery = result.DeliveryDate
	}
	row["LineNumber"] = fmt.Sprint(line.LineNumber)
	row["Номенклатура_Key"] = line.ProductID
	row["Характеристика_Key"] = characteristic
	row["ЕдиницаИзмерения"] = line.UnitID
	row["ЕдиницаИзмерения_Type"] = line.UnitType
	row["Количество"] = json.Number(line.Quantity)
	row["Цена"] = json.Number(line.Price)
	row["ПроцентСкидкиНаценки"] = json.Number(line.DiscountPercent)
	row["СуммаСкидкиНаценки"] = json.Number(line.Discount)
	row["Сумма"] = json.Number(line.Amount)
	row["СтавкаНДС_Key"] = line.VATRateID
	row["СуммаНДС"] = json.Number(line.VAT)
	row["Всего"] = json.Number(line.Total)
	row["ДатаПоступления"] = "0001-01-01T00:00:00"
	if delivery != "" {
		row["ДатаПоступления"] = delivery + "T00:00:00"
	}
	row["СтруктурнаяЕдиницаРезерв_Key"] = result.WarehouseID
	return row
}

func supplierOrderResource(id string) string {
	return supplierOrderResourceName + "(guid'" + strings.ToLower(id) + "')"
}

type editableOrderLine struct {
	LineNumber       int
	ProductID        string
	CharacteristicID string
	UnitID           string
	UnitType         string
	Quantity         Decimal
	Price            Decimal
	DiscountPercent  Decimal
	Discount         Decimal
	VATRateID        string
	Amount           Decimal
	VAT              Decimal
	Total            Decimal
	DeliveryDate     string
	Raw              map[string]any
}

type cancelledOrderLine struct {
	ProductID        string
	CharacteristicID string
	UnitID           string
	Quantity         Decimal
}

type editableOrder struct {
	ID             string
	Number         string
	Date           string
	DataVersion    string
	Posted         bool
	Operation      string
	OrganizationID string
	SupplierID     string
	ContractID     string
	CurrencyID     string
	WarehouseID    string
	DepartmentID   string
	InvoiceNumber  string
	InvoiceDate    string
	DeliveryDate   string
	StateID        string
	Comment        string
	IncludesVAT    bool
	Taxation       string
	Amount         Decimal
	VAT            Decimal
	PaymentType    string
	BankAccountID  string
	CashBoxID      string
	Schedule       []SupplierOrderPayment
	Cancelled      []cancelledOrderLine
	Lines          []editableOrderLine
}

func (o editableOrder) draft() SupplierOrderDraft {
	mode := receiptVATExcluded
	if o.IncludesVAT {
		mode = receiptVATIncluded
	}
	return SupplierOrderDraft{OrganizationID: o.OrganizationID, SupplierID: o.SupplierID, ContractID: o.ContractID, WarehouseID: o.WarehouseID, DepartmentID: o.DepartmentID, Date: o.Date, InvoiceNumber: o.InvoiceNumber, InvoiceDate: o.InvoiceDate, DeliveryDate: o.DeliveryDate, StateID: o.StateID, Comment: o.Comment, VATMode: mode}
}

func (o editableOrder) paymentTypeName() string {
	if o.PaymentType == paymentTypeCash {
		return "cash"
	}
	return "bank"
}

func (o editableOrder) paymentInputs() []SupplierOrderPaymentInput {
	inputs := make([]SupplierOrderPaymentInput, 0, len(o.Schedule))
	for _, payment := range o.Schedule {
		inputs = append(inputs, SupplierOrderPaymentInput{Date: payment.Date, Percent: string(payment.Percent)})
	}
	return inputs
}

func (s Service) readEditableOrder(ctx context.Context, id string) (editableOrder, error) {
	if !linkedGUID(id) {
		return editableOrder{}, errors.New("supplier order ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, supplierOrderResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return editableOrder{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return editableOrder{}, errors.New("invalid OData supplier order response")
	}
	if string(row["DeletionMark"]) != "false" {
		return editableOrder{}, errors.New("supplier order is marked for deletion")
	}
	if rawString(row["DataVersion"]) == "" {
		return editableOrder{}, errors.New("supplier order has no data version for a safe update")
	}
	dateOnly := func(field string) string {
		if value := rawString(row[field]); !strings.HasPrefix(value, "0001-01-01") && len(value) >= 10 {
			return value[:10]
		}
		return ""
	}
	order := editableOrder{
		ID: strings.ToLower(id), Number: strings.TrimSpace(rawString(row["Number"])), Date: rawString(row["Date"]), DataVersion: rawString(row["DataVersion"]),
		Posted: string(row["Posted"]) == "true", Operation: rawString(row["ВидОперации"]),
		OrganizationID: strings.ToLower(rawString(row["Организация_Key"])), SupplierID: strings.ToLower(rawString(row["Контрагент_Key"])), ContractID: strings.ToLower(rawString(row["Договор_Key"])),
		CurrencyID: strings.ToLower(rawString(row["ВалютаДокумента_Key"])), WarehouseID: strings.ToLower(rawString(row["СтруктурнаяЕдиницаРезерв_Key"])), DepartmentID: strings.ToLower(rawString(row["СтруктурнаяЕдиница_Key"])),
		InvoiceNumber: strings.TrimSpace(rawString(row["НомерВходящегоДокумента"])), InvoiceDate: dateOnly("ДатаВходящегоДокумента"), DeliveryDate: dateOnly("ДатаПоступления"),
		StateID: strings.ToLower(rawString(row["СостояниеЗаказа_Key"])), Comment: rawString(row["Комментарий"]), IncludesVAT: string(row["СуммаВключаетНДС"]) == "true",
		Taxation: rawString(row["НалогообложениеНДС"]), Amount: rawDecimal(row["СуммаДокумента"]), PaymentType: rawString(row["ТипДенежныхСредств"]),
		BankAccountID: strings.ToLower(rawString(row["БанковскийСчет_Key"])), CashBoxID: strings.ToLower(rawString(row["Касса_Key"])),
		Schedule: []SupplierOrderPayment{}, Cancelled: []cancelledOrderLine{}, Lines: []editableOrderLine{},
	}
	clearUnlinked(&order.StateID, &order.BankAccountID, &order.CashBoxID, &order.WarehouseID, &order.DepartmentID)
	decodeRows := func(field string) ([]map[string]any, error) {
		var rows []map[string]any
		if raw, ok := row[field]; ok && string(raw) != "null" {
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.UseNumber()
			if err := decoder.Decode(&rows); err != nil {
				return nil, errors.New("invalid OData supplier order " + field)
			}
		}
		for _, line := range rows {
			for key := range line {
				if strings.Contains(key, "@") {
					delete(line, key)
				}
			}
		}
		return rows, nil
	}
	lines, err := decodeRows("Запасы")
	if err != nil {
		return editableOrder{}, err
	}
	vat := &stockSum{}
	for _, line := range lines {
		var lineNumber int
		if _, err := fmt.Sscan(anyString(line["LineNumber"]), &lineNumber); err != nil || lineNumber < 1 {
			return editableOrder{}, errors.New("invalid OData supplier order line number")
		}
		parsed := editableOrderLine{
			LineNumber: lineNumber, ProductID: strings.ToLower(anyString(line["Номенклатура_Key"])), CharacteristicID: strings.ToLower(anyString(line["Характеристика_Key"])),
			UnitID: strings.ToLower(anyString(line["ЕдиницаИзмерения"])), UnitType: anyString(line["ЕдиницаИзмерения_Type"]),
			Quantity: anyDecimal(line["Количество"]), Price: anyDecimal(line["Цена"]), DiscountPercent: anyDecimal(line["ПроцентСкидкиНаценки"]), Discount: anyDecimal(line["СуммаСкидкиНаценки"]),
			VATRateID: strings.ToLower(anyString(line["СтавкаНДС_Key"])), Amount: anyDecimal(line["Сумма"]), VAT: anyDecimal(line["СуммаНДС"]), Total: anyDecimal(line["Всего"]), Raw: line,
		}
		if delivery := anyString(line["ДатаПоступления"]); !strings.HasPrefix(delivery, "0001-01-01") && len(delivery) >= 10 {
			parsed.DeliveryDate = delivery[:10]
		}
		if parsed.CharacteristicID == emptyGUID {
			parsed.CharacteristicID = ""
		}
		_ = vat.Add(parsed.VAT)
		order.Lines = append(order.Lines, parsed)
	}
	order.VAT = vat.Decimal()
	schedule, err := decodeRows("ПлатежныйКалендарь")
	if err != nil {
		return editableOrder{}, err
	}
	for _, line := range schedule {
		date := anyString(line["ДатаОплаты"])
		if len(date) >= 10 {
			date = date[:10]
		}
		order.Schedule = append(order.Schedule, SupplierOrderPayment{Date: date, Percent: anyDecimal(line["ПроцентОплаты"]), Amount: anyDecimal(line["СуммаОплаты"]), VAT: anyDecimal(line["СуммаНДСОплаты"])})
	}
	cancelled, err := decodeRows("ОтмененныеЗапасы")
	if err != nil {
		return editableOrder{}, err
	}
	for _, line := range cancelled {
		characteristic := strings.ToLower(anyString(line["Характеристика_Key"]))
		if characteristic == emptyGUID {
			characteristic = ""
		}
		order.Cancelled = append(order.Cancelled, cancelledOrderLine{ProductID: strings.ToLower(anyString(line["Номенклатура_Key"])), CharacteristicID: characteristic, UnitID: strings.ToLower(anyString(line["ЕдиницаИзмерения"])), Quantity: anyDecimal(line["Количество"])})
	}
	return order, nil
}

func verifyOrderSaved(saved editableOrder, want SupplierOrderWrite) error {
	if saved.Posted && !want.Posted {
		return errors.New("1C saved the order as posted")
	}
	if !strings.EqualFold(saved.SupplierID, want.SupplierID) || !strings.EqualFold(saved.ContractID, want.ContractID) || !strings.EqualFold(saved.WarehouseID, want.WarehouseID) || !strings.EqualFold(saved.DepartmentID, want.DepartmentID) || saved.InvoiceNumber != want.InvoiceNumber {
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
	if len(saved.Schedule) != len(want.Payments) {
		return errors.New("payment schedule does not match")
	}
	for index, payment := range saved.Schedule {
		if payment.Date != want.Payments[index].Date || !decimalEqual(payment.Amount, want.Payments[index].Amount) {
			return fmt.Errorf("payment %d does not match", index+1)
		}
	}
	return nil
}
