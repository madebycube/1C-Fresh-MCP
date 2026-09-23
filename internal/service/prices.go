package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"golang.org/x/sync/errgroup"
)

const pricePageSize = 100
const maxPriceDocuments = 2000

type PriceQuote struct {
	ProductID        string  `json:"product_id"`
	ProductName      string  `json:"product_name"`
	PriceTypeID      string  `json:"price_type_id"`
	PriceTypeName    string  `json:"price_type_name"`
	CharacteristicID string  `json:"characteristic_id"`
	AsOf             string  `json:"as_of"`
	Found            bool    `json:"found"`
	Price            Decimal `json:"price,omitempty"`
	CurrencyID       string  `json:"currency_id,omitempty"`
	SourceDocumentID string  `json:"source_document_id,omitempty"`
	SourceDate       string  `json:"source_date,omitempty"`
	SourceLineNumber int64   `json:"source_line_number,omitempty"`
}

type priceDocument struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Posted  bool   `json:"posted"`
	Deleted bool   `json:"deleted"`
}

type priceLine struct {
	LineNumber       string  `json:"line_number"`
	ProductID        string  `json:"product_id"`
	PriceTypeID      string  `json:"price_type_id"`
	CharacteristicID string  `json:"characteristic_id"`
	Price            Decimal `json:"price"`
	CurrencyID       string  `json:"currency_id"`
}

func (s Service) GetPrice(ctx context.Context, productID, typeName, characteristicID, asOf string) (PriceQuote, error) {
	if !guidPattern.MatchString(productID) || strings.EqualFold(productID, emptyGUID) {
		return PriceQuote{}, errors.New("product ID must be a nonzero GUID")
	}
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return PriceQuote{}, errors.New("price type name is required")
	}
	if characteristicID == "" {
		characteristicID = emptyGUID
	}
	if !guidPattern.MatchString(characteristicID) {
		return PriceQuote{}, errors.New("characteristic ID must be a GUID")
	}
	if asOf == "" {
		asOf = time.Now().Format("2006-01-02")
	}
	if parsed, err := time.Parse("2006-01-02", asOf); err != nil || parsed.Format("2006-01-02") != asOf {
		return PriceQuote{}, errors.New("as-of date must be YYYY-MM-DD")
	}
	product, err := s.priceProduct(ctx, productID)
	if err != nil {
		return PriceQuote{}, err
	}
	priceTypes, err := s.ListPriceTypes(ctx)
	if err != nil {
		return PriceQuote{}, err
	}
	var selected *PriceType
	for index := range priceTypes {
		candidate := &priceTypes[index]
		if strings.EqualFold(candidate.Name, typeName) && !candidate.Deleted && !candidate.Inactive {
			if selected != nil {
				return PriceQuote{}, errors.New("multiple active price types have this name; use an unambiguous name")
			}
			selected = candidate
		}
	}
	if selected == nil {
		return PriceQuote{}, errors.New("active price type not found")
	}
	quote := PriceQuote{
		ProductID: product.ID, ProductName: product.Name,
		PriceTypeID: selected.ID, PriceTypeName: selected.Name,
		CharacteristicID: characteristicID, AsOf: asOf,
	}
	count, err := s.documentCount(ctx, config.PriceDocuments)
	if err != nil {
		return PriceQuote{}, err
	}
	if count > maxPriceDocuments {
		return PriceQuote{}, errors.New("price history exceeds 2000 documents")
	}
	pageCount := (count + pricePageSize - 1) / pricePageSize
	pages := make([][]map[string]json.RawMessage, pageCount)
	group, requestContext := errgroup.WithContext(ctx)
	group.SetLimit(3)
	for index := range pages {
		index := index
		offset := index * pricePageSize
		limit := min(pricePageSize, count-offset)
		group.Go(func() error {
			rows, err := s.priceDocumentPage(requestContext, offset, limit)
			if err != nil {
				return err
			}
			if len(rows) != limit {
				return errors.New("price history changed during lookup")
			}
			pages[index] = rows
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return PriceQuote{}, err
	}
	seen := make(map[string]bool, count)
	for _, rows := range pages {
		for _, row := range rows {
			document, err := bindFields[priceDocument](row, config.PriceDocuments.Fields)
			if err != nil || !guidPattern.MatchString(document.ID) || seen[document.ID] || len(document.Date) < 19 {
				return PriceQuote{}, errors.New("invalid or repeated price document")
			}
			seen[document.ID] = true
			if _, err := time.Parse("2006-01-02T15:04:05", document.Date[:19]); err != nil {
				return PriceQuote{}, errors.New("invalid price document date")
			}
			if !document.Posted || document.Deleted || document.Date[:10] > asOf {
				continue
			}
			var lines []map[string]json.RawMessage
			if err := json.Unmarshal(row[config.PriceDocuments.LinesField], &lines); err != nil || lines == nil {
				return PriceQuote{}, errors.New("invalid price document lines")
			}
			for _, lineRow := range lines {
				line, err := bindFields[priceLine](lineRow, config.PriceDocuments.LineFields)
				if err != nil {
					return PriceQuote{}, errors.New("invalid price line")
				}
				if !strings.EqualFold(line.ProductID, productID) || !strings.EqualFold(line.PriceTypeID, selected.ID) || line.Price == "" {
					continue
				}
				lineCharacteristic := line.CharacteristicID
				if lineCharacteristic == "" {
					lineCharacteristic = emptyGUID
				}
				if !strings.EqualFold(lineCharacteristic, characteristicID) {
					continue
				}
				lineNumber, err := strconv.ParseInt(line.LineNumber, 10, 64)
				if err != nil || lineNumber < 1 {
					return PriceQuote{}, errors.New("invalid price line number")
				}
				if !quote.Found || document.Date > quote.SourceDate || document.Date == quote.SourceDate && (document.ID > quote.SourceDocumentID || document.ID == quote.SourceDocumentID && lineNumber > quote.SourceLineNumber) {
					quote.Found = true
					quote.Price = line.Price
					quote.CurrencyID = line.CurrencyID
					quote.SourceDocumentID = document.ID
					quote.SourceDate = document.Date
					quote.SourceLineNumber = lineNumber
				}
			}
		}
	}
	return quote, nil
}

func (s Service) priceProduct(ctx context.Context, id string) (editableProduct, error) {
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,IsFolder,DeletionMark"}}
	data, err := s.OData.Get(ctx, nomenclatureResource(id), params, 1<<20)
	if err != nil {
		return editableProduct{}, err
	}
	var product editableProduct
	if err := json.Unmarshal(data, &product); err != nil || !strings.EqualFold(product.ID, id) || product.IsFolder == nil || *product.IsFolder || product.Deleted == nil || *product.Deleted {
		return editableProduct{}, errors.New("ID does not identify an active product")
	}
	return product, nil
}

func (s Service) priceDocumentPage(ctx context.Context, offset, limit int) ([]map[string]json.RawMessage, error) {
	plan := config.PriceDocuments
	params := url.Values{
		"$format":  {"json"},
		"$filter":  {documentFilter(plan)},
		"$select":  {sourceFields(plan.Fields) + "," + plan.LinesField},
		"$orderby": {"Ref_Key asc"},
		"$skip":    {strconv.Itoa(offset)},
		"$top":     {strconv.Itoa(limit)},
	}
	data, err := s.OData.Get(ctx, plan.Name, params, 8<<20)
	if err != nil {
		return nil, err
	}
	var response struct {
		Value []map[string]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
		return nil, errors.New("invalid OData price document page")
	}
	return response.Value, nil
}
