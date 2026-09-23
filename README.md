![1C-Fresh CLI and MCP](Images/READMEHeader.png)

A Go command-line tool and local read-only MCP server for a 1C-Fresh application. Both use the same OData client and support product groups, price types, products, customer orders, and cash receipts.

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

## CLI syntax

Commands follow `1c VERB RESOURCE [OPTIONS]`. Use `--help` on a command for its usage and an example. Commands print readable tables by default; add `--json` for structured output.

```sh
1c check
1c login
1c list groups --name КЛИМОВО
1c list price-types
1c search products --limit 10 "диван"
1c search resources --kind catalog --limit 20 "Номенклатура"
1c describe resource Catalog_Номенклатура
1c list orders --limit 20
1c get order --json ORDER_GUID
1c list receipts --kind sale --from 2026-09-01 --to 2026-09-07
1c get receipt --kind refund --json RECEIPT_GUID
1c audit receipts --from 2026-09-01 --before 2026-09-24 --json
```

`list groups` returns product folders and their IDs; `list price-types` returns price type names and IDs. A price type such as `Розничная` is separate from a product group such as `КЛИМОВО НОМЕНКЛАТУРА`. These commands do not retrieve prices from price documents.

`search resources` uses `1c search resources [--kind catalog|document|register|other] [--limit N] [--json] QUERY` to search OData metadata names by resource kind. `describe resource` uses `1c describe resource [--json] EXACT_NAME` to show schema fields for an exact OData resource name. These commands expose OData schema names and fields; they do not enumerate every screen in the 1C interface or provide generic reads of resource data.

Product search checks product name, full name, and article, and excludes folders and deletion-marked products. It returns up to 50 matches. Order listing returns the latest non-deleted orders, up to 100; order details include product lines. Receipt listing accepts an inclusive date range of up to 31 days, returns up to 100 rows per page, and supports `--offset` for the next page. Receipt details include stock lines and cashless payments. JSON represents amounts and quantities as decimal strings to preserve source precision.

### Audit unposted receipts

`audit receipts` checks sale and refund receipts in the half-open application date interval `[--from, --before)`. It reports non-deleted receipts with `Posted=false`, including their kind, ID, number, date, amount, and available related IDs. The audit covers at most 31 calendar days and 1,000 receipts across both kinds; a larger scan fails without returning a partial report.

An unposted receipt may be a draft or part of a cancelled workflow. The command reports status for review; it does not decide whether a receipt is an accounting error. The application returns dates without timezone offsets, so the audit compares calendar dates directly.

## MCP server

Run the server over stdio:

```sh
bin/1c mcp
```

Configure your MCP client to launch this command from the repository directory, or provide the three `ONEC_ODATA_*` variables in the client's environment. The server exposes these read-only tools:

- `check_connection`
- `list_product_groups` and `list_price_types`
- `find_nomenclature`
- `search_odata_resources` and `describe_odata_resource`
- `list_customer_orders` and `get_customer_order`
- `list_cash_receipts` and `get_cash_receipt`
- `audit_unposted_receipts`

No generic OData query or tool for writing, deleting, or posting data is exposed.

## Development

Run the Go tests and static checks from the repository root:

```sh
go test ./...
go vet ./...
```

The OData client uses HTTPS, HTTP Basic authentication, a 30-second timeout, a response-size limit, and does not follow redirects. Errors omit credentials and response bodies. MCP protocol output goes to stdout; CLI errors go to stderr.
