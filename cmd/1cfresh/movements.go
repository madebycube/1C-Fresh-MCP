package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runMovements(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetMovements
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "document kind")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 || *kind == "" {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.GetDocumentMovements(ctx, *kind, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	if len(result.Registers) == 0 {
		_, err := fmt.Fprintln(out, "No movements in the shown registers; the document may be unposted.")
		return err
	}
	for _, register := range result.Registers {
		if _, err := fmt.Fprintf(out, "%s (%d rows)\n", register.Register, register.Count); err != nil {
			return err
		}
		writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		for index, row := range register.Rows {
			fields := make([]string, 0, len(row))
			for field := range row {
				fields = append(fields, field)
			}
			sort.Strings(fields)
			for _, field := range fields {
				if _, err := fmt.Fprintf(writer, "  %d\t%s\t%s\n", index+1, field, flat(fmt.Sprint(row[field]))); err != nil {
					return err
				}
			}
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
	return nil
}
