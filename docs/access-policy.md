# Reports and sensitive data

This policy defines the default CLI and MCP surface. The menu in 1C is a navigation model, not an OData API catalog. A menu item is supported only when a command has an explicit data source, a bounded read contract, and a documented meaning.

## Menu to OData mapping

| 1C menu area | OData evidence in the tested application | Default surface |
| --- | --- | --- |
| Products, prices, customers, suppliers, orders, sales, purchases, and warehouse documents | Catalogs, documents, and the stock balance register used by existing commands | Available through named read commands |
| Cash, bank, acquiring, and retail shifts | Cash/bank/card/shift documents and cash desk, bank account, and register catalogs | Available through `list accounts`, `list money`, and `get money`, with operation and scope restrictions |
| Fiscal cash receipts | `Document_ЧекККМ` and `Document_ЧекККМВозврат` | Available through separate receipt commands; do not merge with card transactions or bank settlement |
| Sales funnel, business analysis, company state, and the menu item “Reports” | UI reports and calculations; no report definition or equivalent output was established from OData metadata | No default report command |
| Payroll and personnel | `Catalog_Сотрудники`, `Document_НачислениеЗарплатыУНФ`, `Document_ПлатежнаяВедомость`, and `Document_Табель` exist | No default record read |
| Tax returns and regulated reporting | `Catalog_РегламентированныеОтчеты` and tax registers exist; menu labels such as declarations are workflows, not verified OData report outputs | No default record read or filing action |

Metadata presence proves that an entity set exists, not that its rows reproduce a 1C screen or report. A name search returning no matching entity set also does not prove the workflow is unavailable in the 1C UI.

## Report queries

A report command needs a named business question and a verified source, including the meaning of totals, currencies, posting status, date boundaries, and relevant organization. Return aggregate figures when they answer the question; omit source rows and personal identifiers unless the task requires them. Require a date range and scope before reading high-volume data. State when the CLI computes a result from OData rather than reproducing a 1C report. Do not label a derived result as the official 1C report without comparing it against that report.

## HR, payroll, and tax boundary

Employee, salary, payment-statement, tax, and regulated-report records are outside the default CLI and MCP tools. Adding one requires a separate domain review, a specific read use case, the minimum fields needed, tenant permission checks, bounded filters, and tests that excluded records stay excluded. Permission to access an OData entity set alone is insufficient to make it a general-purpose tool.

The money commands expose only named cash, bank, card, and shift document kinds. Their operation allowlists exclude payroll, tax, employee-payment, and unknown operations from both list and direct get. New operation values stay hidden until reviewed. Bank account discovery returns organization-owned accounts. `Posted` denotes 1C posting; it does not prove payment or reconciliation.

`search resources` and `describe resource` expose schema metadata only. The default CLI and MCP server do not provide arbitrary OData record queries, report execution, filing, payroll editing, or tax editing. Any future sensitive-domain tool must enforce its boundary in the shared service layer so CLI and MCP behave the same way.
