package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runCustomerList(ctx context.Context, svc service.Service, args []string, out io.Writer, search bool) error {
	command := "list customers"
	if search {
		command = "search customers"
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching customers")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 && !search || flags.NArg() != 1 && search {
		if search {
			return errors.New("usage: 1c search customers [--limit N] [--offset N] [--json] QUERY")
		}
		return errors.New("usage: 1c list customers [--limit N] [--offset N] [--json]")
	}
	query := ""
	if search {
		query = flags.Arg(0)
	}
	page, err := svc.ListCustomers(ctx, query, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CODE\tNAME\tBUYER\tSUPPLIER\tINACTIVE\tID"); err != nil {
		return err
	}
	for _, customer := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", flat(customer.Code), flat(customer.Name), optionalBool(customer.Buyer), optionalBool(customer.Supplier), optionalBool(customer.Inactive), customer.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printPageSummary(out, len(page.Items), page.Total, page.NextOffset)
}

func runCustomerGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get customer", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c get customer [--json] GUID")
	}
	customer, err := svc.GetCustomer(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(customer)
	}
	_, err = fmt.Fprintf(out, "Customer %s\nFull name: %s\nCode: %s\nBuyer: %s\nSupplier: %s\nInactive: %s\nDeleted: %t\nID: %s\n", flat(customer.Name), flat(customer.FullName), flat(customer.Code), optionalBool(customer.Buyer), optionalBool(customer.Supplier), optionalBool(customer.Inactive), customer.Deleted, customer.ID)
	return err
}

func runSalesList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list sales", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "invoice, shipment, or return")
	customer := flags.String("customer", "", "optional customer GUID")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	to := flags.String("to", "", "last date (YYYY-MM-DD)")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching documents")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *kind == "" {
		return errors.New("usage: 1c list sales --kind invoice|shipment|return [--customer GUID] [--from YYYY-MM-DD --to YYYY-MM-DD] [--limit N] [--offset N] [--json]")
	}
	page, err := svc.ListSalesDocuments(ctx, *kind, *customer, *from, *to, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tPOSTED\tAMOUNT\tCUSTOMER ID\tORDER ID\tID"); err != nil {
		return err
	}
	for _, doc := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\t%s\t%s\n", flat(doc.Number), flat(doc.Date), doc.Posted, doc.Amount, doc.CustomerID, doc.OrderID, doc.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printPageSummary(out, len(page.Items), page.Total, page.NextOffset)
}

func runSalesGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get sale", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "invoice, shipment, or return")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *kind == "" {
		return errors.New("usage: 1c get sale --kind invoice|shipment|return [--json] GUID")
	}
	doc, err := svc.GetSalesDocument(ctx, *kind, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(doc)
	}
	if _, err := fmt.Fprintf(out, "%s %s\nDate: %s\nOperation: %s\nPosted: %t\nDeleted: %t\nAmount: %s\nCustomer ID: %s\nOrder ID: %s\nBasis ID: %s\nBasis type: %s\nID: %s\n\n", doc.Kind, flat(doc.Number), flat(doc.Date), flat(doc.Operation), doc.Posted, doc.Deleted, doc.Amount, doc.CustomerID, doc.OrderID, doc.BasisID, doc.BasisType, doc.ID); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "LINE\tPRODUCT ID\tQUANTITY\tUNIT\tPRICE\tAMOUNT\tTOTAL\tVAT"); err != nil {
		return err
	}
	for _, line := range doc.Lines {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", line.LineNumber, line.ProductID, line.Quantity, flat(line.Unit), line.Price, line.Amount, line.Total, line.VAT); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func optionalBool(value *bool) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}

func printPageSummary(out io.Writer, shown, total int, next *int) error {
	if _, err := fmt.Fprintf(out, "%d of %d shown", shown, total); err != nil {
		return err
	}
	if next != nil {
		_, err := fmt.Fprintf(out, "; next offset: %d\n", *next)
		return err
	}
	_, err := fmt.Fprintln(out)
	return err
}
