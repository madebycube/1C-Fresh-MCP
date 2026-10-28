package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type Organization struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Deleted  bool   `json:"deleted"`
}

type Contract struct {
	ID                string `json:"id"`
	Code              string `json:"code"`
	Name              string `json:"name"`
	Number            string `json:"number"`
	Date              string `json:"date"`
	Kind              string `json:"kind"`
	CounterpartyID    string `json:"counterparty_id"`
	OrganizationID    string `json:"organization_id"`
	CurrencyID        string `json:"currency_id"`
	ConventionalUnits bool   `json:"conventional_units"`
	Deleted           bool   `json:"deleted"`
	Inactive          bool   `json:"inactive"`
}

type ContractPage struct {
	Total      int        `json:"total"`
	Count      int        `json:"count"`
	Offset     int        `json:"offset"`
	NextOffset *int       `json:"next_offset,omitempty"`
	Items      []Contract `json:"items"`
}

type ContractFilter struct {
	CounterpartyID string
	OrganizationID string
	Kind           string
	All            bool
}

var contractKinds = map[string]string{"supplier": "СПоставщиком", "customer": "СПокупателем", "other": "Прочее"}

func (s Service) ListOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := s.catalogRows(ctx, config.Organizations)
	if err != nil {
		return nil, err
	}
	items := make([]Organization, 0, len(rows))
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) {
			return nil, errors.New("invalid OData organization deletion mark")
		}
		item, err := bindFields[Organization](row, config.Organizations.Fields)
		if err != nil || !linkedGUID(item.ID) || item.Name == "" {
			return nil, errors.New("invalid OData organization")
		}
		if !item.Deleted {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func (s Service) ListDepartments(ctx context.Context) ([]Warehouse, error) {
	rows, err := s.catalogRows(ctx, config.Warehouses)
	if err != nil {
		return nil, err
	}
	items := make([]Warehouse, 0)
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) || !catalogBoolean(row["Недействителен"]) {
			return nil, errors.New("invalid OData department flags")
		}
		item, err := bindFields[Warehouse](row, config.Warehouses.Fields)
		if err != nil || !linkedGUID(item.ID) || item.Name == "" {
			return nil, errors.New("invalid OData department")
		}
		if item.Kind == "Подразделение" && !item.Deleted {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	return items, nil
}

// ListContracts scans the contract catalog and filters locally, because guid filters fail in the tested tenant.
func (s Service) ListContracts(ctx context.Context, filter ContractFilter, limit, offset int) (ContractPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return ContractPage{}, err
	}
	for label, id := range map[string]string{"counterparty": filter.CounterpartyID, "organization": filter.OrganizationID} {
		if id != "" && !linkedGUID(id) {
			return ContractPage{}, errors.New(label + " ID must be a GUID")
		}
	}
	kind := ""
	if filter.Kind != "" {
		var ok bool
		if kind, ok = contractKinds[filter.Kind]; !ok {
			return ContractPage{}, errors.New("contract kind must be supplier, customer, or other")
		}
	}
	rows, err := s.catalogRows(ctx, config.Contracts)
	if err != nil {
		return ContractPage{}, err
	}
	kindNames := make(map[string]string, len(contractKinds))
	for name, value := range contractKinds {
		kindNames[value] = name
	}
	items := make([]Contract, 0)
	for _, row := range rows {
		if !catalogBoolean(row["DeletionMark"]) || !catalogBoolean(row["Недействителен"]) {
			return ContractPage{}, errors.New("invalid OData contract flags")
		}
		item, err := bindFields[Contract](row, config.Contracts.Fields)
		if err != nil || !linkedGUID(item.ID) {
			return ContractPage{}, errors.New("invalid OData contract")
		}
		if name, ok := kindNames[item.Kind]; ok {
			item.Kind = name
		} else {
			item.Kind = "unknown"
		}
		item.CounterpartyID, item.OrganizationID, item.CurrencyID = strings.ToLower(item.CounterpartyID), strings.ToLower(item.OrganizationID), strings.ToLower(item.CurrencyID)
		switch {
		case item.Deleted || item.Inactive && !filter.All:
		case filter.CounterpartyID != "" && !strings.EqualFold(item.CounterpartyID, filter.CounterpartyID):
		case filter.OrganizationID != "" && !strings.EqualFold(item.OrganizationID, filter.OrganizationID):
		case kind != "" && contractKinds[item.Kind] != kind:
		default:
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	page := ContractPage{Total: len(items), Offset: offset}
	page.Items, page.NextOffset = pageSlice(items, limit, offset)
	page.Count = len(page.Items)
	return page, nil
}

func pageSlice[T any](items []T, limit, offset int) ([]T, *int) {
	if offset >= len(items) {
		return []T{}, nil
	}
	end := min(offset+limit, len(items))
	if end < len(items) {
		return items[offset:end], &end
	}
	return items[offset:end], nil
}

// ListOrganizationAccounts narrows bank accounts to one organization's own accounts.
func (s Service) ListOrganizationAccounts(ctx context.Context, kind, organizationID string) ([]MoneyAccount, error) {
	if organizationID == "" {
		return s.ListMoneyAccounts(ctx, kind)
	}
	if kind != "bank" || !linkedGUID(organizationID) {
		return nil, errors.New("organization filter needs kind bank and an organization GUID")
	}
	accounts, err := s.ListMoneyAccounts(ctx, kind)
	if err != nil {
		return nil, err
	}
	owned := make([]MoneyAccount, 0, len(accounts))
	for _, account := range accounts {
		if strings.EqualFold(account.OwnerID, organizationID) {
			owned = append(owned, account)
		}
	}
	return owned, nil
}
