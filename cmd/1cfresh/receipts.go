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

func runReceiptList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list receipts", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "sale", "sale or refund")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	to := flags.String("to", "", "last date (YYYY-MM-DD)")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching receipts")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *from == "" || *to == "" {
		return errors.New("usage: 1c list receipts --from YYYY-MM-DD --to YYYY-MM-DD [--kind sale|refund] [--limit N] [--offset N] [--json]")
	}
	page, err := svc.ListReceipts(ctx, *kind, *from, *to, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tPOSTED\tAMOUNT\tORDER ID\tID"); err != nil {
		return err
	}
	for _, receipt := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\t%s\n", flat(receipt.Number), flat(receipt.Date), receipt.Posted, receipt.Amount, receipt.OrderID, receipt.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d of %d matching %s receipts shown", len(page.Items), page.Total, page.Kind)
	if err != nil {
		return err
	}
	if page.NextOffset != nil {
		_, err = fmt.Fprintf(out, "; next offset: %d\n", *page.NextOffset)
	} else {
		_, err = fmt.Fprintln(out)
	}
	return err
}

func runReceiptGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get receipt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "sale", "sale or refund")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c get receipt [--kind sale|refund] [--json] GUID")
	}
	receipt, err := svc.GetReceipt(ctx, *kind, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(receipt)
	}
	if _, err := fmt.Fprintf(out, "%s receipt %s\nDate: %s\nPosted: %t\nAmount: %s\nOrder ID: %s\nOriginal receipt ID: %s\nID: %s\n\n", receipt.Kind, flat(receipt.Number), flat(receipt.Date), receipt.Posted, receipt.Amount, receipt.OrderID, receipt.OriginalReceiptID, receipt.ID); err != nil {
		return err
	}
	lines := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(lines, "LINE\tITEM ID\tQUANTITY\tUNIT\tPRICE\tAMOUNT\tTOTAL"); err != nil {
		return err
	}
	for _, line := range receipt.Lines {
		if _, err := fmt.Fprintf(lines, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", line.LineNumber, line.ItemID, line.Quantity, flat(line.Unit), line.Price, line.Amount, line.Total); err != nil {
			return err
		}
	}
	if err := lines.Flush(); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "\nCashless payments:"); err != nil {
		return err
	}
	payments := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(payments, "LINE\tKIND\tAMOUNT\tTERMINAL ID"); err != nil {
		return err
	}
	for _, payment := range receipt.CashlessPayments {
		if _, err := fmt.Fprintf(payments, "%s\t%s\t%s\t%s\n", payment.LineNumber, flat(payment.Kind), payment.Amount, payment.TerminalID); err != nil {
			return err
		}
	}
	return payments.Flush()
}

func runReceiptAudit(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("audit receipts", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "both", "sale, refund, or both")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	before := flags.String("before", "", "exclusive cutoff date (YYYY-MM-DD)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *from == "" || *before == "" {
		return errors.New("usage: 1c audit receipts --from YYYY-MM-DD --before YYYY-MM-DD [--kind sale|refund|both] [--json]")
	}
	report, err := svc.AuditUnpostedReceipts(ctx, *kind, *from, *before)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "KIND\tNUMBER\tDATE\tPOSTED\tAMOUNT\tID"); err != nil {
		return err
	}
	for _, receipt := range report.Findings {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\t%s\n", receipt.Kind, flat(receipt.Number), flat(receipt.Date), receipt.Posted, receipt.Amount, receipt.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d unposted receipts found among %d inspected from %s before %s\n", len(report.Findings), report.Inspected, report.From, report.Before)
	return err
}
