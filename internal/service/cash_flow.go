package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type CashFlowRow struct {
	AccountKind  string  `json:"account_kind"`
	AccountID    string  `json:"account_id"`
	AccountName  string  `json:"account_name"`
	CurrencyID   string  `json:"currency_id"`
	CurrencyCode string  `json:"currency_code"`
	Opening      Decimal `json:"opening"`
	Receipts     Decimal `json:"receipts"`
	Payments     Decimal `json:"payments"`
	Closing      Decimal `json:"closing"`
}

type CashFlowReport struct {
	From          string        `json:"from"`
	To            string        `json:"to"`
	TotalOpening  Decimal       `json:"total_opening"`
	TotalReceipts Decimal       `json:"total_receipts"`
	TotalPayments Decimal       `json:"total_payments"`
	TotalClosing  Decimal       `json:"total_closing"`
	Count         int           `json:"count"`
	Items         []CashFlowRow `json:"items"`
	Source        string        `json:"source"`
}

var cashAccountKinds = map[string]string{"StandardODATA.Catalog_БанковскиеСчета": "bank", "StandardODATA.Catalog_Кассы": "cash"}

// ReportCashFlow reads opening balance, receipts, payments, and closing balance per money account for a period.
// Amounts are in the accounting currency (Сумма); the register also keeps currency amounts, which are not summed here.
func (s Service) ReportCashFlow(ctx context.Context, from, to string) (CashFlowReport, error) {
	start, err := time.Parse("2006-01-02", from)
	if err != nil || start.Format("2006-01-02") != from {
		return CashFlowReport{}, errors.New("from must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", to)
	if err != nil || end.Format("2006-01-02") != to {
		return CashFlowReport{}, errors.New("to must be YYYY-MM-DD")
	}
	if end.Before(start) || end.After(start.AddDate(1, 0, 0)) {
		return CashFlowReport{}, errors.New("date range must be ordered and at most one year")
	}
	register := "AccumulationRegister_ДенежныеСредства"
	rows, err := s.registerRows(ctx, config.RegisterQuery{Register: register, Table: "BalanceAndTurnovers", StartPeriod: from + "T00:00:00", EndPeriod: to + "T23:59:59", Dimensions: "БанковскийСчетКасса,Валюта"})
	if err != nil {
		return CashFlowReport{}, err
	}
	type key struct{ kind, account, currency string }
	type sums struct{ opening, receipts, payments, closing stockSum }
	byKey := map[key]*sums{}
	var total sums
	for _, row := range rows {
		kind, ok := cashAccountKinds[rawString(row["БанковскийСчетКасса_Type"])]
		k := key{kind, strings.ToLower(rawString(row["БанковскийСчетКасса"])), strings.ToLower(rawString(row["Валюта_Key"]))}
		if !ok || !guidPattern.MatchString(k.account) || !guidPattern.MatchString(k.currency) {
			return CashFlowReport{}, errors.New("invalid OData cash flow row")
		}
		item := byKey[k]
		if item == nil {
			item = &sums{}
			byKey[k] = item
		}
		for _, field := range []struct {
			name        string
			item, total *stockSum
		}{
			{"СуммаOpeningBalance", &item.opening, &total.opening},
			{"СуммаReceipt", &item.receipts, &total.receipts},
			{"СуммаExpense", &item.payments, &total.payments},
			{"СуммаClosingBalance", &item.closing, &total.closing},
		} {
			amount := rawDecimal(row[field.name])
			if amount == "" {
				amount = "0"
			}
			if field.item.Add(amount) != nil || field.total.Add(amount) != nil {
				return CashFlowReport{}, errors.New("invalid OData cash flow amount")
			}
		}
	}
	names, codes, err := s.cashLabels(ctx)
	if err != nil {
		return CashFlowReport{}, err
	}
	items := make([]CashFlowRow, 0, len(byKey))
	for k, item := range byKey {
		items = append(items, CashFlowRow{AccountKind: k.kind, AccountID: k.account, AccountName: names[k.account], CurrencyID: k.currency, CurrencyCode: codes[k.currency], Opening: item.opening.Decimal(), Receipts: item.receipts.Decimal(), Payments: item.payments.Decimal(), Closing: item.closing.Decimal()})
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
	return CashFlowReport{From: from, To: to, TotalOpening: total.opening.Decimal(), TotalReceipts: total.receipts.Decimal(), TotalPayments: total.payments.Decimal(), TotalClosing: total.closing.Decimal(), Count: len(items), Items: items, Source: register + "/BalanceAndTurnovers"}, nil
}
