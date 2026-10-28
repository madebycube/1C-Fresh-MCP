package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	transferResourceName = "Document_ПеремещениеЗапасов"
	transferOperation    = "Перемещение"
	maxTransferLines     = 200
)

type TransferLineInput struct {
	ProductID        string `json:"product_id"`
	CharacteristicID string `json:"characteristic_id,omitempty"`
	Quantity         string `json:"quantity"`
	UnitID           string `json:"unit_id,omitempty"`
}

type TransferDraft struct {
	OrganizationID  string              `json:"organization_id"`
	FromWarehouseID string              `json:"from_warehouse_id"`
	ToWarehouseID   string              `json:"to_warehouse_id"`
	Date            string              `json:"date"`
	Comment         string              `json:"comment,omitempty"`
	Lines           []TransferLineInput `json:"lines"`
	Apply           bool                `json:"apply,omitempty"`
}

type TransferPreviewLine struct {
	LineNumber       int     `json:"line_number"`
	ProductID        string  `json:"product_id"`
	CharacteristicID string  `json:"characteristic_id,omitempty"`
	UnitID           string  `json:"unit_id"`
	UnitType         string  `json:"unit_type"`
	UnitFactor       Decimal `json:"unit_factor,omitempty"`
	BaseQuantity     Decimal `json:"base_quantity,omitempty"`
	Quantity         Decimal `json:"quantity"`
}

// TransferWrite is both the preview and the result of a stock transfer create.
type TransferWrite struct {
	Applied         bool                  `json:"applied"`
	ID              string                `json:"id,omitempty"`
	DataVersion     string                `json:"data_version,omitempty"`
	Posted          bool                  `json:"posted"`
	OrganizationID  string                `json:"organization_id"`
	CurrencyID      string                `json:"currency_id"`
	FromWarehouseID string                `json:"from_warehouse_id"`
	ToWarehouseID   string                `json:"to_warehouse_id"`
	Date            string                `json:"date"`
	Comment         string                `json:"comment"`
	Lines           []TransferPreviewLine `json:"lines"`
	Impact          []string              `json:"impact"`
}

// CreateTransferDraft previews a transfer of stock between two warehouses and, with Apply, saves it unposted. It never retries an uncertain POST.
func (s Service) CreateTransferDraft(ctx context.Context, input TransferDraft) (TransferWrite, error) {
	result, err := s.validateTransferHeader(ctx, input)
	if err != nil {
		return TransferWrite{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maxTransferLines {
		return TransferWrite{}, fmt.Errorf("a transfer needs 1–%d lines", maxTransferLines)
	}
	for index, line := range input.Lines {
		preview, err := s.transferLine(ctx, line, index+1)
		if err != nil {
			return TransferWrite{}, fmt.Errorf("line %d: %w", index+1, err)
		}
		result.Lines = append(result.Lines, preview)
	}
	result.Impact = []string{
		"saved unposted: no stock movements until it is posted in 1C",
		fmt.Sprintf("posting would move %d lines from warehouse %s to warehouse %s; 1C checks the available stock when it posts", len(result.Lines), result.FromWarehouseID, result.ToWarehouseID),
	}
	if !input.Apply {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return TransferWrite{}, errors.New("OData client does not support writes")
	}
	operation, err := s.predefinedOperation(ctx, transferOperation)
	if err != nil {
		return TransferWrite{}, err
	}
	fields := map[string]any{
		"Ref_Key": emptyGUID, "Date": result.Date, "Организация_Key": result.OrganizationID, "ВидОперации": transferOperation,
		"ХозяйственнаяОперация_Key": operation, "ВалютаДокумента_Key": result.CurrencyID, "СуммаВключаетНДС": true,
		"СтруктурнаяЕдиница_Key": result.FromWarehouseID, "СтруктурнаяЕдиницаПолучатель_Key": result.ToWarehouseID,
		"ПоложениеСтруктурнойЕдиницы": "ВШапке", "ПоложениеСтруктурнойЕдиницыПолучателя": "ВШапке", "Комментарий": result.Comment,
	}
	rows := make([]map[string]any, 0, len(result.Lines))
	for _, line := range result.Lines {
		characteristic := line.CharacteristicID
		if characteristic == "" {
			characteristic = emptyGUID
		}
		rows = append(rows, map[string]any{
			"LineNumber": fmt.Sprint(line.LineNumber), "Номенклатура_Key": line.ProductID, "Характеристика_Key": characteristic,
			"ЕдиницаИзмерения": line.UnitID, "ЕдиницаИзмерения_Type": line.UnitType, "Количество": json.Number(line.Quantity), "КлючСвязи": line.LineNumber,
		})
	}
	fields["Запасы"] = rows
	body, err := json.Marshal(fields)
	if err != nil {
		return TransferWrite{}, errors.New("could not encode transfer")
	}
	response, err := writer.Write(ctx, http.MethodPost, transferResourceName, body, "")
	if err != nil {
		return TransferWrite{}, fmt.Errorf("could not confirm transfer creation; check list warehouse-docs --kind transfer before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return TransferWrite{}, errors.New("1C may have created the transfer but did not return its ID; check list warehouse-docs --kind transfer before retrying")
	}
	saved, err := s.readTransfer(ctx, created.ID)
	if err != nil {
		return TransferWrite{}, fmt.Errorf("transfer %s was created but could not be read back: %w", created.ID, err)
	}
	if err := verifyTransferSaved(saved, result); err != nil {
		return TransferWrite{}, fmt.Errorf("transfer %s was created but differs from the request: %w", created.ID, err)
	}
	result.Applied, result.ID, result.DataVersion, result.Posted = true, strings.ToLower(created.ID), saved.DataVersion, saved.Posted
	return result, nil
}

func (s Service) validateTransferHeader(ctx context.Context, input TransferDraft) (TransferWrite, error) {
	for label, id := range map[string]string{"organization": input.OrganizationID, "source warehouse": input.FromWarehouseID, "destination warehouse": input.ToWarehouseID} {
		if !linkedGUID(id) {
			return TransferWrite{}, fmt.Errorf("%s ID must be a nonzero GUID", label)
		}
	}
	if strings.EqualFold(input.FromWarehouseID, input.ToWarehouseID) {
		return TransferWrite{}, errors.New("source and destination warehouses must differ")
	}
	date, err := receiptDate(input.Date, false)
	if err != nil {
		return TransferWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
	}
	comment := strings.TrimSpace(input.Comment)
	if utf8.RuneCountInString(comment) > 1024 || strings.IndexFunc(comment, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) >= 0 {
		return TransferWrite{}, errors.New("comment must be at most 1024 characters")
	}
	if _, err := s.activeRecord(ctx, "Catalog_Организации", input.OrganizationID, "Ref_Key,DeletionMark"); err != nil {
		return TransferWrite{}, fmt.Errorf("organization: %w", err)
	}
	for label, id := range map[string]string{"source warehouse": input.FromWarehouseID, "destination warehouse": input.ToWarehouseID} {
		unit, err := s.activeRecord(ctx, config.Warehouses.Name, id, "Ref_Key,DeletionMark,ТипСтруктурнойЕдиницы,Недействителен")
		if err != nil {
			return TransferWrite{}, fmt.Errorf("%s: %w", label, err)
		}
		if rawString(unit["ТипСтруктурнойЕдиницы"]) != "Склад" || string(unit["Недействителен"]) == "true" {
			return TransferWrite{}, fmt.Errorf("%s ID must identify an active warehouse", label)
		}
	}
	currencyID, err := s.baseCurrency(ctx)
	if err != nil {
		return TransferWrite{}, err
	}
	return TransferWrite{
		OrganizationID: strings.ToLower(input.OrganizationID), CurrencyID: currencyID, FromWarehouseID: strings.ToLower(input.FromWarehouseID),
		ToWarehouseID: strings.ToLower(input.ToWarehouseID), Date: date, Comment: comment, Lines: []TransferPreviewLine{},
	}, nil
}

func (s Service) baseCurrency(ctx context.Context) (string, error) {
	rows, err := s.scanRows(ctx, config.Currencies.Name, "Ref_Key,DeletionMark,Code", "", 500)
	if err != nil {
		return "", err
	}
	found := ""
	for _, row := range rows {
		if rawString(row["Code"]) != receiptBaseCurrency || string(row["DeletionMark"]) == "true" {
			continue
		}
		if found != "" {
			return "", errors.New("more than one active ruble currency; cannot choose the document currency")
		}
		found = strings.ToLower(rawString(row["Ref_Key"]))
	}
	if !linkedGUID(found) {
		return "", unsupported("the ruble currency (code 643) was not found")
	}
	return found, nil
}

func (s Service) transferLine(ctx context.Context, input TransferLineInput, number int) (TransferPreviewLine, error) {
	product, err := s.activeRecord(ctx, config.Products.Name, input.ProductID, "Ref_Key,DeletionMark,IsFolder,ТипНоменклатуры,ЕдиницаИзмерения_Key")
	if err != nil {
		return TransferPreviewLine{}, fmt.Errorf("product: %w", err)
	}
	if string(product["IsFolder"]) != "false" || rawString(product["ТипНоменклатуры"]) != receiptStockProducts {
		return TransferPreviewLine{}, errors.New("product must be an active stock item")
	}
	unitID := rawString(product["ЕдиницаИзмерения_Key"])
	if !linkedGUID(unitID) {
		return TransferPreviewLine{}, unsupported("product has no base unit")
	}
	unit := lineUnit{ID: strings.ToLower(unitID), Type: unitClassifierType}
	if input.UnitID != "" && !strings.EqualFold(input.UnitID, unitID) {
		if unit, err = s.productPackagingUnit(ctx, input.ProductID, input.UnitID); err != nil {
			return TransferPreviewLine{}, err
		}
	}
	characteristic := strings.ToLower(input.CharacteristicID)
	if characteristic == emptyGUID {
		characteristic = ""
	}
	if characteristic != "" {
		row, err := s.activeRecord(ctx, config.ProductCharacteristics.Name, characteristic, "Ref_Key,DeletionMark,Owner,Недействителен")
		if err != nil {
			return TransferPreviewLine{}, fmt.Errorf("characteristic: %w", err)
		}
		if !strings.EqualFold(rawString(row["Owner"]), input.ProductID) || string(row["Недействителен"]) == "true" {
			return TransferPreviewLine{}, errors.New("characteristic must be active and belong to the product")
		}
	}
	quantity, err := exactDecimal(input.Quantity, maxQuantityScale, false)
	if err != nil {
		return TransferPreviewLine{}, fmt.Errorf("quantity must be positive with at most %d decimals", maxQuantityScale)
	}
	line := TransferPreviewLine{
		LineNumber: number, ProductID: strings.ToLower(input.ProductID), CharacteristicID: characteristic,
		UnitID: unit.ID, UnitType: unit.Type, UnitFactor: unit.Factor, Quantity: ratDecimal(quantity, maxQuantityScale),
	}
	if unit.Factor != "" {
		line.BaseQuantity = ratDecimal(new(big.Rat).Mul(decimalRat(unit.Factor), quantity), maxQuantityScale+3)
	}
	return line, nil
}

type savedTransfer struct {
	DataVersion     string
	Posted          bool
	Operation       string
	FromWarehouseID string
	ToWarehouseID   string
	Lines           []TransferPreviewLine
}

func (s Service) readTransfer(ctx context.Context, id string) (savedTransfer, error) {
	data, err := s.OData.Get(ctx, transferResourceName+"(guid'"+strings.ToLower(id)+"')", url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return savedTransfer{}, err
	}
	var row struct {
		ID          string `json:"Ref_Key"`
		DataVersion string `json:"DataVersion"`
		Posted      bool   `json:"Posted"`
		Operation   string `json:"ВидОперации"`
		From        string `json:"СтруктурнаяЕдиница_Key"`
		To          string `json:"СтруктурнаяЕдиницаПолучатель_Key"`
		Lines       []struct {
			ProductID string  `json:"Номенклатура_Key"`
			UnitID    string  `json:"ЕдиницаИзмерения"`
			Quantity  Decimal `json:"Количество"`
		} `json:"Запасы"`
	}
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(row.ID, id) {
		return savedTransfer{}, errors.New("invalid OData transfer response")
	}
	saved := savedTransfer{DataVersion: row.DataVersion, Posted: row.Posted, Operation: row.Operation, FromWarehouseID: row.From, ToWarehouseID: row.To}
	for _, line := range row.Lines {
		saved.Lines = append(saved.Lines, TransferPreviewLine{ProductID: line.ProductID, UnitID: line.UnitID, Quantity: line.Quantity})
	}
	return saved, nil
}

func verifyTransferSaved(saved savedTransfer, want TransferWrite) error {
	if saved.Posted {
		return errors.New("1C saved the transfer as posted")
	}
	if saved.Operation != transferOperation || !strings.EqualFold(saved.FromWarehouseID, want.FromWarehouseID) || !strings.EqualFold(saved.ToWarehouseID, want.ToWarehouseID) {
		return errors.New("header fields do not match")
	}
	if len(saved.Lines) != len(want.Lines) {
		return errors.New("line count does not match")
	}
	for index, line := range saved.Lines {
		expected := want.Lines[index]
		if !strings.EqualFold(line.ProductID, expected.ProductID) || !strings.EqualFold(line.UnitID, expected.UnitID) || !decimalEqual(line.Quantity, expected.Quantity) {
			return fmt.Errorf("line %d does not match", index+1)
		}
	}
	return nil
}
