package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

type repeated []string

func (r *repeated) String() string         { return strings.Join(*r, " ") }
func (r *repeated) Set(value string) error { *r = append(*r, value); return nil }

func parseReceiptLine(value string) (service.ReceiptLineInput, error) {
	usage := errors.New("line must be PRODUCT_GUID:QUANTITY:PRICE:VAT_CODE[:CHARACTERISTIC_GUID][:unit=GUID][:amount=A][:vat=V]")
	parts := strings.Split(value, ":")
	if len(parts) < 4 {
		return service.ReceiptLineInput{}, usage
	}
	line := service.ReceiptLineInput{ProductID: parts[0], Quantity: parts[1], Price: parts[2], VATRate: parts[3]}
	seen := map[string]bool{}
	for index, part := range parts[4:] {
		key, option, ok := strings.Cut(part, "=")
		if !ok && index == 0 {
			line.CharacteristicID = part
			continue
		}
		if !ok || seen[key] {
			return service.ReceiptLineInput{}, usage
		}
		seen[key] = true
		switch key {
		case "unit":
			line.UnitID = option
		case "amount":
			line.Amount = option
		case "vat":
			line.VAT = option
		default:
			return service.ReceiptLineInput{}, usage
		}
	}
	return line, nil
}

func parseLineChange(value string) (service.ReceiptLineChange, error) {
	number, assignments, ok := strings.Cut(value, ":")
	lineNumber, err := strconv.Atoi(number)
	if !ok || err != nil || lineNumber < 1 {
		return service.ReceiptLineChange{}, errors.New("set-line must be N:quantity=Q,price=P,vat=CODE,amount=A,vat-amount=V")
	}
	change := service.ReceiptLineChange{LineNumber: lineNumber}
	for _, assignment := range strings.Split(assignments, ",") {
		key, raw, ok := strings.Cut(assignment, "=")
		value := raw
		switch {
		case !ok:
			return service.ReceiptLineChange{}, errors.New("set-line must be N:quantity=Q,price=P,vat=CODE,amount=A,vat-amount=V")
		case key == "quantity":
			change.Quantity = &value
		case key == "price":
			change.Price = &value
		case key == "vat":
			change.VATRate = &value
		case key == "amount":
			change.Amount = &value
		case key == "vat-amount":
			change.VAT = &value
		default:
			return service.ReceiptLineChange{}, errors.New("set-line accepts quantity, price, vat, amount, and vat-amount")
		}
	}
	return change, nil
}

func runReceiptCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreateReceipt
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.ReceiptDraft
	var lines repeated
	flags.StringVar(&input.OrganizationID, "organization", "", "organization GUID")
	flags.StringVar(&input.SupplierID, "supplier", "", "supplier GUID")
	flags.StringVar(&input.ContractID, "contract", "", "supplier contract GUID")
	flags.StringVar(&input.WarehouseID, "warehouse", "", "warehouse GUID")
	flags.StringVar(&input.DepartmentID, "department", "", "department GUID")
	flags.StringVar(&input.Date, "date", "", "accounting date")
	flags.StringVar(&input.VATMode, "vat", "", "inclusive or exclusive prices")
	flags.StringVar(&input.IncomingNumber, "incoming-number", "", "supplier document number")
	flags.StringVar(&input.IncomingDate, "incoming-date", "", "supplier document date")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.Var(&lines, "line", "goods line")
	flags.BoolVar(&input.AllowDuplicate, "allow-duplicate", false, "save despite a likely duplicate")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	for _, value := range lines {
		line, err := parseReceiptLine(value)
		if err != nil {
			return err
		}
		input.Lines = append(input.Lines, line)
	}
	result, err := svc.CreateReceiptDraft(ctx, input)
	if err != nil {
		return err
	}
	return printReceiptWrite(out, result, *asJSON)
}

func runReceiptUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.UpdateReceipt
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.ReceiptAmendment
	var setLines, addLines repeated
	text := map[string]**string{"date": &change.Date, "warehouse": &change.WarehouseID, "department": &change.DepartmentID, "incoming-number": &change.IncomingNumber, "incoming-date": &change.IncomingDate, "comment": &change.Comment}
	values := map[string]*string{}
	for name := range text {
		values[name] = flags.String(name, "", name)
	}
	flags.Var(&setLines, "set-line", "line change")
	flags.Var(&addLines, "add-line", "new goods line")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
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
		lineChange, err := parseLineChange(value)
		if err != nil {
			return err
		}
		change.ChangeLines = append(change.ChangeLines, lineChange)
	}
	for _, value := range addLines {
		line, err := parseReceiptLine(value)
		if err != nil {
			return err
		}
		change.AddLines = append(change.AddLines, line)
	}
	result, err := svc.AmendReceipt(ctx, flags.Arg(0), change)
	if err != nil {
		return err
	}
	return printReceiptWrite(out, result, *asJSON)
}

func runReceiptPosting(ctx context.Context, svc service.Service, args []string, out io.Writer, action string) error {
	spec := operations.PostReceipt
	if action == "unpost" {
		spec = operations.UnpostReceipt
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "run the action")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetReceiptPosting(ctx, flags.Arg(0), action, *expected, *apply)
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
	if _, err := fmt.Fprintf(out, "%s: %s receipt %s\nPosted: %t -> %t\nAmount: %s, lines: %d\nData version: %s\n", state, action, result.ID, result.PostedFrom, result.Posted, result.Amount, result.Lines, result.DataVersion); err != nil {
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

func printReceiptWrite(out io.Writer, result service.ReceiptWrite, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved unposted receipt " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nDate: %s\nIncoming: %s %s\nSupplier ID: %s\nContract ID: %s\nWarehouse ID: %s\nDepartment ID: %s\nVAT: %s, %s\nTotal: %s, VAT %s\n", state, result.Date, flat(result.IncomingNumber), result.IncomingDate, result.SupplierID, result.ContractID, result.WarehouseID, result.DepartmentID, result.VATMode, result.Taxation, result.Amount, result.VAT); err != nil {
		return err
	}
	if result.Before != nil {
		if _, err := fmt.Fprintf(out, "Before: total %s, VAT %s, %d lines\nChanges: %s\nData version: %s\n", result.Before.Amount, result.Before.VAT, result.Before.Lines, strings.Join(result.Changes, ", "), result.DataVersion); err != nil {
			return err
		}
	}
	for _, line := range result.Lines {
		if _, err := fmt.Fprintf(out, "  %d  %s  %s × %s  %s  VAT %s  total %s\n", line.LineNumber, line.ProductID, line.Quantity, line.Price, line.VATRate, line.VAT, line.Total); err != nil {
			return err
		}
		if line.UnitFactor != "" && line.UnitFactor != "1" {
			if _, err := fmt.Fprintf(out, "     unit %s × %s = %s base units\n", line.UnitID, line.UnitFactor, line.BaseQuantity); err != nil {
				return err
			}
		}
		if line.ComputedAmount != "" || line.ComputedVAT != "" {
			if _, err := fmt.Fprintf(out, "     document amounts differ from computed: amount %s, VAT %s\n", or(line.ComputedAmount, line.Amount), or(line.ComputedVAT, line.VAT)); err != nil {
				return err
			}
		}
	}
	for _, duplicate := range result.Duplicates {
		if _, err := fmt.Fprintf(out, "Possible duplicate: %s %s posted %t (%s)\n", flat(duplicate.Number), duplicate.Date, duplicate.Posted, duplicate.ID); err != nil {
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

func or(value, fallback service.Decimal) service.Decimal {
	if value == "" {
		return fallback
	}
	return value
}

func runReceiptBatchPreview(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.PreviewReceipts.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("file", "", "JSON array of receipt drafts")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *path == "" {
		return errors.New("usage: " + operations.PreviewReceipts.Usage)
	}
	file, err := os.Open(*path)
	if err != nil {
		return errors.New("could not open the receipts file")
	}
	defer file.Close()
	var drafts []service.ReceiptDraft
	decoder := json.NewDecoder(io.LimitReader(file, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&drafts); err != nil {
		return fmt.Errorf("receipts file must be a JSON array of receipt drafts: %w", err)
	}
	preview, err := svc.PreviewReceiptBatch(ctx, drafts)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(preview)
	}
	for _, item := range preview.Items {
		line := fmt.Sprintf("%d  %s", item.Index, item.Status)
		if item.Preview != nil {
			line += fmt.Sprintf("  total %s, VAT %s, %d lines", item.Preview.Amount, item.Preview.VAT, len(item.Preview.Lines))
		}
		if item.Reason != "" {
			line += "  " + flat(item.Reason)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "ready %d, exists %d, needs review %d, unsupported %d\n%s\n", preview.Ready, preview.Exists, preview.NeedsReview, preview.Unsupported, preview.Note)
	return err
}
