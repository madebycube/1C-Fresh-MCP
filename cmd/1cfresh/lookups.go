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

func runOrganizationList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list organizations", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListOrganizations.Usage)
	}
	items, err := svc.ListOrganizations(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(items)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tCODE\tID"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\n", flat(item.Name), flat(item.Code), item.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func runDepartmentList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list departments", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListDepartments.Usage)
	}
	items, err := svc.ListDepartments(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(items)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tCODE\tINACTIVE\tID"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\n", flat(item.Name), flat(item.Code), item.Inactive, item.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func runContractList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list contracts", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	counterpartyID := flags.String("counterparty", "", "optional counterparty GUID")
	organizationID := flags.String("organization", "", "optional organization GUID")
	kind := flags.String("kind", "", "supplier or customer")
	all := flags.Bool("all", false, "include inactive contracts")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListContracts.Usage)
	}
	items, err := svc.ListContracts(ctx, service.ContractFilter{CounterpartyID: *counterpartyID, OrganizationID: *organizationID, Kind: *kind, All: *all})
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(items)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tNUMBER\tKIND\tCOUNTERPARTY\tORGANIZATION\tCURRENCY\tINACTIVE\tID"); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\n", flat(item.Name), flat(item.Number), item.Kind, item.CounterpartyID, item.OrganizationID, item.CurrencyID, item.Inactive, item.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}
