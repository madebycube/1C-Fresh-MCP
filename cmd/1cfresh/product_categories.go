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

func runProductCategoryList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.ListProductCategories.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "match a category name or path")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + operations.ListProductCategories.Usage)
	}
	categories, err := svc.ListProductCategories(ctx, *name)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(categories)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "PATH\tTYPE\tUNIT ID\tID"); err != nil {
		return err
	}
	for _, category := range categories {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", flat(category.Path), flat(category.Type), category.UnitID, category.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d product categories\n", len(categories))
	return err
}
