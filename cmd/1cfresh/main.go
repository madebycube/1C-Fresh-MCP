package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"github.com/madebycube/1C-Fresh-MCP/internal/mcpserver"
	"github.com/madebycube/1C-Fresh-MCP/internal/odata"
	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "1cfresh:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printHelp(out)
		return nil
	}
	command := args[0]
	if command == "products" && len(args) > 1 {
		command += " " + args[1]
		args = args[1:]
	}
	if command != operations.Check.Command && command != operations.SearchProducts.Command && command != "mcp" {
		return errors.New("unknown command; run '1cfresh help'")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	svc := service.Service{OData: odata.New(cfg)}
	switch command {
	case operations.Check.Command:
		flags := flag.NewFlagSet("check", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		asJSON := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			return errors.New("usage: 1cfresh check [--json]")
		}
		count, err := svc.Check(ctx)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(struct {
				OK        bool `json:"ok"`
				Resources int  `json:"resources"`
			}{OK: true, Resources: count})
		}
		_, err = fmt.Fprintf(out, "OData connection OK; %d resources available\n", count)
		return err
	case operations.SearchProducts.Command:
		flags := flag.NewFlagSet("products search", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		limit := flags.Int("limit", 20, "maximum results (1-50)")
		asJSON := flags.Bool("json", false, "print JSON")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 1 {
			return errors.New("usage: 1cfresh products search [--limit N] [--json] QUERY")
		}
		products, err := svc.SearchProducts(ctx, flags.Arg(0), *limit)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(products)
		}
		writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(writer, "CODE\tARTICLE\tNAME\tID"); err != nil {
			return err
		}
		for _, product := range products {
			if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", flat(product.Code), flat(product.Article), flat(product.Name), product.ID); err != nil {
				return err
			}
		}
		return writer.Flush()
	case "mcp":
		if len(args) != 1 {
			return errors.New("usage: 1cfresh mcp")
		}
		return mcpserver.New(svc).Run(ctx, &mcp.StdioTransport{})
	}
	return nil
}

func flat(value string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(value)
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "Usage: 1cfresh COMMAND")
	fmt.Fprintln(out)
	for _, operation := range operations.All {
		fmt.Fprintf(out, "  %-22s %s\n", operation.Command, operation.Description)
	}
	fmt.Fprintln(out, "  mcp                    Run the MCP server over stdio.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Environment: ONEC_ODATA_BASE_URL, ONEC_ODATA_USERNAME, ONEC_ODATA_PASSWORD")
	fmt.Fprintln(out, "Reads .env in the working directory; ONEC_ENV_FILE selects another file.")
}
