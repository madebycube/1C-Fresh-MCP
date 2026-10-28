package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func parseTransferLine(value string) (service.TransferLineInput, error) {
	usage := errors.New("line must be PRODUCT_GUID:QUANTITY[:CHARACTERISTIC_GUID][:unit=GUID]")
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 4 {
		return service.TransferLineInput{}, usage
	}
	line := service.TransferLineInput{ProductID: parts[0], Quantity: parts[1]}
	for index, part := range parts[2:] {
		unit, isUnit := strings.CutPrefix(part, "unit=")
		switch {
		case isUnit && line.UnitID == "":
			line.UnitID = unit
		case !strings.Contains(part, "=") && index == 0:
			line.CharacteristicID = part
		default:
			return service.TransferLineInput{}, usage
		}
	}
	return line, nil
}

func runTransferCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreateTransfer
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.TransferDraft
	var lines repeated
	flags.StringVar(&input.OrganizationID, "organization", "", "organization GUID")
	flags.StringVar(&input.FromWarehouseID, "from", "", "source warehouse GUID")
	flags.StringVar(&input.ToWarehouseID, "to", "", "destination warehouse GUID")
	flags.StringVar(&input.Date, "date", "", "document date")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.Var(&lines, "line", "goods line")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	for _, value := range lines {
		line, err := parseTransferLine(value)
		if err != nil {
			return err
		}
		input.Lines = append(input.Lines, line)
	}
	result, err := svc.CreateTransferDraft(ctx, input)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved unposted transfer " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nDate: %s\nFrom warehouse ID: %s\nTo warehouse ID: %s\n", state, result.Date, result.FromWarehouseID, result.ToWarehouseID); err != nil {
		return err
	}
	for _, line := range result.Lines {
		if _, err := fmt.Fprintf(out, "  %d  %s  %s\n", line.LineNumber, line.ProductID, line.Quantity); err != nil {
			return err
		}
		if line.UnitFactor != "" && line.UnitFactor != "1" {
			if _, err := fmt.Fprintf(out, "     unit %s × %s = %s base units\n", line.UnitID, line.UnitFactor, line.BaseQuantity); err != nil {
				return err
			}
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	return nil
}
