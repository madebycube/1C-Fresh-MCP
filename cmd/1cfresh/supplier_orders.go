package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func parseOrderLine(value string) (service.SupplierOrderLineInput, error) {
	usage := errors.New("line must be PRODUCT_GUID:QUANTITY:PRICE:VAT_CODE[:CHARACTERISTIC_GUID][:unit=GUID][:discount=PERCENT][:amount=A][:vat=V][:delivery=YYYY-MM-DD]")
	parts := strings.Split(value, ":")
	if len(parts) < 4 {
		return service.SupplierOrderLineInput{}, usage
	}
	line := service.SupplierOrderLineInput{ProductID: parts[0], Quantity: parts[1], Price: parts[2], VATRate: parts[3]}
	seen := map[string]bool{}
	for index, part := range parts[4:] {
		key, option, ok := strings.Cut(part, "=")
		if !ok && index == 0 {
			line.CharacteristicID = part
			continue
		}
		if !ok || seen[key] {
			return service.SupplierOrderLineInput{}, usage
		}
		seen[key] = true
		switch key {
		case "unit":
			line.UnitID = option
		case "discount":
			line.DiscountPercent = option
		case "amount":
			line.Amount = option
		case "vat":
			line.VAT = option
		case "delivery":
			line.DeliveryDate = option
		default:
			return service.SupplierOrderLineInput{}, usage
		}
	}
	return line, nil
}

func parseOrderPayment(value string) (service.SupplierOrderPaymentInput, error) {
	date, rest, ok := strings.Cut(value, ":")
	key, amount, okValue := strings.Cut(rest, "=")
	if !ok || !okValue || key != "percent" && key != "amount" {
		return service.SupplierOrderPaymentInput{}, errors.New("payment must be YYYY-MM-DD:percent=P or YYYY-MM-DD:amount=A")
	}
	payment := service.SupplierOrderPaymentInput{Date: date}
	if key == "percent" {
		payment.Percent = amount
	} else {
		payment.Amount = amount
	}
	return payment, nil
}

func parseOrderLineChange(value string) (service.SupplierOrderLineChange, error) {
	usage := errors.New("set-line must be N:quantity=Q,price=P,vat=CODE,discount=PERCENT,amount=A,vat-amount=V,delivery=YYYY-MM-DD")
	number, assignments, ok := strings.Cut(value, ":")
	lineNumber, err := strconv.Atoi(number)
	if !ok || err != nil || lineNumber < 1 {
		return service.SupplierOrderLineChange{}, usage
	}
	change := service.SupplierOrderLineChange{LineNumber: lineNumber}
	targets := map[string]**string{"quantity": &change.Quantity, "price": &change.Price, "vat": &change.VATRate, "discount": &change.DiscountPercent, "amount": &change.Amount, "vat-amount": &change.VAT, "delivery": &change.DeliveryDate}
	for _, assignment := range strings.Split(assignments, ",") {
		key, raw, ok := strings.Cut(assignment, "=")
		target, known := targets[key]
		if !ok || !known {
			return service.SupplierOrderLineChange{}, usage
		}
		value := raw
		*target = &value
	}
	return change, nil
}

func runSupplierOrderGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetSupplierOrder
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	status, err := svc.GetSupplierOrderStatus(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(status)
	}
	if _, err := fmt.Fprintf(out, "Supplier order %s %s\nPosted: %t\nSupplier ID: %s\nContract ID: %s\nWarehouse ID: %s\nInvoice: %s %s\nDelivery: %s\nTotal: %s, VAT %s\nDelivery status: %s\nPayment status: %s, paid %s, returned %s, outstanding %s\nID: %s\nData version: %s\n",
		flat(status.Number), status.Date, status.Posted, status.SupplierID, status.ContractID, status.WarehouseID, flat(status.InvoiceNumber), status.InvoiceDate, status.DeliveryDate,
		status.Amount, status.VAT, status.DeliveryStatus, status.Payments.Status, status.Payments.Paid, status.Payments.Returned, status.Payments.Outstanding, status.ID, status.DataVersion); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Line  Product  Ordered  Cancelled  Received  Pending  Expected  Amount"); err != nil {
		return err
	}
	for _, line := range status.Lines {
		if _, err := fmt.Fprintf(out, "%d  %s  %s  %s  %s  %s  %s  %s\n", line.LineNumber, line.ProductID, line.Ordered, line.Cancelled, line.Received, line.Pending, line.Expected, line.Amount); err != nil {
			return err
		}
	}
	for _, receipt := range status.Receipts {
		if _, err := fmt.Fprintf(out, "Receipt: %s %s posted %t total %s (%s)\n", flat(receipt.Number), receipt.Date, receipt.Posted, receipt.Amount, receipt.ID); err != nil {
			return err
		}
	}
	for _, payment := range status.Schedule {
		if _, err := fmt.Fprintf(out, "Planned payment: %s %s (%s%%)\n", payment.Date, payment.Amount, payment.Percent); err != nil {
			return err
		}
	}
	for _, document := range status.Payments.Documents {
		if _, err := fmt.Fprintf(out, "Payment document: %s %s %s posted %t for order %s counted %t\n", document.Kind, flat(document.Number), document.Date, document.Posted, document.Amount, document.Counted); err != nil {
			return err
		}
	}
	for _, note := range append(status.Notes, status.Payments.Notes...) {
		if _, err := fmt.Fprintln(out, "Note: "+flat(note)); err != nil {
			return err
		}
	}
	return nil
}

func runSupplierOrderCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreateSupplierOrder
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.SupplierOrderDraft
	var lines, payments repeated
	for name, target := range map[string]*string{"organization": &input.OrganizationID, "supplier": &input.SupplierID, "contract": &input.ContractID, "warehouse": &input.WarehouseID, "department": &input.DepartmentID, "date": &input.Date, "vat": &input.VATMode, "invoice-number": &input.InvoiceNumber, "invoice-date": &input.InvoiceDate, "delivery-date": &input.DeliveryDate, "state": &input.StateID, "comment": &input.Comment, "payment-type": &input.PaymentType, "bank-account": &input.BankAccountID, "cash-box": &input.CashBoxID} {
		flags.StringVar(target, name, "", name)
	}
	flags.Var(&lines, "line", "goods line")
	flags.Var(&payments, "payment", "planned payment")
	flags.BoolVar(&input.AllowDuplicate, "allow-duplicate", false, "save despite a likely duplicate")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	for _, value := range lines {
		line, err := parseOrderLine(value)
		if err != nil {
			return err
		}
		input.Lines = append(input.Lines, line)
	}
	for _, value := range payments {
		payment, err := parseOrderPayment(value)
		if err != nil {
			return err
		}
		input.Payments = append(input.Payments, payment)
	}
	result, err := svc.CreateSupplierOrderDraft(ctx, input)
	if err != nil {
		return err
	}
	return printSupplierOrderWrite(out, result, *asJSON)
}

func runSupplierOrderUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.UpdateSupplierOrder
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.SupplierOrderAmendment
	var setLines, addLines, payments repeated
	text := map[string]**string{"date": &change.Date, "warehouse": &change.WarehouseID, "department": &change.DepartmentID, "invoice-number": &change.InvoiceNumber, "invoice-date": &change.InvoiceDate, "delivery-date": &change.DeliveryDate, "state": &change.StateID, "comment": &change.Comment, "payment-type": &change.PaymentType, "bank-account": &change.BankAccountID, "cash-box": &change.CashBoxID}
	values := map[string]*string{}
	for name := range text {
		values[name] = flags.String(name, "", name)
	}
	flags.Var(&setLines, "set-line", "line change")
	flags.Var(&addLines, "add-line", "new goods line")
	flags.Var(&payments, "payment", "planned payment")
	flags.BoolVar(&change.ClearPayments, "clear-payments", false, "remove the payment schedule")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&change.Repost, "repost", false, "unpost a posted order, amend it, and post it again")
	flags.BoolVar(&change.Apply, "apply", false, "save the change")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	flags.Visit(func(option *flag.Flag) {
		if target, ok := text[option.Name]; ok {
			*target = values[option.Name]
		}
	})
	for _, value := range setLines {
		lineChange, err := parseOrderLineChange(value)
		if err != nil {
			return err
		}
		change.ChangeLines = append(change.ChangeLines, lineChange)
	}
	for _, value := range addLines {
		line, err := parseOrderLine(value)
		if err != nil {
			return err
		}
		change.AddLines = append(change.AddLines, line)
	}
	for _, value := range payments {
		payment, err := parseOrderPayment(value)
		if err != nil {
			return err
		}
		change.Payments = append(change.Payments, payment)
	}
	result, err := svc.AmendSupplierOrder(ctx, flags.Arg(0), change)
	if err != nil {
		return err
	}
	return printSupplierOrderWrite(out, result, *asJSON)
}

func runSupplierOrderPosting(ctx context.Context, svc service.Service, args []string, out io.Writer, action string) error {
	spec := operations.PostSupplierOrder
	if action == "unpost" {
		spec = operations.UnpostSupplierOrder
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "run the action")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetSupplierOrderPosting(ctx, flags.Arg(0), action, *expected, *apply)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview"
	if result.Applied {
		state = "Done"
	}
	if _, err := fmt.Fprintf(out, "%s: %s supplier order %s\nPosted: %t -> %t\nAmount: %s, lines: %d\nData version: %s\n", state, action, result.ID, result.PostedFrom, result.Posted, result.Amount, result.Lines, result.DataVersion); err != nil {
		return err
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied && result.PostedFrom != (action == "post") {
		_, err := fmt.Fprintln(out, "Run again with --apply to "+action+".")
		return err
	}
	return nil
}

func runReceiptFromOrder(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreateReceiptFromOrder
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.SupplierOrderReceiptDraft
	var lines repeated
	flags.StringVar(&input.Date, "date", "", "accounting date")
	flags.StringVar(&input.WarehouseID, "warehouse", "", "warehouse GUID")
	flags.StringVar(&input.IncomingNumber, "incoming-number", "", "supplier document number")
	flags.StringVar(&input.IncomingDate, "incoming-date", "", "supplier document date")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.Var(&lines, "line", "received order line")
	flags.BoolVar(&input.AllRemaining, "all-remaining", false, "receive everything still expected")
	flags.BoolVar(&input.AllowDuplicate, "allow-duplicate", false, "save despite a likely duplicate")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	input.OrderID = flags.Arg(0)
	for _, value := range lines {
		number, quantity, ok := strings.Cut(value, ":")
		lineNumber, err := strconv.Atoi(number)
		if !ok || err != nil || lineNumber < 1 {
			return errors.New("line must be ORDER_LINE:QUANTITY")
		}
		input.Lines = append(input.Lines, service.OrderReceiptLine{LineNumber: lineNumber, Quantity: quantity})
	}
	result, err := svc.CreateReceiptFromOrder(ctx, input)
	if err != nil {
		return err
	}
	return printReceiptWrite(out, result, *asJSON)
}

func runReceiptOrderLink(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.LinkReceiptToOrder
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.ReceiptOrderLinkChange
	flags.BoolVar(&change.Repost, "repost", false, "unpost a posted receipt, link it, and post it again")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&change.Apply, "apply", false, "save the link")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 2 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.LinkReceiptToOrder(ctx, flags.Arg(0), flags.Arg(1), change)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	switch {
	case result.Unchanged:
		state = "Already linked"
	case result.Applied:
		state = "Linked"
	}
	if _, err := fmt.Fprintf(out, "%s: receipt %s %s -> supplier order %s %s\nPosted: %t\nData version: %s\n", state, flat(result.ReceiptNumber), result.ReceiptID, flat(result.OrderNumber), result.OrderID, result.Posted, result.DataVersion); err != nil {
		return err
	}
	for _, line := range result.Lines {
		if _, err := fmt.Fprintf(out, "  receipt line %d  %s  %s -> order line %d (expected %s) %s\n", line.ReceiptLine, line.ProductID, line.Quantity, line.OrderLine, line.Expected, flat(line.Problem)); err != nil {
			return err
		}
	}
	for _, problem := range result.Problems {
		if _, err := fmt.Fprintln(out, "Problem: "+flat(problem)); err != nil {
			return err
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied && !result.Unchanged && len(result.Problems) == 0 {
		_, err := fmt.Fprintln(out, "Run again with --apply to link.")
		return err
	}
	return nil
}

func printSupplierOrderWrite(out io.Writer, result service.SupplierOrderWrite, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved supplier order " + flat(result.Number) + " " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nPosted: %t\nDate: %s\nInvoice: %s %s\nDelivery: %s\nSupplier ID: %s\nContract ID: %s\nWarehouse ID: %s\nDepartment ID: %s\nState ID: %s %s\nVAT: %s, %s\nTotal: %s, VAT %s\n", state, result.Posted, result.Date, flat(result.InvoiceNumber), result.InvoiceDate, result.DeliveryDate, result.SupplierID, result.ContractID, result.WarehouseID, result.DepartmentID, result.StateID, flat(result.StateSource), result.VATMode, result.Taxation, result.Amount, result.VAT); err != nil {
		return err
	}
	if result.Before != nil {
		if _, err := fmt.Fprintf(out, "Before: total %s, VAT %s, %d lines\nChanges: %s\nData version: %s\n", result.Before.Amount, result.Before.VAT, result.Before.Lines, strings.Join(result.Changes, ", "), result.DataVersion); err != nil {
			return err
		}
	}
	for _, line := range result.Lines {
		if _, err := fmt.Fprintf(out, "  %d  %s  %s × %s  discount %s%% (%s)  %s  VAT %s  total %s  delivery %s\n", line.LineNumber, line.ProductID, line.Quantity, line.Price, line.DiscountPercent, line.Discount, line.VATRate, line.VAT, line.Total, or(service.Decimal(line.DeliveryDate), service.Decimal(result.DeliveryDate))); err != nil {
			return err
		}
	}
	for _, payment := range result.Payments {
		if _, err := fmt.Fprintf(out, "Planned payment: %s %s (%s%%), VAT %s\n", payment.Date, payment.Amount, payment.Percent, payment.VAT); err != nil {
			return err
		}
	}
	for _, duplicate := range result.Duplicates {
		if _, err := fmt.Fprintf(out, "Possible duplicate: %s %s posted %t, %s (%s)\n", flat(duplicate.Number), duplicate.Date, duplicate.Posted, duplicate.Reason, duplicate.ID); err != nil {
			return err
		}
	}
	for _, note := range result.Unsupported {
		if _, err := fmt.Fprintln(out, "Not supported: "+note); err != nil {
			return err
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied {
		_, err := fmt.Fprintln(out, "Run again with --apply to save.")
		return err
	}
	return nil
}
