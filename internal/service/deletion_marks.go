package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

type DeletionMark struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Label       string `json:"label"`
	Applied     bool   `json:"applied"`
	MarkedFrom  bool   `json:"marked_before"`
	Marked      bool   `json:"marked"`
	DataVersion string `json:"data_version"`
	Impact      string `json:"impact"`
}

type markable struct {
	resource string
	document bool
}

// markableKinds lists what may get a deletion mark: the documents this tool creates and the main catalog items.
// A deletion mark is reversible; nothing here deletes a record.
var markableKinds = map[string]markable{
	"receipt":        {resource: receiptResourceName, document: true},
	"payment-plan":   {resource: paymentPlanResourceName, document: true},
	"supplier-order": {resource: supplierOrderResourceName, document: true},
	"transfer":       {resource: transferResourceName, document: true},
	"price-document": {resource: config.PriceDocuments.Name, document: true},
	"product":        {resource: config.Products.Name},
	"counterparty":   {resource: config.Customers.Name},
}

// SetDeletionMark previews and, with apply, sets or clears the 1C deletion mark on one record.
// Posted documents must be unposted first, so marking never silently removes register movements.
func (s Service) SetDeletionMark(ctx context.Context, kind, id string, mark bool, expectedDataVersion string, apply bool) (DeletionMark, error) {
	target, ok := markableKinds[kind]
	if !ok {
		return DeletionMark{}, errors.New("kind must be receipt, payment-plan, supplier-order, transfer, price-document, product, or counterparty")
	}
	if !linkedGUID(id) {
		return DeletionMark{}, errors.New("ID must be a GUID")
	}
	id = strings.ToLower(id)
	resource := target.resource + "(guid'" + id + "')"
	current, posted, err := s.readMarkable(ctx, target, resource, id)
	if err != nil {
		return DeletionMark{}, err
	}
	current.Kind = kind
	if mark {
		current.Impact = "hides the record from lists and selection in 1C; an administrator can delete marked records later, and the mark can be cleared until then"
	} else {
		current.Impact = "clears the deletion mark so the record is usable again; documents stay unposted"
	}
	if mark && posted {
		return DeletionMark{}, errors.New("document is posted; unpost it before marking it for deletion")
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return DeletionMark{}, errors.New("record changed since the preview; preview again")
	}
	if !apply || current.MarkedFrom == mark {
		return current, nil
	}
	if current.DataVersion == "" {
		return DeletionMark{}, errors.New("record has no data version; refusing to change it")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return DeletionMark{}, errors.New("OData client does not support writes")
	}
	body, _ := json.Marshal(map[string]bool{"DeletionMark": mark})
	if _, err := writer.Write(ctx, http.MethodPatch, resource, body, current.DataVersion); err != nil {
		return DeletionMark{}, fmt.Errorf("1C rejected the deletion mark change: %w", err)
	}
	saved, _, err := s.readMarkable(ctx, target, resource, id)
	if err != nil {
		return DeletionMark{}, fmt.Errorf("the change was sent but the record could not be read back: %w", err)
	}
	if saved.MarkedFrom != mark {
		return DeletionMark{}, errors.New("the change was accepted but the deletion mark did not change")
	}
	current.Applied, current.Marked, current.DataVersion = true, saved.MarkedFrom, saved.DataVersion
	return current, nil
}

func (s Service) readMarkable(ctx context.Context, target markable, resource, id string) (DeletionMark, bool, error) {
	data, err := s.OData.Get(ctx, resource, url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return DeletionMark{}, false, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return DeletionMark{}, false, errors.New("invalid OData record response")
	}
	var marked, posted, folder bool
	if json.Unmarshal(row["DeletionMark"], &marked) != nil {
		return DeletionMark{}, false, errors.New("invalid OData record response")
	}
	label := rawString(row["Description"])
	if target.document {
		if json.Unmarshal(row["Posted"], &posted) != nil {
			return DeletionMark{}, false, errors.New("invalid OData document response")
		}
		label = rawString(row["Number"])
	} else if raw, ok := row["IsFolder"]; ok && json.Unmarshal(raw, &folder) == nil && folder {
		return DeletionMark{}, false, errors.New("record is a group; groups cannot be marked here")
	}
	return DeletionMark{ID: id, Label: label, MarkedFrom: marked, Marked: marked, DataVersion: rawString(row["DataVersion"])}, posted, nil
}
