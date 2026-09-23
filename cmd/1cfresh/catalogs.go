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

func runGroupList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list groups", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "match this name or path")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list groups [--name TEXT] [--json]")
	}
	groups, err := svc.ListGroups(ctx, *name)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(groups)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "PATH\tCODE\tDELETED\tID"); err != nil {
		return err
	}
	for _, group := range groups {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\n", flat(group.Path), flat(group.Code), group.Deleted, group.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d groups\n", len(groups))
	return err
}

func runPriceTypeList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list price-types", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list price-types [--json]")
	}
	priceTypes, err := svc.ListPriceTypes(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(priceTypes)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tINACTIVE\tDELETED\tID"); err != nil {
		return err
	}
	for _, priceType := range priceTypes {
		if _, err := fmt.Fprintf(writer, "%s\t%t\t%t\t%s\n", flat(priceType.Name), priceType.Inactive, priceType.Deleted, priceType.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d price types\n", len(priceTypes))
	return err
}
