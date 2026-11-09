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

func runPriceGroupList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListPriceGroups.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "match a price group name")
	includeDeleted := flags.Bool("include-deleted", false, "include deletion-marked groups")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListPriceGroups.Usage)
	}
	groups, err := svc.ListPriceGroups(ctx, *name, *includeDeleted)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(groups)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tCODE\tDELETED\tID"); err != nil {
		return err
	}
	for _, group := range groups {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\n", flat(group.Name), flat(group.Code), group.Deleted, group.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d price groups\n", len(groups))
	return err
}

func runPriceGroupCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.CreatePriceGroup.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "new price group name")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *name == "" {
		return errors.New("usage: " + operations.CreatePriceGroup.Usage)
	}
	change, err := svc.CreatePriceGroup(ctx, *name)
	if err != nil {
		return err
	}
	return printPriceGroupChange(out, change, *asJSON)
}

func runPriceGroupUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.UpdatePriceGroup.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "replacement price group name")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *name == "" {
		return errors.New("usage: " + operations.UpdatePriceGroup.Usage)
	}
	change, err := svc.UpdatePriceGroup(ctx, flags.Arg(0), *name)
	if err != nil {
		return err
	}
	return printPriceGroupChange(out, change, *asJSON)
}

func printPriceGroupChange(out io.Writer, change service.PriceGroupChange, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	if !change.Applied {
		_, err := fmt.Fprintf(out, "Price group already has that name (%s)\n", change.ID)
		return err
	}
	if change.ID == "" {
		_, err := fmt.Fprintf(out, "Created price group %s; 1C did not return its ID\n", flat(change.Name))
		return err
	}
	_, err := fmt.Fprintf(out, "Saved price group %s (%s)\n", flat(change.Name), change.ID)
	return err
}
