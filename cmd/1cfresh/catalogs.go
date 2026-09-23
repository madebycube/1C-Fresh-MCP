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

func runProductList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListProducts.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 20, "maximum products (1-100)")
	offset := flags.Int("offset", 0, "raw catalog offset from the previous page")
	group := flags.String("group", "", "direct parent group GUID or root")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListProducts.Usage)
	}
	page, err := svc.ListProductsInGroup(ctx, *limit, *offset, *group)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "CODE\tARTICLE\tNAME\tGROUP ID\tID"); err != nil {
		return err
	}
	for _, product := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", flat(product.Code), flat(product.Article), flat(product.Name), product.ParentID, product.ID); err != nil {
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

func runProductGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.GetProduct.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + operations.GetProduct.Usage)
	}
	product, err := svc.GetProduct(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(product)
	}
	_, err = fmt.Fprintf(out, "ID: %s\nCode: %s\nName: %s\nFull name: %s\nArticle: %s\nGroup ID: %s\nType: %s\nUnit ID: %s\n", product.ID, flat(product.Code), flat(product.Name), flat(product.FullName), flat(product.Article), product.ParentID, flat(product.Type), product.UnitID)
	return err
}

func runGroupList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list groups", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "match this name or path")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list groups [--name TEXT] [--json]")
	}
	groups, err := svc.ListGroups(ctx, *name)
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
	_, err = fmt.Fprintf(out, "%d groups\n", len(groups))
	return err
}

func runPriceTypeList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("list price-types", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: 1c list price-types [--json]")
	}
	priceTypes, err := svc.ListPriceTypes(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(priceTypes)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tINACTIVE\tDELETED\tID"); err != nil {
		return err
	}
	for _, priceType := range priceTypes {
		if _, err := fmt.Fprintf(writer, "%s\t%t\t%t\t%s\n", flat(priceType.Name), priceType.Inactive, priceType.Deleted, priceType.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d price types\n", len(priceTypes))
	return err
}
