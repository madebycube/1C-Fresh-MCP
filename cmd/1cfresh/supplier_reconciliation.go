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

func runSupplierReconciliation(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.ReconcileSupplier
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var query service.SupplierReconciliationQuery
	flags.StringVar(&query.OrganizationID, "organization", "", "organization GUID")
	flags.StringVar(&query.SupplierID, "supplier", "", "supplier GUID")
	flags.StringVar(&query.ContractID, "contract", "", "contract GUID")
	flags.StringVar(&query.AccountID, "account", "", "bank account or cash desk GUID for the allocation list")
	flags.StringVar(&query.From, "from", "", "first day")
	flags.StringVar(&query.To, "to", "", "last day")
	flags.IntVar(&query.Limit, "limit", 50, "movements per page")
	flags.IntVar(&query.Offset, "offset", 0, "movement offset")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.ReconcileSupplier(ctx, query)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	if _, err := fmt.Fprintf(out, "Supplier %s, organization %s, %s to %s (derived from OData, not the official UNF report)\nSigns: %s\nComplete: %t\n", result.SupplierID, result.OrganizationID, result.From, result.To, result.SignConvention, result.Complete); err != nil {
		return err
	}
	for _, balance := range result.Balances {
		if _, err := fmt.Fprintf(out, "  contract %s %s (currency %s): opening %s, +%s, -%s, closing %s; accounting %s -> %s\n", balance.ContractID, balance.SettlementType, flat(balance.CurrencyID), balance.Opening, balance.Increase, balance.Decrease, balance.Closing, balance.OpeningAccounting, balance.ClosingAccounting); err != nil {
			return err
		}
	}
	for _, item := range result.OpenItems {
		if _, err := fmt.Fprintf(out, "Open: %s %s %s %s %s\n", item.Kind, item.SettlementType, item.DocumentType, item.DocumentID, item.Balance); err != nil {
			return err
		}
	}
	for _, allocation := range result.Allocations {
		if _, err := fmt.Fprintf(out, "Allocation: %s %s %s -> %s %s amount %s %s\n", allocation.Period[:10], allocation.Source, allocation.RecorderID, allocation.DocumentType, allocation.DocumentID, allocation.Amount, allocation.AllocationMethod); err != nil {
			return err
		}
	}
	for _, item := range append(result.Exclusions, result.NotPayments...) {
		if _, err := fmt.Fprintf(out, "Not counted: %s %s %s amount %s (%s)\n", item.Kind, flat(item.Number), item.Date[:10], item.Amount, item.Reason); err != nil {
			return err
		}
	}
	for _, scan := range result.Scans {
		if scan.Error != "" {
			if _, err := fmt.Fprintf(out, "Unavailable: %s: %s\n", scan.Resource, scan.Error); err != nil {
				return err
			}
		}
	}
	for _, note := range result.Notes {
		if _, err := fmt.Fprintln(out, "Note: "+note); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "Movements: %d in period (shown with --json, from offset %d)\n", result.MovementsTotal, result.Offset)
	return err
}
