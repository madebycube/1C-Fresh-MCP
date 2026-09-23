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

func runCurrencyList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListCurrencies.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListCurrencies.Usage)
	}
	items, err := svc.ListCurrencies(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(items)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CODE\tNAME\tSYMBOL\tID"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", flat(item.Code), flat(item.Name), flat(item.Symbol), item.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d currencies\n", len(items))
	return err
}
