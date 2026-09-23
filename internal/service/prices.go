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
	ProductCode      string  `json:"product_code,omitempty"`
	ProductName      string  `json:"product_name"`
	ProductArticle   string  `json:"product_article,omitempty"`
	PriceTypeID      string  `json:"price_type_id"`
	PriceTypeName    string  `json:"price_type_name"`
	CharacteristicID string  `json:"characteristic_id"`
	AsOf             string  `json:"as_of"`
	Found            bool    `json:"found"`
	Price            Decimal `json:"price,omitempty"`
	CurrencyID       string  `json:"currency_id,omitempty"`
	CurrencyCode     string  `json:"currency_code,omitempty"`
	CurrencySymbol   string  `json:"currency_symbol,omitempty"`
	SourceDocumentID string  `json:"source_document_id,omitempty"`
	SourceDate       string  `json:"source_date,omitempty"`
	SourceLineNumber int64   `json:"source_line_number,omitempty"`
}

type PricePage struct {
	PriceTypeID      string       `json:"price_type_id"`
	PriceTypeName    string       `json:"price_type_name"`
	CharacteristicID string       `json:"characteristic_id"`
	AsOf             string       `json:"as_of"`
	GroupID          string       `json:"group_id,omitempty"`
	Offset           int          `json:"offset"`
	Scanned          int          `json:"scanned"`
	NextOffset       *int         `json:"next_offset,omitempty"`
	Items            []PriceQuote `json:"items"`
}

type PriceDocumentLine struct {
	LineNumber       int64   `json:"line_number"`
	ProductID        string  `json:"product_id"`
	PriceTypeID      string  `json:"price_type_id"`
	CharacteristicID string  `json:"characteristic_id"`
	CurrencyID       string  `json:"currency_id"`
	Price            Decimal `json:"price"`
}

type PriceDocument struct {
	ID         string              `json:"id"`
	Date       string              `json:"date"`
	Posted     bool                `json:"posted"`
	Deleted    bool                `json:"deleted"`
	ProductID  string              `json:"product_id,omitempty"`
	Total      int                 `json:"total"`
	Offset     int                 `json:"offset"`
	NextOffset *int                `json:"next_offset,omitempty"`
	Lines      []PriceDocumentLine `json:"lines"`
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

func (s Service) GetPriceDocument(ctx context.Context, id, productID string, limit, offset int) (PriceDocument, error) {
	if !linkedGUID(id) {
		return PriceDocument{}, errors.New("price document ID must be a nonzero GUID")
	}
	if productID != "" && !linkedGUID(productID) {
		return PriceDocument{}, errors.New("product ID must be a nonzero GUID")
	}
	if limit < 1 || limit > 100 || offset < 0 {
		return PriceDocument{}, errors.New("limit must be 1-100 and offset must be nonnegative")
	}
	plan := config.PriceDocuments
	resource := plan.Name + "(guid'" + strings.ToLower(id) + "')"
	params := url.Values{"$format": {"json"}, "$select": {sourceFields(plan.Fields) + "," + plan.LinesField}}
	data, err := s.OData.Get(ctx, resource, params, 8<<20)
	if err != nil {
		return PriceDocument{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || row == nil {
		return PriceDocument{}, errors.New("invalid OData price document response")
	}
	header, err := bindFields[priceDocument](row, plan.Fields)
	if err != nil || !strings.EqualFold(header.ID, id) || len(header.Date) < 19 {
		return PriceDocument{}, errors.New("invalid OData price document")
	}
	if _, err := time.Parse("2006-01-02T15:04:05", header.Date[:19]); err != nil {
		return PriceDocument{}, errors.New("invalid price document date")
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(row[plan.LinesField], &rows); err != nil || rows == nil {
		return PriceDocument{}, errors.New("invalid price document lines")
	}
	result := PriceDocument{ID: header.ID, Date: header.Date, Posted: header.Posted, Deleted: header.Deleted, ProductID: productID, Offset: offset, Lines: make([]PriceDocumentLine, 0)}
	for _, lineRow := range rows {
		line, err := bindFields[priceLine](lineRow, plan.LineFields)
		if err != nil {
			return PriceDocument{}, errors.New("invalid price line")
		}
		if productID != "" && !strings.EqualFold(line.ProductID, productID) {
			continue
		}
		result.Total++
		if result.Total <= offset || len(result.Lines) >= limit {
			continue
		}
		lineNumber, err := strconv.ParseInt(line.LineNumber, 10, 64)
		if err != nil || lineNumber < 1 || !linkedGUID(line.ProductID) || !linkedGUID(line.PriceTypeID) || line.Price == "" {
			return PriceDocument{}, errors.New("invalid price line")
		}
		if line.CharacteristicID == "" {
			line.CharacteristicID = emptyGUID
		}
		result.Lines = append(result.Lines, PriceDocumentLine{LineNumber: lineNumber, ProductID: line.ProductID, PriceTypeID: line.PriceTypeID, CharacteristicID: line.CharacteristicID, CurrencyID: line.CurrencyID, Price: line.Price})
	}
	if offset+len(result.Lines) < result.Total {
		next := offset + len(result.Lines)
		result.NextOffset = &next
	}
	return result, nil
}

func (s Service) GetPrice(ctx context.Context, productID, typeName, characteristicID, asOf string) (PriceQuote, error) {
	if !guidPattern.MatchString(productID) || strings.EqualFold(productID, emptyGUID) {
		return PriceQuote{}, errors.New("product ID must be a nonzero GUID")
	}
	typeName, characteristicID, asOf, err := priceFilters(typeName, characteristicID, asOf)
	if err != nil {
		return PriceQuote{}, err
	}
	product, err := s.priceProduct(ctx, productID)
	if err != nil {
		return PriceQuote{}, err
	}
	selected, err := s.activePriceType(ctx, typeName)
	if err != nil {
		return PriceQuote{}, err
	}
	quote := PriceQuote{
		ProductID: product.ID, ProductCode: product.Code, ProductName: product.Name, ProductArticle: product.Article,
		PriceTypeID: selected.ID, PriceTypeName: selected.Name,
		CharacteristicID: characteristicID, AsOf: asOf,
	}
	quotes := map[string]*PriceQuote{strings.ToLower(productID): &quote}
	if err := s.applyPriceHistory(ctx, selected.ID, characteristicID, asOf, quotes); err != nil {
		return PriceQuote{}, err
	}
	s.enrichPriceCurrencies(ctx, []*PriceQuote{&quote})
	return quote, nil
}

func (s Service) ListPrices(ctx context.Context, typeName, groupID, characteristicID, asOf string, limit, offset int) (PricePage, error) {
	typeName, characteristicID, asOf, err := priceFilters(typeName, characteristicID, asOf)
	if err != nil {
		return PricePage{}, err
	}
	selected, err := s.activePriceType(ctx, typeName)
	if err != nil {
		return PricePage{}, err
	}
	products, err := s.ListProductsInGroup(ctx, limit, offset, groupID)
	if err != nil {
		return PricePage{}, err
	}
	page := PricePage{
		PriceTypeID: selected.ID, PriceTypeName: selected.Name,
		CharacteristicID: characteristicID, AsOf: asOf, GroupID: products.GroupID,
		Offset: products.Offset, Scanned: products.Scanned, NextOffset: products.NextOffset,
		Items: make([]PriceQuote, 0, len(products.Items)),
	}
	quotes := make(map[string]*PriceQuote, len(products.Items))
	for _, product := range products.Items {
		page.Items = append(page.Items, PriceQuote{
			ProductID: product.ID, ProductCode: product.Code, ProductName: product.Name, ProductArticle: product.Article,
			PriceTypeID: selected.ID, PriceTypeName: selected.Name,
			CharacteristicID: characteristicID, AsOf: asOf,
		})
		quotes[strings.ToLower(product.ID)] = &page.Items[len(page.Items)-1]
	}
	if len(quotes) != 0 {
		if err := s.applyPriceHistory(ctx, selected.ID, characteristicID, asOf, quotes); err != nil {
			return PricePage{}, err
		}
		items := make([]*PriceQuote, 0, len(page.Items))
		for index := range page.Items {
			items = append(items, &page.Items[index])
		}
		s.enrichPriceCurrencies(ctx, items)
	}
	return page, nil
}

func (s Service) enrichPriceCurrencies(ctx context.Context, quotes []*PriceQuote) {
	needLookup := false
	for _, quote := range quotes {
		if quote.Found && linkedGUID(quote.CurrencyID) {
			needLookup = true
			break
		}
	}
	if !needLookup {
		return
	}
	currencies, err := s.ListCurrencies(ctx)
	if err != nil {
		return
	}
	byID := make(map[string]Currency, len(currencies))
	for _, currency := range currencies {
		byID[strings.ToLower(currency.ID)] = currency
	}
	for _, quote := range quotes {
		if currency, ok := byID[strings.ToLower(quote.CurrencyID)]; ok {
			quote.CurrencyCode = currency.Code
			quote.CurrencySymbol = currency.Symbol
		}
	}
}

func priceFilters(typeName, characteristicID, asOf string) (string, string, string, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return "", "", "", errors.New("price type name is required")
	}
	if characteristicID == "" {
		characteristicID = emptyGUID
	}
	if !guidPattern.MatchString(characteristicID) {
		return "", "", "", errors.New("characteristic ID must be a GUID")
	}
	if asOf == "" {
		asOf = time.Now().Format("2006-01-02")
	}
	if parsed, err := time.Parse("2006-01-02", asOf); err != nil || parsed.Format("2006-01-02") != asOf {
		return "", "", "", errors.New("as-of date must be YYYY-MM-DD")
	}
	return typeName, characteristicID, asOf, nil
}

func (s Service) activePriceType(ctx context.Context, name string) (PriceType, error) {
	priceTypes, err := s.ListPriceTypes(ctx)
	if err != nil {
		return PriceType{}, err
	}
	var selected *PriceType
	for index := range priceTypes {
		candidate := &priceTypes[index]
		if strings.EqualFold(candidate.Name, name) && !candidate.Deleted && !candidate.Inactive {
			if selected != nil {
				return PriceType{}, errors.New("multiple active price types have this name; use an unambiguous name")
			}
			selected = candidate
		}
	}
	if selected == nil {
		return PriceType{}, errors.New("active price type not found")
	}
	return *selected, nil
}

func (s Service) applyPriceHistory(ctx context.Context, priceTypeID, characteristicID, asOf string, quotes map[string]*PriceQuote) error {
	count, err := s.documentCount(ctx, config.PriceDocuments)
	if err != nil {
		return err
	}
	if count > maxPriceDocuments {
		return errors.New("price history exceeds 2000 documents")
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
		return err
	}
	seen := make(map[string]bool, count)
	for _, rows := range pages {
		for _, row := range rows {
			document, err := bindFields[priceDocument](row, config.PriceDocuments.Fields)
			if err != nil || !guidPattern.MatchString(document.ID) || seen[document.ID] || len(document.Date) < 19 {
				return errors.New("invalid or repeated price document")
			}
			seen[document.ID] = true
			if _, err := time.Parse("2006-01-02T15:04:05", document.Date[:19]); err != nil {
				return errors.New("invalid price document date")
			}
			if !document.Posted || document.Deleted || document.Date[:10] > asOf {
				continue
			}
			var lines []map[string]json.RawMessage
			if err := json.Unmarshal(row[config.PriceDocuments.LinesField], &lines); err != nil || lines == nil {
				return errors.New("invalid price document lines")
			}
			for _, lineRow := range lines {
				line, err := bindFields[priceLine](lineRow, config.PriceDocuments.LineFields)
				if err != nil {
					return errors.New("invalid price line")
				}
				quote := quotes[strings.ToLower(line.ProductID)]
				if quote == nil || !strings.EqualFold(line.PriceTypeID, priceTypeID) || line.Price == "" {
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
					return errors.New("invalid price line number")
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
	return nil
}

func (s Service) priceProduct(ctx context.Context, id string) (editableProduct, error) {
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Code,Description,Артикул,IsFolder,DeletionMark"}}
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
