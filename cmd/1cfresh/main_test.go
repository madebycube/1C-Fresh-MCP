package main

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"
)

func TestHelpExplainsCommandsWithoutCredentials(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"login", "--help"}, {"list", "groups", "--help"}, {"create", "group", "--help"}, {"update", "group", "--help"}, {"update", "product", "--help"}, {"help", "list", "price-types"}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err != nil {
			t.Fatalf("help %v: %v", args, err)
		}
		if !strings.Contains(output.String(), "1c ") {
			t.Fatalf("help %v has no example or usage: %s", args, output.String())
		}
	}
}

func TestFlagsCanFollowPositionalArguments(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	limit := flags.Int("limit", 20, "")
	asJSON := flags.Bool("json", false, "")
	if err := parseFlags(flags, []string{"диван", "--json", "--limit", "2"}); err != nil {
		t.Fatal(err)
	}
	if flags.NArg() != 1 || flags.Arg(0) != "диван" || !*asJSON || *limit != 2 {
		t.Fatalf("args=%v json=%t limit=%d", flags.Args(), *asJSON, *limit)
	}
}

func TestLegacyCommandsRemainAliases(t *testing.T) {
	for legacy, canonical := range map[string]string{
		"products search": "search products", "orders list": "list orders", "orders get": "get order",
		"receipts list": "list receipts", "receipts get": "get receipt", "receipts audit-unposted": "audit receipts",
	} {
		command, _ := parseCommand(strings.Fields(legacy))
		if command != canonical {
			t.Errorf("%s became %s; want %s", legacy, command, canonical)
		}
	}
}
