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

func runPriceList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListPrices.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	priceType := flags.String("price-type", "", "price type name")
	group := flags.String("group", "", "direct parent group GUID or root")
	characteristic := flags.String("characteristic", "", "product characteristic GUID")
	asOf := flags.String("as-of", "", "application date, YYYY-MM-DD; defaults to today")
	limit := flags.Int("limit", 20, "maximum products (1-100)")
	offset := flags.Int("offset", 0, "raw catalog offset from the previous page")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *priceType == "" {
		return errors.New("usage: " + operations.ListPrices.Usage)
	}
	page, err := svc.ListPrices(ctx, *priceType, *group, *characteristic, *asOf, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CODE\tARTICLE\tPRODUCT\tFOUND\tPRICE\tCURRENCY ID\tSOURCE DOCUMENT\tPRODUCT ID"); err != nil {
		return err
	}
	for _, quote := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%t\t%s\t%s\t%s\t%s\n", flat(quote.ProductCode), flat(quote.ProductArticle), flat(quote.ProductName), quote.Found, quote.Price, quote.CurrencyID, quote.SourceDocumentID, quote.ProductID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if page.NextOffset != nil {
		_, err = fmt.Fprintf(out, "%d products; scanned %d catalog records; next offset %d\n", len(page.Items), page.Scanned, *page.NextOffset)
	} else {
		_, err = fmt.Fprintf(out, "%d products; end of catalog\n", len(page.Items))
	}
	return err
}

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
