package config

type FieldBinding struct {
	Output string
	Source string
}

type SearchResource struct {
	Name         string
	SearchFields []string
	Fields       []FieldBinding
	FolderField  string
	DeletedField string
}

var Products = SearchResource{
	Name:         "Catalog_Номенклатура",
	SearchFields: []string{"Description", "НаименованиеПолное", "Артикул"},
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "code", Source: "Code"},
		{Output: "name", Source: "Description"},
		{Output: "full_name", Source: "НаименованиеПолное"},
		{Output: "article", Source: "Артикул"},
		{Output: "parent_id", Source: "Parent_Key"},
	},
	FolderField:  "IsFolder",
	DeletedField: "DeletionMark",
}

type DocumentResource struct {
	Name          string
	Filter        string
	DateField     string
	DeletedField  string
	Fields        []FieldBinding
	LinesField    string
	LineFields    []FieldBinding
	PaymentsField string
	PaymentFields []FieldBinding
}

type CatalogListResource struct {
	Name   string
	Filter string
	Fields []FieldBinding
}

var ProductGroups = CatalogListResource{
	Name:   "Catalog_Номенклатура",
	Filter: "IsFolder eq true",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "code", Source: "Code"},
		{Output: "name", Source: "Description"},
		{Output: "parent_id", Source: "Parent_Key"},
		{Output: "is_folder", Source: "IsFolder"},
		{Output: "deleted", Source: "DeletionMark"},
	},
}

var PriceTypes = CatalogListResource{
	Name: "Catalog_ВидыЦен",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "name", Source: "Description"},
		{Output: "deleted", Source: "DeletionMark"},
		{Output: "inactive", Source: "Недействителен"},
	},
}

var Warehouses = CatalogListResource{
	Name: "Catalog_СтруктурныеЕдиницы",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "code", Source: "Code"},
		{Output: "name", Source: "Description"},
		{Output: "kind", Source: "ТипСтруктурнойЕдиницы"},
		{Output: "deleted", Source: "DeletionMark"},
		{Output: "inactive", Source: "Недействителен"},
	},
}

var Customers = CatalogListResource{
	Name: "Catalog_Контрагенты",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "code", Source: "Code"},
		{Output: "name", Source: "Description"},
		{Output: "full_name", Source: "НаименованиеПолное"},
		{Output: "is_folder", Source: "IsFolder"},
		{Output: "deleted", Source: "DeletionMark"},
		{Output: "inactive", Source: "Недействителен"},
		{Output: "buyer", Source: "Покупатель"},
		{Output: "supplier", Source: "Поставщик"},
	},
}

type BalanceResource struct {
	Name         string
	ProductField string
	Fields       []FieldBinding
}

var WarehouseStock = BalanceResource{
	Name:         "AccumulationRegister_ЗапасыНаСкладах",
	ProductField: "Номенклатура_Key",
	Fields: []FieldBinding{
		{Output: "product_id", Source: "Номенклатура_Key"},
		{Output: "warehouse_id", Source: "СтруктурнаяЕдиница_Key"},
		{Output: "characteristic_id", Source: "Характеристика_Key"},
		{Output: "organization_id", Source: "Организация_Key"},
		{Output: "batch_id", Source: "Партия_Key"},
		{Output: "cell_id", Source: "Ячейка_Key"},
		{Output: "quantity", Source: "КоличествоBalance"},
	},
}

var PriceDocuments = DocumentResource{
	Name:         "Document_УстановкаЦенНоменклатуры",
	Filter:       "Posted eq true and DeletionMark eq false",
	DateField:    "Date",
	DeletedField: "DeletionMark",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "date", Source: "Date"},
		{Output: "posted", Source: "Posted"},
		{Output: "deleted", Source: "DeletionMark"},
	},
	LinesField: "Запасы",
	LineFields: []FieldBinding{
		{Output: "line_number", Source: "LineNumber"},
		{Output: "product_id", Source: "Номенклатура_Key"},
		{Output: "price_type_id", Source: "ВидЦены_Key"},
		{Output: "characteristic_id", Source: "Характеристика_Key"},
		{Output: "price", Source: "Цена"},
		{Output: "currency_id", Source: "Валюта_Key"},
	},
}

var CustomerOrders = DocumentResource{
	Name:         "Document_ЗаказПокупателя",
	DateField:    "Date",
	DeletedField: "DeletionMark",
	Fields: []FieldBinding{
		{Output: "id", Source: "Ref_Key"},
		{Output: "number", Source: "Number"},
		{Output: "date", Source: "Date"},
		{Output: "posted", Source: "Posted"},
		{Output: "state", Source: "СостояниеЗаказа"},
		{Output: "amount", Source: "СуммаДокумента"},
		{Output: "customer_id", Source: "Контрагент_Key"},
	},
	LinesField: "Запасы",
	LineFields: []FieldBinding{
		{Output: "line_number", Source: "LineNumber"},
		{Output: "item", Source: "Номенклатура"},
		{Output: "quantity", Source: "Количество"},
		{Output: "unit", Source: "ЕдиницаИзмерения"},
		{Output: "price", Source: "Цена"},
		{Output: "amount", Source: "Сумма"},
		{Output: "total", Source: "Всего"},
	},
}

var salesLineFields = []FieldBinding{
	{Output: "line_number", Source: "LineNumber"},
	{Output: "product_id", Source: "Номенклатура_Key"},
	{Output: "quantity", Source: "Количество"},
	{Output: "unit", Source: "ЕдиницаИзмерения"},
	{Output: "price", Source: "Цена"},
	{Output: "amount", Source: "Сумма"},
	{Output: "total", Source: "Всего"},
	{Output: "vat", Source: "СуммаНДС"},
}

var SalesDocuments = map[string]DocumentResource{
	"invoice": {
		Name: "Document_СчетНаОплату", DateField: "Date", DeletedField: "DeletionMark",
		Fields: []FieldBinding{
			{Output: "id", Source: "Ref_Key"},
			{Output: "number", Source: "Number"},
			{Output: "date", Source: "Date"},
			{Output: "posted", Source: "Posted"},
			{Output: "deleted", Source: "DeletionMark"},
			{Output: "amount", Source: "СуммаДокумента"},
			{Output: "customer_id", Source: "Контрагент_Key"},
			{Output: "basis_id", Source: "ДокументОснование"},
			{Output: "basis_type", Source: "ДокументОснование_Type"},
		},
		LinesField: "Запасы", LineFields: salesLineFields,
	},
	"shipment": {
		Name: "Document_РасходнаяНакладная", DateField: "Date", DeletedField: "DeletionMark",
		Fields: []FieldBinding{
			{Output: "id", Source: "Ref_Key"},
			{Output: "number", Source: "Number"},
			{Output: "date", Source: "Date"},
			{Output: "posted", Source: "Posted"},
			{Output: "deleted", Source: "DeletionMark"},
			{Output: "amount", Source: "СуммаДокумента"},
			{Output: "customer_id", Source: "Контрагент_Key"},
			{Output: "operation", Source: "ВидОперации"},
			{Output: "order_id", Source: "Заказ"},
			{Output: "order_type", Source: "Заказ_Type"},
			{Output: "basis_id", Source: "ДокументОснование"},
			{Output: "basis_type", Source: "ДокументОснование_Type"},
		},
		LinesField: "Запасы", LineFields: salesLineFields,
	},
	"return": {
		Name: "Document_ПриходнаяНакладная", DateField: "Date", DeletedField: "DeletionMark",
		Fields: []FieldBinding{
			{Output: "id", Source: "Ref_Key"},
			{Output: "number", Source: "Number"},
			{Output: "date", Source: "Date"},
			{Output: "posted", Source: "Posted"},
			{Output: "deleted", Source: "DeletionMark"},
			{Output: "amount", Source: "СуммаДокумента"},
			{Output: "customer_id", Source: "Контрагент_Key"},
			{Output: "operation", Source: "ВидОперации"},
			{Output: "order_id", Source: "Заказ"},
			{Output: "order_type", Source: "Заказ_Type"},
			{Output: "basis_id", Source: "ДокументОснование"},
			{Output: "basis_type", Source: "ДокументОснование_Type"},
		},
		LinesField: "Запасы", LineFields: salesLineFields,
	},
}

var CashReceipts = map[string]DocumentResource{
	"sale": {
		Name:         "Document_ЧекККМ",
		DateField:    "Date",
		DeletedField: "DeletionMark",
		Fields: []FieldBinding{
			{Output: "id", Source: "Ref_Key"},
			{Output: "number", Source: "Number"},
			{Output: "date", Source: "Date"},
			{Output: "posted", Source: "Posted"},
			{Output: "amount", Source: "СуммаДокумента"},
			{Output: "order_id", Source: "Заказ_Key"},
			{Output: "customer_id", Source: "Контрагент_Key"},
			{Output: "register_id", Source: "КассаККМ_Key"},
			{Output: "receipt_number", Source: "НомерЧекаККМ"},
			{Output: "payment_form", Source: "ФормаОплаты"},
		},
		LinesField: "Запасы",
		LineFields: []FieldBinding{
			{Output: "line_number", Source: "LineNumber"},
			{Output: "item_id", Source: "Номенклатура_Key"},
			{Output: "quantity", Source: "Количество"},
			{Output: "unit", Source: "ЕдиницаИзмерения"},
			{Output: "price", Source: "Цена"},
			{Output: "amount", Source: "Сумма"},
			{Output: "total", Source: "Всего"},
		},
		PaymentsField: "БезналичнаяОплата",
		PaymentFields: []FieldBinding{
			{Output: "line_number", Source: "LineNumber"},
			{Output: "kind", Source: "ВидОплаты"},
			{Output: "card_type", Source: "ВидПлатежнойКарты"},
			{Output: "amount", Source: "Сумма"},
			{Output: "terminal_id", Source: "ЭквайринговыйТерминал_Key"},
		},
	},
	"refund": {
		Name:         "Document_ЧекККМВозврат",
		DateField:    "Date",
		DeletedField: "DeletionMark",
		Fields: []FieldBinding{
			{Output: "id", Source: "Ref_Key"},
			{Output: "number", Source: "Number"},
			{Output: "date", Source: "Date"},
			{Output: "posted", Source: "Posted"},
			{Output: "amount", Source: "СуммаДокумента"},
			{Output: "order_id", Source: "Заказ_Key"},
			{Output: "customer_id", Source: "Контрагент_Key"},
			{Output: "register_id", Source: "КассаККМ_Key"},
			{Output: "receipt_number", Source: "НомерЧекаККМ"},
			{Output: "original_receipt_id", Source: "ЧекККМ_Key"},
		},
		LinesField: "Запасы",
		LineFields: []FieldBinding{
			{Output: "line_number", Source: "LineNumber"},
			{Output: "item_id", Source: "Номенклатура_Key"},
			{Output: "quantity", Source: "Количество"},
			{Output: "unit", Source: "ЕдиницаИзмерения"},
			{Output: "price", Source: "Цена"},
			{Output: "amount", Source: "Сумма"},
			{Output: "total", Source: "Всего"},
		},
		PaymentsField: "БезналичнаяОплата",
		PaymentFields: []FieldBinding{
			{Output: "line_number", Source: "LineNumber"},
			{Output: "kind", Source: "ВидОплаты"},
			{Output: "card_type", Source: "ВидПлатежнойКарты"},
			{Output: "amount", Source: "Сумма"},
			{Output: "terminal_id", Source: "ЭквайринговыйТерминал_Key"},
			{Output: "cancelled", Source: "ОплатаОтменена"},
		},
	},
}
