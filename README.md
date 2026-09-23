[English version](README.en.md)

![1C-Fresh: CLI и MCP](Images/READMEHeader.png)

`1c` — CLI и локальный MCP-сервер на Go для работы с приложением 1С:Фреш через OData. У них общий клиент; операции с данными доступны через оба интерфейса. Сейчас доступны группы номенклатуры, папки контрагентов, виды и поиск цен, товары, склады и остатки, покупатели и поставщики, заказы, документы продаж и закупок, складские документы, движение денег, кассовые чеки и просмотр схемы OData. Запись реализована для групп номенклатуры, папок контрагентов, товаров типов «Запас» и «Услуга», покупателей и поставщиков.

## Требования

- Go 1.25 или новее
- Ссылка на приложение 1С:Фреш и учётная запись с доступом к OData

## Подключение

После сборки запустите `1c login`. Команда спросит `1C Link/Ссылка на 1C`, `User/Юзер` и `Password/Пароль` (ввод пароля скрыт), проверит доступ к OData и сохранит `.env` с правами доступа только для вашего пользователя. Если URL и имя пользователя уже сохранены, они предлагаются как значения по умолчанию. Пустой пароль оставляет прежний пароль.

Можно создать файл вручную: скопируйте `.env.example` в `.env` и заполните поля:

```dotenv
ONEC_ODATA_BASE_URL=https://your-1cfresh-host/a/your-app/your-tenant
ONEC_ODATA_USERNAME=your-username
ONEC_ODATA_PASSWORD=your-password
```

Указывайте ссылку на приложение, без `/odata/` в конце. `.env` исключён из Git. Переменные окружения имеют приоритет над значениями файла. Для другого файла задайте `ONEC_ENV_FILE`. По умолчанию CLI ищет `.env` в текущем каталоге. Если бинарный файл находится в каталоге `bin` этого проекта, CLI также может найти `.env` в корне проекта.

## Установка

Соберите CLI в корне репозитория:

```sh
go build -o bin/1c ./cmd/1cfresh
bin/1c --help
```

Чтобы запускать `1c` из любого каталога и использовать проектный `.env`, создайте ссылку на бинарный файл в каталоге из `PATH`:

```sh
ln -s /path/to/1C-Fresh-MCP/bin/1c "$HOME/.local/bin/1c"
```

Если скопировать бинарный файл в другое место, укажите `ONEC_ENV_FILE` или переменные `ONEC_ODATA_*`.

### Промпт для агента

Скопируйте этот текст в задачу агенту, который имеет доступ к вашему компьютеру:

```text
Установи https://github.com/madebycube/1C-Fresh-MCP на мой компьютер. Клонируй репозиторий, собери CLI командой go build -o bin/1c ./cmd/1cfresh и добавь ссылку на собранный bin/1c в каталог из PATH, чтобы работала команда 1c. Настрой мой MCP-клиент запускать абсолютный путь к bin/1c с аргументом mcp. Проверь установку командой 1c --help. Для учётных данных дай мне выполнить 1c login в терминале; пароль в переписке не запрашивай. После входа проверь соединение командой 1c check. Если доступ к репозиторию или настройки MCP-клиента недоступны, назови конкретное препятствие.
```

## Команды CLI

Основной синтаксис — `1c ДЕЙСТВИЕ РЕСУРС [ПАРАМЕТРЫ]`. `1c --help` показывает разделы, `1c help prices` — команды цен, `1c help all` — все команды. Для синтаксиса и примера используйте `1c ДЕЙСТВИЕ РЕСУРС --help`. По умолчанию выводятся читаемые таблицы; `--json` включает структурированный вывод.

```sh
1c login
1c check
1c list groups --name "Пример группы"
1c list product-categories
1c list characteristics --product PRODUCT_GUID
1c create group --name "Новая группа"
1c create group --name "Подгруппа" --parent PARENT_GUID
1c update group GROUP_GUID --name "Новое название"
1c update group GROUP_GUID --parent PARENT_GUID
1c list counterparty-groups
1c create counterparty-group --name "Пример партнёров"
1c update counterparty-group FOLDER_GUID --parent root
1c update product PRODUCT_GUID --name "Новое название" --article NEW-ARTICLE
1c update product PRODUCT_GUID --group GROUP_GUID
1c list price-types
1c list currencies
1c list prices --price-type "Пример цены" --group GROUP_GUID --limit 20
1c list unit-types
1c get price PRODUCT_GUID --price-type "Пример цены" --as-of 2026-09-23
1c list price-documents --posted false --limit 20
1c get price-document DOCUMENT_GUID --product PRODUCT_GUID --limit 20
1c list warehouses
1c get stock PRODUCT_GUID --warehouse WAREHOUSE_GUID
1c list products --limit 20
1c list products --group GROUP_GUID --limit 20
1c list products --limit 20 --offset NEXT_OFFSET
1c get product PRODUCT_GUID --json
1c create product --name "Пример товара" --type stock --unit UNIT_GUID --category CATEGORY_GUID --group GROUP_GUID
1c search products --limit 10 "название товара"
1c list customers --limit 20
1c list customers --group FOLDER_GUID --limit 20
1c search customers "Пример компании"
1c get customer CUSTOMER_GUID
1c create customer --name "Пример покупателя" --parent FOLDER_GUID
1c update customer CUSTOMER_GUID --name "Новое название"
1c list suppliers --limit 20
1c search suppliers "Пример поставщика"
1c get supplier SUPPLIER_GUID
1c create supplier --name "Пример поставщика"
1c update supplier SUPPLIER_GUID --name "Новое название"
1c list orders --limit 20
1c list orders --customer CUSTOMER_GUID --from 2026-09-01 --to 2026-09-30 --offset 20
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

`list product-categories` показывает действующие категории номенклатуры, их пути, типы и единицы по умолчанию.

`list characteristics` показывает действующие характеристики номенклатуры, их идентификаторы и ID товара-владельца. Фильтры `--product` и `--name` помогают найти GUID для `get price --characteristic` и `get stock --characteristic`. Справочник ограничен 5000 записями на вызов.

`list groups` показывает папки номенклатуры и их идентификаторы; `list price-types` — виды цен и их идентификаторы. `list currencies` показывает действующие валюты, их коды, символы и ID. `list unit-types` показывает элементы классификатора единиц измерения. ID базовой единицы товара из `get product` ссылается на этот классификатор в проверенной базе. Группы товаров и виды цен — разные сущности. Эти команды не получают цены товаров из документов установки цен.

`list prices` показывает товары группы и действующие цены указанного вида на дату `--as-of`. Группу задают через `--group GROUP_GUID` или `--group root`; без группы просматривается весь каталог. Страницы используют смещение по исходным строкам каталога, как `list products`, и включают товары без цены с `found: false`. Каждая запись содержит код, артикул товара, валюту и документ-источник найденной цены. История документов установки цен читается один раз на страницу, а не отдельно для каждого товара.

`get price` ищет последнюю цену товара для указанного вида цен на дату `--as-of` (по умолчанию сегодня). Команда учитывает только проведённые документы без пометки на удаление. Без `--characteristic GUID` ищется цена без характеристики; для варианта передайте его GUID. Если цены нет, результат содержит `found: false`. JSON включает точную десятичную строку цены, ID валюты и, если справочник доступен, её код и символ, а также ID, дату и номер строки документа-источника. Поиск просматривает историю документов и может занять время.

`list price-documents` показывает номера, даты, GUID и состояние проведения документов установки цен, начиная с новых. Парные `--from` и `--to` ограничивают диапазон до 31 дня; `--posted true|false` выбирает проведённые или непроведённые документы. `--limit` и `--offset` делят результат на страницы. `get price-document` читает документ по GUID из списка либо по `source_document_id` из результата `get price` или `list prices`. Показывает номер, дату, состояние проведения и строки с ID товара, вида цен, характеристики, валюты и точной десятичной ценой. `--product PRODUCT_GUID` оставляет строки одного товара; `--limit` и `--offset` делят строки на страницы. Обе команды только читают данные.

`list warehouses` показывает склады и розничные точки из справочника структурных единиц. `get stock` показывает текущие остатки товара по складам; `--warehouse` ограничивает ответ одним складом, `--characteristic` — одной характеристикой. Количество берётся из виртуальной таблицы `AccumulationRegister_ЗапасыНаСкладах/Balance` и суммируется по характеристикам, партиям, ячейкам и организациям, если характеристика не выбрана. JSON также содержит исходные строки баланса и ID единицы измерения товара. Если 1С не находит описание единицы, её ID остаётся в результате. Это остаток на складе, а не обещание доступности товара для продажи.

`create group` создаёт группу в корне каталога либо внутри группы, указанной через `--parent`. `update group` переименовывает группу или перемещает её через `--parent GROUP_GUID`; `--parent root` переносит группу в корень. `list counterparty-groups` показывает общие папки покупателей и поставщиков с путями и GUID. `create counterparty-group` создаёт такую папку; `update counterparty-group` переименовывает или перемещает её. Перед перемещением проверяется цепочка родителей, чтобы не создать цикл. `list product-categories` показывает действующие категории номенклатуры, их пути, типы и единицы по умолчанию. `create product` создаёт запас или услугу с единицей из `list unit-types`; для запаса требуется `--category` из списка категорий, а `--group` задаёт необязательную папку номенклатуры. ID единицы и категории проверяются до отправки запроса. `update product` меняет название, полное название (`--full-name`), артикул (`--article`) и/или группу (`--group GUID`) товара. Группа должна существовать и не быть помеченной на удаление. Пустая строка очищает полное название или артикул. `create customer` и `create supplier` создают контрагента с соответствующим признаком; полное название по умолчанию совпадает с названием, а `--parent` задаёт папку контрагентов. `update customer` и `update supplier` меняют название и/или полное название существующего контрагента с нужным признаком. Пустое `--full-name ""` очищает полное название. Эти команды **изменяют данные в действующей 1С**. Запись в действующую базу не проверялась. При неопределённом результате создания товара команда не повторяет POST: сначала найдите запись по названию или артикулу. Удаления и общего редактора OData пока нет.

`list products` просматривает каталог без поискового запроса, исключая папки и помеченные на удаление записи. `--group GROUP_GUID` оставляет товары непосредственно в этой группе; `--group root` оставляет товары в корне. Фильтрация группы выполняется локально, поэтому страница может быть пустой при наличии `next_offset`: продолжайте с этим смещением для просмотра каталога. `get product` показывает один действующий товар по GUID, включая тип номенклатуры и ID базовой единицы измерения. `next_offset` — смещение по исходным строкам OData: передавайте его в следующем `--offset`, а не прибавляйте число выведенных товаров. За один вызов команда просматривает не более 500 строк; поэтому страница может содержать меньше запрошенного `--limit`, даже если есть `next_offset`. При изменениях каталога между запросами смещение не гарантирует снимок данных. Поиск товаров проверяет название, полное название и артикул, локально исключает папки и удалённые записи и возвращает не более 50 результатов. По каждому полю он просматривает не более 500 совпавших строк OData. Просмотр заказа включает товарные строки. Список чеков принимает включительный диапазон до 31 дня, возвращает до 100 записей на страницу и поддерживает `--offset`. Детали чека включают товарные строки и безналичные платежи. В JSON суммы и количества представлены строками с десятичными числами, чтобы сохранить точность источника.

`list customers` и `search customers` показывают контрагентов с признаком «Покупатель», исключая папки и помеченные на удаление записи. `--group FOLDER_GUID` оставляет непосредственных детей папки контрагентов, `--group root` — записи в корне; то же работает для поставщиков. Результаты содержат `parent_id`, а страницы формируются после фильтрации. `list sales` читает счета на оплату (`invoice`), расходные накладные продажи (`shipment`) и приходные накладные с операцией возврата от покупателя (`return`). `--customer`, `--from` и `--to` ограничивают выборку; `--limit` и `--offset` делят её на страницы. `get sale` показывает дату, сумму, факт проведения, товарные строки и идентификаторы связанного заказа или документа-основания, когда 1С возвращает их с подходящим типом связи. `Posted` означает проведение документа, а не оплату. В проверенной базе ресурс счетов на оплату пуст; операции чтения счёта по GUID на живой записи пока не проверялись. Списки читают историю ресурсов и могут занять несколько секунд.

`list orders` показывает заказы покупателей, начиная с новых. `--customer`, парные `--from` и `--to`, `--limit` и `--offset` ограничивают выборку и делят её на страницы. JSON возвращает `items`, `total` и `next_offset`.

`list suppliers` и `search suppliers` показывают контрагентов с признаком «Поставщик». `list purchases --kind order` читает заказы поставщикам, а `--kind receipt` — только приходные накладные с операцией «ПоступлениеОтПоставщика». `list warehouse-docs` читает заказы на перемещение (`transfer-order`), перемещения запасов с операцией «Перемещение» (`transfer`), оприходования (`stock-receipt`) и списания (`stock-writeoff`). `--supplier` и `--warehouse` выбирают записи по ID; для перемещения склад может быть исходным или конечным. Оба списка поддерживают даты, `--limit` и `--offset`. `get purchase` и `get warehouse-doc` показывают товарные строки и доступные ID связанных документов. `Posted` показывает проведение в 1С, а не получение товара или исполнение заказа. В проверенной базе заказов на перемещение нет, поэтому получение такого документа по GUID на живой записи не проверялось.

`list accounts` показывает кассы (`cash`), банковские счета организации (`bank`) и кассы ККМ (`register`). `list money` читает поступления и расходы кассы (`cash-in`, `cash-out`), банка (`bank-in`, `bank-out`), операции по платёжным картам (`card-payment`) и кассовые смены (`cash-shift`). Укажите даты включительно, не более 31 дня, и ID кассы, счёта, кассы ККМ или терминала через `--account`, `--register`, `--terminal`. `--limit` и `--offset` делят результат на страницы; `get money` показывает документ по GUID. Операции зарплаты, налогов, выплат работникам и неизвестные виды операций исключены. Кассовый чек, операция по карте и зачисление на банковский счёт — отдельные документы; `Posted` означает проведение, а не подтверждение оплаты или сверки. Команды только читают данные и могут просматривать историю ресурса несколько секунд.

### Проверка непроведённых чеков

`audit receipts` проверяет чеки продажи и возврата за интервал дат приложения `[--from, --before)`, где дата `--before` не входит в интервал. Отчёт показывает чеки без пометки на удаление с `Posted=false`: тип, ID, номер, дату, сумму и доступные связанные ID. Максимум — 31 календарный день и 1000 чеков обоих типов; при превышении команда завершается ошибкой, не выдавая частичный отчёт.

Непроведённый чек может быть черновиком или частью отменённой операции. Команда сообщает статус для проверки, но не определяет, есть ли ошибка в учёте. Приложение возвращает даты без часового пояса, поэтому сравниваются календарные даты.

### Схема OData

```sh
1c search resources --kind catalog --limit 20 "Номенклатура"
1c describe resource Catalog_Номенклатура
```

`search resources` ищет имена ресурсов OData; фильтр `--kind` принимает `catalog`, `document`, `register` или `other`. `describe resource` показывает поля, типы, ключи и связи ресурса с точным именем. Эти команды показывают схему OData, а не все экраны интерфейса 1С. Они не читают произвольные записи ресурсов.

## MCP-сервер

Запустите сервер через stdio:

```sh
bin/1c mcp
```

Настройте MCP-клиент на запуск команды из каталога репозитория либо передайте ему три переменные `ONEC_ODATA_*`. Сервер предоставляет инструменты:

| Чтение | Изменение |
| --- | --- |
| `check_connection` | `create_product_group` |
| `list_product_groups`, `list_product_categories`, `list_product_characteristics`, `list_price_types`, `list_currencies`, `get_product_price`, `list_price_documents`, `get_price_document`, `list_product_prices` | `update_product_group` |
| `list_counterparty_groups` | `create_counterparty_group`, `update_counterparty_group` |
| `list_warehouses`, `get_product_stock`, `list_unit_types` | |
| `find_nomenclature` | `create_product`, `update_product` |
| `list_products`, `get_product` | |
| `list_customers`, `search_customers`, `get_customer` | `create_customer`, `update_customer` |
| `list_suppliers`, `search_suppliers`, `get_supplier` | `create_supplier`, `update_supplier` |
| `list_customer_orders`, `get_customer_order` | |
| `list_sales_documents`, `get_sales_document` | |
| `list_purchase_documents`, `get_purchase_document` | |
| `list_warehouse_documents`, `get_warehouse_document` | |
| `list_money_accounts`, `list_money_documents`, `get_money_document` | |
| `list_cash_receipts`, `get_cash_receipt` | |
| `audit_unposted_receipts` | |
| `search_odata_resources`, `describe_odata_resource` | |

Инструменты создания и изменения групп и контрагентов записывают данные в 1С. Инструментов для удаления, проведения документов и произвольных запросов OData нет.

Границы доступа к отчётам, кадровым и налоговым данным описаны в [политике доступа](docs/access-policy.md).

## Разработка

Из корня репозитория:

```sh
go test ./...
go vet ./...
```

Клиент OData использует HTTPS, HTTP Basic, тайм-аут 30 секунд, ограничение размера ответа и не следует перенаправлениям. Сообщения об ошибках не включают учётные данные и тела ответов. MCP-протокол пишет в stdout, ошибки CLI — в stderr.
