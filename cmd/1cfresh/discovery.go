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

func runResourceSearch(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("search resources", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "catalog, document, register, or other")
	limit := flags.Int("limit", 50, "maximum results (1-200)")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c search resources [--kind catalog|document|register|other] [--limit N] [--json] QUERY")
	}
	result, err := svc.SearchResources(ctx, flags.Arg(0), *kind, *limit)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NAME\tKIND\tFIELDS"); err != nil {
		return err
	}
	for _, item := range result.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%d\n", flat(item.Name), item.Kind, item.PropertyCount); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%d of %d matching resources shown\n", len(result.Items), result.Total)
	return err
}

func runResourceDescribe(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("describe resource", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c describe resource [--json] EXACT_NAME")
	}
	resource, err := svc.DescribeResource(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(resource)
	}
	if _, err := fmt.Fprintf(out, "%s (%s)\nEntity type: %s\nKeys: %v\n\n", resource.Name, resource.Kind, resource.EntityType, resource.Keys); err != nil {
		return err
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "FIELD\tTYPE\tNULLABLE"); err != nil {
		return err
	}
	for _, property := range resource.Properties {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\n", flat(property.Name), property.Type, property.Nullable); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if len(resource.Navigation) > 0 {
		_, err = fmt.Fprintf(out, "\nLinks: %v\n", resource.Navigation)
	}
	return err
}
