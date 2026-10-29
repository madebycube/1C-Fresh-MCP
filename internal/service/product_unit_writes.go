package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const maxUnitFactorScale = 3

type ProductUnitCreate struct {
	ProductID  string `json:"product_id"`
	UnitTypeID string `json:"unit_type_id"`
	Factor     string `json:"factor"`
	Name       string `json:"name,omitempty"`
}

type ProductUnitPatch struct {
	Name   *string `json:"name,omitempty"`
	Factor *string `json:"factor,omitempty"`
}

type ProductUnitChange struct {
	ID         string   `json:"id"`
	ProductID  string   `json:"product_id"`
	UnitTypeID string   `json:"unit_type_id"`
	Name       string   `json:"name"`
	Factor     Decimal  `json:"factor"`
	Applied    bool     `json:"applied"`
	Warnings   []string `json:"warnings,omitempty"`
}

// CreateProductUnit adds a packaging unit to a product's card with a positive factor to its base unit.
func (s Service) CreateProductUnit(ctx context.Context, input ProductUnitCreate) (ProductUnitChange, error) {
	product, err := s.activeRecord(ctx, config.Products.Name, input.ProductID, "Ref_Key,DeletionMark,IsFolder,ЕдиницаИзмерения_Key")
	if err != nil {
		return ProductUnitChange{}, fmt.Errorf("product: %w", err)
	}
	if string(product["IsFolder"]) != "false" {
		return ProductUnitChange{}, errors.New("product ID must identify a product, not a group")
	}
	factor, err := unitFactor(input.Factor)
	if err != nil {
		return ProductUnitChange{}, err
	}
	if !linkedGUID(input.UnitTypeID) {
		return ProductUnitChange{}, errors.New("unit type ID must be a nonzero classifier GUID from list unit-types")
	}
	unitType, err := s.activeRecord(ctx, config.UnitTypes.Name, input.UnitTypeID, "Ref_Key,DeletionMark,Description")
	if err != nil {
		return ProductUnitChange{}, fmt.Errorf("unit type must be an active classifier entry from list unit-types: %w", err)
	}
	unitTypeID := strings.ToLower(input.UnitTypeID)
	name := input.Name
	if strings.TrimSpace(name) == "" {
		name = rawString(unitType["Description"])
	}
	if name, err = unitName(name); err != nil {
		return ProductUnitChange{}, err
	}
	existing, err := s.ListProductUnits(ctx, input.ProductID)
	if err != nil {
		return ProductUnitChange{}, err
	}
	for _, unit := range existing.Items {
		if strings.EqualFold(unit.Name, name) || strings.EqualFold(unit.ClassifierUnitID, unitTypeID) && decimalEqual(unit.Factor, factor) {
			return ProductUnitChange{}, fmt.Errorf("product already has unit %s (%s) with that name or unit type and factor", unit.Name, unit.ID)
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ProductUnitChange{}, errors.New("OData client does not support writes")
	}
	productID := strings.ToLower(input.ProductID)
	body, _ := json.Marshal(map[string]any{
		"Ref_Key": emptyGUID, "Owner": productID, "Owner_Type": productOwnerType, "Description": name,
		"Коэффициент": json.Number(factor), "ЕдиницаИзмеренияПоКлассификатору_Key": unitTypeID,
	})
	response, err := writer.Write(ctx, http.MethodPost, config.ProductUnits.Name, body, "")
	if err != nil {
		return ProductUnitChange{}, fmt.Errorf("could not confirm unit creation; run list product-units before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return ProductUnitChange{}, errors.New("1C may have created the unit but did not return its ID; run list product-units before retrying")
	}
	return ProductUnitChange{ID: strings.ToLower(created.ID), ProductID: productID, UnitTypeID: unitTypeID, Name: name, Factor: factor, Applied: true}, nil
}

// UpdateProductUnit renames a product's own unit or changes its factor, patching only what differs.
func (s Service) UpdateProductUnit(ctx context.Context, id string, patch ProductUnitPatch) (ProductUnitChange, error) {
	if patch.Name == nil && patch.Factor == nil {
		return ProductUnitChange{}, errors.New("provide a unit name or factor to update")
	}
	fields := map[string]any{}
	var name string
	var factor Decimal
	var err error
	if patch.Name != nil {
		if name, err = unitName(*patch.Name); err != nil {
			return ProductUnitChange{}, err
		}
	}
	if patch.Factor != nil {
		if factor, err = unitFactor(*patch.Factor); err != nil {
			return ProductUnitChange{}, err
		}
	}
	row, err := s.activeRecord(ctx, config.ProductUnits.Name, id, "Ref_Key,DeletionMark,Description,Owner,Owner_Type,Коэффициент,ЕдиницаИзмеренияПоКлассификатору_Key,DataVersion")
	if err != nil {
		return ProductUnitChange{}, fmt.Errorf("unit: %w", err)
	}
	if rawString(row["Owner_Type"]) != productOwnerType {
		return ProductUnitChange{}, errors.New("ID must identify a product's own unit from list product-units")
	}
	version := rawString(row["DataVersion"])
	if version == "" {
		return ProductUnitChange{}, errors.New("unit has no data version for a safe update")
	}
	change := ProductUnitChange{
		ID: strings.ToLower(id), ProductID: strings.ToLower(rawString(row["Owner"])), UnitTypeID: strings.ToLower(rawString(row["ЕдиницаИзмеренияПоКлассификатору_Key"])),
		Name: rawString(row["Description"]), Factor: Decimal(string(row["Коэффициент"])),
	}
	if patch.Name != nil && name != change.Name {
		fields["Description"], change.Name = name, name
	}
	if patch.Factor != nil && !decimalEqual(factor, change.Factor) {
		fields["Коэффициент"], change.Factor = json.Number(factor), factor
		change.Warnings = append(change.Warnings, "documents already using this unit keep their stored base quantities until they are reposted; reposting converts with the new factor")
	}
	if len(fields) == 0 {
		return change, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ProductUnitChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(fields)
	if _, err := writer.Write(ctx, http.MethodPatch, config.ProductUnits.Name+"(guid'"+change.ID+"')", body, version); err != nil {
		return ProductUnitChange{}, err
	}
	change.Applied = true
	return change, nil
}

func unitFactor(value string) (Decimal, error) {
	factor, err := exactDecimal(value, maxUnitFactorScale, false)
	if err != nil {
		return "", fmt.Errorf("factor must be positive with at most %d decimals, for example 12 for a box of 12", maxUnitFactorScale)
	}
	return ratDecimal(factor, maxUnitFactorScale), nil
}

func unitName(value string) (string, error) {
	name, err := productValue(value, false)
	if err != nil || utf8.RuneCountInString(name) > 25 {
		return "", errors.New("unit name must be 1–25 characters without control characters")
	}
	return name, nil
}
