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

func runGroupCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("create group", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "new group name")
	parent := flags.String("parent", "", "parent group GUID; omit for root")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *name == "" {
		return errors.New("usage: 1c create group --name NAME [--parent GUID] [--json]")
	}
	change, err := svc.CreateGroup(ctx, *name, *parent)
	if err != nil {
		return err
	}
	return printGroupChange(out, change, *asJSON)
}

func runGroupUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("update group", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	name := flags.String("name", "", "replacement group name")
	parent := flags.String("parent", "", "destination group GUID or root")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: 1c update group GUID [--name NAME] [--parent GROUP_GUID|root] [--json]")
	}
	var patch service.GroupPatch
	flags.Visit(func(option *flag.Flag) {
		switch option.Name {
		case "name":
			patch.Name = name
		case "parent":
			patch.ParentID = parent
		}
	})
	change, err := svc.UpdateGroup(ctx, flags.Arg(0), patch)
	if err != nil {
		return err
	}
	return printGroupChange(out, change, *asJSON)
}

func printGroupChange(out io.Writer, change service.GroupChange, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(change)
	}
	if !change.Applied {
		_, err := fmt.Fprintf(out, "Group already has those values (%s)\n", change.ID)
		return err
	}
	if change.ID == "" {
		_, err := fmt.Fprintf(out, "Created group %s; 1C did not return its ID\n", flat(change.Name))
		return err
	}
	_, err := fmt.Fprintf(out, "Saved group %s (%s)\n", flat(change.Name), change.ID)
	return err
}
