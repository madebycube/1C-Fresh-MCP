package main

import (
	"bytes"
	"context"
	"flag"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
)

func TestHelpExplainsCommandsWithoutCredentials(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"login", "--help"}, {"list", "groups", "--help"}, {"list", "product-categories", "--help"}, {"list", "characteristics", "--help"}, {"list", "currencies", "--help"}, {"list", "prices", "--help"}, {"create", "group", "--help"}, {"update", "group", "--help"}, {"list", "products", "--help"}, {"get", "product", "--help"}, {"update", "product", "--help"}, {"get", "price", "--help"}, {"list", "warehouses", "--help"}, {"get", "stock", "--help"}, {"list", "customers", "--help"}, {"search", "customers", "--help"}, {"get", "customer", "--help"}, {"create", "customer", "--help"}, {"update", "customer", "--help"}, {"list", "suppliers", "--help"}, {"search", "suppliers", "--help"}, {"get", "supplier", "--help"}, {"create", "supplier", "--help"}, {"update", "supplier", "--help"}, {"list", "sales", "--help"}, {"get", "sale", "--help"}, {"list", "purchases", "--help"}, {"get", "purchase", "--help"}, {"list", "warehouse-docs", "--help"}, {"get", "warehouse-doc", "--help"}, {"list", "accounts", "--help"}, {"list", "money", "--help"}, {"get", "money", "--help"}, {"help", "list", "price-types"}, {"list", "unit-types", "--help"}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err != nil {
			t.Fatalf("help %v: %v", args, err)
		}
		if !strings.Contains(output.String(), "1c ") {
			t.Fatalf("help %v has no example or usage: %s", args, output.String())
		}
	}
}

func TestTopicHelpKeepsEveryCommandDiscoverable(t *testing.T) {
	var overview bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &overview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(overview.String(), "1c help prices") || strings.Contains(overview.String(), "source documents by ID") {
		t.Fatalf("unexpected overview: %s", overview.String())
	}
	var all bytes.Buffer
	if err := run(context.Background(), []string{"help", "all"}, &all); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, operation := range operations.All {
		if seen[operation.Command] || !strings.Contains(all.String(), operation.Command) {
			t.Fatalf("command missing or repeated: %s", operation.Command)
		}
		seen[operation.Command] = true
	}
	var prices bytes.Buffer
	if err := run(context.Background(), []string{"help", "prices"}, &prices); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prices.String(), "get price-document") || strings.Contains(prices.String(), "list customers") {
		t.Fatalf("unexpected price topic: %s", prices.String())
	}
}

func TestFlagsCanFollowPositionalArguments(t *testing.T) {
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	limit := flags.Int("limit", 20, "")
	asJSON := flags.Bool("json", false, "")
	if err := parseFlags(flags, []string{"название товара", "--json", "--limit", "2"}); err != nil {
		t.Fatal(err)
	}
	if flags.NArg() != 1 || flags.Arg(0) != "название товара" || !*asJSON || *limit != 2 {
		t.Fatalf("args=%v json=%t limit=%d", flags.Args(), *asJSON, *limit)
	}
}

func TestPriceDocumentHelp(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"get", "price-document", "--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "1c get price-document GUID") {
		t.Fatalf("unexpected help: %s", output.String())
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
