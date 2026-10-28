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

func runDebtList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list debts", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "customer or supplier")
	asOf := flags.String("as-of", "", "inclusive date, YYYY-MM-DD (default today)")
	counterpartyID := flags.String("counterparty", "", "optional counterparty GUID")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "results to skip")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *kind == "" {
		return errors.New("usage: " + operations.ListDebts.Usage)
	}
	page, err := svc.ListDebts(ctx, *kind, *asOf, *counterpartyID, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "COUNTERPARTY\tDEBT\tADVANCE\tNET\tID"); err != nil {
		return err
	}
	for _, item := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", flat(item.CounterpartyName), item.Debt, item.Advance, item.Net, item.CounterpartyID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "TOTAL\t%s\t%s\t%s\t\n", page.TotalDebt, page.TotalAdvance, page.TotalNet); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printNextOffset(out, page.NextOffset)
}

func runSalesReport(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("report sales", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	from := flags.String("from", "", "first day, YYYY-MM-DD")
	to := flags.String("to", "", "last day, YYYY-MM-DD")
	by := flags.String("by", "product", "product, customer, or product-customer")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "results to skip")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *from == "" || *to == "" {
		return errors.New("usage: " + operations.ReportSales.Usage)
	}
	report, err := svc.ReportSales(ctx, *by, *from, *to, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "PRODUCT\tCUSTOMER\tQUANTITY\tAMOUNT\tVAT\tCOST"); err != nil {
		return err
	}
	for _, item := range report.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", flat(item.ProductName), flat(item.CustomerName), item.Quantity, item.Amount, item.VAT, item.Cost); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "TOTAL\t\t\t%s\t%s\t%s\n", report.TotalAmount, report.TotalVAT, report.TotalCost); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printNextOffset(out, report.NextOffset)
}

func runCashBalanceList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list cash-balances", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asOf := flags.String("as-of", "", "inclusive date, YYYY-MM-DD (default today)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListCashBalances.Usage)
	}
	balances, err := svc.ListCashBalances(ctx, *asOf)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(balances)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "KIND\tACCOUNT\tCURRENCY\tAMOUNT\tCURRENCY_AMOUNT\tID"); err != nil {
		return err
	}
	for _, item := range balances.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", item.AccountKind, flat(item.AccountName), item.CurrencyCode, item.Amount, item.CurrencyAmount, item.AccountID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(writer, "TOTAL\t\t\t%s\t\t\n", balances.Total); err != nil {
		return err
	}
	return writer.Flush()
}

func printNextOffset(out io.Writer, next *int) error {
	if next == nil {
		return nil
	}
	_, err := fmt.Fprintf(out, "next offset %d\n", *next)
	return err
}
