[Russian version](README.md)

![1C-Fresh CLI and MCP](Images/EnglishREADMEBanner.png)

A Go CLI and local MCP server for a 1C-Fresh application. Both use the same OData client and support product groups, price types and lookups, products, warehouses and stock, customers and suppliers, sales and purchase documents, warehouse documents, money movements, cash receipts, and OData schema discovery. Writes currently cover product groups and selected product fields.

## Requirements

- Go 1.25 or newer
- A 1C-Fresh application URL and an account with access to its OData data

## Configure

Run `1c login` after building the CLI. It prompts for `1C Link/Ссылка на 1C`, `User/Юзер`, and a hidden `Password/Пароль`, checks OData access, then saves `.env` with permissions limited to your account. Existing URL and user values are offered as defaults. A blank password keeps the saved one.

For manual setup, copy `.env.example` to `.env`, then set the URL and credentials:

```dotenv
ONEC_ODATA_BASE_URL=https://your-1cfresh-host/a/your-app/your-tenant
ONEC_ODATA_USERNAME=your-username
ONEC_ODATA_PASSWORD=your-password
```

Use the application URL, not an `/odata/` endpoint. Keep credentials out of source control; `.env` is ignored by Git. Environment variables override values in the file. Set `ONEC_ENV_FILE` to load a different file. By default, the CLI reads `.env` from the current directory. When run through a binary in this project's `bin` directory, it can also find the project `.env`.

## Install

Build the CLI from the repository root:

```sh
go build -o bin/1c ./cmd/1cfresh
```

Run `bin/1c --help` to see the commands. To run `1c` from anywhere while using the project `.env`, symlink `bin/1c` into a directory on your `PATH`:

```sh
ln -s /path/to/1C-Fresh-MCP/bin/1c "$HOME/.local/bin/1c"
```

If you copy the binary elsewhere, provide `ONEC_ENV_FILE` or the `ONEC_ODATA_*` environment variables.

### Prompt for an agent

Copy this into a task for an agent with access to your computer and the private repository:

```text
Install https://github.com/madebycube/1C-Fresh-MCP on my computer. Clone the repository using my GitHub access, build the CLI with go build -o bin/1c ./cmd/1cfresh, and symlink the built bin/1c into a directory on PATH so I can run 1c. Configure my MCP client to launch the absolute path to bin/1c with the mcp argument. Verify installation with 1c --help. Let me enter credentials using 1c login in the terminal; do not request my password in chat. After login, verify the connection with 1c check. If repository access or MCP client configuration is unavailable, identify the exact blocker.
```

## CLI syntax

Commands follow `1c VERB RESOURCE [OPTIONS]`. Use `--help` on a command for its usage and an example. Commands print readable tables by default; add `--json` for structured output.

```sh
1c check
1c login
1c list groups --name "Пример группы"
1c create group --name "Новая группа"
1c create group --name "Подгруппа" --parent PARENT_GUID
1c update group GROUP_GUID --name "Новое название"
1c update group GROUP_GUID --parent PARENT_GUID
1c update product PRODUCT_GUID --name "New name" --article NEW-ARTICLE
1c update product PRODUCT_GUID --group GROUP_GUID
1c list price-types
1c get price PRODUCT_GUID --price-type "Пример цены" --as-of 2026-09-23
1c list warehouses
1c get stock PRODUCT_GUID --warehouse WAREHOUSE_GUID
1c list products --limit 20
1c list products --limit 20 --offset NEXT_OFFSET
1c get product PRODUCT_GUID --json
1c search products --limit 10 "название товара"
1c list customers --limit 20
1c search customers "Example company"
1c get customer CUSTOMER_GUID
1c create customer --name "Example customer"
1c update customer CUSTOMER_GUID --name "New name"
1c list suppliers --limit 20
1c search suppliers "Example supplier"
1c get supplier SUPPLIER_GUID
1c create supplier --name "Example supplier"
1c update supplier SUPPLIER_GUID --name "New name"
1c search resources --kind catalog --limit 20 "Номенклатура"
1c describe resource Catalog_Номенклатура
1c list orders --limit 20
1c get order --json ORDER_GUID
1c list sales --kind shipment --limit 20
1c list sales --kind return --customer CUSTOMER_GUID
1c list sales --kind invoice --from 2026-09-01 --to 2026-09-30
1c get sale --kind shipment DOCUMENT_GUID
1c list purchases --kind order --limit 20
1c list purchases --kind receipt --supplier SUPPLIER_GUID
1c get purchase --kind receipt DOCUMENT_GUID
1c list warehouse-docs --kind transfer --warehouse WAREHOUSE_GUID
1c list warehouse-docs --kind stock-writeoff --limit 20
1c get warehouse-doc --kind transfer DOCUMENT_GUID
1c list accounts --kind bank
1c list accounts --kind cash
1c list accounts --kind register
1c list money --kind bank-in --account BANK_ACCOUNT_GUID --from 2026-09-01 --to 2026-09-07
1c list money --kind card-payment --register REGISTER_GUID --from 2026-09-01 --to 2026-09-07
1c get money --kind bank-in DOCUMENT_GUID
1c list receipts --kind sale --from 2026-09-01 --to 2026-09-07
1c get receipt --kind refund --json RECEIPT_GUID
1c audit receipts --from 2026-09-01 --before 2026-09-24 --json
```

`list groups` returns product folders and their IDs; `list price-types` returns price type names and IDs. Product groups and price types are separate entities. These commands do not retrieve prices from price documents.

`get price` finds the latest price for a product and named price type as of `--as-of` (today by default). It uses posted, non-deleted documents. Omit `--characteristic GUID` for a price without a characteristic or pass a variant's GUID. When no price exists, the result has `found: false`. JSON includes the exact decimal price string, currency, and source document ID, date, and line number. The command scans price document history, so it may take some time.

`list warehouses` shows warehouses and retail stores from the structural units catalog. `get stock` shows current product balances by warehouse; `--warehouse` selects one, and `--characteristic` selects a product characteristic. Quantities come from the `AccumulationRegister_ЗапасыНаСкладах/Balance` virtual table and are summed across characteristics, batches, cells, and organizations when no characteristic is selected. JSON also includes the source balance rows and product unit ID. If 1C cannot resolve the unit's description, its ID remains available. A stock balance does not promise that the product is available to sell.

`create group` creates a group at the catalog root or under the group specified by `--parent`. `update group` renames a group or moves it with `--parent GROUP_GUID`; `--parent root` moves it to the catalog root. Parent chains are checked to prevent cycles. `update product` changes a product's name, full name (`--full-name`), article (`--article`), and/or group (`--group GUID`). The group must exist and not be deletion-marked. An empty string clears the full name or article. `create customer` and `create supplier` create a counterparty with the matching role flag. The full name defaults to the name; `--parent` selects a counterparty folder. `update customer` and `update supplier` change the name and/or full name of an existing counterparty with the matching role. Pass `--full-name ""` to clear the full name. These commands **change live 1C data**. A live write has not been verified. Product creation, deletion, and generic OData editing are not available yet.

`search resources` uses `1c search resources [--kind catalog|document|register|other] [--limit N] [--json] QUERY` to search OData metadata names by resource kind. `describe resource` uses `1c describe resource [--json] EXACT_NAME` to show schema fields for an exact OData resource name. These commands expose OData schema names and fields; they do not enumerate every screen in the 1C interface or provide generic reads of resource data.

`list products` browses the catalog without a query and excludes folders and deletion-marked records. `get product` reads one active product by GUID, including its product type and base unit reference. `next_offset` is an offset into raw OData rows: pass it as the next `--offset` rather than adding the number of products shown. Each call scans at most 500 rows, so a page can contain fewer than `--limit` products while still returning `next_offset`. Offsets do not provide a snapshot if the catalog changes between requests. Product search checks product name, full name, and article, excludes folders and deletion-marked rows locally, and returns up to 50 matches. It scans at most 500 OData candidates per field. Order listing returns the latest non-deleted orders, up to 100; order details include product lines. Receipt listing accepts an inclusive date range of up to 31 days, returns up to 100 rows per page, and supports `--offset` for the next page. Receipt details include stock lines and cashless payments. JSON represents amounts and quantities as decimal strings to preserve source precision.

`list customers` and `search customers` show counterparties marked as buyers, excluding folders and deletion-marked records. `list sales` reads invoices (`invoice`), customer shipments (`shipment`), and incoming goods documents whose operation is customer return (`return`). Use `--customer`, `--from`, and `--to` to narrow results, then `--limit` and `--offset` for paging. `get sale` includes date, amount, posting status, product lines, and linked order or source document IDs when 1C supplies a matching relationship type. `Posted` means the document was posted in 1C; it does not mean paid. The invoice resource is empty in the tested tenant, so lookup of a live invoice by GUID has not been verified. Listings scan resource history and may take several seconds.

`list suppliers` and `search suppliers` show counterparties marked as suppliers. `list purchases --kind order` reads supplier orders, while `--kind receipt` selects incoming goods documents whose operation is supplier receipt. `list warehouse-docs` reads transfer orders (`transfer-order`), stock movements whose operation is transfer (`transfer`), stock receipts (`stock-receipt`), and stock writeoffs (`stock-writeoff`). Filter by supplier or warehouse ID; a transfer matches either its source or destination warehouse. Both lists support dates, `--limit`, and `--offset`. `get purchase` and `get warehouse-doc` include product lines and available linked document IDs. `Posted` indicates posting in 1C, not fulfillment or physical receipt. There are no transfer orders in the tested tenant, so getting one by GUID has not been verified live.

`list accounts` discovers cash desks (`cash`), organization-owned bank accounts (`bank`), and retail registers (`register`). `list money` reads cash receipts and disbursements (`cash-in`, `cash-out`), bank receipts and disbursements (`bank-in`, `bank-out`), card transactions (`card-payment`), and cash shifts (`cash-shift`). Supply an inclusive date range of at most 31 days and a cash desk, bank account, register, or terminal ID with `--account`, `--register`, or `--terminal`. `--limit` and `--offset` page results; `get money` reads a document by GUID. Payroll, tax, employee-payment, and unknown operation types are excluded. Fiscal receipts, card transactions, and bank settlements are distinct documents; `Posted` indicates 1C posting, not payment or reconciliation. These are read-only commands and may scan resource history for several seconds.

### Audit unposted receipts

`audit receipts` checks sale and refund receipts in the half-open application date interval `[--from, --before)`. It reports non-deleted receipts with `Posted=false`, including their kind, ID, number, date, amount, and available related IDs. The audit covers at most 31 calendar days and 1,000 receipts across both kinds; a larger scan fails without returning a partial report.

An unposted receipt may be a draft or part of a cancelled workflow. The command reports status for review; it does not decide whether a receipt is an accounting error. The application returns dates without timezone offsets, so the audit compares calendar dates directly.

## MCP server

Run the server over stdio:

```sh
bin/1c mcp
```

Configure your MCP client to launch this command from the repository directory, or provide the three `ONEC_ODATA_*` variables in the client's environment. The server exposes:

- `check_connection`
- `list_product_groups` and `list_price_types`
- `get_product_price`
- `list_warehouses` and `get_product_stock`
- `find_nomenclature`
- `list_products` and `get_product`
- `update_product` (write operation)
- `search_odata_resources` and `describe_odata_resource`
- `list_customer_orders` and `get_customer_order`
- `list_customers`, `search_customers`, and `get_customer`
- `create_customer` and `update_customer` (write operations)
- `list_suppliers`, `search_suppliers`, and `get_supplier`
- `create_supplier` and `update_supplier` (write operations)
- `list_sales_documents` and `get_sales_document`
- `list_purchase_documents` and `get_purchase_document`
- `list_warehouse_documents` and `get_warehouse_document`
- `list_money_accounts`, `list_money_documents`, and `get_money_document`
- `list_cash_receipts` and `get_cash_receipt`
- `audit_unposted_receipts`
- `create_product_group` and `update_product_group` (write operations)

No generic OData query or tool for deleting or posting data is exposed.

See the [access policy](docs/access-policy.md) for reporting, personnel, payroll, and tax boundaries.

## Development

Run the Go tests and static checks from the repository root:

```sh
go test ./...
go vet ./...
```

The OData client uses HTTPS, HTTP Basic authentication, a 30-second timeout, a response-size limit, and does not follow redirects. Errors omit credentials and response bodies. MCP protocol output goes to stdout; CLI errors go to stderr.
