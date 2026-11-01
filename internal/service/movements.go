package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type recordSetReader interface {
	GetRecordSet(context.Context, string, string, string, int64) ([]byte, bool, error)
}

type RegisterMovements struct {
	Register string           `json:"register"`
	Count    int              `json:"count"`
	Rows     []map[string]any `json:"rows"`
}

type DocumentMovements struct {
	Kind      string              `json:"kind"`
	ID        string              `json:"id"`
	Document  string              `json:"document"`
	Registers []RegisterMovements `json:"registers"`
	Note      string              `json:"note"`
}

// movementRegisters are the accumulation registers whose movements may be shown; all hold stock, sales, settlements, or money.
var movementRegisters = []string{
	"AccumulationRegister_ЗапасыНаСкладах",
	"AccumulationRegister_Продажи",
	"AccumulationRegister_РасчетыСПокупателями",
	"AccumulationRegister_РасчетыСПоставщиками",
	"AccumulationRegister_ДенежныеСредства",
	"AccumulationRegister_ПлатежныйКалендарь",
}

const maxMovementRows = 1000

// GetDocumentMovements shows what a posted document recorded in the allowed registers.
// The document is first read through its own named command, so the same kind and operation allowlists apply.
func (s Service) GetDocumentMovements(ctx context.Context, kind, id string) (DocumentMovements, error) {
	if !linkedGUID(id) {
		return DocumentMovements{}, errors.New("document ID must be a GUID")
	}
	id = strings.ToLower(id)
	document, err := s.movementDocument(ctx, kind, id)
	if err != nil {
		return DocumentMovements{}, err
	}
	reader, ok := s.OData.(recordSetReader)
	if !ok {
		return DocumentMovements{}, errors.New("OData client does not support register record sets")
	}
	result := DocumentMovements{Kind: kind, ID: id, Document: document, Registers: []RegisterMovements{}, Note: "only stock, sales, settlement, money, and payment-calendar registers are read; unposted documents have no movements"}
	total := 0
	for _, register := range movementRegisters {
		data, found, err := reader.GetRecordSet(ctx, register, document, id, 8<<20)
		if err != nil {
			return DocumentMovements{}, err
		}
		if !found {
			continue
		}
		var set struct {
			RecordSet []map[string]json.RawMessage `json:"RecordSet"`
		}
		if err := json.Unmarshal(data, &set); err != nil {
			return DocumentMovements{}, errors.New("invalid OData record set response")
		}
		if len(set.RecordSet) == 0 {
			continue
		}
		total += len(set.RecordSet)
		if total > maxMovementRows {
			return DocumentMovements{}, errors.New("document has more than 1000 register movements")
		}
		rows := make([]map[string]any, 0, len(set.RecordSet))
		for _, raw := range set.RecordSet {
			row := map[string]any{}
			for field, value := range raw {
				if strings.HasPrefix(field, "Recorder") || strings.HasSuffix(field, "@odata.navigationLinkUrl") {
					continue
				}
				var scalar any
				decoder := json.NewDecoder(strings.NewReader(string(value)))
				decoder.UseNumber()
				if decoder.Decode(&scalar) != nil {
					continue
				}
				switch scalar.(type) {
				case string, bool, json.Number, nil:
					row[field] = scalar
				}
			}
			rows = append(rows, row)
		}
		result.Registers = append(result.Registers, RegisterMovements{Register: register, Count: len(rows), Rows: rows})
	}
	sort.SliceStable(result.Registers, func(i, j int) bool { return result.Registers[i].Register < result.Registers[j].Register })
	return result, nil
}

// movementDocument checks the document through the existing read path for its kind and returns its entity name.
func (s Service) movementDocument(ctx context.Context, kind, id string) (string, error) {
	switch kind {
	case "invoice", "shipment", "return":
		if _, err := s.GetSalesDocument(ctx, kind, id); err != nil {
			return "", err
		}
		return config.SalesDocuments[kind].Name, nil
	case "supplier-order", "goods-receipt":
		purchase := map[string]string{"supplier-order": "order", "goods-receipt": "receipt"}[kind]
		if _, err := s.GetOperationalDocument(ctx, "purchase", purchase, id); err != nil {
			return "", err
		}
		return config.OperationalDocuments[kind].Name, nil
	case "transfer-order", "transfer", "stock-receipt", "stock-writeoff":
		if _, err := s.GetOperationalDocument(ctx, "warehouse", kind, id); err != nil {
			return "", err
		}
		return config.OperationalDocuments[kind].Name, nil
	case "cash-in", "cash-out", "bank-in", "bank-out", "card-payment", "cash-shift":
		if _, err := s.GetMoneyDocument(ctx, kind, id); err != nil {
			return "", err
		}
		return config.MoneyDocuments[kind].Name, nil
	case "payment-plan":
		if _, err := s.GetSupplierPaymentPlan(ctx, id); err != nil {
			return "", err
		}
		return paymentPlanResourceName, nil
	}
	return "", errors.New("kind must be invoice, shipment, return, supplier-order, goods-receipt, transfer-order, transfer, stock-receipt, stock-writeoff, cash-in, cash-out, bank-in, bank-out, card-payment, cash-shift, or payment-plan")
}
