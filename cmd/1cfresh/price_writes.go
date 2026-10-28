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

func runPriceSet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.SetPrice
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.PriceDraft
	flags.StringVar(&input.PriceType, "price-type", "", "price type name")
	flags.StringVar(&input.Price, "price", "", "price in rubles")
	flags.StringVar(&input.Date, "date", "", "date the price applies from")
	flags.StringVar(&input.CharacteristicID, "characteristic", "", "characteristic GUID")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	input.ProductID = flags.Arg(0)
	result, err := svc.CreatePriceDraft(ctx, input)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved unposted price document " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nDate: %s\nProduct ID: %s\nPrice type: %s (%s)\nPrice: %s per base unit %s\n", state, result.Date, result.ProductID, result.PriceTypeName, result.PriceTypeID, result.Price, result.UnitID); err != nil {
		return err
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	return nil
}
