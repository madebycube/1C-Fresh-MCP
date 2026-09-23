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
		fmt.Fprintln(os.Stderr, "1c:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || len(args) == 1 && args[0] == "help" {
		printHelp(out)
		return nil
	}
	helpRequested := args[0] == "help"
	if helpRequested {
		args = args[1:]
	}
	command, commandArgs := parseCommand(args)
	if command == "" {
		return errors.New("unknown command; run '1c --help'")
	}
	if helpRequested || len(commandArgs) == 1 && (commandArgs[0] == "--help" || commandArgs[0] == "-h") {
		printCommandHelp(out, command)
		return nil
	}
	if command == "login" {
		return runLogin(ctx, commandArgs, out)
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
		if err := parseFlags(flags, commandArgs); err != nil || flags.NArg() != 0 {
			return errors.New("usage: 1c check [--json]")
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
	case operations.ListGroups.Command:
		return runGroupList(ctx, svc, commandArgs, out)
	case operations.CreateGroup.Command:
		return runGroupCreate(ctx, svc, commandArgs, out)
	case operations.UpdateGroup.Command:
		return runGroupUpdate(ctx, svc, commandArgs, out)
	case operations.ListPriceTypes.Command:
		return runPriceTypeList(ctx, svc, commandArgs, out)
	case operations.GetPrice.Command:
		return runPriceGet(ctx, svc, commandArgs, out)
	case operations.ListWarehouses.Command:
		return runWarehouseList(ctx, svc, commandArgs, out)
	case operations.GetStock.Command:
		return runStockGet(ctx, svc, commandArgs, out)
	case operations.SearchProducts.Command:
		flags := flag.NewFlagSet("search products", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		limit := flags.Int("limit", 20, "maximum results (1-50)")
		asJSON := flags.Bool("json", false, "print JSON")
		if err := parseFlags(flags, commandArgs); err != nil || flags.NArg() != 1 {
			return errors.New("usage: 1c search products [--limit N] [--json] QUERY")
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
	case operations.UpdateProduct.Command:
		return runProductUpdate(ctx, svc, commandArgs, out)
	case operations.ListOrders.Command:
		return runOrderList(ctx, svc, commandArgs, out)
	case operations.GetOrder.Command:
		return runOrderGet(ctx, svc, commandArgs, out)
	case operations.ListCustomers.Command:
		return runCustomerList(ctx, svc, commandArgs, out, false, "customer")
	case operations.SearchCustomers.Command:
		return runCustomerList(ctx, svc, commandArgs, out, true, "customer")
	case operations.GetCustomer.Command:
		return runCustomerGet(ctx, svc, commandArgs, out, "customer")
	case operations.ListSuppliers.Command:
		return runCustomerList(ctx, svc, commandArgs, out, false, "supplier")
	case operations.SearchSuppliers.Command:
		return runCustomerList(ctx, svc, commandArgs, out, true, "supplier")
	case operations.GetSupplier.Command:
		return runCustomerGet(ctx, svc, commandArgs, out, "supplier")
	case operations.ListSales.Command:
		return runSalesList(ctx, svc, commandArgs, out)
	case operations.GetSale.Command:
		return runSalesGet(ctx, svc, commandArgs, out)
	case operations.ListPurchases.Command:
		return runOperationalList(ctx, svc, commandArgs, out, "purchase")
	case operations.GetPurchase.Command:
		return runOperationalGet(ctx, svc, commandArgs, out, "purchase")
	case operations.ListWarehouseDocuments.Command:
		return runOperationalList(ctx, svc, commandArgs, out, "warehouse")
	case operations.GetWarehouseDocument.Command:
		return runOperationalGet(ctx, svc, commandArgs, out, "warehouse")
	case operations.ListMoneyAccounts.Command:
		return runMoneyAccountList(ctx, svc, commandArgs, out)
	case operations.ListMoney.Command:
		return runMoneyList(ctx, svc, commandArgs, out)
	case operations.GetMoney.Command:
		return runMoneyGet(ctx, svc, commandArgs, out)
	case operations.ListReceipts.Command:
		return runReceiptList(ctx, svc, commandArgs, out)
	case operations.GetReceipt.Command:
		return runReceiptGet(ctx, svc, commandArgs, out)
	case operations.AuditUnpostedReceipts.Command:
		return runReceiptAudit(ctx, svc, commandArgs, out)
	case operations.SearchResources.Command:
		return runResourceSearch(ctx, svc, commandArgs, out)
	case operations.DescribeResource.Command:
		return runResourceDescribe(ctx, svc, commandArgs, out)
	case "mcp":
		if len(commandArgs) != 0 {
			return errors.New("usage: 1c mcp")
		}
		return mcpserver.New(svc).Run(ctx, &mcp.StdioTransport{})
	}
	return nil
}

func parseCommand(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	command := args[0]
	remaining := args[1:]
	if command != "check" && command != "mcp" && command != "login" {
		if len(args) < 2 {
			return "", nil
		}
		command = args[0] + " " + args[1]
		remaining = args[2:]
	}
	aliases := map[string]string{
		"products search":         operations.SearchProducts.Command,
		"orders list":             operations.ListOrders.Command,
		"orders get":              operations.GetOrder.Command,
		"receipts list":           operations.ListReceipts.Command,
		"receipts get":            operations.GetReceipt.Command,
		"receipts audit-unposted": operations.AuditUnpostedReceipts.Command,
		"list group":              operations.ListGroups.Command,
		"list price-type":         operations.ListPriceTypes.Command,
	}
	if canonical, ok := aliases[command]; ok {
		command = canonical
	}
	if command == "mcp" || command == "login" {
		return command, remaining
	}
	for _, operation := range operations.All {
		if operation.Command == command {
			return command, remaining
		}
	}
	return "", nil
}

func flat(value string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(value)
}

func printHelp(out io.Writer) {
	fmt.Fprintln(out, "1c reads products, prices, stock, sales, purchases, warehouse, money, and retail data from 1C-Fresh.")
	fmt.Fprintln(out, "Create and update commands write to 1C. 'Posted' means a document was processed in 1C; it does not prove payment, receipt, or fulfillment.")
	fmt.Fprintln(out, "\nUsage: 1c VERB RESOURCE [OPTIONS]")
	fmt.Fprintln(out, "       1c COMMAND --help")
	fmt.Fprintln(out, "\nCommands:")
	fmt.Fprintln(out, "  login              Connect to 1C and save credentials in .env.")
	for _, operation := range operations.All {
		if !operation.Advanced {
			fmt.Fprintf(out, "  %-18s %s\n", operation.Command, operation.Description)
		}
	}
	fmt.Fprintln(out, "  mcp                Run the MCP server for AI clients.")
	fmt.Fprintln(out, "\nExplore the raw 1C schema:")
	for _, operation := range operations.All {
		if operation.Advanced {
			fmt.Fprintf(out, "  %-18s %s\n", operation.Command, operation.Description)
		}
	}
	fmt.Fprintln(out, "\nExamples:")
	fmt.Fprintln(out, "  1c list groups --name \"Пример группы\"")
	fmt.Fprintln(out, "  1c list price-types")
	fmt.Fprintln(out, "  1c search products \"название товара\"")
	fmt.Fprintln(out, "  1c search customers \"Пример компании\"")
	fmt.Fprintln(out, "  1c search suppliers \"Пример поставщика\"")
	fmt.Fprintln(out, "  1c list sales --kind shipment --limit 20")
	fmt.Fprintln(out, "  1c list purchases --kind receipt --limit 20")
	fmt.Fprintln(out, "  1c list warehouse-docs --kind transfer --limit 20")
	fmt.Fprintln(out, "  1c list accounts --kind bank")
	fmt.Fprintln(out, "  1c list money --kind bank-in --from 2026-09-01 --to 2026-09-30 --account BANK_ACCOUNT_GUID")
	fmt.Fprintln(out, "  1c list receipts --from 2026-09-01 --to 2026-09-07")
	fmt.Fprintln(out, "\nGroups are product folders. Price types are labels in a separate catalog.")
	fmt.Fprintln(out, "Use --json for structured output. Commands read credentials from .env or ONEC_ODATA_* environment variables.")
}

func printCommandHelp(out io.Writer, command string) {
	if command == "login" {
		fmt.Fprintln(out, "Prompt for the 1C link, user, and hidden password; check OData access; save .env.\n\nUsage: 1c login")
		return
	}
	if command == "mcp" {
		fmt.Fprintln(out, "Run the MCP server over stdio for an AI client. Read tools and product group write tools are available.\n\nUsage: 1c mcp")
		return
	}
	for _, operation := range operations.All {
		if operation.Command == command {
			fmt.Fprintf(out, "%s\n\nUsage: %s\nExample: %s\n", operation.Description, operation.Usage, operation.Example)
			return
		}
	}
}
