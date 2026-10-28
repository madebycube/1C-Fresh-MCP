# AGENTS.md

Guidance for coding agents working in this repository. Read this before you change code.

## What this is

`1c` is a Go CLI and a local stdio MCP server for a 1C-Fresh application. Both talk to 1C through the same OData client and the same service layer, so every operation behaves the same in the terminal and in an MCP client. The data is a real business's books: products, prices, stock, customers, suppliers, documents, and money. Treat it that way.

## Commands

Requires Go 1.25+ (see `go.mod`).

```sh
go build -o bin/1c ./cmd/1cfresh   # build the CLI (bin/ is gitignored)
go test -race ./...                # what CI runs
go vet ./...
go build ./...
gofmt -l .                         # must print nothing
bin/1c --help                      # topic overview; works without credentials
bin/1c help all                    # every command
bin/1c mcp                         # run the MCP server over stdio
```

CI (`.github/workflows/ci.yml`) runs `go test -race ./...`, `go vet ./...`, and `go build ./...` on every PR. Run all three before you push.

Tests never touch a real 1C tenant. They use in-process stubs of the OData reader/writer. Do not add tests that need network access or credentials.

## Layout

| Path | Role |
| --- | --- |
| `cmd/1cfresh/` | CLI entry point. `main.go` dispatches commands; one file per domain (`prices.go`, `group_writes.go`, ...) parses flags and prints tables or `--json`. `login.go` writes `.env`. |
| `internal/operations/specs.go` | Single source of truth for every operation: CLI command, MCP tool name, description, usage, example. `Topics` groups them for help; `All` lists them. |
| `internal/service/` | Business logic and all validation. One file per domain plus `_test.go`. `Service{OData: Reader}` is the only entry point. |
| `internal/mcpserver/server.go` | Registers MCP tools: input structs with `jsonschema` tags, annotations, and thin handlers that call the service. |
| `internal/odata/client.go` | HTTP client for `/odata/standard.odata/`. Basic auth, 30s timeout, no redirects, size-limited responses. `Write` allows only POST and PATCH, and PATCH requires `If-Match`. |
| `internal/config/` | `.env` loading and saving (`config.go`, `save.go`) and OData resource and field mappings (`resources.go`). |
| `docs/access-policy.md` | What the CLI and MCP may and may not expose. Read it before adding any new data area. |
| `README.md` / `README.en.md` | Russian (primary) and English user docs. Keep them in sync. |

## Configuration and secrets

- Config comes from `.env` (or the file named by `ONEC_ENV_FILE`) and is overridden by environment variables: `ONEC_ODATA_BASE_URL`, `ONEC_ODATA_USERNAME`, `ONEC_ODATA_PASSWORD`. See `.env.example`.
- The base URL must be the HTTPS application URL, without `/odata/`, credentials, query, or fragment. `config.New` enforces this.
- Never commit `.env`, print credentials, put them in tests, logs, or errors, or ask a person for their password in chat. They run `1c login` themselves.
- Use made-up names and zero-padded GUIDs in tests, examples, and docs (`00000000-0000-0000-0000-000000000004`, `"Пример товара"`). Never paste real tenant data, customer names, or GUIDs from a live system.

## Adding an operation

A new read or write usually touches the same set of files. Commit #80 (price documents) and #60 (product creation) are good models.

1. **Spec**: add a `Spec` in `internal/operations/specs.go` with `Command` (`"VERB RESOURCE"`), `Tool` (snake_case), `Description`, `Usage`, `Example`. Add it to `All` and to the right entry in `Topics`. The help test fails if a command is missing or repeated.
2. **Resource mapping**: if it reads a new entity set, describe it in `internal/config/resources.go` (entity name, filter, `FieldBinding`s from 1C field names to output names). Do not build OData resource names from user input.
3. **Service**: implement it in `internal/service/<domain>.go` as a method on `Service`. All validation lives here, not in the CLI or MCP layer. Add `<domain>_test.go` with a stub `Reader` (see `group_writes_test.go`).
4. **CLI**: add a `run...` function in `cmd/1cfresh/<domain>.go` using `flag.NewFlagSet(..., flag.ContinueOnError)`, `SetOutput(io.Discard)`, `parseFlags`, and a `--json` flag. On bad input return `errors.New("usage: " + ...)` matching the spec's `Usage`. Wire it into the `switch` in `main.go` using `operations.X.Command`. Pass free text through `flat()` before printing it in tables.
5. **MCP**: add an input struct with `json` and `jsonschema` tags and an `mcp.AddTool` call in `internal/mcpserver/server.go` using `operations.X.Tool` and `.Description`. Default `limit` to 20 when it is zero, as the other list tools do.
6. **MCP test**: in `internal/mcpserver/server_test.go`, bump the tool count (currently 49), add a call case, extend `stubReader` if needed, and add write tools to the annotation checks.
7. **Docs**: add the command to both `README.md` and `README.en.md`.

### Conventions

- IDs are GUIDs. Validate with `guidPattern`. Use `linkedGUID` when the zero GUID is not allowed. `root` maps to the empty GUID for parent fields.
- Lists are bounded: `limit` 1 to 100 (search 1 to 50), default 20, with `offset` for paging. Date ranges use `YYYY-MM-DD`, and `from` requires `to`. Keep high-volume reads behind a required range, like receipts (31 days max).
- Kinds are allowlists (`shipment`, `cash-in`, `sale`, ...). Unknown values are errors, not pass-throughs.
- Errors are short, lowercase, and say what the caller must fix. Do not echo raw OData responses or credentials.
- Output structs carry `json` tags in snake_case, and list outputs include a `count`.
- Match the surrounding style: small files per domain, sparse comments, no new dependencies unless clearly needed.

## Safety rules for live 1C writes

Writes go to a real accounting database. A bad write can corrupt someone's books, and there is no delete or undo tool.

**When working on code:**

- Every write goes through `Service` and the `odataWriter` interface. Never call `odata.Client.Write` from the CLI or MCP layer, and never add an MCP tool or CLI command that takes a raw OData resource, filter, or body.
- Before writing, read the target and the records it links to, and check they are what the caller claims: the right kind (`IsFolder`), not deleted (`DeletionMark`), and still active. See `CreateGroup` and `CreateProduct`.
- Updates must PATCH only the changed fields and pass the record's `DataVersion` as `If-Match`. If the data version is empty, refuse. If nothing changed, return `Applied: false` without writing.
- Allowed methods are POST (create) and PATCH (update). Do not add DELETE, deletion marks, posting or unposting of documents, or creation of financial documents without an explicit request from the maintainer and a design review.
- MCP annotations must be honest. Reads set `ReadOnlyHint: true`. Creates set `DestructiveHint: &additive` (false). Updates set `IdempotentHint: true`. Keep the test in `server_test.go` that enforces this up to date.
- Respect `docs/access-policy.md`. Payroll, HR, tax, regulated reports, and arbitrary record queries are out of scope. Money operations use allowlists that hide unknown operation types. A new sensitive area needs its boundary enforced in the service layer, with tests showing excluded records stay excluded.
- Test every write with a stub that records the method, resource, body, and `If-Match`, and assert that invalid input causes zero writes (`writeHits`).

**When running the binary against a real tenant:**

- Read-only commands (`check`, `list`, `get`, `search`, `describe`) are safe to run if the person has set up `.env`.
- Do not run `create` or `update` commands, or call write tools through MCP, unless the person asked for that specific change. Show them the exact command first.
- Never write to production 1C to "test" a change. Use stubbed unit tests instead.

## Pull requests

- One feature per PR, with tests, both READMEs, and specs updated together.
- Commit subjects are short and imperative, like the history: "Add price document browsing to CLI and MCP".
- CI must be green. Never skip or delete a test to get there.
