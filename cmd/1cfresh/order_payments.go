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

func runOrderPayments(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetOrderPayments
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query service.OrderPaymentsQuery
	flags.StringVar(&query.Number, "number", "", "order number")
	flags.StringVar(&query.OrganizationID, "organization", "", "organization GUID to narrow a number")
	flags.StringVar(&query.CustomerID, "customer", "", "customer GUID to narrow a number")
	flags.StringVar(&query.Date, "date", "", "order date to narrow a number")
	flags.StringVar(&query.AsOf, "as-of", "", "settlement date, default today")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() > 1 || flags.NArg() == 1 && query.Number != "" || flags.NArg() == 0 && query.Number == "" {
		return errors.New("usage: " + spec.Usage)
	}
	query.OrderID = flags.Arg(0)
	result, err := svc.GetOrderPayments(ctx, query)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	order, summary := result.Order, result.Summary
	if _, err := fmt.Fprintf(out, "Order %s of %s, amount %s, customer %s, contract %s\nAs of %s: status %s, needs review %t, complete %t\nPayments %s, refunds %s, offsets %s, net paid %s; remaining %s, overpaid %s\nShipped per settlements %s, settlement balance %s\n",
		flat(order.Number), order.Date, order.Amount, order.CustomerID, order.ContractID, result.AsOf, summary.Status, summary.NeedsReview, summary.Complete,
		summary.Payments, summary.Refunds, summary.Offsets, summary.NetPaid, summary.Remaining, summary.Overpaid, summary.Shipped, summary.RegisterBalance); err != nil {
		return err
	}
	print := func(label string, doc service.OrderPaymentDocument) error {
		date := doc.Date
		if date == "" {
			date = doc.Period
		}
		_, err := fmt.Fprintf(out, "%s: %s %s %s %s allocated %s of %s %s %s\n", label, doc.Kind, doc.Role, flat(doc.Number), date, doc.Allocated, doc.DocumentAmount, doc.ID, doc.Reason)
		return err
	}
	for _, doc := range result.Documents {
		if err := print("Document", doc); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Customer advance without an order: %s\n", result.Unallocated.AdvanceBalance); err != nil {
		return err
	}
	for _, doc := range result.Unallocated.Documents {
		if err := print("Without order", doc); err != nil {
			return err
		}
	}
	for _, doc := range result.NotCounted {
		if err := print("Not counted", doc); err != nil {
			return err
		}
	}
	for _, issue := range result.Issues {
		if _, err := fmt.Fprintf(out, "Review: %s %s %s\n", issue.Kind, issue.DocumentID, issue.Reason); err != nil {
			return err
		}
	}
	for _, scan := range result.Scans {
		if scan.Error != "" {
			if _, err := fmt.Fprintf(out, "Unavailable: %s: %s\n", scan.Resource, scan.Error); err != nil {
				return err
			}
		}
	}
	for _, note := range result.Notes {
		if _, err := fmt.Fprintln(out, "Note: "+note); err != nil {
			return err
		}
	}
	return nil
}
