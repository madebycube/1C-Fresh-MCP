package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runPrepaymentPlanCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreatePrepaymentPlan
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.PrepaymentRequest
	flags.StringVar(&input.InvoiceID, "invoice", "", "unposted supplier invoice GUID")
	flags.StringVar(&input.Source, "source", "", "bank or cash")
	flags.StringVar(&input.AccountID, "account", "", "organization bank account GUID")
	flags.StringVar(&input.SupplierAccountID, "supplier-account", "", "supplier bank account GUID")
	flags.StringVar(&input.CashID, "cash", "", "cash desk GUID")
	flags.StringVar(&input.PaymentDate, "payment-date", "", "planned payment date")
	flags.StringVar(&input.Amount, "amount", "", "prepayment amount")
	flags.StringVar(&input.DepartmentID, "department", "", "department GUID")
	flags.StringVar(&input.Purpose, "purpose", "", "payment purpose")
	comment := flags.String("comment", "", "replacement comment")
	flags.StringVar(&input.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&input.Apply, "apply", false, "save the plan")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || input.InvoiceID == "" || input.PaymentDate == "" {
		return errors.New("usage: " + spec.Usage)
	}
	flags.Visit(func(option *flag.Flag) {
		if option.Name == "comment" {
			input.Comment = comment
		}
	})
	result, err := svc.PlanInvoicePrepayment(ctx, input)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	switch {
	case result.Applied:
		state = "Saved prepayment plan on supplier invoice " + result.ID
	case result.AlreadyPlanned:
		state = "Already planned on supplier invoice " + result.ID + " (nothing written)"
	}
	source := "bank: paying account " + result.AccountID + ", supplier account " + result.SupplierAccountID
	if result.Source == "cash" {
		source = "cash desk " + result.CashID
	}
	if _, err := fmt.Fprintf(out, "%s\nInvoice: %s %s, incoming %s %s, posted %t, status %s\nSource: %s\nPayment article ID: %s\nInvoice total: %s, paid %s, left %s\nPlanned: %s (%s%%), VAT %s on %s\nComment: %s\nData version: %s\nApproval: %s\n",
		state, flat(result.Number), result.Date, flat(result.IncomingNumber), result.IncomingDate, result.Posted, result.Status, source, flat(result.PaymentArticleID),
		result.DocumentAmount, result.Paid, result.Remaining, result.Amount, result.Percent, result.VAT, result.PaymentDate, flat(result.Comment), result.DataVersion, result.Approval); err != nil {
		return err
	}
	for _, payment := range result.Payments.Payments {
		if _, err := fmt.Fprintf(out, "Linked payment: %s %s %s amount %s posted %t (%s)\n", payment.Kind, flat(payment.Number), payment.Date, payment.Amount, payment.Posted, payment.Link); err != nil {
			return err
		}
	}
	for _, lines := range [][]string{result.Payments.Incomplete, result.Impact, result.NotStored} {
		for _, line := range lines {
			if _, err := fmt.Fprintln(out, "- "+flat(line)); err != nil {
				return err
			}
		}
	}
	if !result.Applied && !result.AlreadyPlanned {
		_, err := fmt.Fprintln(out, "Run again with --apply to save.")
		return err
	}
	return nil
}
