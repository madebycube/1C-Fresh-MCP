package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const (
	propertyDefinitionsResource = "ChartOfCharacteristicTypes_ДополнительныеРеквизитыИСведения"
	propertySetsResource        = "Catalog_НаборыДополнительныхРеквизитовИСведений"
	receiptPropertiesField      = "ДополнительныеРеквизиты"
	maxLinkPropertyLength       = 1024
	receiptPropertySet          = "Документ_ПриходнаяНакладная"
)

var storedLink = regexp.MustCompile(`(?s)^<a href\s*=\s*"([^"]*)"\s*>(.*)</a>$`)

// LinkProperty is a configured additional property that 1C shows as a hyperlink on documents.
type LinkProperty struct {
	ID             string   `json:"id"`
	Description    string   `json:"description"`
	Title          string   `json:"title"`
	Name           string   `json:"name"`
	MaxLength      int      `json:"max_length"`
	OwnerSet       string   `json:"owner_set,omitempty"`
	MemberOf       []string `json:"member_of"`
	ReceiptsUsing  int      `json:"receipts_using"`
	ForReceipts    bool     `json:"for_receipts"`
	ReceiptsReason string   `json:"receipts_reason"`
}

type LinkPropertyList struct {
	Properties   []LinkProperty `json:"properties"`
	ReceiptsRead int            `json:"receipts_read"`
	Notes        []string       `json:"notes"`
}

// ReceiptLink is one receipt's value of a hyperlink property. URL and caption are kept apart:
// 1C stores the caption in Значение and the full <a href = "URL">caption</a> markup in ТекстоваяСтрока.
type ReceiptLink struct {
	ReceiptID   string `json:"receipt_id"`
	Number      string `json:"number"`
	Posted      bool   `json:"posted"`
	DataVersion string `json:"data_version"`
	PropertyID  string `json:"property_id"`
	Property    string `json:"property"`
	Present     bool   `json:"present"`
	URL         string `json:"url"`
	Caption     string `json:"caption"`
	Format      string `json:"format"`
}

type ReceiptLinkChange struct {
	Property            string  `json:"property,omitempty"`
	URL                 *string `json:"url,omitempty"`
	Caption             *string `json:"caption,omitempty"`
	Clear               bool    `json:"clear,omitempty"`
	ExpectedDataVersion string  `json:"expected_data_version,omitempty"`
	Apply               bool    `json:"apply,omitempty"`
}

type ReceiptLinkWrite struct {
	Applied   bool        `json:"applied"`
	Unchanged bool        `json:"unchanged"`
	Before    ReceiptLink `json:"before"`
	After     ReceiptLink `json:"after"`
	Impact    []string    `json:"impact"`
}

// ListLinkProperties lists hyperlink string properties and the evidence that they belong on supplier receipts.
func (s Service) ListLinkProperties(ctx context.Context) (LinkPropertyList, error) {
	definitions, err := s.scanRows(ctx, propertyDefinitionsResource, "Ref_Key,DeletionMark,Description,Заголовок,Имя,ValueType,ВыводитьВВидеГиперссылки,ЭтоДополнительноеСведение,НаборСвойств_Key", "", maxDocumentScan)
	if err != nil {
		return LinkPropertyList{}, err
	}
	sets, err := s.scanRows(ctx, propertySetsResource, "Ref_Key,DeletionMark,Description,ИмяПредопределенногоНабора,Используется,ДополнительныеРеквизиты", "", maxDocumentScan)
	if err != nil {
		return LinkPropertyList{}, err
	}
	receipts, err := s.scanRows(ctx, receiptResourceName, "Ref_Key,ВидОперации,"+receiptPropertiesField, "", maxDocumentScan)
	if err != nil {
		return LinkPropertyList{}, err
	}
	setNames := make(map[string]string, len(sets))
	receiptSets := make(map[string]bool)
	members := make(map[string][]string)
	for _, set := range sets {
		id := strings.ToLower(rawString(set["Ref_Key"]))
		setNames[id] = rawString(set["Description"])
		// The live set of a document is named by ИмяПредопределенногоНабора; legacy УдалитьДокумент_* sets are unused.
		if string(set["DeletionMark"]) != "true" && string(set["Используется"]) == "true" && rawString(set["ИмяПредопределенногоНабора"]) == receiptPropertySet {
			receiptSets[id] = true
		}
		var rows []map[string]json.RawMessage
		_ = json.Unmarshal(set["ДополнительныеРеквизиты"], &rows)
		for _, row := range rows {
			if string(row["ПометкаУдаления"]) != "true" {
				property := strings.ToLower(rawString(row["Свойство_Key"]))
				members[property] = append(members[property], id)
			}
		}
	}
	usage := make(map[string]int)
	for _, receipt := range receipts {
		if rawString(receipt["ВидОперации"]) != receiptOperation {
			continue
		}
		var rows []map[string]json.RawMessage
		_ = json.Unmarshal(receipt[receiptPropertiesField], &rows)
		for _, row := range rows {
			usage[strings.ToLower(rawString(row["Свойство_Key"]))]++
		}
	}
	list := LinkPropertyList{Properties: []LinkProperty{}, ReceiptsRead: len(receipts), Notes: []string{
		"A property is offered for receipts only when the receipt property set (" + receiptPropertySet + ") owns or includes it.",
	}}
	for _, row := range definitions {
		id := strings.ToLower(rawString(row["Ref_Key"]))
		var valueType struct {
			Types   []string `json:"Types"`
			Strings struct {
				Length string `json:"Length"`
			} `json:"StringQualifiers"`
		}
		_ = json.Unmarshal(row["ValueType"], &valueType)
		if string(row["DeletionMark"]) != "false" || string(row["ВыводитьВВидеГиперссылки"]) != "true" || string(row["ЭтоДополнительноеСведение"]) == "true" || len(valueType.Types) != 1 || valueType.Types[0] != "String" {
			continue
		}
		property := LinkProperty{ID: id, Description: rawString(row["Description"]), Title: rawString(row["Заголовок"]), Name: rawString(row["Имя"]), MemberOf: []string{}, ReceiptsUsing: usage[id]}
		if _, err := fmt.Sscan(valueType.Strings.Length, &property.MaxLength); err != nil || property.MaxLength <= 0 {
			property.MaxLength = maxLinkPropertyLength
		}
		owner := strings.ToLower(rawString(row["НаборСвойств_Key"]))
		property.OwnerSet = setNames[owner]
		for _, set := range members[id] {
			property.MemberOf = append(property.MemberOf, setNames[set])
		}
		switch {
		case receiptSets[owner]:
			property.ForReceipts, property.ReceiptsReason = true, "owned by a receipt property set"
		case anyIn(members[id], receiptSets):
			property.ForReceipts, property.ReceiptsReason = true, "included in a receipt property set"
		default:
			property.ReceiptsReason = "the receipt property set does not include it"
		}
		list.Properties = append(list.Properties, property)
	}
	return list, nil
}

func anyIn(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

// receiptLinkProperty resolves a property by GUID, description, title or name. Without a selector it needs exactly one candidate.
func (s Service) receiptLinkProperty(ctx context.Context, selector string) (LinkProperty, error) {
	list, err := s.ListLinkProperties(ctx)
	if err != nil {
		return LinkProperty{}, err
	}
	selector = strings.TrimSpace(selector)
	matches := make([]LinkProperty, 0)
	for _, property := range list.Properties {
		if selector == "" && property.ForReceipts || selector != "" && (strings.EqualFold(property.ID, selector) || strings.EqualFold(property.Description, selector) || strings.EqualFold(property.Title, selector) || strings.EqualFold(property.Name, selector)) {
			matches = append(matches, property)
		}
	}
	switch {
	case len(matches) == 0 && selector == "":
		return LinkProperty{}, errors.New("no hyperlink property is set up for supplier receipts; configure one in 1C first")
	case len(matches) == 0:
		return LinkProperty{}, fmt.Errorf("no hyperlink property matches %q; list them with list link-properties", selector)
	case len(matches) > 1:
		names := make([]string, 0, len(matches))
		for _, property := range matches {
			names = append(names, property.ID+" ("+property.Description+")")
		}
		return LinkProperty{}, errors.New("several hyperlink properties match; choose one by ID: " + strings.Join(names, ", "))
	case !matches[0].ForReceipts:
		return LinkProperty{}, errors.New("property " + matches[0].Description + " is not used for supplier receipts: " + matches[0].ReceiptsReason)
	}
	return matches[0], nil
}

type editableLinkReceipt struct {
	ID          string
	Number      string
	Posted      bool
	DataVersion string
	Rows        []map[string]any
}

func (s Service) readLinkReceipt(ctx context.Context, id string) (editableLinkReceipt, error) {
	if !linkedGUID(id) {
		return editableLinkReceipt{}, errors.New("receipt ID must be a nonzero GUID")
	}
	params := url.Values{"$format": {"json"}, "$select": {"Ref_Key,Number,Posted,DeletionMark,DataVersion,ВидОперации," + receiptPropertiesField}}
	data, err := s.OData.Get(ctx, receiptResource(id), params, 4<<20)
	if err != nil {
		return editableLinkReceipt{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return editableLinkReceipt{}, errors.New("invalid OData receipt response")
	}
	if rawString(row["ВидОперации"]) != receiptOperation || string(row["DeletionMark"]) != "false" {
		return editableLinkReceipt{}, errors.New("ID must identify an active goods receipt from a supplier")
	}
	if rawString(row["DataVersion"]) == "" {
		return editableLinkReceipt{}, errors.New("receipt has no data version for a safe update")
	}
	receipt := editableLinkReceipt{ID: strings.ToLower(id), Number: strings.TrimSpace(rawString(row["Number"])), Posted: string(row["Posted"]) == "true", DataVersion: rawString(row["DataVersion"]), Rows: []map[string]any{}}
	if raw, ok := row[receiptPropertiesField]; ok && string(raw) != "null" {
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(&receipt.Rows); err != nil {
			return editableLinkReceipt{}, errors.New("invalid OData additional properties")
		}
	}
	for _, line := range receipt.Rows {
		for key := range line {
			if strings.Contains(key, "@") {
				delete(line, key)
			}
		}
	}
	return receipt, nil
}

func (r editableLinkReceipt) link(property LinkProperty) (ReceiptLink, int, error) {
	link := ReceiptLink{ReceiptID: r.ID, Number: r.Number, Posted: r.Posted, DataVersion: r.DataVersion, PropertyID: property.ID, Property: property.Description, Format: "absent"}
	index := -1
	for i, row := range r.Rows {
		if !strings.EqualFold(anyString(row["Свойство_Key"]), property.ID) {
			continue
		}
		if index >= 0 {
			return ReceiptLink{}, -1, errors.New("the receipt stores this property twice; fix it in 1C")
		}
		index = i
		value, text := anyString(row["Значение"]), anyString(row["ТекстоваяСтрока"])
		link.Present = true
		if match := storedLink.FindStringSubmatch(text); match != nil {
			link.URL, link.Caption, link.Format = match[1], match[2], "hyperlink"
		} else {
			link.Caption, link.Format = value, "text"
		}
	}
	return link, index, nil
}

// GetReceiptLink reads a receipt's hyperlink property without following the URL.
func (s Service) GetReceiptLink(ctx context.Context, receiptID, propertySelector string) (ReceiptLink, error) {
	receipt, err := s.readLinkReceipt(ctx, receiptID)
	if err != nil {
		return ReceiptLink{}, err
	}
	property, err := s.receiptLinkProperty(ctx, propertySelector)
	if err != nil {
		return ReceiptLink{}, err
	}
	link, _, err := receipt.link(property)
	return link, err
}

// SetReceiptLink previews and, with Apply, writes one hyperlink property of one receipt.
// URL and caption change independently, other properties and every other field are left as read,
// and the posting state is checked after the save rather than changed by this tool.
func (s Service) SetReceiptLink(ctx context.Context, receiptID string, change ReceiptLinkChange) (ReceiptLinkWrite, error) {
	property, err := s.receiptLinkProperty(ctx, change.Property)
	if err != nil {
		return ReceiptLinkWrite{}, err
	}
	receipt, err := s.readLinkReceipt(ctx, receiptID)
	if err != nil {
		return ReceiptLinkWrite{}, err
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != receipt.DataVersion {
		return ReceiptLinkWrite{}, errors.New("receipt changed since the preview; preview again")
	}
	before, index, err := receipt.link(property)
	if err != nil {
		return ReceiptLinkWrite{}, err
	}
	after := before
	if change.Clear {
		if change.URL != nil || change.Caption != nil {
			return ReceiptLinkWrite{}, errors.New("clear cannot be combined with url or caption")
		}
		after.Present, after.URL, after.Caption, after.Format = false, "", "", "absent"
	} else {
		if change.URL == nil && change.Caption == nil {
			return ReceiptLinkWrite{}, errors.New("give a url, a caption, or clear")
		}
		if change.URL != nil {
			after.URL = strings.TrimSpace(*change.URL)
		}
		if change.Caption != nil {
			after.Caption = strings.TrimSpace(*change.Caption)
		}
		if err := validateLink(after.URL, after.Caption, property.MaxLength); err != nil {
			return ReceiptLinkWrite{}, err
		}
		if after.Caption == "" {
			after.Caption = after.URL
		}
		after.Present, after.Format = true, "hyperlink"
	}
	result := ReceiptLinkWrite{Before: before, After: after, Unchanged: before.Present == after.Present && before.URL == after.URL && before.Caption == after.Caption && before.Format == after.Format}
	result.Impact = []string{"changes only the " + property.Description + " property; other properties, lines, payments and comments stay as read", "does not open the URL or change sharing on it"}
	if receipt.Posted {
		result.Impact = append(result.Impact, "the receipt is posted; 1C rewrites it on save, and the tool checks that it stays posted")
	}
	if !change.Apply || result.Unchanged {
		return result, nil
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return ReceiptLinkWrite{}, errors.New("OData client does not support writes")
	}
	rows := make([]map[string]any, 0, len(receipt.Rows)+1)
	for i, row := range receipt.Rows {
		if i != index {
			rows = append(rows, row)
		}
	}
	if after.Present {
		row := map[string]any{"Свойство_Key": property.ID, "Значение_Type": "Edm.String"}
		if index >= 0 {
			for key, value := range receipt.Rows[index] {
				row[key] = value
			}
		}
		row["Значение"] = after.Caption
		row["ТекстоваяСтрока"] = `<a href = "` + after.URL + `">` + after.Caption + `</a>`
		if index >= 0 {
			rows = append(rows[:index], append([]map[string]any{row}, rows[index:]...)...)
		} else {
			rows = append(rows, row)
		}
	}
	for i, row := range rows {
		row["LineNumber"] = fmt.Sprint(i + 1)
	}
	body, err := json.Marshal(map[string]any{receiptPropertiesField: rows})
	if err != nil {
		return ReceiptLinkWrite{}, errors.New("could not encode additional properties")
	}
	if _, err := writer.Write(ctx, http.MethodPatch, receiptResource(receipt.ID), body, receipt.DataVersion); err != nil {
		hint := ""
		if receipt.Posted {
			hint = "; if 1C refuses to rewrite a posted receipt, unpost it with unpost receipt, set the link, then post receipt again"
		}
		return ReceiptLinkWrite{}, fmt.Errorf("could not confirm the link update; read it before retrying%s: %w", hint, err)
	}
	saved, err := s.readLinkReceipt(ctx, receipt.ID)
	if err != nil {
		return ReceiptLinkWrite{}, fmt.Errorf("the link was saved but the receipt could not be read back: %w", err)
	}
	savedLink, _, err := saved.link(property)
	if err != nil {
		return ReceiptLinkWrite{}, err
	}
	if saved.Posted != receipt.Posted {
		return ReceiptLinkWrite{}, errors.New("the link was saved but 1C changed the receipt's posting state; check it and run post receipt if it must stay posted")
	}
	if savedLink.Present != after.Present || savedLink.URL != after.URL || savedLink.Caption != after.Caption || len(saved.Rows) != len(rows) {
		return ReceiptLinkWrite{}, errors.New("the link was saved but differs on read-back")
	}
	result.Applied, result.After = true, savedLink
	return result, nil
}

func validateLink(link, caption string, maxLength int) error {
	parsed, err := url.Parse(link)
	if link == "" || err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(link, "\"<> \t\r\n") {
		return errors.New("url must be an absolute http or https address without spaces, quotes or angle brackets")
	}
	if strings.ContainsAny(caption, "<>\r\n") {
		return errors.New("caption must be one line without angle brackets")
	}
	shown := caption
	if shown == "" {
		shown = link
	}
	if len([]rune(shown)) > maxLength || len([]rune(link)) > maxLength {
		return fmt.Errorf("url and caption must each be at most %d characters", maxLength)
	}
	return nil
}
