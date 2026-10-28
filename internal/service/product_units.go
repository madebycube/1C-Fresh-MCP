package service

import (
	"context"
	"errors"
	"math/big"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const productOwnerType = "StandardODATA.Catalog_Номенклатура"

type ProductUnit struct {
	ID               string  `json:"id"`
	Code             string  `json:"code,omitempty"`
	Name             string  `json:"name"`
	Base             bool    `json:"base"`
	Factor           Decimal `json:"factor,omitempty"`
	FactorKnown      bool    `json:"factor_known"`
	ClassifierUnitID string  `json:"classifier_unit_id,omitempty"`
}

type ProductUnits struct {
	ProductID string        `json:"product_id"`
	Count     int           `json:"count"`
	Items     []ProductUnit `json:"items"`
}

// ListProductUnits returns the product's base unit and its own units; a unit without a positive factor is reported as unknown.
func (s Service) ListProductUnits(ctx context.Context, productID string) (ProductUnits, error) {
	product, err := s.activeRecord(ctx, config.Products.Name, productID, "Ref_Key,DeletionMark,IsFolder,ЕдиницаИзмерения_Key")
	if err != nil {
		return ProductUnits{}, err
	}
	if string(product["IsFolder"]) != "false" {
		return ProductUnits{}, errors.New("ID must identify a product, not a group")
	}
	result := ProductUnits{ProductID: strings.ToLower(productID), Items: []ProductUnit{}}
	if baseID := strings.ToLower(rawString(product["ЕдиницаИзмерения_Key"])); linkedGUID(baseID) {
		base := ProductUnit{ID: baseID, Base: true, Factor: "1", FactorKnown: true, ClassifierUnitID: baseID}
		if row, err := s.activeRecord(ctx, config.UnitTypes.Name, baseID, "Ref_Key,DeletionMark,Code,Description"); err == nil {
			base.Code, base.Name = rawString(row["Code"]), rawString(row["Description"])
		}
		result.Items = append(result.Items, base)
	}
	rows, err := s.scanRows(ctx, config.ProductUnits.Name, sourceFields(config.ProductUnits.Fields), "", maxCatalogRows)
	if err != nil {
		return ProductUnits{}, err
	}
	units := []ProductUnit{}
	for _, row := range rows {
		if !strings.EqualFold(rawString(row["Owner"]), productID) || rawString(row["Owner_Type"]) != productOwnerType || string(row["DeletionMark"]) != "false" {
			continue
		}
		unit := ProductUnit{ID: strings.ToLower(rawString(row["Ref_Key"])), Code: rawString(row["Code"]), Name: rawString(row["Description"]), ClassifierUnitID: strings.ToLower(rawString(row["ЕдиницаИзмеренияПоКлассификатору_Key"]))}
		if factor, ok := new(big.Rat).SetString(string(row["Коэффициент"])); ok && factor.Sign() > 0 {
			unit.Factor, unit.FactorKnown = Decimal(string(row["Коэффициент"])), true
		}
		units = append(units, unit)
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Name == units[j].Name {
			return units[i].ID < units[j].ID
		}
		return units[i].Name < units[j].Name
	})
	result.Items = append(result.Items, units...)
	result.Count = len(result.Items)
	return result, nil
}
