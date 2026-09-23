package service

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxMetadataSize = 16 << 20
const maxResourceMatches = 200

type ResourceSummary struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	PropertyCount int    `json:"property_count"`
}

type ResourceSearch struct {
	Query string            `json:"query"`
	Kind  string            `json:"kind,omitempty"`
	Total int               `json:"total"`
	Items []ResourceSummary `json:"items"`
}

type ResourceProperty struct {
	Name     string `json:"name" xml:"Name,attr"`
	Type     string `json:"type" xml:"Type,attr"`
	Nullable bool   `json:"nullable" xml:"-"`
}

type ResourceDetail struct {
	Name       string             `json:"name"`
	Kind       string             `json:"kind"`
	EntityType string             `json:"entity_type"`
	Keys       []string           `json:"keys"`
	Properties []ResourceProperty `json:"properties"`
	Navigation []string           `json:"navigation"`
}

type metadataEntityType struct {
	Name string `xml:"Name,attr"`
	Keys []struct {
		Name string `xml:"Name,attr"`
	} `xml:"Key>PropertyRef"`
	Properties []struct {
		Name     string `xml:"Name,attr"`
		Type     string `xml:"Type,attr"`
		Nullable string `xml:"Nullable,attr"`
	} `xml:"Property"`
	Navigation []struct {
		Name string `xml:"Name,attr"`
	} `xml:"NavigationProperty"`
}

type metadataEntitySet struct {
	Name       string `xml:"Name,attr"`
	EntityType string `xml:"EntityType,attr"`
}

func (s Service) SearchResources(ctx context.Context, query, kind string, limit int) (ResourceSearch, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 120 || strings.IndexFunc(query, unicode.IsControl) >= 0 {
		return ResourceSearch{}, errors.New("query must be 1–120 characters without control characters")
	}
	if kind != "" && kind != "catalog" && kind != "document" && kind != "register" && kind != "other" {
		return ResourceSearch{}, errors.New("kind must be catalog, document, register, or other")
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxResourceMatches {
		return ResourceSearch{}, errors.New("limit must be 1–200")
	}
	resources, err := s.resources(ctx)
	if err != nil {
		return ResourceSearch{}, err
	}
	result := ResourceSearch{Query: query, Kind: kind, Items: make([]ResourceSummary, 0)}
	needle := strings.ToLower(query)
	for _, resource := range resources {
		if (kind == "" || resource.Kind == kind) && strings.Contains(strings.ToLower(resource.Name), needle) {
			result.Total++
			if len(result.Items) < limit {
				result.Items = append(result.Items, ResourceSummary{Name: resource.Name, Kind: resource.Kind, PropertyCount: len(resource.Properties)})
			}
		}
	}
	return result, nil
}

func (s Service) DescribeResource(ctx context.Context, name string) (ResourceDetail, error) {
	if name == "" || utf8.RuneCountInString(name) > 200 || strings.IndexFunc(name, unicode.IsControl) >= 0 || strings.ContainsAny(name, "/?#") {
		return ResourceDetail{}, errors.New("resource name must be an exact OData entity-set name")
	}
	resources, err := s.resources(ctx)
	if err != nil {
		return ResourceDetail{}, err
	}
	for _, resource := range resources {
		if resource.Name == name {
			return resource, nil
		}
	}
	return ResourceDetail{}, errors.New("resource not found in OData metadata")
}

func (s Service) resources(ctx context.Context) ([]ResourceDetail, error) {
	data, err := s.OData.Get(ctx, "$metadata", nil, maxMetadataSize)
	if err != nil {
		return nil, err
	}
	return parseResources(data)
}

func parseResources(data []byte) ([]ResourceDetail, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	types := make(map[string]metadataEntityType)
	sets := make([]metadataEntitySet, 0)
	namespace := ""
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("invalid OData metadata")
		}
		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "Schema":
				for _, attribute := range element.Attr {
					if attribute.Name.Local == "Namespace" {
						namespace = attribute.Value
					}
				}
			case "EntityType":
				var entity metadataEntityType
				if err := decoder.DecodeElement(&entity, &element); err != nil {
					return nil, errors.New("invalid OData entity type")
				}
				types[namespace+"."+entity.Name] = entity
			case "EntitySet":
				var set metadataEntitySet
				if err := decoder.DecodeElement(&set, &element); err != nil {
					return nil, errors.New("invalid OData entity set")
				}
				sets = append(sets, set)
			}
		case xml.EndElement:
			if element.Name.Local == "Schema" {
				namespace = ""
			}
		}
	}
	if len(sets) == 0 {
		return nil, errors.New("OData metadata contained no resources")
	}
	resources := make([]ResourceDetail, 0, len(sets))
	for _, set := range sets {
		entity, ok := types[set.EntityType]
		if !ok || set.Name == "" {
			return nil, errors.New("OData metadata has an unresolved resource type")
		}
		detail := ResourceDetail{
			Name: set.Name, Kind: resourceKind(set.Name), EntityType: set.EntityType,
			Keys: make([]string, 0, len(entity.Keys)), Properties: make([]ResourceProperty, 0, len(entity.Properties)),
			Navigation: make([]string, 0, len(entity.Navigation)),
		}
		for _, key := range entity.Keys {
			detail.Keys = append(detail.Keys, key.Name)
		}
		for _, property := range entity.Properties {
			detail.Properties = append(detail.Properties, ResourceProperty{
				Name: property.Name, Type: property.Type, Nullable: property.Nullable != "false",
			})
		}
		for _, navigation := range entity.Navigation {
			detail.Navigation = append(detail.Navigation, navigation.Name)
		}
		resources = append(resources, detail)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return resources, nil
}

func resourceKind(name string) string {
	switch {
	case strings.HasPrefix(name, "Catalog_"):
		return "catalog"
	case strings.HasPrefix(name, "Document_"):
		return "document"
	case strings.Contains(name, "Register_"):
		return "register"
	default:
		return "other"
	}
}
