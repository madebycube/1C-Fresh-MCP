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

func runProductCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(operations.CreateProduct.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "product name")
	fullName := flags.String("full-name", "", "optional full name")
	article := flags.String("article", "", "optional article")
	productType := flags.String("type", "", "stock or service")
	unitID := flags.String("unit", "", "active measurement-unit classifier GUID")
	groupID := flags.String("group", "", "optional product group GUID")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *name == "" || *productType == "" || *unitID == "" {
		return errors.New("usage: " + operations.CreateProduct.Usage)
	}
	created, err := svc.CreateProduct(ctx, service.ProductCreate{
		Name: *name, FullName: *fullName, Article: *article,
		Type: *productType, UnitID: *unitID, GroupID: *groupID,
	})
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(created)
	}
	_, err = fmt.Fprintf(out, "Created product %s (%s)\n", flat(created.Name), created.ID)
	return err
}
