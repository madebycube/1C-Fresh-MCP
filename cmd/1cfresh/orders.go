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

func runOrderList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list orders", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list orders [--limit N] [--json]")
	}
	orders, err := svc.ListOrders(ctx, *limit)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(orders)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tSTATE\tPOSTED\tAMOUNT\tID"); err != nil {
		return err
	}
	for _, order := range orders {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\t%s\n", flat(order.Number), flat(order.Date), flat(order.State), order.Posted, order.Amount, order.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func runOrderGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get order", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c get order [--json] GUID")
	}
	order, err := svc.GetOrder(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(order)
	}
	if _, err := fmt.Fprintf(out, "Order %s\nDate: %s\nState: %s\nPosted: %t\nAmount: %s\nCustomer ID: %s\nID: %s\n\n", flat(order.Number), flat(order.Date), flat(order.State), order.Posted, order.Amount, order.CustomerID, order.ID); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "LINE\tITEM\tQUANTITY\tUNIT\tPRICE\tAMOUNT\tTOTAL"); err != nil {
		return err
	}
	for _, line := range order.Lines {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", line.LineNumber, flat(line.Item), line.Quantity, flat(line.Unit), line.Price, line.Amount, line.Total); err != nil {
			return err
		}
	}
	return writer.Flush()
}
