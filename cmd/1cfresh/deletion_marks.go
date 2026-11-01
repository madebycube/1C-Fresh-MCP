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

func runDeletionMark(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.SetDeletionMark
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	clear := flags.Bool("clear", false, "clear the deletion mark")
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "make the change")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 2 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetDeletionMark(ctx, flags.Arg(0), flags.Arg(1), !*clear, *expected, *apply)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview"
	if result.Applied {
		state = "Done"
	}
	if _, err := fmt.Fprintf(out, "%s: deletion mark on %s %s %s\nMarked: %t -> %t\nData version: %s\nImpact: %s\n", state, result.Kind, flat(result.Label), result.ID, result.MarkedFrom, !*clear, result.DataVersion, result.Impact); err != nil {
		return err
	}
	if !result.Applied && result.MarkedFrom == *clear {
		_, err := fmt.Fprintln(out, "Run again with --apply to make the change.")
		return err
	}
	return nil
}
