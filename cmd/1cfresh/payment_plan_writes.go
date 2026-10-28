package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runPaymentPlanCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreatePaymentPlan
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.PaymentPlanDraft
	flags.StringVar(&input.ReceiptID, "receipt", "", "supplier goods receipt GUID")
	flags.StringVar(&input.AccountID, "account", "", "organization bank account GUID")
	flags.StringVar(&input.SupplierAccountID, "supplier-account", "", "supplier bank account GUID")
	flags.StringVar(&input.Date, "date", "", "invoice date")
	flags.StringVar(&input.PaymentDate, "payment-date", "", "planned payment date")
	flags.StringVar(&input.Amount, "amount", "", "planned amount; defaults to the outstanding debt")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.BoolVar(&input.AllowDuplicate, "allow-duplicate", false, "save despite an existing plan")
	flags.BoolVar(&input.Apply, "apply", false, "save the plan")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.CreatePaymentPlanDraft(ctx, input)
	if err != nil {
		return err
	}
	return printPaymentPlanWrite(out, result, *asJSON)
}

func runPaymentPlanUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.UpdatePaymentPlan
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.PaymentPlanAmendment
	text := map[string]**string{"payment-date": &change.PaymentDate, "amount": &change.Amount, "account": &change.AccountID, "supplier-account": &change.SupplierAccountID, "comment": &change.Comment}
	values := map[string]*string{}
	for name := range text {
		values[name] = flags.String(name, "", name)
	}
	flags.StringVar(&change.ReceiptID, "receipt", "", "receipt GUID, needed to check a new amount")
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
	result, err := svc.AmendPaymentPlan(ctx, flags.Arg(0), change)
	if err != nil {
		return err
	}
	return printPaymentPlanWrite(out, result, *asJSON)
}

func runPaymentPlanPosting(ctx context.Context, svc service.Service, args []string, out io.Writer, action string) error {
	spec := operations.PostPaymentPlan
	if action == "unpost" {
		spec = operations.UnpostPaymentPlan
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "run the action")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetPaymentPlanPosting(ctx, flags.Arg(0), action, *expected, *apply)
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
	planned := make([]string, 0, len(result.PlannedAmounts))
	for _, amount := range result.PlannedAmounts {
		planned = append(planned, string(amount))
	}
	if _, err := fmt.Fprintf(out, "%s: %s payment plan %s\nPosted: %t -> %t\nInvoice total: %s, planned: %s\nData version: %s\n", state, action, result.ID, result.PostedFrom, result.Posted, result.Amount, flat(strings.Join(planned, ", ")), result.DataVersion); err != nil {
		return err
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if result.Calendar != nil {
		if _, err := fmt.Fprintf(out, "Payment calendar: %d records, approval %s\n", len(result.Calendar.Records), flat(strings.Join(result.Calendar.ApprovalStates, ", "))); err != nil {
			return err
		}
	}
	if !result.Applied && result.PostedFrom != (action == "post") {
		_, err := fmt.Fprintln(out, "Run again with --apply to "+action+".")
		return err
	}
	return nil
}

func printPaymentPlanWrite(out io.Writer, result service.PaymentPlanWrite, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved unposted payment plan " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nReceipt: %s %s %s, status %s, outstanding %s\nDate: %s, payment date %s\nIncoming: %s %s\nPaying account ID: %s\nSupplier account ID: %s\nPayment article ID: %s\nInvoice total: %s\nPlanned: %s (%s%%), VAT %s\nComment: %s\nApproval: %s\n",
		state, flat(result.ReceiptNumber), result.ReceiptDate, result.ReceiptID, flat(result.ReceiptStatus), flat(string(result.Outstanding)), result.Date, result.PaymentDate,
		flat(result.IncomingNumber), result.IncomingDate, result.AccountID, result.SupplierAccountID, flat(result.PaymentArticleID), result.DocumentAmount,
		result.Amount, result.Percent, result.VAT, flat(result.Comment), result.Approval); err != nil {
		return err
	}
	if result.Before != nil {
		if _, err := fmt.Fprintf(out, "Before: %s on %s from %s\nChanges: %s\nData version: %s\n", result.Before.Amount, result.Before.PaymentDate, result.Before.AccountID, flat(strings.Join(result.Changes, ", ")), result.DataVersion); err != nil {
			return err
		}
	}
	for _, plan := range result.Duplicates {
		if _, err := fmt.Fprintf(out, "Existing plan: %s %s %s amount %s posted %t (%s)\n", plan.ID, plan.Number, plan.Date, plan.Amount, plan.Posted, plan.Link); err != nil {
			return err
		}
	}
	for _, payment := range result.Payments {
		if _, err := fmt.Fprintf(out, "Linked payment: %s %s %s amount %s posted %t\n", payment.Kind, payment.Number, payment.Date, payment.Amount, payment.Posted); err != nil {
			return err
		}
	}
	for _, candidate := range result.Candidates {
		if _, err := fmt.Fprintf(out, "Candidate (amount only, not linked): %s %s %s amount %s\n", candidate.Kind, candidate.Number, candidate.Date, candidate.Amount); err != nil {
			return err
		}
	}
	for _, source := range result.Incomplete {
		if _, err := fmt.Fprintln(out, "Incomplete: "+source); err != nil {
			return err
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied && (result.Before == nil || len(result.Changes) > 0) {
		_, err := fmt.Fprintln(out, "Run again with --apply to save.")
		return err
	}
	return nil
}
