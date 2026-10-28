package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type PriceDraft struct {
	ProductID        string `json:"product_id"`
	CharacteristicID string `json:"characteristic_id,omitempty"`
	PriceType        string `json:"price_type"`
	Price            string `json:"price"`
	Date             string `json:"date"`
	Comment          string `json:"comment,omitempty"`
	Apply            bool   `json:"apply,omitempty"`
}

// PriceWrite is both the preview and the result of an unposted price-setting document.
type PriceWrite struct {
	Applied          bool     `json:"applied"`
	ID               string   `json:"id,omitempty"`
	DataVersion      string   `json:"data_version,omitempty"`
	Posted           bool     `json:"posted"`
	ProductID        string   `json:"product_id"`
	CharacteristicID string   `json:"characteristic_id,omitempty"`
	PriceTypeID      string   `json:"price_type_id"`
	PriceTypeName    string   `json:"price_type_name"`
	CurrencyID       string   `json:"currency_id"`
	UnitID           string   `json:"unit_id"`
	Price            Decimal  `json:"price"`
	Date             string   `json:"date"`
	Comment          string   `json:"comment"`
	Impact           []string `json:"impact"`
}

// CreatePriceDraft previews one product price and, with Apply, saves it as an unposted price-setting document. It never retries an uncertain POST.
func (s Service) CreatePriceDraft(ctx context.Context, input PriceDraft) (PriceWrite, error) {
	date, err := receiptDate(input.Date, false)
	if err != nil {
		return PriceWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
	}
	price, err := exactDecimal(input.Price, maxPriceScale, false)
	if err != nil {
		return PriceWrite{}, fmt.Errorf("price must be positive with at most %d decimals", maxPriceScale)
	}
	comment := strings.TrimSpace(input.Comment)
	if utf8.RuneCountInString(comment) > 1024 || strings.IndexFunc(comment, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		return PriceWrite{}, errors.New("comment must be at most 1024 characters")
	}
	if strings.TrimSpace(input.PriceType) == "" {
		return PriceWrite{}, errors.New("price type is required")
	}
	product, err := s.activeRecord(ctx, config.Products.Name, input.ProductID, "Ref_Key,DeletionMark,IsFolder,ЕдиницаИзмерения_Key")
	if err != nil {
		return PriceWrite{}, fmt.Errorf("product: %w", err)
	}
	if string(product["IsFolder"]) != "false" {
		return PriceWrite{}, errors.New("product ID must identify a product, not a group")
	}
	unitID := rawString(product["ЕдиницаИзмерения_Key"])
	if !linkedGUID(unitID) {
		return PriceWrite{}, unsupported("product has no base unit")
	}
	characteristic := strings.ToLower(input.CharacteristicID)
	if characteristic == emptyGUID {
		characteristic = ""
	}
	if characteristic != "" {
		row, err := s.activeRecord(ctx, config.ProductCharacteristics.Name, characteristic, "Ref_Key,DeletionMark,Owner,Недействителен")
		if err != nil {
			return PriceWrite{}, fmt.Errorf("characteristic: %w", err)
		}
		if !strings.EqualFold(rawString(row["Owner"]), input.ProductID) || string(row["Недействителен"]) == "true" {
			return PriceWrite{}, errors.New("characteristic must be active and belong to the product")
		}
	}
	priceType, err := s.activePriceType(ctx, strings.TrimSpace(input.PriceType))
	if err != nil {
		return PriceWrite{}, err
	}
	currencyID, err := s.baseCurrency(ctx)
	if err != nil {
		return PriceWrite{}, err
	}
	if err := s.checkWritablePriceType(ctx, priceType.ID, currencyID); err != nil {
		return PriceWrite{}, err
	}
	result := PriceWrite{
		ProductID: strings.ToLower(input.ProductID), CharacteristicID: characteristic, PriceTypeID: strings.ToLower(priceType.ID), PriceTypeName: priceType.Name,
		CurrencyID: currencyID, UnitID: strings.ToLower(unitID), Price: ratDecimal(price, maxPriceScale), Date: date, Comment: comment,
		Impact: []string{
			"saved unposted: the current price does not change until the document is posted in 1C",
			fmt.Sprintf("posting would set the %s price of product %s to %s from %s, replacing any price set for that date", priceType.Name, strings.ToLower(input.ProductID), ratDecimal(price, maxPriceScale), date[:10]),
		},
	}
	if !input.Apply {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return PriceWrite{}, errors.New("OData client does not support writes")
	}
	characteristicKey := characteristic
	if characteristicKey == "" {
		characteristicKey = emptyGUID
	}
	fields := map[string]any{
		"Ref_Key": emptyGUID, "Date": date, "Комментарий": comment,
		"ЗаписыватьНовыеЦеныПоверхУстановленных": true, "СлужебныйНовыйФормат": true,
		"Запасы": []map[string]any{{
			"LineNumber": "1", "Номенклатура_Key": result.ProductID, "Характеристика_Key": characteristicKey, "ВидЦены_Key": result.PriceTypeID,
			"Валюта_Key": currencyID, "ЕдиницаИзмерения": result.UnitID, "ЕдиницаИзмерения_Type": unitClassifierType, "Цена": json.Number(result.Price),
		}},
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return PriceWrite{}, errors.New("could not encode price document")
	}
	response, err := writer.Write(ctx, http.MethodPost, config.PriceDocuments.Name, body, "")
	if err != nil {
		return PriceWrite{}, fmt.Errorf("could not confirm price document creation; check list price-documents --posted false before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return PriceWrite{}, errors.New("1C may have created the price document but did not return its ID; check list price-documents --posted false before retrying")
	}
	saved, err := s.readSavedPriceDocument(ctx, created.ID)
	if err != nil {
		return PriceWrite{}, fmt.Errorf("price document %s was created but could not be read back: %w", created.ID, err)
	}
	if saved.Posted {
		return PriceWrite{}, fmt.Errorf("price document %s was saved as posted", created.ID)
	}
	if len(saved.Lines) != 1 || !strings.EqualFold(saved.Lines[0].ProductID, result.ProductID) || !strings.EqualFold(saved.Lines[0].PriceTypeID, result.PriceTypeID) || !decimalEqual(saved.Lines[0].Price, result.Price) {
		return PriceWrite{}, fmt.Errorf("price document %s was created but its line differs from the request", created.ID)
	}
	result.Applied, result.ID, result.DataVersion = true, strings.ToLower(created.ID), saved.DataVersion
	return result, nil
}

// checkWritablePriceType refuses price types 1C computes itself or keeps in another currency; the fields are optional in the published metadata.
func (s Service) checkWritablePriceType(ctx context.Context, id, currencyID string) error {
	data, err := s.OData.Get(ctx, config.PriceTypes.Name+"(guid'"+strings.ToLower(id)+"')", url.Values{"$format": {"json"}}, 1<<20)
	if err != nil {
		return fmt.Errorf("price type: %w", err)
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return errors.New("invalid OData price type response")
	}
	if string(row["РассчитываетсяДинамически"]) == "true" {
		return unsupported("price type is calculated by 1C from another price type and cannot be set directly")
	}
	if currency := rawString(row["ВалютаЦены_Key"]); linkedGUID(currency) && !strings.EqualFold(currency, currencyID) {
		return unsupported("price type is kept in a currency other than rubles")
	}
	return nil
}

type savedPriceDocument struct {
	DataVersion string `json:"DataVersion"`
	Posted      bool   `json:"Posted"`
	Lines       []struct {
		ProductID   string  `json:"Номенклатура_Key"`
		PriceTypeID string  `json:"ВидЦены_Key"`
		Price       Decimal `json:"Цена"`
	} `json:"Запасы"`
}

func (s Service) readSavedPriceDocument(ctx context.Context, id string) (savedPriceDocument, error) {
	data, err := s.OData.Get(ctx, config.PriceDocuments.Name+"(guid'"+strings.ToLower(id)+"')", url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return savedPriceDocument{}, err
	}
	var saved savedPriceDocument
	if err := json.Unmarshal(data, &saved); err != nil {
		return savedPriceDocument{}, errors.New("invalid OData price document response")
	}
	return saved, nil
}
