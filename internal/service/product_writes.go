package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ProductPatch struct {
	Name     *string `json:"name,omitempty"`
	FullName *string `json:"full_name,omitempty"`
	Article  *string `json:"article,omitempty"`
	GroupID  *string `json:"group_id,omitempty"`
}

type ProductChange struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Article  string `json:"article"`
	GroupID  string `json:"group_id"`
	Applied  bool   `json:"applied"`
}

type editableProduct struct {
	ID          string `json:"Ref_Key"`
	Code        string `json:"Code"`
	Name        string `json:"Description"`
	FullName    string `json:"НаименованиеПолное"`
	Article     string `json:"Артикул"`
	GroupID     string `json:"Parent_Key"`
	IsFolder    *bool  `json:"IsFolder"`
	Deleted     *bool  `json:"DeletionMark"`
	DataVersion string `json:"DataVersion"`
}

func (s Service) UpdateProduct(ctx context.Context, id string, patch ProductPatch) (ProductChange, error) {
	if !guidPattern.MatchString(id) || strings.EqualFold(id, emptyGUID) {
		return ProductChange{}, errors.New("product ID must be a nonzero GUID")
	}
	if patch.Name == nil && patch.FullName == nil && patch.Article == nil && patch.GroupID == nil {
		return ProductChange{}, errors.New("provide at least one product field to update")
	}
	fields := make(map[string]string)
	var err error
	if patch.Name != nil {
		fields["Description"], err = productValue(*patch.Name, false)
		if err != nil {
			return ProductChange{}, errors.New("invalid product name")
		}
	}
	if patch.FullName != nil {
		fields["НаименованиеПолное"], err = productValue(*patch.FullName, true)
		if err != nil {
			return ProductChange{}, errors.New("invalid product full name")
		}
	}
	if patch.Article != nil {
		fields["Артикул"], err = productValue(*patch.Article, true)
		if err != nil {
			return ProductChange{}, errors.New("invalid product article")
		}
	}
	if patch.GroupID != nil {
		if !linkedGUID(*patch.GroupID) {
			return ProductChange{}, errors.New("group ID must be a nonzero GUID")
		}
		group, err := s.readGroup(ctx, *patch.GroupID)
		if err != nil {
			return ProductChange{}, err
		}
		if !strings.EqualFold(group.ID, *patch.GroupID) || !group.IsFolder || group.Deleted {
			return ProductChange{}, errors.New("group ID must identify an active product group")
		}
		fields["Parent_Key"] = strings.ToLower(*patch.GroupID)
	}
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,НаименованиеПолное,Артикул,Parent_Key,IsFolder,DeletionMark,DataVersion"}}
	data, err := s.OData.Get(ctx, nomenclatureResource(id), params, 1<<20)
	if err != nil {
		return ProductChange{}, err
	}
	var current editableProduct
	if err := json.Unmarshal(data, &current); err != nil || !strings.EqualFold(current.ID, id) || current.IsFolder == nil || *current.IsFolder || current.Deleted == nil || *current.Deleted {
		return ProductChange{}, errors.New("ID does not identify an active product")
	}
	if current.DataVersion == "" {
		return ProductChange{}, errors.New("product has no data version for a safe update")
	}
	change := ProductChange{ID: current.ID, Name: current.Name, FullName: current.FullName, Article: current.Article, GroupID: current.GroupID}
	if value, ok := fields["Description"]; ok {
		change.Name = value
		if value == current.Name {
			delete(fields, "Description")
		}
	}
	if value, ok := fields["НаименованиеПолное"]; ok {
		change.FullName = value
		if value == current.FullName {
			delete(fields, "НаименованиеПолное")
		}
	}
	if value, ok := fields["Артикул"]; ok {
		change.Article = value
		if value == current.Article {
			delete(fields, "Артикул")
		}
	}
	if value, ok := fields["Parent_Key"]; ok {
		change.GroupID = value
		if strings.EqualFold(value, current.GroupID) {
			delete(fields, "Parent_Key")
		}
	}
	if len(fields) == 0 {
		return change, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ProductChange{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(fields)
	if _, err := writer.Write(ctx, http.MethodPatch, nomenclatureResource(id), body, current.DataVersion); err != nil {
		return ProductChange{}, err
	}
	change.Applied = true
	return change, nil
}

func productValue(value string, allowEmpty bool) (string, error) {
	value = strings.TrimSpace(value)
	if !allowEmpty && value == "" || utf8.RuneCountInString(value) > 4096 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", errors.New("invalid text")
	}
	return value, nil
}
