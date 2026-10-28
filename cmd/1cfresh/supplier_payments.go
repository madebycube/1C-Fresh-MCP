package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runPaymentPlanList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.ListPaymentPlans
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var filter service.SupplierPaymentPlanFilter
	flags.StringVar(&filter.OrganizationID, "organization", "", "optional organization GUID")
	flags.StringVar(&filter.SupplierID, "supplier", "", "optional supplier GUID")
	flags.StringVar(&filter.ContractID, "contract", "", "optional contract GUID")
	flags.StringVar(&filter.AccountID, "account", "", "optional source bank account GUID")
	flags.StringVar(&filter.From, "from", "", "first date (YYYY-MM-DD)")
	flags.StringVar(&filter.To, "to", "", "last date (YYYY-MM-DD)")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching documents")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	page, err := svc.ListSupplierPaymentPlans(ctx, filter, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tPOSTED\tAMOUNT\tINCOMING\tSUPPLIER ID\tACCOUNT ID\tID"); err != nil {
		return err
	}
	for _, item := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\n", flat(item.Number), flat(item.Date), item.Posted, item.Amount, flat(item.IncomingNumber), item.SupplierID, item.AccountID, item.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printPageSummary(out, len(page.Items), page.Total, page.NextOffset)
}

func runPaymentPlanGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetPaymentPlan
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	item, err := svc.GetSupplierPaymentPlan(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(item)
	}
	if _, err := fmt.Fprintf(out, "Payment plan %s\nDate: %s\nPosted: %t\nAmount: %s\nIncoming: %s %s\nOrganization ID: %s\nSupplier ID: %s\nContract ID: %s\nAccount ID: %s\nSupplier account ID: %s\nPlan payment: %t\nBasis ID: %s\nID: %s\n\nSchedule:\n", flat(item.Number), flat(item.Date), item.Posted, item.Amount, flat(item.IncomingNumber), flat(item.IncomingDate), item.OrganizationID, item.SupplierID, item.ContractID, item.AccountID, item.SupplierAccountID, item.PlanPayment, item.BasisID, item.ID); err != nil {
		return err
	}
	for _, line := range item.Schedule {
		if _, err := fmt.Fprintf(out, "  %s  %s  %s%%  %s\n", line.LineNumber, flat(line.Date), line.Percent, line.Amount); err != nil {
			return err
		}
	}
	if item.Calendar == nil || !item.Calendar.Available {
		reason := "not read"
		if item.Calendar != nil {
			reason = item.Calendar.Error
		}
		_, err := fmt.Fprintf(out, "\nPayment calendar: unavailable (%s)\n", reason)
		return err
	}
	if _, err := fmt.Fprintf(out, "\nPayment calendar records: %d\nApproval states: %v\n", len(item.Calendar.Records), item.Calendar.ApprovalStates); err != nil {
		return err
	}
	return nil
}

func runReceiptPaymentTrace(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.TraceReceiptPayments
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	trace, err := svc.TraceSupplierReceipt(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(trace)
	}
	receipt := trace.Receipt
	if _, err := fmt.Fprintf(out, "Receipt %s %s, amount %s, posted %t\nStatus: %s (complete: %t)\nLinked settlement: %s\nOutstanding by links: %s\n", flat(receipt.Number), flat(receipt.Date), receipt.Amount, receipt.Posted, trace.Summary.Status, trace.Summary.Complete, trace.Summary.LinkedSettlement, trace.Summary.OutstandingByLinks); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "\nKIND\tLINK\tNUMBER\tDATE\tPOSTED\tAMOUNT\tID"); err != nil {
		return err
	}
	for _, plan := range trace.PaymentPlans {
		if _, err := fmt.Fprintf(writer, "payment-plan\t%s\t%s\t%s\t%t\t%s\t%s\n", plan.Link, flat(plan.Number), flat(plan.Date), plan.Posted, plan.Amount, plan.ID); err != nil {
			return err
		}
	}
	for _, payment := range trace.Payments {
		for _, allocation := range payment.Allocations {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n", payment.Kind, allocation.Link, flat(payment.Number), flat(payment.Date), payment.Posted, allocation.SettlementAmount, payment.ID); err != nil {
				return err
			}
		}
		if len(payment.Allocations) == 0 {
			link := "basis"
			if payment.PaymentOrderID != "" {
				link = "payment-order"
			}
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%t\t%s\t%s\n", payment.Kind, link, flat(payment.Number), flat(payment.Date), payment.Posted, payment.Amount, payment.ID); err != nil {
				return err
			}
		}
	}
	for _, prepayment := range trace.Prepayments {
		if _, err := fmt.Fprintf(writer, "prepayment\toffset\t\t\t\t%s\t%s\n", prepayment.SettlementAmount, prepayment.DocumentID); err != nil {
			return err
		}
	}
	for _, candidate := range trace.Candidates {
		if _, err := fmt.Fprintf(writer, "%s\tcandidate\t%s\t%s\t\t%s\t%s\n", candidate.Kind, flat(candidate.Number), flat(candidate.Date), candidate.Amount, candidate.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	for _, note := range trace.Summary.Notes {
		if _, err := fmt.Fprintln(out, "Note: "+note); err != nil {
			return err
		}
	}
	for _, scan := range trace.Scans {
		if scan.Error != "" {
			if _, err := fmt.Fprintf(out, "Not scanned: %s (%s)\n", scan.Resource, scan.Error); err != nil {
				return err
			}
		}
	}
	return nil
}
