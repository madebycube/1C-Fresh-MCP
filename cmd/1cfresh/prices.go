package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runPriceGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("get price", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	priceType := flags.String("price-type", "", "price type name")
	characteristic := flags.String("characteristic", "", "product characteristic GUID")
	asOf := flags.String("as-of", "", "application date, YYYY-MM-DD; defaults to today")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *priceType == "" {
		return errors.New("usage: 1c get price PRODUCT_GUID --price-type NAME [--characteristic GUID] [--as-of YYYY-MM-DD] [--json]")
	}
	quote, err := svc.GetPrice(ctx, flags.Arg(0), *priceType, *characteristic, *asOf)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(quote)
	}
	if !quote.Found {
		_, err = fmt.Fprintf(out, "No price for %s (%s), type %s, as of %s\n", flat(quote.ProductName), quote.ProductID, flat(quote.PriceTypeName), quote.AsOf)
		return err
	}
	_, err = fmt.Fprintf(out, "%s: %s (type %s, as of %s; document %s at %s)\n", flat(quote.ProductName), quote.Price, flat(quote.PriceTypeName), quote.AsOf, quote.SourceDocumentID, quote.SourceDate)
	return err
}
