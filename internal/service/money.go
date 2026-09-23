package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type MoneyAccount struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Deleted     bool   `json:"deleted"`
	Inactive    *bool  `json:"inactive,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	OwnerType   string `json:"owner_type,omitempty"`
	BankID      string `json:"bank_id,omitempty"`
	WarehouseID string `json:"warehouse_id,omitempty"`
}

type MoneyDocument struct {
	Kind           string  `json:"kind"`
	ID             string  `json:"id"`
	Number         string  `json:"number"`
	Date           string  `json:"date"`
	Posted         bool    `json:"posted"`
	Deleted        bool    `json:"deleted"`
	Amount         Decimal `json:"amount,omitempty"`
	Operation      string  `json:"operation,omitempty"`
	AccountID      string  `json:"account_id,omitempty"`
	RegisterID     string  `json:"register_id,omitempty"`
	TerminalID     string  `json:"terminal_id,omitempty"`
	CounterpartyID string  `json:"counterparty_id,omitempty"`
	CurrencyID     string  `json:"currency_id,omitempty"`
	BasisID        string  `json:"basis_id,omitempty"`
	BasisType      string  `json:"basis_type,omitempty"`
}

type MoneyPage struct {
	Kind       string          `json:"kind"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	AccountID  string          `json:"account_id,omitempty"`
	RegisterID string          `json:"register_id,omitempty"`
	TerminalID string          `json:"terminal_id,omitempty"`
	Total      int             `json:"total"`
	Offset     int             `json:"offset"`
	NextOffset *int            `json:"next_offset,omitempty"`
	Items      []MoneyDocument `json:"items"`
}

var moneyOperations = map[string]map[string]bool{
	"cash-in":      {"ОтПокупателя": true, "РозничнаяВыручка": true},
	"cash-out":     {"Поставщику": true, "Покупателю": true, "ВзносНаличнымиВБанк": true},
	"bank-in":      {"ОтПокупателя": true, "ПоступлениеОплатыПоКартам": true, "ВзносНаличными": true, "ПереводСДругогоСчета": true},
	"bank-out":     {"Поставщику": true, "КомиссияБанка": true, "ПереводНаДругойСчет": true, "ВозвратОплатыНаПлатежныеКарты": true},
	"card-payment": {"ПоступлениеОплатыОтПокупателя": true, "ВозвратОплатыПокупателю": true},
}

func (s Service) ListMoneyAccounts(ctx context.Context, kind string) ([]MoneyAccount, error) {
	plan, ok := config.MoneyAccounts[kind]
	if !ok {
		return nil, errors.New("account kind must be cash, bank, or register")
	}
	rows, err := s.catalogRows(ctx, plan)
	if err != nil {
		return nil, err
	}
	accounts := make([]MoneyAccount, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData account deletion mark")
		}
		if string(row["DeletionMark"]) == "true" || kind == "bank" && string(row["Owner_Type"]) != `"StandardODATA.Catalog_Организации"` {
			continue
		}
		account, err := bindFields[MoneyAccount](row, plan.Fields)
		if err != nil || !linkedGUID(account.ID) {
			return nil, errors.New("invalid OData account")
		}
		account.Kind = kind
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		first, second := strings.ToLower(accounts[i].Name), strings.ToLower(accounts[j].Name)
		if first == second {
			return accounts[i].ID < accounts[j].ID
		}
		return first < second
	})
	return accounts, nil
}

func (s Service) ListMoneyDocuments(ctx context.Context, kind, from, to, accountID, registerID, terminalID string, limit, offset int) (MoneyPage, error) {
	plan, ok := config.MoneyDocuments[kind]
	if !ok {
		return MoneyPage{}, errors.New("kind must be cash-in, cash-out, bank-in, bank-out, card-payment, or cash-shift")
	}
	if err := validateListPage(limit, offset); err != nil {
		return MoneyPage{}, err
	}
	startDate, endDate, err := documentDateRange(from, to)
	if err != nil {
		return MoneyPage{}, err
	}
	if err := validateMoneyScope(kind, accountID, registerID, terminalID); err != nil {
		return MoneyPage{}, err
	}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return MoneyPage{}, err
	}
	matched := make([]MoneyDocument, 0)
	for _, row := range rows {
		doc, err := moneyDocument(row, plan, kind)
		if err != nil {
			return MoneyPage{}, err
		}
		if !moneyOperationAllowed(doc) || doc.Date < startDate || doc.Date >= endDate || accountID != "" && !strings.EqualFold(doc.AccountID, accountID) || registerID != "" && !strings.EqualFold(doc.RegisterID, registerID) || terminalID != "" && !strings.EqualFold(doc.TerminalID, terminalID) {
			continue
		}
		matched = append(matched, doc)
	}
	page := MoneyPage{Kind: kind, From: from, To: to, AccountID: accountID, RegisterID: registerID, TerminalID: terminalID, Total: len(matched), Offset: offset, Items: make([]MoneyDocument, 0)}
	start := min(offset, len(matched))
	end := start + min(limit, len(matched)-start)
	for index := len(matched) - start - 1; index >= len(matched)-end; index-- {
		page.Items = append(page.Items, matched[index])
	}
	if end < len(matched) {
		page.NextOffset = &end
	}
	return page, nil
}

func (s Service) GetMoneyDocument(ctx context.Context, kind, id string) (MoneyDocument, error) {
	plan, ok := config.MoneyDocuments[kind]
	if !ok {
		return MoneyDocument{}, errors.New("kind must be cash-in, cash-out, bank-in, bank-out, card-payment, or cash-shift")
	}
	if !guidPattern.MatchString(id) {
		return MoneyDocument{}, errors.New("document ID must be a GUID")
	}
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields)}}
	data, err := s.OData.Get(ctx, resource, params, 1<<20)
	if err != nil {
		return MoneyDocument{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return MoneyDocument{}, errors.New("invalid OData money document response")
	}
	doc, err := moneyDocument(row, plan, kind)
	if err != nil || !strings.EqualFold(doc.ID, id) {
		return MoneyDocument{}, errors.New("invalid OData money document")
	}
	if !moneyOperationAllowed(doc) {
		return MoneyDocument{}, errors.New("document operation is outside the money read scope")
	}
	return doc, nil
}

func validateMoneyScope(kind, accountID, registerID, terminalID string) error {
	for _, id := range []string{accountID, registerID, terminalID} {
		if id != "" && !linkedGUID(id) {
			return errors.New("scope IDs must be nonzero GUIDs")
		}
	}
	switch kind {
	case "cash-in", "cash-out":
		if accountID == "" && registerID == "" || terminalID != "" {
			return errors.New("cash documents require a cash account or register ID")
		}
	case "bank-in", "bank-out":
		if accountID == "" {
			return errors.New("bank documents require a bank account ID")
		}
	case "card-payment":
		if registerID == "" && terminalID == "" {
			return errors.New("card payments require a register or terminal ID")
		}
	case "cash-shift":
		if registerID == "" || accountID != "" || terminalID != "" {
			return errors.New("cash shifts require a register ID")
		}
	}
	return nil
}

func moneyDocument(row map[string]json.RawMessage, plan config.DocumentResource, kind string) (MoneyDocument, error) {
	if !catalogBoolean(row["Posted"]) || !catalogBoolean(row["DeletionMark"]) {
		return MoneyDocument{}, errors.New("invalid OData money document flags")
	}
	doc, err := bindFields[MoneyDocument](row, plan.Fields)
	if err != nil || !linkedGUID(doc.ID) || len(doc.Date) < 19 {
		return MoneyDocument{}, errors.New("invalid OData money document fields")
	}
	doc.Kind = kind
	if doc.BasisType == "StandardODATA.Undefined" || !linkedGUID(doc.BasisID) {
		doc.BasisID = ""
		doc.BasisType = ""
	}
	for _, field := range []*string{&doc.AccountID, &doc.RegisterID, &doc.TerminalID, &doc.CounterpartyID, &doc.CurrencyID} {
		if !linkedGUID(*field) {
			*field = ""
		}
	}
	return doc, nil
}

func moneyOperationAllowed(doc MoneyDocument) bool {
	if doc.Kind == "cash-shift" {
		return true
	}
	return moneyOperations[doc.Kind][doc.Operation]
}
