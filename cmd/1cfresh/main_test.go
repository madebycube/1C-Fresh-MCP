package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpExplainsCommandsWithoutCredentials(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"list", "groups", "--help"}, {"help", "list", "price-types"}} {
		var output bytes.Buffer
		if err := run(context.Background(), args, &output); err != nil {
			t.Fatalf("help %v: %v", args, err)
		}
		if !strings.Contains(output.String(), "1c ") {
			t.Fatalf("help %v has no example or usage: %s", args, output.String())
		}
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
