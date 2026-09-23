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
	DateField     string
	DeletedField  string
	Fields        []FieldBinding
	LinesField    string
	LineFields    []FieldBinding
	PaymentsField string
	PaymentFields []FieldBinding
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
