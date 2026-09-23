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

func runMoneyAccountList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListMoneyAccounts.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "cash, bank, or register")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *kind == "" {
		return errors.New("usage: " + operations.ListMoneyAccounts.Usage)
	}
	accounts, err := svc.ListMoneyAccounts(ctx, *kind)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(accounts)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CODE\tNAME\tID"); err != nil {
		return err
	}
	for _, account := range accounts {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", flat(account.Code), flat(account.Name), account.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func runMoneyList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListMoney.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "cash-in, cash-out, bank-in, bank-out, card-payment, or cash-shift")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	to := flags.String("to", "", "last date (YYYY-MM-DD)")
	account := flags.String("account", "", "cash or bank account GUID")
	register := flags.String("register", "", "retail register GUID")
	terminal := flags.String("terminal", "", "acquiring terminal GUID")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching documents")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *kind == "" || *from == "" || *to == "" {
		return errors.New("usage: " + operations.ListMoney.Usage)
	}
	page, err := svc.ListMoneyDocuments(ctx, *kind, *from, *to, *account, *register, *terminal, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tOPERATION\tPOSTED\tAMOUNT\tACCOUNT ID\tREGISTER ID\tID"); err != nil {
		return err
	}
	for _, doc := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\n", flat(doc.Number), flat(doc.Date), flat(doc.Operation), doc.Posted, doc.Amount, doc.AccountID, doc.RegisterID, doc.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return printPageSummary(out, len(page.Items), page.Total, page.NextOffset)
}

func runMoneyGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.GetMoney.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "cash-in, cash-out, bank-in, bank-out, card-payment, or cash-shift")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *kind == "" {
		return errors.New("usage: " + operations.GetMoney.Usage)
	}
	doc, err := svc.GetMoneyDocument(ctx, *kind, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(doc)
	}
	_, err = fmt.Fprintf(out, "%s %s\nDate: %s\nOperation: %s\nPosted: %t\nDeleted: %t\nAmount: %s\nAccount ID: %s\nRegister ID: %s\nTerminal ID: %s\nCounterparty ID: %s\nCurrency ID: %s\nBasis ID: %s\nBasis type: %s\nID: %s\n", doc.Kind, flat(doc.Number), flat(doc.Date), flat(doc.Operation), doc.Posted, doc.Deleted, doc.Amount, doc.AccountID, doc.RegisterID, doc.TerminalID, doc.CounterpartyID, doc.CurrencyID, doc.BasisID, doc.BasisType, doc.ID)
	return err
}
