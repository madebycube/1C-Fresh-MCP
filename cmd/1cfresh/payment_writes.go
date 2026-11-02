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

func runPaymentUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.UpdatePayment
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.PaymentAmendment
	var setLines, addLines, removeLines repeated
	kind := flags.String("kind", "", "bank-out or cash-out")
	text := map[string]**string{"date": &change.Date, "comment": &change.Comment, "amount": &change.Amount}
	values := map[string]*string{}
	for name := range text {
		values[name] = flags.String(name, "", name)
	}
	flags.Var(&setLines, "set-line", "allocation line change")
	flags.Var(&addLines, "add-line", "new allocation line")
	flags.Var(&removeLines, "remove-line", "allocation line to remove")
	flags.BoolVar(&change.Repost, "repost", false, "unpost a posted payment, amend it, and post it again")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&change.Apply, "apply", false, "save the change")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *kind == "" {
		return errors.New("usage: " + spec.Usage)
	}
	flags.Visit(func(option *flag.Flag) {
		if target, ok := text[option.Name]; ok {
			*target = values[option.Name]
		}
	})
	for _, value := range setLines {
		lineChange, err := parseAllocationChange(value)
		if err != nil {
			return err
		}
		change.ChangeLines = append(change.ChangeLines, lineChange)
	}
	for _, value := range addLines {
		parts := strings.Split(value, ":")
		if len(parts) < 3 || len(parts) > 4 {
			return errors.New("add-line must be CONTRACT_GUID:AMOUNT:VAT_CODE[:RECEIPT_GUID]")
		}
		line := service.PaymentAllocationInput{ContractID: parts[0], Amount: parts[1], VATRate: parts[2]}
		if len(parts) == 4 {
			line.DocumentID = parts[3]
		}
		change.AddLines = append(change.AddLines, line)
	}
	for _, value := range removeLines {
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 {
			return errors.New("remove-line must be a line number")
		}
		change.RemoveLines = append(change.RemoveLines, number)
	}
	result, err := svc.AmendSupplierPayment(ctx, *kind, flags.Arg(0), change)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved payment " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\n%s %s, posted %t\nDate: %s\nSupplier ID: %s\nAmount: %s (before %s, %d lines)\nChanges: %s\nData version: %s\n", state, result.Kind, flat(result.Number), result.Posted, result.Date, result.SupplierID, result.Amount, result.Before.Amount, result.Before.Lines, strings.Join(result.Changes, ", "), result.DataVersion); err != nil {
		return err
	}
	for _, line := range result.Allocations {
		document := line.DocumentID
		if line.Advance {
			document = "advance"
		}
		if _, err := fmt.Fprintf(out, "  %d  contract %s  %s  %s  %s VAT %s\n", line.LineNumber, line.ContractID, document, line.Amount, orText(line.VATRate, line.VATRateID), line.VAT); err != nil {
			return err
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	return nil
}

func parseAllocationChange(value string) (service.PaymentAllocationChange, error) {
	usage := errors.New("set-line must be N:contract=GUID,document=GUID,amount=A,vat=CODE")
	number, assignments, ok := strings.Cut(value, ":")
	lineNumber, err := strconv.Atoi(number)
	if !ok || err != nil || lineNumber < 1 {
		return service.PaymentAllocationChange{}, usage
	}
	change := service.PaymentAllocationChange{LineNumber: lineNumber}
	for _, assignment := range strings.Split(assignments, ",") {
		key, raw, ok := strings.Cut(assignment, "=")
		value := raw
		switch {
		case !ok:
			return service.PaymentAllocationChange{}, usage
		case key == "contract":
			change.ContractID = &value
		case key == "document":
			change.DocumentID = &value
		case key == "amount":
			change.Amount = &value
		case key == "vat":
			change.VATRate = &value
		default:
			return service.PaymentAllocationChange{}, errors.New("set-line accepts contract, document, amount, and vat")
		}
	}
	return change, nil
}

func orText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
