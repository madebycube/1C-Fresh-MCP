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

func runWarehouseList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list warehouses", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list warehouses [--json]")
	}
	warehouses, err := svc.ListWarehouses(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(warehouses)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tKIND\tCODE\tINACTIVE\tDELETED\tID"); err != nil {
		return err
	}
	for _, warehouse := range warehouses {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%t\t%s\n", flat(warehouse.Name), warehouse.Kind, flat(warehouse.Code), warehouse.Inactive, warehouse.Deleted, warehouse.ID); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func runStockGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get stock", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	warehouseID := flags.String("warehouse", "", "optional warehouse GUID")
	characteristicID := flags.String("characteristic", "", "optional product characteristic GUID")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c get stock PRODUCT_GUID [--warehouse GUID] [--characteristic GUID] [--json]")
	}
	stock, err := svc.GetStock(ctx, flags.Arg(0), *warehouseID, *characteristicID)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(stock)
	}
	if _, err := fmt.Fprintf(out, "%s (%s)\n", flat(stock.ProductName), stock.ProductID); err != nil {
		return err
	}
	unit := stock.UnitName
	if unit == "" {
		unit = stock.UnitID
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "WAREHOUSE\tQUANTITY\tUNIT\tID"); err != nil {
		return err
	}
	for _, warehouse := range stock.Warehouses {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", flat(warehouse.Name), warehouse.Quantity, flat(unit), warehouse.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Total: %s %s; source: %s\n", stock.Total, flat(unit), stock.SourceRegister)
	return err
}
