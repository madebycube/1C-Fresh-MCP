# 1C-Fresh CLI and MCP

A local, read-only interface to a 1C-Fresh application through its standard OData endpoint. The CLI and MCP server use the same service and OData client. OData resource names and field mappings live in `internal/config/resources.go`.

## Setup

Requires Go 1.25 or newer. Copy `.env.example` to `.env` and set the application URL, username, and password. The existing `.env` file in this workspace is already ignored by Git. Process environment variables override file values. Set `ONEC_ENV_FILE` to load a different file.

```sh
go build -o bin/1cfresh ./cmd/1cfresh
bin/1cfresh check
bin/1cfresh products search --limit 10 "диван"
bin/1cfresh products search --json "диван"
bin/1cfresh orders list --limit 20
bin/1cfresh orders get --json ORDER_GUID
bin/1cfresh receipts list --kind sale --from 2026-09-01 --to 2026-09-01
bin/1cfresh receipts list --kind refund --from 2026-09-01 --to 2026-09-01 --json
bin/1cfresh receipts get --kind refund --json RECEIPT_GUID
bin/1cfresh receipts audit-unposted --from 2026-09-01 --before 2026-09-24 --json
```

The CLI prints a table by default and JSON with `--json`. Product searches inspect name, full name, and article, exclude folders and deletion-marked records, and return at most 50 products. Order listing returns the most recent non-deleted orders, up to 100; order lookup returns the stock line items. Receipt listing uses an inclusive application date range of at most 31 days, returns up to 100 records per page, and exposes `next_offset` in JSON for further pages. Receipt lookup includes stock lines and cashless payments. Amounts and quantities are decimal strings in JSON so their source precision is preserved. No command writes to 1C.

## Receipt audit

`receipts audit-unposted` and `audit_unposted_receipts` inspect the non-deleted `Document_ЧекККМ` (sale) and `Document_ЧекККМВозврат` (refund) resources. They report documents with `Posted=false` in the application date interval [`from`, `before`). The report includes the kind, document ID, number, date, posted status, amount, and available related IDs, plus the number of receipts inspected. For example, a sale receipt numbered `REDACTED` dated `2026-09-02T12:00:00` with `Posted=false` is a finding; a posted receipt on the same date is inspected but omitted. The application returns dates without a timezone offset, so the audit compares their calendar dates directly and makes no UTC conversion.

An unposted receipt can be an intentional draft or a cancelled workflow; the audit identifies the status for review and does not infer an accounting error. Deleted documents are excluded. The interval is at most 31 calendar days, with at most 1,000 receipts across both kinds. Larger scans fail and require smaller intervals; there is no partial report.

## MCP

Run the binary as a local stdio MCP server:

```sh
bin/1cfresh mcp
```

Configure an MCP client to launch that command from this directory or provide the three `ONEC_ODATA_*` variables in the client configuration. The server exposes `check_connection`, `find_nomenclature`, `list_customer_orders`, `get_customer_order`, `list_cash_receipts`, `get_cash_receipt`, and `audit_unposted_receipts`. All are read-only. No generic OData query, write, delete, or posting tool is exposed.

The current 1C application rejects OData filters on document `Date` with HTTP 500 and ignores descending date order. Order listing uses a count and the tail of the ascending date order. Receipt date ranges use count, indexed date lookups, and bounded pages; returned dates are checked before results are reported.

## Development

```sh
go test ./...
go vet ./...
```

The OData client uses HTTPS, HTTP Basic authentication, a 30-second timeout, a response-size limit, and no redirects. Errors omit credentials and response bodies. The MCP server writes protocol data to stdout; CLI errors go to stderr.
