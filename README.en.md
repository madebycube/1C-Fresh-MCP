[Russian version](README.md)

![1C-Fresh CLI and MCP](Images/EnglishREADMEBanner.png)

A Go CLI and local MCP server for a 1C-Fresh application. Both use the same OData client and support product groups, counterparty folders, price types and lookups, products, warehouses and stock, customers and suppliers, sales and purchase documents, warehouse documents, money movements, cash receipts, and OData schema discovery. Writes cover product groups, counterparty folders, stock items and services, customers, and suppliers.

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

Copy this into a task for an agent with access to your computer:

```text
Install https://github.com/madebycube/1C-Fresh-MCP on my computer. Clone the repository, build the CLI with go build -o bin/1c ./cmd/1cfresh, and symlink the built bin/1c into a directory on PATH so I can run 1c. Configure my MCP client to launch the absolute path to bin/1c with the mcp argument. Verify installation with 1c --help. Let me enter credentials using 1c login in the terminal; do not request my password in chat. After login, verify the connection with 1c check. If repository access or MCP client configuration is unavailable, identify the exact blocker.
```

## CLI syntax

Commands follow `1c VERB RESOURCE [OPTIONS]`. `1c --help` lists topics, `1c help prices` lists price commands, and `1c help all` lists every command. Use `--help` on a command for its usage and an example. Commands print readable tables by default; add `--json` for structured output.

```sh
1c check
1c login
1c list product-categories
1c list characteristics --product PRODUCT_GUID
1c list groups --name "Пример группы"
1c create group --name "Новая группа"
1c create group --name "Подгруппа" --parent PARENT_GUID
1c update group GROUP_GUID --name "Новое название"
1c update group GROUP_GUID --parent PARENT_GUID
1c list counterparty-groups
1c create counterparty-group --name "Example partners"
1c update counterparty-group FOLDER_GUID --parent root
1c update product PRODUCT_GUID --name "New name" --article NEW-ARTICLE
1c update product PRODUCT_GUID --group GROUP_GUID
1c list price-types
1c list currencies
1c list prices --price-type "Example price" --group GROUP_GUID --limit 20
1c list unit-types
1c list product-units PRODUCT_GUID
1c get price PRODUCT_GUID --price-type "Example price" --as-of 2026-09-23
1c list price-documents --posted false --limit 20
1c get price-document DOCUMENT_GUID --product PRODUCT_GUID --limit 20
1c list organizations
1c list departments
1c list contracts --counterparty SUPPLIER_GUID --kind supplier
1c list warehouses
1c get stock PRODUCT_GUID --warehouse WAREHOUSE_GUID
1c list products --limit 20
1c list products --group GROUP_GUID --limit 20
1c list products --limit 20 --offset NEXT_OFFSET
1c get product PRODUCT_GUID --json
1c create product --name "Example item" --type stock --unit UNIT_GUID --category CATEGORY_GUID --group GROUP_GUID
1c search products --limit 10 "название товара"
1c list customers --limit 20
1c list customers --group FOLDER_GUID --limit 20
1c search customers "Example company"
1c get customer CUSTOMER_GUID
1c create customer --name "Example customer" --parent FOLDER_GUID
1c update customer CUSTOMER_GUID --name "New name"
1c list suppliers --limit 20
1c search suppliers "Example supplier"
1c get supplier SUPPLIER_GUID
1c create supplier --name "Example supplier"
1c update supplier SUPPLIER_GUID --name "New name"
1c search resources --kind catalog --limit 20 "Номенклатура"
1c describe resource Catalog_Номенклатура
1c list orders --limit 20
1c list orders --customer CUSTOMER_GUID --from 2026-09-01 --to 2026-09-30 --offset 20
1c get order --json ORDER_GUID
1c report sales --from 2026-09-01 --to 2026-09-30 --by customer
1c list sales --kind shipment --limit 20
1c list sales --kind return --customer CUSTOMER_GUID
1c list sales --kind invoice --from 2026-09-01 --to 2026-09-30
1c get sale --kind shipment DOCUMENT_GUID
1c list purchases --kind order --limit 20
1c list purchases --kind receipt --supplier SUPPLIER_GUID
1c get purchase --kind receipt DOCUMENT_GUID
1c list payment-plans --supplier SUPPLIER_GUID
1c get payment-plan INVOICE_GUID
1c trace receipt-payments RECEIPT_GUID
1c reconcile supplier --organization ORG_GUID --supplier SUPPLIER_GUID --from 2026-01-01 --to 2026-09-30
1c create receipt --organization ORG_GUID --supplier SUPPLIER_GUID --contract CONTRACT_GUID --warehouse WAREHOUSE_GUID --department DEPARTMENT_GUID --date 2026-10-01 --vat inclusive --line PRODUCT_GUID:3:33.35:НДС22 --incoming-number EX-1
1c preview receipts --file receipts.json
1c update receipt RECEIPT_GUID --set-line 1:quantity=4 --expect-version DATA_VERSION --apply
1c post receipt RECEIPT_GUID --apply
1c unpost receipt RECEIPT_GUID --apply
1c list link-properties
1c get receipt-link RECEIPT_GUID
1c set receipt-link RECEIPT_GUID --url https://drive.example.test/d/1 --caption "Накладная №1" --apply
1c create payment-plan --receipt RECEIPT_GUID --account ACCOUNT_GUID --date 2026-10-01 --payment-date 2026-10-10 --amount 400
1c update payment-plan INVOICE_GUID --amount 500 --receipt RECEIPT_GUID --expect-version DATA_VERSION --apply
1c post payment-plan INVOICE_GUID --apply
1c list warehouse-docs --kind transfer --warehouse WAREHOUSE_GUID
1c list warehouse-docs --kind stock-writeoff --limit 20
1c get warehouse-doc --kind transfer DOCUMENT_GUID
1c list cash-balances --as-of 2026-09-30
1c list debts --kind customer --as-of 2026-09-30
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

`list product-categories` shows active product categories, paths, default types, and units.

`list characteristics` shows active product characteristics, their IDs, and owning product IDs. Use `--product` or `--name` to find a GUID for `get price --characteristic` and `get stock --characteristic`. A call is limited to catalogs of 5,000 records.

`list groups` returns product folders and their IDs; `list price-types` returns price type names and IDs. `list currencies` returns active currency codes, symbols, and IDs. `list unit-types` returns measurement-unit classifier entries. Product base-unit IDs from `get product` refer to this classifier in the tested tenant. Product groups and price types are separate entities. These commands do not retrieve prices from price documents.

`list prices` shows products in the selected group and their effective prices for a named price type as of `--as-of`. Use `--group GROUP_GUID` or `--group root`; without a group, it browses the catalog. Its pages use raw catalog offsets like `list products` and include products without a price as `found: false`. Each item includes product code, article, currency, and the source price document when found. It scans price document history once per page instead of once per product.

`get price` finds the latest price for a product and named price type as of `--as-of` (today by default). It uses posted, non-deleted documents. Omit `--characteristic GUID` for a price without a characteristic or pass a variant's GUID. When no price exists, the result has `found: false`. JSON includes the exact decimal price string, currency ID and, when the catalog is available, its code and symbol, plus source document ID, date, and line number. The command scans price document history, so it may take some time.

`list price-documents` shows price-setting document numbers, dates, GUIDs, and posting status, newest first. Paired `--from` and `--to` bound a date range to 31 days; `--posted true|false` filters posted or unposted documents. `--limit` and `--offset` page through results. `get price-document` reads a GUID from the list or the `source_document_id` from a `get price` or `list prices` result. It shows the document number, date, posting status, and price lines with product, price type, characteristic, and currency IDs plus exact decimal prices. `--product PRODUCT_GUID` filters to one product; `--limit` and `--offset` page through matching lines. Both commands are read-only.

`list organizations` shows the company's own organizations; `list departments` shows departments from the structural units catalog. `list contracts` shows counterparty contracts with owner, organization, kind (`supplier` or `customer`), and settlement currency; `--counterparty` and `--organization` narrow the list, and `--kind other` shows other contracts, `--all` includes inactive ones, and `--limit`/`--offset` page the result. Filters run locally after reading the catalog (up to 5,000 records). `list accounts --kind bank --organization GUID` keeps one organization's own bank accounts. Use these IDs with `create receipt` and `create payment-plan`. These commands are read-only.

`list warehouses` shows warehouses and retail stores from the structural units catalog. `get stock` shows current product balances by warehouse; `--warehouse` selects one, and `--characteristic` selects a product characteristic. Quantities come from the `AccumulationRegister_ЗапасыНаСкладах/Balance` virtual table and are summed across characteristics, batches, cells, and organizations when no characteristic is selected. JSON also includes the source balance rows and product unit ID. If 1C cannot resolve the unit's description, its ID remains available. A stock balance does not promise that the product is available to sell.

`create group` creates a group at the catalog root or under the group specified by `--parent`. `update group` renames a group or moves it with `--parent GROUP_GUID`; `--parent root` moves it to the catalog root. `list counterparty-groups` shows shared customer and supplier folders with paths and GUIDs. `create counterparty-group` creates one; `update counterparty-group` renames or moves it. Parent chains are checked to prevent cycles. `list product-categories` shows active product categories, paths, default types, and units. `create product` creates a stock item or service using a unit from `list unit-types`; stock requires `--category` from the category list, and `--group` selects an optional product group. Unit and category IDs are checked before the POST, and the category default type must match the product type. `update product` changes a product's name, full name (`--full-name`), article (`--article`), and/or group (`--group GUID`). The group must exist and not be deletion-marked. An empty string clears the full name or article. `create customer` and `create supplier` create a counterparty with the matching role flag. The full name defaults to the name; `--parent` selects a counterparty folder. `update customer` and `update supplier` change the name and/or full name of an existing counterparty with the matching role. Pass `--full-name ""` to clear the full name. These commands **change live 1C data**. A live write has not been verified. If product creation has an uncertain result, the command does not retry the POST; search by name or article before trying again. Deletion and generic OData editing are not available yet.

`search resources` uses `1c search resources [--kind catalog|document|register|other] [--limit N] [--json] QUERY` to search OData metadata names by resource kind. `describe resource` uses `1c describe resource [--json] EXACT_NAME` to show schema fields for an exact OData resource name. These commands expose OData schema names and fields; they do not enumerate every screen in the 1C interface or provide generic reads of resource data.

`list products` browses the catalog without a query and excludes folders and deletion-marked records. `--group GROUP_GUID` keeps direct children of that group; `--group root` keeps products at the catalog root. Group matching is local, so a page can be empty while `next_offset` is present; continue from that offset to cover the catalog. `get product` reads one active product by GUID, including its product type and base unit reference. `next_offset` is an offset into raw OData rows: pass it as the next `--offset` rather than adding the number of products shown. Each call scans at most 500 rows, so a page can contain fewer than `--limit` products while still returning `next_offset`. Offsets do not provide a snapshot if the catalog changes between requests. Product search checks product name, full name, and article, excludes folders and deletion-marked rows locally, and returns up to 50 matches. It scans at most 500 OData candidates per field. Order details include product lines. Receipt listing accepts an inclusive date range of up to 31 days, returns up to 100 rows per page, and supports `--offset` for the next page. Receipt details include stock lines and cashless payments. JSON represents amounts and quantities as decimal strings to preserve source precision.

`list customers` and `search customers` show counterparties marked as buyers, excluding folders and deletion-marked records. `--group FOLDER_GUID` keeps direct children of a counterparty folder; `--group root` keeps records at the catalog root. Supplier commands support the same filter. Results include `parent_id`, and paging applies after filtering. `list sales` reads invoices (`invoice`), customer shipments (`shipment`), and incoming goods documents whose operation is customer return (`return`). Use `--customer`, `--from`, and `--to` to narrow results, then `--limit` and `--offset` for paging. `get sale` includes date, amount, posting status, product lines, and linked order or source document IDs when 1C supplies a matching relationship type. `Posted` means the document was posted in 1C; it does not mean paid. The invoice resource is empty in the tested tenant, so lookup of a live invoice by GUID has not been verified. Listings scan resource history and may take several seconds.

`list orders` shows customer orders newest first. Use `--customer`, paired `--from` and `--to`, `--limit`, and `--offset` to filter and page results. JSON returns `items`, `total`, and `next_offset`.

`list suppliers` and `search suppliers` show counterparties marked as suppliers. `list purchases --kind order` reads supplier orders, while `--kind receipt` selects incoming goods documents whose operation is supplier receipt. `list warehouse-docs` reads transfer orders (`transfer-order`), stock movements whose operation is transfer (`transfer`), stock receipts (`stock-receipt`), and stock writeoffs (`stock-writeoff`). Filter by supplier or warehouse ID; a transfer matches either its source or destination warehouse. Both lists support dates, `--limit`, and `--offset`. `get purchase` and `get warehouse-doc` include product lines and available linked document IDs. `Posted` indicates posting in 1C, not fulfillment or physical receipt. There are no transfer orders in the tested tenant, so getting one by GUID has not been verified live.

The tested tenant has no payment-request document («Заявка на расходование денежных средств»), so planned supplier payments are read from supplier invoices («Счет на оплату поставщика»). `list payment-plans` filters them by organization, supplier, contract, paying bank account, and dates. `get payment-plan` shows the payment schedule and the payment-calendar register records, including the approval state as stored; an unreadable register is reported as unavailable. `trace receipt-payments` takes a goods receipt and finds supplier invoices based on it or on its supplier order, plus payment orders, bank expenses, and cash expenses with the supplier-payment operation whose payment breakdown references the receipt, invoice, or order, and the receipt's own prepayment offsets. Only posted bank and cash expenses allocated directly to the receipt and the prepayment offsets count as settlement; payment orders, invoices, and approval never do. Equal amounts without a link are listed as candidates, and missing links are reported as "no linked payment found", not unpaid. The overall status comes from the receipt's own movements in the supplier-settlement register: debt (receipt movements) minus closing by payments, advance offsets, or debt offsets (expense movements), in settlement currency; the document-link status is reported separately. Documents and registers are read without `$filter` because the tested tenant answers HTTP 500 to reference filters.

`reconcile supplier` is a read-only supplier reconciliation from the supplier-settlement register. It needs an organization, a supplier, and a period of up to five years. For each contract and settlement type (debt, advance), plus a contract total, it returns the opening balance, increases, decreases, and closing balance in settlement currency and in accounting currency; contracts and currencies are never netted. Positive means the organization owes the supplier; negative means an advance or a supplier credit. It also lists open items at the end of the period (unpaid receipts, unallocated advances), payment and advance allocations in the period with the allocation method, a page of movements (`--limit`, `--offset`), unposted and deletion-marked documents, and payment orders and plans, which never count as payment. `--contract` limits every figure to one contract; `--account` narrows only the allocation list to payments from that bank account or cash desk and never changes balances. The result is computed from OData and has not yet been compared with the UNF supplier-settlement report; it is labeled `derived`.

Supplier goods receipts are created, amended, posted, and unposted only after a preview: without `--apply` (or `apply` in MCP) the command validates the organization, supplier, contract (active, of that organization, in rubles), warehouse, department, stock products, and VAT rates by code (`НДС22`, `НДС5`, `БезНДС`…), computes exact amounts and VAT, and lists possible duplicates by incoming number and date. A duplicate blocks creation unless `--allow-duplicate` is given. New receipts are saved unposted, then read back and verified. Only an unposted receipt can be amended; other fields and lines are kept as read, and `--expect-version` rejects the save if the receipt changed since the preview. Posting and unposting use the 1C `Post`/`Unpost` document actions, and 1C's own error text is returned. If a save could not be confirmed, check the receipt list before retrying.

A line may name its unit with `:unit=GUID` (or `unit_id` in MCP). `list product-units PRODUCT_GUID` shows the product's base unit and its own units with their conversion factor to the base unit; a unit without a positive factor is shown as `unknown` and is refused, and the preview shows the quantity in base units. When the supplier document's line amount or VAT differs from quantity × price because the price was rounded, pass them with `:amount=A:vat=V` (or `amount`, `vat`; `--set-line N:amount=A,vat-amount=V` when amending). The amount must be within half a kopeck per unit of quantity × price, and the VAT within one kopeck of the rate's VAT; otherwise the line is refused, never adjusted silently. The preview shows the computed values next to the document's ones.

`preview receipts --file receipts.json` (MCP `preview_supplier_receipts`) checks up to 20 receipt drafts in the `create receipt` JSON format without writing anything. Each draft gets a status: `ready` (with its full preview), `exists` (a receipt with the same organization, supplier, incoming number and date is already in 1C), `needs_review` (missing or invalid references, amount/VAT/unit mismatches, or the same supplier document twice in the batch), or `unsupported` (for example a foreign-currency contract or a service line). Save the ready ones one by one with `create receipt`.

The primary-document link is an additional property that 1C shows as a hyperlink on the receipt. `list link-properties` discovers such properties (string type, shown as hyperlink) and whether the «Документ_ПриходнаяНакладная» property set includes them; no GUID or name is hard-coded. `get receipt-link` reads the URL and caption separately: the caption is stored in «Значение» and `<a href = "…">caption</a>` in «ТекстоваяСтрока». `set receipt-link` changes the URL and/or caption (an empty caption shows the URL) or removes the link with `--clear`; other properties, lines, payments, and the comment stay as read. A missing or ambiguous property stops the command without creating configuration. Saves are guarded by the data version and read back; if 1C changes the posting state on save, the command reports it instead of reposting. The URL is never opened and its sharing is not changed.

`create payment-plan` creates a supplier invoice («Счет на оплату поставщика») with one payment-calendar line for a posted goods receipt; in the tested tenant this invoice is the payment request. Without `--apply` it shows the debt from the supplier-settlement register, existing plans, linked payments, and amount-only candidates. By default it plans the whole outstanding debt; `--amount` plans part of it, never more than is outstanding. The organization account (`--account`) must be chosen explicitly; the supplier account is the supplier's only active account or `--supplier-account`. 1C takes the cash-flow item from the contract, and the preview shows it. The invoice basis field can only reference a supplier order, so the receipt is named in the comment ("Приходная накладная № … от …") and its incoming number and date are copied; `trace receipt-payments` links the plan by them (`incoming-document`). A settled receipt, incomplete evidence, or an existing plan blocks saving (the last unless `--allow-duplicate`). The invoice is saved unposted; `post payment-plan` puts it in the payment calendar but never approves it or creates a payment order: 1C sets the approval state, which is shown after posting. `update payment-plan` changes only an unposted single-line plan; a new amount is checked against the outstanding debt and needs `--receipt`. The paying and supplier bank accounts must be in the contract's settlement currency and must not be closed by the payment date.

`list cash-balances` shows money balances by bank account and cash desk with currency at the end of `--as-of` (today by default) from `AccumulationRegister_ДенежныеСредства/Balance`. `list debts --kind customer|supplier` sums customer or supplier settlement-register balances by counterparty: debt (`Долг`), advance (`Аванс`, stored as a negative amount), and net. Counterparties whose debt and advance cancel out to a zero net are left out. `--counterparty` keeps one counterparty. `report sales` sums quantity, amount, VAT, and cost from `AccumulationRegister_Продажи/Turnovers` over a period of up to one year, by product (`--by product`), customer (`customer`), or both (`product-customer`), largest amount first; sales without a counterparty appear as `(no counterparty)`. It is computed from OData, not the official 1C report; compare it with the report before relying on it. These commands are read-only.

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
- `list_product_groups`, `list_product_categories`, `list_product_characteristics`, `list_price_types`, and `list_currencies`
- `get_product_price`, `list_product_prices`, `list_price_documents`, and `get_price_document`
- `list_warehouses`, `get_product_stock`, and `list_unit_types`
- `find_nomenclature`
- `list_products` and `get_product`
- `list_counterparty_groups`
- `create_counterparty_group` and `update_counterparty_group` (write operations)
- `create_product` and `update_product` (write operations)
- `search_odata_resources` and `describe_odata_resource`
- `list_customer_orders` and `get_customer_order`
- `list_customers`, `search_customers`, and `get_customer`
- `create_customer` and `update_customer` (write operations)
- `list_suppliers`, `search_suppliers`, and `get_supplier`
- `create_supplier` and `update_supplier` (write operations)
- `list_sales_documents` and `get_sales_document`
- `list_purchase_documents` and `get_purchase_document`
- `list_supplier_payment_plans`, `get_supplier_payment_plan`, and `trace_supplier_receipt_payments`
- `reconcile_supplier_balances`
- `create_supplier_receipt`, `update_supplier_receipt`, and `set_supplier_receipt_posting` (write operations)
- `list_receipt_link_properties`, `get_receipt_document_link`, and `set_receipt_document_link` (write operation)
- `create_supplier_payment_plan`, `update_supplier_payment_plan`, and `set_supplier_payment_plan_posting` (write operations)
- `list_warehouse_documents` and `get_warehouse_document`
- `list_money_accounts`, `list_money_documents`, and `get_money_document`
- `list_cash_receipts` and `get_cash_receipt`
- `audit_unposted_receipts`
- `create_product_group` and `update_product_group` (write operations)

No generic OData query or tool for deleting data is exposed; posting is limited to supplier goods receipts and supplier payment plans.

See the [access policy](docs/access-policy.md) for reporting, personnel, payroll, and tax boundaries.

## Development

Run the Go tests and static checks from the repository root:

```sh
go test ./...
go vet ./...
```

The OData client uses HTTPS, HTTP Basic authentication, a 30-second timeout, a response-size limit, and does not follow redirects. Errors omit credentials and response bodies. MCP protocol output goes to stdout; CLI errors go to stderr.
