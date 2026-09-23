package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const stockPageSize = 100
const maxStockRows = 5000

type StockBalance struct {
	ProductID        string  `json:"product_id"`
	WarehouseID      string  `json:"warehouse_id"`
	CharacteristicID string  `json:"characteristic_id"`
	OrganizationID   string  `json:"organization_id"`
	BatchID          string  `json:"batch_id"`
	CellID           string  `json:"cell_id"`
	Quantity         Decimal `json:"quantity"`
}

type WarehouseStock struct {
	ID       string  `json:"id"`
	Name     string  `json:"name,omitempty"`
	Kind     string  `json:"kind,omitempty"`
	Quantity Decimal `json:"quantity"`
}

type StockResult struct {
	ProductID        string           `json:"product_id"`
	ProductName      string           `json:"product_name"`
	CharacteristicID string           `json:"characteristic_id,omitempty"`
	UnitID           string           `json:"unit_id,omitempty"`
	UnitName         string           `json:"unit_name,omitempty"`
	SourceRegister   string           `json:"source_register"`
	Total            Decimal          `json:"total"`
	Warehouses       []WarehouseStock `json:"warehouses"`
	Balances         []StockBalance   `json:"balances"`
}

type stockProduct struct {
	ID       string `json:"Ref_Key"`
	Name     string `json:"Description"`
	UnitID   string `json:"ЕдиницаИзмерения_Key"`
	IsFolder *bool  `json:"IsFolder"`
	Deleted  *bool  `json:"DeletionMark"`
}

type stockUnit struct {
	ID   string `json:"Ref_Key"`
	Name string `json:"Description"`
}

type stockBalanceReader interface {
	GetStockBalance(context.Context, string, url.Values, int64) ([]byte, error)
}

func (s Service) GetStock(ctx context.Context, productID, warehouseID, characteristicID string) (StockResult, error) {
	if !guidPattern.MatchString(productID) || strings.EqualFold(productID, emptyGUID) {
		return StockResult{}, errors.New("product ID must be a nonzero GUID")
	}
	if warehouseID != "" && (!guidPattern.MatchString(warehouseID) || strings.EqualFold(warehouseID, emptyGUID)) {
		return StockResult{}, errors.New("warehouse ID must be a nonzero GUID")
	}
	if characteristicID != "" && !guidPattern.MatchString(characteristicID) {
		return StockResult{}, errors.New("characteristic ID must be a GUID")
	}
	product, err := s.readStockProduct(ctx, productID)
	if err != nil {
		return StockResult{}, fmt.Errorf("read product: %w", err)
	}
	warehouses, err := s.ListWarehouses(ctx)
	if err != nil {
		return StockResult{}, fmt.Errorf("list warehouses: %w", err)
	}
	byID := make(map[string]Warehouse, len(warehouses))
	for _, warehouse := range warehouses {
		byID[strings.ToLower(warehouse.ID)] = warehouse
	}
	if warehouseID != "" {
		if _, ok := byID[strings.ToLower(warehouseID)]; !ok {
			return StockResult{}, errors.New("warehouse ID not found")
		}
	}
	result := StockResult{
		ProductID: product.ID, ProductName: product.Name, CharacteristicID: characteristicID,
		UnitID: product.UnitID, SourceRegister: config.WarehouseStock.Name + "/Balance",
		Total: "0", Warehouses: []WarehouseStock{}, Balances: []StockBalance{},
	}
	if product.UnitID != "" && !strings.EqualFold(product.UnitID, emptyGUID) {
		unit, err := s.readStockUnit(ctx, product.UnitID)
		if err == nil {
			result.UnitName = unit.Name
		}
	}
	rows, err := s.stockRows(ctx, productID)
	if err != nil {
		return StockResult{}, fmt.Errorf("read stock balances: %w", err)
	}
	var total stockSum
	byWarehouse := make(map[string]*stockSum)
	for _, warehouse := range warehouses {
		if warehouseID != "" && !strings.EqualFold(warehouse.ID, warehouseID) {
			continue
		}
		if warehouseID == "" && (warehouse.Deleted || warehouse.Inactive) {
			continue
		}
		byWarehouse[strings.ToLower(warehouse.ID)] = &stockSum{}
	}
	for _, row := range rows {
		balance, err := bindFields[StockBalance](row, config.WarehouseStock.Fields)
		if err != nil || !strings.EqualFold(balance.ProductID, productID) || !guidPattern.MatchString(balance.WarehouseID) || balance.Quantity == "" {
			return StockResult{}, errors.New("invalid stock balance row")
		}
		if warehouseID != "" && !strings.EqualFold(balance.WarehouseID, warehouseID) {
			continue
		}
		if characteristicID != "" && !strings.EqualFold(balance.CharacteristicID, characteristicID) {
			continue
		}
		if err := total.Add(balance.Quantity); err != nil {
			return StockResult{}, err
		}
		key := strings.ToLower(balance.WarehouseID)
		if byWarehouse[key] == nil {
			byWarehouse[key] = &stockSum{}
		}
		if err := byWarehouse[key].Add(balance.Quantity); err != nil {
			return StockResult{}, err
		}
		result.Balances = append(result.Balances, balance)
	}
	result.Total = total.Decimal()
	for id, sum := range byWarehouse {
		warehouse := byID[id]
		result.Warehouses = append(result.Warehouses, WarehouseStock{ID: id, Name: warehouse.Name, Kind: warehouse.Kind, Quantity: sum.Decimal()})
	}
	sort.Slice(result.Warehouses, func(i, j int) bool {
		if result.Warehouses[i].Name == result.Warehouses[j].Name {
			return result.Warehouses[i].ID < result.Warehouses[j].ID
		}
		return result.Warehouses[i].Name < result.Warehouses[j].Name
	})
	return result, nil
}

func (s Service) readStockProduct(ctx context.Context, id string) (stockProduct, error) {
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description,ЕдиницаИзмерения_Key,IsFolder,DeletionMark"}}
	data, err := s.OData.Get(ctx, nomenclatureResource(id), params, 1<<20)
	if err != nil {
		return stockProduct{}, err
	}
	var product stockProduct
	if err := json.Unmarshal(data, &product); err != nil || !strings.EqualFold(product.ID, id) || product.IsFolder == nil || *product.IsFolder || product.Deleted == nil || *product.Deleted {
		return stockProduct{}, errors.New("ID does not identify an active product")
	}
	return product, nil
}

func (s Service) readStockUnit(ctx context.Context, id string) (stockUnit, error) {
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Description"}}
	data, err := s.OData.Get(ctx, "Catalog_ЕдиницыИзмерения(guid'"+strings.ToLower(id)+"')", params, 1<<20)
	if err != nil {
		return stockUnit{}, err
	}
	var unit stockUnit
	if err := json.Unmarshal(data, &unit); err != nil || !strings.EqualFold(unit.ID, id) || unit.Name == "" {
		return stockUnit{}, errors.New("invalid stock unit")
	}
	return unit, nil
}

func (s Service) stockRows(ctx context.Context, productID string) ([]map[string]json.RawMessage, error) {
	reader, ok := s.OData.(stockBalanceReader)
	if !ok {
		return nil, errors.New("OData client does not support stock balances")
	}
	params := url.Values{
		"$format": {"json"}, "$inlinecount": {"allpages"},
		"$select": {sourceFields(config.WarehouseStock.Fields)},
		"$top":    {strconv.Itoa(stockPageSize)},
	}
	rows := make([]map[string]json.RawMessage, 0)
	count := -1
	for len(rows) < maxStockRows {
		params.Set("$skip", strconv.Itoa(len(rows)))
		data, err := reader.GetStockBalance(ctx, productID, params, 8<<20)
		if err != nil {
			return nil, err
		}
		var response struct {
			Count string                       `json:"odata.count"`
			Value []map[string]json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Value == nil {
			return nil, errors.New("invalid OData stock balance response")
		}
		pageCount, err := strconv.Atoi(response.Count)
		if err != nil || pageCount < 0 || pageCount > maxStockRows || count >= 0 && pageCount != count {
			return nil, errors.New("invalid or excessive OData stock balance count")
		}
		count = pageCount
		if len(response.Value) == 0 && len(rows) < count || len(response.Value) > stockPageSize || len(rows)+len(response.Value) > count {
			return nil, errors.New("incomplete OData stock balance page")
		}
		rows = append(rows, response.Value...)
		if len(rows) == count {
			return rows, nil
		}
	}
	return nil, errors.New("OData stock balance exceeds 5000 rows")
}

type stockSum struct {
	value big.Rat
	scale int
}

func (s *stockSum) Add(value Decimal) error {
	text := string(value)
	number, ok := new(big.Rat).SetString(text)
	if !ok {
		return errors.New("invalid stock quantity")
	}
	parts := strings.SplitN(strings.ToLower(text), "e", 2)
	scale := 0
	if dot := strings.IndexByte(parts[0], '.'); dot >= 0 {
		scale = len(parts[0]) - dot - 1
	}
	if len(parts) == 2 {
		exponent, err := strconv.Atoi(parts[1])
		if err != nil {
			return errors.New("invalid stock quantity exponent")
		}
		scale -= exponent
	}
	if scale > 18 {
		return errors.New("stock quantity exceeds supported decimal precision")
	}
	if scale > s.scale {
		s.scale = scale
	}
	s.value.Add(&s.value, number)
	return nil
}

func (s *stockSum) Decimal() Decimal {
	text := s.value.FloatString(s.scale)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return Decimal(text)
}
