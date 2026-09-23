# 1C-Fresh CLI and MCP

A local, read-only interface to a 1C-Fresh application through its standard OData endpoint. The CLI and MCP server use the same service and OData client. OData resource names and field mappings live in `internal/config/resources.go`.

## Setup

Requires Go 1.25 or newer. Copy `.env.example` to `.env` and set the application URL, username, and password. The existing `.env` file in this workspace is already ignored by Git. Process environment variables override file values. Set `ONEC_ENV_FILE` to load a different file.

```sh
go build -o bin/1cfresh ./cmd/1cfresh
bin/1cfresh check
bin/1cfresh products search --limit 10 "диван"
bin/1cfresh products search --json "диван"
```

The CLI prints a table by default and JSON with `--json`. Searches inspect product name, full name, and article, exclude folders and deletion-marked records, and return at most 50 products. No command writes to 1C.

## MCP

Run the binary as a local stdio MCP server:

```sh
bin/1cfresh mcp
```

Configure an MCP client to launch that command from this directory or provide the three `ONEC_ODATA_*` variables in the client configuration. The server exposes `check_connection` and `find_nomenclature`. Both are read-only. No generic OData query, write, delete, or posting tool is exposed.

## Development

```sh
go test ./...
go vet ./...
```

The OData client uses HTTPS, HTTP Basic authentication, a 30-second timeout, a response-size limit, and no redirects. Errors omit credentials and response bodies. The MCP server writes protocol data to stdout; CLI errors go to stderr.
