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

func runDraftPosting(ctx context.Context, svc service.Service, args []string, out io.Writer, kind string, spec operations.Spec, action string) error {
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "run the action")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetDraftPosting(ctx, kind, flags.Arg(0), action, *expected, *apply)
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
	if _, err := fmt.Fprintf(out, "%s: %s %s %s %s\nPosted: %t -> %t\nLines: %d\nData version: %s\n", state, action, kind, flat(result.Number), result.ID, result.PostedFrom, result.Posted, result.Lines, result.DataVersion); err != nil {
		return err
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied && result.PostedFrom != (action == "post") {
		_, err := fmt.Fprintln(out, "Run again with --apply to "+action+".")
		return err
	}
	return nil
}
