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

func runProductUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("update product", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "replacement product name")
	fullName := flags.String("full-name", "", "replacement full name")
	article := flags.String("article", "", "replacement article")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c update product GUID [--name TEXT] [--full-name TEXT] [--article TEXT] [--json]")
	}
	var patch service.ProductPatch
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "name":
			patch.Name = name
		case "full-name":
			patch.FullName = fullName
		case "article":
			patch.Article = article
		}
	})
	change, err := svc.UpdateProduct(ctx, flags.Arg(0), patch)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	if !change.Applied {
		_, err = fmt.Fprintf(out, "Product already has those values (%s)\n", change.ID)
	} else {
		_, err = fmt.Fprintf(out, "Updated product %s (%s)\n", flat(change.Name), change.ID)
	}
	return err
}
