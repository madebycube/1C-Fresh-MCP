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

func runLinkPropertyList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.ListLinkProperties
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	list, err := svc.ListLinkProperties(ctx)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(list)
	}
	for _, property := range list.Properties {
		if _, err := fmt.Fprintf(out, "%s  %s  receipts: %t (%s)  stored on %d receipts  max %d\n", property.ID, flat(property.Description), property.ForReceipts, property.ReceiptsReason, property.ReceiptsUsing, property.MaxLength); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "%d hyperlink properties, %d receipts read\n", len(list.Properties), list.ReceiptsRead)
	return err
}

func runReceiptLinkGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetReceiptLink
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	property := flags.String("property", "", "property GUID or name")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	link, err := svc.GetReceiptLink(ctx, flags.Arg(0), *property)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(link)
	}
	return printReceiptLink(out, "", link)
}

func runReceiptLinkSet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.SetReceiptLink
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.ReceiptLinkChange
	address := flags.String("url", "", "link address")
	caption := flags.String("caption", "", "link caption")
	flags.StringVar(&change.Property, "property", "", "property GUID or name")
	flags.BoolVar(&change.Clear, "clear", false, "remove the link")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&change.Apply, "apply", false, "save the change")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "url":
			change.URL = address
		case "caption":
			change.Caption = caption
		}
	})
	result, err := svc.SetReceiptLink(ctx, flags.Arg(0), change)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	switch {
	case result.Unchanged:
		state = "Unchanged (nothing saved)"
	case result.Applied:
		state = "Saved"
	}
	if _, err := fmt.Fprintln(out, state); err != nil {
		return err
	}
	if err := printReceiptLink(out, "Before ", result.Before); err != nil {
		return err
	}
	if err := printReceiptLink(out, "After ", result.After); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Impact: "+strings.Join(result.Impact, "; ")); err != nil {
		return err
	}
	if !result.Applied && !result.Unchanged {
		_, err := fmt.Fprintln(out, "Run again with --apply --expect-version "+result.Before.DataVersion+" to save.")
		return err
	}
	return nil
}

func printReceiptLink(out io.Writer, prefix string, link service.ReceiptLink) error {
	_, err := fmt.Fprintf(out, "%sreceipt %s %s (posted %t, version %s)\n  property: %s\n  format: %s\n  url: %s\n  caption: %s\n", prefix, link.Number, link.ReceiptID, link.Posted, link.DataVersion, flat(link.Property), link.Format, flat(link.URL), flat(link.Caption))
	return err
}
