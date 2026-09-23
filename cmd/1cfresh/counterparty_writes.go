package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runCounterpartyCreate(ctx context.Context, svc service.Service, args []string, out io.Writer, role string) error {
	spec := operations.CreateCustomer
	if role == "supplier" {
		spec = operations.CreateSupplier
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "counterparty name")
	fullName := flags.String("full-name", "", "optional full name")
	parent := flags.String("parent", "", "optional counterparty folder GUID")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *name == "" {
		return errors.New("usage: " + spec.Usage)
	}
	change, err := svc.CreateCounterparty(ctx, role, *name, *fullName, *parent)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	_, err = fmt.Fprintf(out, "Created %s %s (%s)\n", role, flat(change.Name), change.ID)
	return err
}

func runCounterpartyUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer, role string) error {
	spec := operations.UpdateCustomer
	if role == "supplier" {
		spec = operations.UpdateSupplier
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "replacement name")
	fullName := flags.String("full-name", "", "replacement full name; empty clears it")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	var patch service.CounterpartyPatch
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "name":
			patch.Name = name
		case "full-name":
			patch.FullName = fullName
		}
	})
	change, err := svc.UpdateCounterparty(ctx, role, flags.Arg(0), patch)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	if !change.Applied {
		_, err = fmt.Fprintf(out, "%s already has those values (%s)\n", role, change.ID)
	} else {
		_, err = fmt.Fprintf(out, "Updated %s %s (%s)\n", role, flat(change.Name), change.ID)
	}
	return err
}
