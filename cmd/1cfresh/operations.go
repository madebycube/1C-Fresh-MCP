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

func runOperationalList(ctx context.Context, svc service.Service, args []string, out io.Writer, domain string) error {
	spec := operations.ListPurchases
	if domain == "warehouse" {
		spec = operations.ListWarehouseDocuments
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "document kind")
	supplier := ""
	if domain == "purchase" {
		flags.StringVar(&supplier, "supplier", "", "optional supplier GUID")
	}
	warehouse := flags.String("warehouse", "", "optional warehouse GUID")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	to := flags.String("to", "", "last date (YYYY-MM-DD)")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching documents")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *kind == "" {
		return errors.New("usage: " + spec.Usage)
	}
	page, err := svc.ListOperationalDocuments(ctx, domain, *kind, supplier, *warehouse, *from, *to, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tPOSTED\tAMOUNT\tSUPPLIER ID\tWAREHOUSE ID\tDESTINATION ID\tID"); err != nil {
		return err
	}
	for _, doc := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\t%s\t%s\t%s\n", flat(doc.Number), flat(doc.Date), doc.Posted, doc.Amount, doc.SupplierID, doc.WarehouseID, doc.DestinationWarehouseID, doc.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printPageSummary(out, len(page.Items), page.Total, page.NextOffset)
}

func runOperationalGet(ctx context.Context, svc service.Service, args []string, out io.Writer, domain string) error {
	spec := operations.GetPurchase
	if domain == "warehouse" {
		spec = operations.GetWarehouseDocument
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "document kind")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *kind == "" {
		return errors.New("usage: " + spec.Usage)
	}
	doc, err := svc.GetOperationalDocument(ctx, domain, *kind, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(doc)
	}
	if _, err := fmt.Fprintf(out, "%s %s %s\nDate: %s\nOperation: %s\nPosted: %t\nDeleted: %t\nAmount: %s\nSupplier ID: %s\nWarehouse ID: %s\nReserve warehouse ID: %s\nDestination warehouse ID: %s\nState ID: %s\nSupplier order ID: %s\nTransfer order ID: %s\nBasis ID: %s\nBasis type: %s\nID: %s\n\n", doc.Domain, doc.Kind, flat(doc.Number), flat(doc.Date), flat(doc.Operation), doc.Posted, doc.Deleted, doc.Amount, doc.SupplierID, doc.WarehouseID, doc.ReserveWarehouseID, doc.DestinationWarehouseID, doc.StateID, doc.SupplierOrderID, doc.TransferOrderID, doc.BasisID, doc.BasisType, doc.ID); err != nil {
		return err
	}
	return printDocumentLines(out, doc.Lines)
}
