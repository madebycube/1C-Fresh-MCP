package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const registerPageSize = 500
const maxRegisterRows = 10000

type registerTableReader interface {
	GetRegisterTable(context.Context, config.RegisterQuery, url.Values, int64) ([]byte, error)
}

type Debt struct {
	CounterpartyID   string  `json:"counterparty_id"`
	CounterpartyName string  `json:"counterparty_name"`
	Debt             Decimal `json:"debt"`
	Advance          Decimal `json:"advance"`
	Net              Decimal `json:"net"`
}

type DebtPage struct {
	Kind         string  `json:"kind"`
	AsOf         string  `json:"as_of"`
	TotalDebt    Decimal `json:"total_debt"`
	TotalAdvance Decimal `json:"total_advance"`
	TotalNet     Decimal `json:"total_net"`
	Total        int     `json:"total"`
	Count        int     `json:"count"`
	Offset       int     `json:"offset"`
	NextOffset   *int    `json:"next_offset,omitempty"`
	Items        []Debt  `json:"items"`
	Source       string  `json:"source"`
}

type SalesRow struct {
	ProductID    string  `json:"product_id,omitempty"`
	ProductCode  string  `json:"product_code,omitempty"`
	ProductName  string  `json:"product_name,omitempty"`
	CustomerID   string  `json:"customer_id,omitempty"`
	CustomerName string  `json:"customer_name,omitempty"`
	Quantity     Decimal `json:"quantity"`
	Amount       Decimal `json:"amount"`
	VAT          Decimal `json:"vat"`
	Cost         Decimal `json:"cost"`
}

type SalesReport struct {
	By          string     `json:"by"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	TotalAmount Decimal    `json:"total_amount"`
	TotalVAT    Decimal    `json:"total_vat"`
	TotalCost   Decimal    `json:"total_cost"`
	Total       int        `json:"total"`
	Count       int        `json:"count"`
	Offset      int        `json:"offset"`
	NextOffset  *int       `json:"next_offset,omitempty"`
	Items       []SalesRow `json:"items"`
	Source      string     `json:"source"`
}

type CashBalance struct {
	AccountKind    string  `json:"account_kind"`
	AccountID      string  `json:"account_id"`
	AccountName    string  `json:"account_name"`
	CurrencyID     string  `json:"currency_id"`
	CurrencyCode   string  `json:"currency_code"`
	Amount         Decimal `json:"amount"`
	CurrencyAmount Decimal `json:"currency_amount"`
}

type CashBalances struct {
	AsOf   string        `json:"as_of"`
	Total  Decimal       `json:"total"`
	Count  int           `json:"count"`
	Items  []CashBalance `json:"items"`
	Source string        `json:"source"`
}

var debtRegisters = map[string]string{"customer": "AccumulationRegister_РасчетыСПокупателями", "supplier": "AccumulationRegister_РасчетыСПоставщиками"}

var salesDimensions = map[string]string{"product": "Номенклатура", "customer": "Контрагент", "product-customer": "Номенклатура,Контрагент"}

// ListDebts sums settlement-register balances by counterparty, split by the register's settlement type.
func (s Service) ListDebts(ctx context.Context, kind, asOf, counterpartyID string, limit, offset int) (DebtPage, error) {
	register, ok := debtRegisters[kind]
	if !ok {
		return DebtPage{}, errors.New("debt kind must be customer or supplier")
	}
	if err := validateListPage(limit, offset); err != nil {
		return DebtPage{}, err
	}
	if counterpartyID != "" && !linkedGUID(counterpartyID) {
		return DebtPage{}, errors.New("counterparty ID must be a GUID")
	}
	asOf, period, err := registerAsOf(asOf)
	if err != nil {
		return DebtPage{}, err
	}
	rows, err := s.registerRows(ctx, config.RegisterQuery{Register: register, Table: "Balance", Period: period, Dimensions: "Контрагент,ТипРасчетов"})
	if err != nil {
		return DebtPage{}, err
	}
	type sums struct{ debt, advance, net stockSum }
	byID := map[string]*sums{}
	var totals sums
	for _, row := range rows {
		id := strings.ToLower(rawString(row["Контрагент_Key"]))
		amount := rawDecimal(row["СуммаBalance"])
		if !guidPattern.MatchString(id) || amount == "" {
			return DebtPage{}, errors.New("invalid OData settlement balance row")
		}
		if counterpartyID != "" && !strings.EqualFold(id, counterpartyID) {
			continue
		}
		item := byID[id]
		if item == nil {
			item = &sums{}
			byID[id] = item
		}
		var bucket, total *stockSum
		switch rawString(row["ТипРасчетов"]) {
		case "Долг":
			bucket, total = &item.debt, &totals.debt
		case "Аванс":
			bucket, total = &item.advance, &totals.advance
		default:
			return DebtPage{}, errors.New("unknown settlement type in OData balance")
		}
		for _, sum := range []*stockSum{bucket, total, &item.net, &totals.net} {
			if err := sum.Add(amount); err != nil {
				return DebtPage{}, errors.New("invalid OData settlement amount")
			}
		}
	}
	names, err := s.counterpartyNames(ctx)
	if err != nil {
		return DebtPage{}, err
	}
	items := make([]Debt, 0, len(byID))
	for id, item := range byID {
		// Debt and advance that cancel out leave no open balance, so they are not listed.
		net := item.net.Decimal()
		if net == "0" {
			continue
		}
		items = append(items, Debt{CounterpartyID: id, CounterpartyName: names[id], Debt: item.debt.Decimal(), Advance: item.advance.Decimal(), Net: net})
	}
	sort.Slice(items, func(i, j int) bool {
		first, second := decimalRat(items[i].Net), decimalRat(items[j].Net)
		if c := first.Cmp(second); c != 0 {
			return c > 0
		}
		return items[i].CounterpartyID < items[j].CounterpartyID
	})
	page := DebtPage{Kind: kind, AsOf: asOf, TotalDebt: totals.debt.Decimal(), TotalAdvance: totals.advance.Decimal(), TotalNet: totals.net.Decimal(), Total: len(items), Offset: offset, Source: register + "/Balance"}
	page.Items, page.NextOffset = pageSlice(items, limit, offset)
	page.Count = len(page.Items)
	return page, nil
}

// ReportSales sums sales-register turnovers for a period by product, customer, or both.
func (s Service) ReportSales(ctx context.Context, by, from, to string, limit, offset int) (SalesReport, error) {
	dimensions, ok := salesDimensions[by]
	if !ok {
		return SalesReport{}, errors.New("sales grouping must be product, customer, or product-customer")
	}
	if err := validateListPage(limit, offset); err != nil {
		return SalesReport{}, err
	}
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return SalesReport{}, errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Format("2006-01-02") != to {
		return SalesReport{}, errors.New("to must be YYYY-MM-DD")
	}
	if end.Before(start) || end.After(start.AddDate(1, 0, 0)) {
		return SalesReport{}, errors.New("date range must be ordered and at most one year")
	}
	register := "AccumulationRegister_Продажи"
	rows, err := s.registerRows(ctx, config.RegisterQuery{Register: register, Table: "Turnovers", StartPeriod: from + "T00:00:00", EndPeriod: to + "T23:59:59", Dimensions: dimensions})
	if err != nil {
		return SalesReport{}, err
	}
	type sums struct{ quantity, amount, vat, cost stockSum }
	type key struct{ product, customer string }
	byKey := map[key]*sums{}
	var totals sums
	for _, row := range rows {
		k := key{strings.ToLower(rawString(row["Номенклатура_Key"])), strings.ToLower(rawString(row["Контрагент_Key"]))}
		if by != "customer" && !guidPattern.MatchString(k.product) || by != "product" && !guidPattern.MatchString(k.customer) {
			return SalesReport{}, errors.New("invalid OData sales turnover row")
		}
		item := byKey[k]
		if item == nil {
			item = &sums{}
			byKey[k] = item
		}
		for _, pair := range []struct {
			sums  [2]*stockSum
			field string
		}{
			{[2]*stockSum{&item.quantity, &totals.quantity}, "КоличествоTurnover"},
			{[2]*stockSum{&item.amount, &totals.amount}, "СуммаTurnover"},
			{[2]*stockSum{&item.vat, &totals.vat}, "СуммаНДСTurnover"},
			{[2]*stockSum{&item.cost, &totals.cost}, "СебестоимостьTurnover"},
		} {
			value := rawDecimal(row[pair.field])
			if value == "" {
				value = "0"
			}
			for _, sum := range pair.sums {
				if err := sum.Add(value); err != nil {
					return SalesReport{}, errors.New("invalid OData sales amount")
				}
			}
		}
	}
	items := make([]SalesRow, 0, len(byKey))
	for k, item := range byKey {
		items = append(items, SalesRow{ProductID: k.product, CustomerID: k.customer, Quantity: item.quantity.Decimal(), Amount: item.amount.Decimal(), VAT: item.vat.Decimal(), Cost: item.cost.Decimal()})
	}
	sort.Slice(items, func(i, j int) bool {
		if c := decimalRat(items[i].Amount).Cmp(decimalRat(items[j].Amount)); c != 0 {
			return c > 0
		}
		return items[i].ProductID+items[i].CustomerID < items[j].ProductID+items[j].CustomerID
	})
	report := SalesReport{By: by, From: from, To: to, TotalAmount: totals.amount.Decimal(), TotalVAT: totals.vat.Decimal(), TotalCost: totals.cost.Decimal(), Total: len(items), Offset: offset, Source: register + "/Turnovers"}
	report.Items, report.NextOffset = pageSlice(items, limit, offset)
	report.Count = len(report.Items)
	var names map[string]string
	if by != "product" {
		if names, err = s.counterpartyNames(ctx); err != nil {
			return SalesReport{}, err
		}
	}
	for i := range report.Items {
		item := &report.Items[i]
		item.CustomerName = names[item.CustomerID]
		if item.CustomerID == emptyGUID {
			item.CustomerName = "(no counterparty)"
		}
		if item.ProductID != "" && item.ProductID != emptyGUID {
			product, err := s.productLabel(ctx, item.ProductID)
			if err != nil {
				return SalesReport{}, err
			}
			item.ProductCode, item.ProductName = product.Code, product.Name
		}
	}
	return report, nil
}

// ListCashBalances sums the money register by bank account or cash desk and currency.
func (s Service) ListCashBalances(ctx context.Context, asOf string) (CashBalances, error) {
	asOf, period, err := registerAsOf(asOf)
	if err != nil {
		return CashBalances{}, err
	}
	register := "AccumulationRegister_ДенежныеСредства"
	rows, err := s.registerRows(ctx, config.RegisterQuery{Register: register, Table: "Balance", Period: period, Dimensions: "БанковскийСчетКасса,Валюта"})
	if err != nil {
		return CashBalances{}, err
	}
	accountKinds := map[string]string{"StandardODATA.Catalog_БанковскиеСчета": "bank", "StandardODATA.Catalog_Кассы": "cash"}
	type key struct{ kind, account, currency string }
	type sums struct{ amount, currencyAmount stockSum }
	byKey := map[key]*sums{}
	var total stockSum
	for _, row := range rows {
		kind, ok := accountKinds[rawString(row["БанковскийСчетКасса_Type"])]
		k := key{kind, strings.ToLower(rawString(row["БанковскийСчетКасса"])), strings.ToLower(rawString(row["Валюта_Key"]))}
		if !ok || !guidPattern.MatchString(k.account) || !guidPattern.MatchString(k.currency) {
			return CashBalances{}, errors.New("invalid OData money balance row")
		}
		item := byKey[k]
		if item == nil {
			item = &sums{}
			byKey[k] = item
		}
		amount, currencyAmount := rawDecimal(row["СуммаBalance"]), rawDecimal(row["СуммаВалBalance"])
		if amount == "" || currencyAmount == "" || item.amount.Add(amount) != nil || item.currencyAmount.Add(currencyAmount) != nil || total.Add(amount) != nil {
			return CashBalances{}, errors.New("invalid OData money amount")
		}
	}
	names := map[string]string{}
	for _, kind := range []string{"bank", "cash"} {
		accounts, err := s.ListMoneyAccounts(ctx, kind)
		if err != nil {
			return CashBalances{}, err
		}
		for _, account := range accounts {
			names[strings.ToLower(account.ID)] = account.Name
		}
	}
	currencies, err := s.ListCurrencies(ctx)
	if err != nil {
		return CashBalances{}, err
	}
	codes := map[string]string{}
	for _, currency := range currencies {
		codes[strings.ToLower(currency.ID)] = currency.Code
	}
	items := make([]CashBalance, 0, len(byKey))
	for k, item := range byKey {
		items = append(items, CashBalance{AccountKind: k.kind, AccountID: k.account, AccountName: names[k.account], CurrencyID: k.currency, CurrencyCode: codes[k.currency], Amount: item.amount.Decimal(), CurrencyAmount: item.currencyAmount.Decimal()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].AccountKind != items[j].AccountKind {
			return items[i].AccountKind < items[j].AccountKind
		}
		if items[i].AccountName != items[j].AccountName {
			return items[i].AccountName < items[j].AccountName
		}
		return items[i].AccountID+items[i].CurrencyID < items[j].AccountID+items[j].CurrencyID
	})
	return CashBalances{AsOf: asOf, Total: total.Decimal(), Count: len(items), Items: items, Source: register + "/Balance"}, nil
}

// registerAsOf turns an inclusive YYYY-MM-DD date into 1C's exclusive balance period; empty means today.
func registerAsOf(asOf string) (string, string, error) {
	if asOf == "" {
		asOf = time.Now().Format("2006-01-02")
	}
	date, err := time.Parse("2006-01-02", asOf)
	if err != nil || date.Format("2006-01-02") != asOf {
		return "", "", errors.New("as-of date must be YYYY-MM-DD")
	}
	return asOf, date.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00", nil
}

func (s Service) registerRows(ctx context.Context, query config.RegisterQuery) ([]map[string]json.RawMessage, error) {
	reader, ok := s.OData.(registerTableReader)
	if !ok {
		return nil, errors.New("OData client does not support register tables")
	}
	params := url.Values{"$format": {"json"}, "$inlinecount": {"allpages"}, "$top": {strconv.Itoa(registerPageSize)}}
	rows := make([]map[string]json.RawMessage, 0)
	count := -1
	for len(rows) < maxRegisterRows {
		params.Set("$skip", strconv.Itoa(len(rows)))
		data, err := reader.GetRegisterTable(ctx, query, params, 16<<20)
		if err != nil {
			return nil, err
		}
		var response struct {
			Count string                       `json:"odata.count"`
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
			return nil, errors.New("invalid OData register response")
		}
		pageCount, err := strconv.Atoi(response.Count)
		if err != nil || pageCount < 0 || pageCount > maxRegisterRows || count >= 0 && pageCount != count {
			return nil, errors.New("invalid or excessive OData register count")
		}
		count = pageCount
		if len(response.Value) == 0 && len(rows) < count || len(response.Value) > registerPageSize || len(rows)+len(response.Value) > count {
			return nil, errors.New("incomplete OData register page")
		}
		rows = append(rows, response.Value...)
		if len(rows) == count {
			return rows, nil
		}
	}
	return nil, errors.New("OData register result exceeds 10000 rows")
}

func (s Service) counterpartyNames(ctx context.Context) (map[string]string, error) {
	rows, err := s.catalogRows(ctx, config.CounterpartyGroups)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[strings.ToLower(rawString(row["Ref_Key"]))] = rawString(row["Description"])
	}
	return names, nil
}

type productLabel struct {
	Code string `json:"Code"`
	Name string `json:"Description"`
}

func (s Service) productLabel(ctx context.Context, id string) (productLabel, error) {
	data, err := s.OData.Get(ctx, nomenclatureResource(id), url.Values{"$format": {"json"}, "$select": {"Ref_Key,Code,Description"}}, 1<<20)
	if err != nil {
		return productLabel{}, err
	}
	var label productLabel
	if err := json.Unmarshal(data, &label); err != nil {
		return productLabel{}, errors.New("invalid OData product")
	}
	return label, nil
}

func decimalRat(value Decimal) *big.Rat {
	number, ok := new(big.Rat).SetString(string(value))
	if !ok {
		return new(big.Rat)
	}
	return number
}
