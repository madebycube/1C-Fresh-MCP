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

func runCounterpartyGroupList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListCounterpartyGroups.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "match a folder name or path")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListCounterpartyGroups.Usage)
	}
	groups, err := svc.ListCounterpartyGroups(ctx, *name)
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
	_, err = fmt.Fprintf(out, "%d counterparty groups\n", len(groups))
	return err
}

func runCounterpartyGroupCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.CreateCounterpartyGroup.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "new folder name")
	parent := flags.String("parent", "", "parent folder GUID; omit for root")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *name == "" {
		return errors.New("usage: " + operations.CreateCounterpartyGroup.Usage)
	}
	change, err := svc.CreateCounterpartyGroup(ctx, *name, *parent)
	if err != nil {
		return err
	}
	return printCounterpartyGroupChange(out, change, *asJSON)
}

func runCounterpartyGroupUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.UpdateCounterpartyGroup.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "replacement folder name")
	parent := flags.String("parent", "", "destination folder GUID or root")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + operations.UpdateCounterpartyGroup.Usage)
	}
	var patch service.CounterpartyGroupPatch
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "name":
			patch.Name = name
		case "parent":
			patch.ParentID = parent
		}
	})
	change, err := svc.UpdateCounterpartyGroup(ctx, flags.Arg(0), patch)
	if err != nil {
		return err
	}
	return printCounterpartyGroupChange(out, change, *asJSON)
}

func printCounterpartyGroupChange(out io.Writer, change service.CounterpartyGroupChange, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	if !change.Applied {
		_, err := fmt.Fprintf(out, "Counterparty group already has those values (%s)\n", change.ID)
		return err
	}
	if change.ID == "" {
		_, err := fmt.Fprintf(out, "Created counterparty group %s; 1C did not return its ID\n", flat(change.Name))
		return err
	}
	_, err := fmt.Fprintf(out, "Saved counterparty group %s (%s)\n", flat(change.Name), change.ID)
	return err
}
