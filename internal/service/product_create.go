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

type ProductCreate struct {
	Name     string `json:"name"`
	FullName string `json:"full_name,omitempty"`
	Article  string `json:"article,omitempty"`
	Type     string `json:"type"`
	UnitID   string `json:"unit_id"`
	GroupID  string `json:"group_id,omitempty"`
}

type ProductCreation struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Article  string `json:"article"`
	Type     string `json:"type"`
	UnitID   string `json:"unit_id"`
	GroupID  string `json:"group_id"`
	Applied  bool   `json:"applied"`
}

func (s Service) CreateProduct(ctx context.Context, input ProductCreate) (ProductCreation, error) {
	name, err := productValue(input.Name, false)
	if err != nil || utf8.RuneCountInString(name) > 120 {
		return ProductCreation{}, errors.New("product name must be 1–120 characters without control characters")
	}
	fullName := input.FullName
	if fullName == "" {
		fullName = name
	}
	fullName, err = productValue(fullName, false)
	if err != nil {
		return ProductCreation{}, errors.New("invalid product full name")
	}
	article, err := productValue(input.Article, true)
	if err != nil || utf8.RuneCountInString(article) > 120 {
		return ProductCreation{}, errors.New("invalid product article")
	}
	var oneCType string
	switch input.Type {
	case "stock":
		oneCType = "Запас"
	case "service":
		oneCType = "Услуга"
	default:
		return ProductCreation{}, errors.New("product type must be stock or service")
	}
	if !linkedGUID(input.UnitID) {
		return ProductCreation{}, errors.New("unit ID must be a nonzero classifier GUID from list unit-types")
	}
	units, err := s.ListUnitTypes(ctx)
	if err != nil {
		return ProductCreation{}, err
	}
	unitID := strings.ToLower(input.UnitID)
	found := false
	for _, unit := range units {
		if strings.EqualFold(unit.ID, unitID) {
			found = true
			break
		}
	}
	if !found {
		return ProductCreation{}, errors.New("unit ID must identify an active classifier entry")
	}
	groupID := input.GroupID
	if groupID == "" || groupID == "root" {
		groupID = emptyGUID
	}
	if !guidPattern.MatchString(groupID) {
		return ProductCreation{}, errors.New("group ID must be a GUID or root")
	}
	if linkedGUID(groupID) {
		group, err := s.readGroup(ctx, groupID)
		if err != nil {
			return ProductCreation{}, err
		}
		if !group.IsFolder || group.Deleted || !strings.EqualFold(group.ID, groupID) {
			return ProductCreation{}, errors.New("group ID must identify an active product group")
		}
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ProductCreation{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]any{
		"Ref_Key": emptyGUID, "Description": name, "НаименованиеПолное": fullName,
		"Артикул": article, "ТипНоменклатуры": oneCType,
		"ЕдиницаИзмерения_Key": unitID, "Parent_Key": strings.ToLower(groupID), "IsFolder": false,
	})
	response, err := writer.Write(ctx, http.MethodPost, config.Products.Name, body, "")
	if err != nil {
		return ProductCreation{}, fmt.Errorf("could not confirm product creation; search before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return ProductCreation{}, errors.New("1C may have created the product but did not return its ID; search before retrying")
	}
	return ProductCreation{
		ID: created.ID, Name: name, FullName: fullName, Article: article,
		Type: input.Type, UnitID: unitID, GroupID: strings.ToLower(groupID), Applied: true,
	}, nil
}
