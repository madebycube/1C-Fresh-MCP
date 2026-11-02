package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const (
	advanceReportResourceName = "Document_АвансовыйОтчет"
	advanceReportOperation    = "АвансовыйОтчет"
	accountableRegister       = "AccumulationRegister_РасчетыСПодотчетниками"
	employeesResourceName     = "Catalog_Сотрудники"
	businessLinesResourceName = "Catalog_НаправленияДеятельности"
	maxAdvanceReportLines     = 200
	maxAdvanceIssues          = 50
	advanceGoods              = "goods"
	advanceExpense            = "expense"
)

// advanceIssueKinds are the cash and bank documents that can hand money to an employee; other types are refused.
var advanceIssueKinds = map[string]string{
	"cash-out": "Document_РасходИзКассы",
	"bank-out": "Document_РасходСоСчета",
}

// expenseProductTypes are product types an advance report's expense section accepts; stock items belong to goods.
var expenseProductTypes = map[string]bool{"Услуга": true, "Работа": true}

// AdvanceLine is one purchase on an advance report: goods go to a warehouse, expenses to a department.
type AdvanceLine struct {
	ReceiptPreviewLine
	Section        string `json:"section"`
	WarehouseID    string `json:"warehouse_id,omitempty"`
	DepartmentID   string `json:"department_id,omitempty"`
	BusinessLineID string `json:"business_line_id,omitempty"`
	SupplierID     string `json:"supplier_id,omitempty"`
	IncomingNumber string `json:"incoming_number,omitempty"`
	IncomingDate   string `json:"incoming_date,omitempty"`
	Content        string `json:"content,omitempty"`
}

// AdvanceIssue links an advance report to a cash or bank document that gave the employee money.
type AdvanceIssue struct {
	LineNumber int     `json:"line_number"`
	Kind       string  `json:"kind"`
	DocumentID string  `json:"document_id"`
	Number     string  `json:"number,omitempty"`
	Date       string  `json:"date,omitempty"`
	Issued     Decimal `json:"issued,omitempty"`
	UsedBefore Decimal `json:"used_elsewhere,omitempty"`
	Amount     Decimal `json:"amount"`
	Preserved  bool    `json:"preserved,omitempty"`
}

// AdvanceSupplierPayment is a supplier payment the employee made; this tool keeps such lines but does not write new ones.
type AdvanceSupplierPayment struct {
	LineNumber int     `json:"line_number"`
	SupplierID string  `json:"supplier_id"`
	ContractID string  `json:"contract_id"`
	Amount     Decimal `json:"amount"`
}

// AdvanceReport is an advance report as read from 1C, with its spending balance.
type AdvanceReport struct {
	ID             string                   `json:"id,omitempty"`
	Number         string                   `json:"number,omitempty"`
	DataVersion    string                   `json:"data_version,omitempty"`
	Posted         bool                     `json:"posted"`
	OrganizationID string                   `json:"organization_id"`
	EmployeeID     string                   `json:"employee_id"`
	CurrencyID     string                   `json:"currency_id"`
	Date           string                   `json:"date"`
	VATMode        string                   `json:"vat_mode"`
	Taxation       string                   `json:"taxation"`
	Purpose        string                   `json:"purpose"`
	Comment        string                   `json:"comment"`
	Goods          []AdvanceLine            `json:"goods"`
	Expenses       []AdvanceLine            `json:"expenses"`
	Advances       []AdvanceIssue           `json:"advances"`
	Payments       []AdvanceSupplierPayment `json:"supplier_payments"`
	Amount         Decimal                  `json:"amount"`
	VAT            Decimal                  `json:"vat"`
	Issued         Decimal                  `json:"issued"`
	Spent          Decimal                  `json:"spent"`
	Remainder      Decimal                  `json:"remainder"`
	Overspend      Decimal                  `json:"overspend"`
}

type AdvanceConflict struct {
	Section    string `json:"section"`
	LineNumber int    `json:"line_number"`
	Document   string `json:"document"`
	ID         string `json:"id"`
	Number     string `json:"number"`
	Date       string `json:"date"`
	Posted     bool   `json:"posted"`
	Reason     string `json:"reason"`
}

type AdvanceReportSummary struct {
	ID             string  `json:"id"`
	Number         string  `json:"number"`
	Date           string  `json:"date"`
	Posted         bool    `json:"posted"`
	OrganizationID string  `json:"organization_id"`
	EmployeeID     string  `json:"employee_id"`
	Amount         Decimal `json:"amount"`
}

// AdvanceReportWrite is the preview and the result of an advance report create or amend.
type AdvanceReportWrite struct {
	AdvanceReport
	Applied            bool                   `json:"applied"`
	EmployeeBalance    *AccountableBalance    `json:"employee_balance,omitempty"`
	Before             *ReceiptTotals         `json:"before,omitempty"`
	Changes            []string               `json:"changes,omitempty"`
	Duplicates         []AdvanceReportSummary `json:"duplicates"`
	AccountedElsewhere []AdvanceConflict      `json:"accounted_elsewhere"`
	Warnings           []string               `json:"warnings"`
	Impact             []string               `json:"impact"`
}

type AdvanceLineInput struct {
	Section          string `json:"section"`
	ProductID        string `json:"product_id"`
	CharacteristicID string `json:"characteristic_id,omitempty"`
	Quantity         string `json:"quantity"`
	Price            string `json:"price"`
	VATRate          string `json:"vat_rate"`
	UnitID           string `json:"unit_id,omitempty"`
	Amount           string `json:"amount,omitempty"`
	VAT              string `json:"vat,omitempty"`
	WarehouseID      string `json:"warehouse_id,omitempty"`
	DepartmentID     string `json:"department_id,omitempty"`
	BusinessLineID   string `json:"business_line_id,omitempty"`
	SupplierID       string `json:"supplier_id,omitempty"`
	IncomingNumber   string `json:"incoming_number,omitempty"`
	IncomingDate     string `json:"incoming_date,omitempty"`
	Content          string `json:"content,omitempty"`
}

type AdvanceIssueInput struct {
	Kind       string `json:"kind"`
	DocumentID string `json:"document_id"`
	Amount     string `json:"amount"`
}

type AdvanceReportDraft struct {
	OrganizationID string              `json:"organization_id"`
	EmployeeID     string              `json:"employee_id"`
	Date           string              `json:"date"`
	VATMode        string              `json:"vat_mode"`
	WarehouseID    string              `json:"warehouse_id,omitempty"`
	DepartmentID   string              `json:"department_id,omitempty"`
	BusinessLineID string              `json:"business_line_id,omitempty"`
	Purpose        string              `json:"purpose,omitempty"`
	Comment        string              `json:"comment,omitempty"`
	Advances       []AdvanceIssueInput `json:"advances,omitempty"`
	Lines          []AdvanceLineInput  `json:"lines"`
	AllowDuplicate bool                `json:"allow_duplicate,omitempty"`
	Apply          bool                `json:"apply,omitempty"`
}

type AdvanceLineChange struct {
	Section    string  `json:"section"`
	LineNumber int     `json:"line_number"`
	Quantity   *string `json:"quantity,omitempty"`
	Price      *string `json:"price,omitempty"`
	VATRate    *string `json:"vat_rate,omitempty"`
	Amount     *string `json:"amount,omitempty"`
	VAT        *string `json:"vat,omitempty"`
}

type AdvanceReportAmendment struct {
	Date                *string             `json:"date,omitempty"`
	Purpose             *string             `json:"purpose,omitempty"`
	Comment             *string             `json:"comment,omitempty"`
	ChangeLines         []AdvanceLineChange `json:"change_lines,omitempty"`
	AddLines            []AdvanceLineInput  `json:"add_lines,omitempty"`
	AddAdvances         []AdvanceIssueInput `json:"add_advances,omitempty"`
	ExpectedDataVersion string              `json:"expected_data_version,omitempty"`
	AllowDuplicate      bool                `json:"allow_duplicate,omitempty"`
	Apply               bool                `json:"apply,omitempty"`
}

type AccountableBalance struct {
	OrganizationID string  `json:"organization_id,omitempty"`
	EmployeeID     string  `json:"employee_id"`
	AsOf           string  `json:"as_of"`
	Balance        Decimal `json:"balance"`
	State          string  `json:"state"`
	Source         string  `json:"source"`
}

// GetAdvanceReport reads one advance report with its lines, linked advances, and spending balance.
func (s Service) GetAdvanceReport(ctx context.Context, id string) (AdvanceReport, error) {
	report, err := s.readAdvanceReport(ctx, id)
	if err != nil {
		return AdvanceReport{}, err
	}
	return report.AdvanceReport, nil
}

type AdvanceReportPage struct {
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	Total      int                    `json:"total"`
	Count      int                    `json:"count"`
	Offset     int                    `json:"offset"`
	NextOffset *int                   `json:"next_offset,omitempty"`
	Items      []AdvanceReportSummary `json:"items"`
}

// ListAdvanceReports lists advance reports in a date range of at most 31 days, optionally for one employee.
func (s Service) ListAdvanceReports(ctx context.Context, employeeID, from, to string, limit, offset int) (AdvanceReportPage, error) {
	if err := validateListPage(limit, offset); err != nil {
		return AdvanceReportPage{}, err
	}
	if employeeID != "" && !linkedGUID(employeeID) {
		return AdvanceReportPage{}, errors.New("employee ID must be a nonzero GUID")
	}
	start, end, err := documentDateRange(from, to)
	if err != nil {
		return AdvanceReportPage{}, err
	}
	all, err := s.advanceReportSummaries(ctx)
	if err != nil {
		return AdvanceReportPage{}, err
	}
	items := make([]AdvanceReportSummary, 0)
	for _, item := range all {
		if item.Date >= start && item.Date < end && (employeeID == "" || strings.EqualFold(item.EmployeeID, employeeID)) {
			items = append(items, item)
		}
	}
	page := AdvanceReportPage{From: from, To: to, Total: len(items), Offset: offset}
	page.Items, page.NextOffset = pageSlice(items, limit, offset)
	page.Count = len(page.Items)
	return page, nil
}

// GetAccountableBalance reads one employee's accountable-funds balance; it never lists other employees.
func (s Service) GetAccountableBalance(ctx context.Context, organizationID, employeeID, asOf string) (AccountableBalance, error) {
	if !linkedGUID(employeeID) {
		return AccountableBalance{}, errors.New("employee ID must be a nonzero GUID")
	}
	if organizationID != "" && !linkedGUID(organizationID) {
		return AccountableBalance{}, errors.New("organization ID must be a nonzero GUID")
	}
	asOf, period, err := registerAsOf(asOf)
	if err != nil {
		return AccountableBalance{}, err
	}
	rows, err := s.registerRows(ctx, config.RegisterQuery{Register: accountableRegister, Table: "Balance", Period: period, Dimensions: "Организация,Сотрудник"})
	if err != nil {
		return AccountableBalance{}, err
	}
	total := &stockSum{}
	for _, row := range rows {
		if !strings.EqualFold(rawString(row["Сотрудник_Key"]), employeeID) || organizationID != "" && !strings.EqualFold(rawString(row["Организация_Key"]), organizationID) {
			continue
		}
		if err := total.Add(rawDecimal(row["СуммаBalance"])); err != nil {
			return AccountableBalance{}, errors.New("invalid OData accountable balance row")
		}
	}
	balance := AccountableBalance{OrganizationID: strings.ToLower(organizationID), EmployeeID: strings.ToLower(employeeID), AsOf: asOf, Balance: total.Decimal(), Source: accountableRegister + "/Balance"}
	switch decimalRat(balance.Balance).Sign() {
	case 1:
		balance.State = "the employee holds unreported accountable funds"
	case -1:
		balance.State = "the organization owes the employee for overspending"
	default:
		balance.State = "settled"
	}
	return balance, nil
}

// CreateAdvanceReportDraft previews an advance report and, with Apply, saves it unposted.
// It never creates a cash issue or a supplier receipt, and never retries an uncertain POST.
func (s Service) CreateAdvanceReportDraft(ctx context.Context, input AdvanceReportDraft) (AdvanceReportWrite, error) {
	result, err := s.validateAdvanceHeader(ctx, input.OrganizationID, input.EmployeeID, input.Date, input.VATMode, input.Purpose, input.Comment)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > maxAdvanceReportLines {
		return AdvanceReportWrite{}, fmt.Errorf("an advance report needs 1–%d goods or expense lines", maxAdvanceReportLines)
	}
	if len(input.Advances) > maxAdvanceIssues {
		return AdvanceReportWrite{}, fmt.Errorf("an advance report links at most %d issued advances", maxAdvanceIssues)
	}
	rates, err := s.vatRates(ctx)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	for index, line := range input.Lines {
		if line.WarehouseID == "" {
			line.WarehouseID = input.WarehouseID
		}
		if line.DepartmentID == "" {
			line.DepartmentID = input.DepartmentID
		}
		if line.BusinessLineID == "" {
			line.BusinessLineID = input.BusinessLineID
		}
		section := &result.Goods
		if line.Section == advanceExpense {
			section = &result.Expenses
		}
		preview, err := s.advanceLine(ctx, line, len(*section)+1, result.VATMode, rates)
		if err != nil {
			return AdvanceReportWrite{}, fmt.Errorf("line %d: %w", index+1, err)
		}
		*section = append(*section, preview)
	}
	used, err := s.advanceIssueUsage(ctx, "")
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	for index, issue := range input.Advances {
		linked, err := s.advanceIssue(ctx, issue, index+1, result.AdvanceReport, used)
		if err != nil {
			return AdvanceReportWrite{}, fmt.Errorf("advance %d: %w", index+1, err)
		}
		result.Advances = append(result.Advances, linked)
	}
	if err := uniqueAdvanceIssues(result.Advances); err != nil {
		return AdvanceReportWrite{}, err
	}
	finishAdvanceTotals(&result.AdvanceReport)
	if err := s.advanceChecks(ctx, &result, ""); err != nil {
		return AdvanceReportWrite{}, err
	}
	result.Impact = advanceDraftImpact(result.AdvanceReport)
	if !input.Apply {
		return result, nil
	}
	if (len(result.Duplicates) > 0 || len(result.AccountedElsewhere) > 0) && !input.AllowDuplicate {
		return AdvanceReportWrite{}, errors.New("a matching advance report or a document with the same receipt number exists; review duplicates and accounted_elsewhere or allow them explicitly")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return AdvanceReportWrite{}, errors.New("OData client does not support writes")
	}
	operation, err := s.advanceBusinessOperation(ctx)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	fields := advanceHeaderFields(result.AdvanceReport)
	fields["Ref_Key"] = emptyGUID
	fields["Организация_Key"] = result.OrganizationID
	fields["Сотрудник_Key"] = result.EmployeeID
	fields["ВалютаДокумента_Key"] = result.CurrencyID
	fields["Курс"] = 1
	fields["Кратность"] = 1
	fields["НДСВключатьВСтоимость"] = true
	fields["ХозяйственнаяОперация_Key"] = operation
	fields["Запасы"] = advanceLineRows(result.Goods, nil)
	fields["Расходы"] = advanceLineRows(result.Expenses, nil)
	fields["ВыданныеАвансы"] = advanceIssueRows(result.Advances, nil)
	body, err := json.Marshal(fields)
	if err != nil {
		return AdvanceReportWrite{}, errors.New("could not encode advance report")
	}
	response, err := writer.Write(ctx, "POST", advanceReportResourceName, body, "")
	if err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("could not confirm advance report creation; check list advance-reports for this employee and date before retrying: %w", err)
	}
	var created struct {
		ID string `json:"Ref_Key"`
	}
	if err := json.Unmarshal(response, &created); err != nil || !linkedGUID(created.ID) {
		return AdvanceReportWrite{}, errors.New("1C may have created the advance report but did not return its ID; check list advance-reports before retrying")
	}
	saved, err := s.readAdvanceReport(ctx, created.ID)
	if err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("advance report %s was created but could not be read back: %w", created.ID, err)
	}
	if err := verifyAdvanceSaved(saved.AdvanceReport, result.AdvanceReport); err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("advance report %s was created but differs from the request: %w", created.ID, err)
	}
	result.Applied, result.ID, result.Number, result.DataVersion, result.Posted = true, strings.ToLower(created.ID), saved.Number, saved.DataVersion, saved.Posted
	return result, nil
}

// AmendAdvanceReport previews and, with Apply, saves changes to an unposted advance report, keeping every other field and line as read.
func (s Service) AmendAdvanceReport(ctx context.Context, id string, change AdvanceReportAmendment) (AdvanceReportWrite, error) {
	current, err := s.readAdvanceReport(ctx, id)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	if current.Posted {
		return AdvanceReportWrite{}, errors.New("advance report is posted; unpost it explicitly first, then amend and post it again")
	}
	if change.ExpectedDataVersion != "" && change.ExpectedDataVersion != current.DataVersion {
		return AdvanceReportWrite{}, errors.New("advance report changed since the preview; preview again")
	}
	date, purpose, comment := current.Date, current.Purpose, current.Comment
	changes := make([]string, 0)
	if change.Date != nil {
		normalized, err := receiptDate(*change.Date, false)
		if err != nil {
			return AdvanceReportWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
		}
		if normalized != current.Date {
			date = normalized
			changes = append(changes, "date")
		}
	}
	if change.Purpose != nil && strings.TrimSpace(*change.Purpose) != current.Purpose {
		purpose = *change.Purpose
		changes = append(changes, "purpose")
	}
	if change.Comment != nil && strings.TrimSpace(*change.Comment) != current.Comment {
		comment = *change.Comment
		changes = append(changes, "comment")
	}
	headerChanged := len(changes) > 0
	result, err := s.validateAdvanceHeader(ctx, current.OrganizationID, current.EmployeeID, date, current.VATMode, purpose, comment)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	result.Payments = current.Payments
	rates, err := s.vatRates(ctx)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	for _, pair := range []struct {
		from []AdvanceLine
		to   *[]AdvanceLine
	}{{current.Goods, &result.Goods}, {current.Expenses, &result.Expenses}} {
		for _, line := range pair.from {
			if _, err := existingReceiptLine(editableReceiptLine{LineNumber: line.LineNumber, UnitType: line.UnitType, VATRateID: line.VATRateID}, result.VATMode, rates); err != nil {
				return AdvanceReportWrite{}, fmt.Errorf("existing %s line %d: %w", line.Section, line.LineNumber, err)
			}
			line.Preserved = true
			*pair.to = append(*pair.to, line)
		}
	}
	for _, issue := range current.Advances {
		issue.Preserved = true
		result.Advances = append(result.Advances, issue)
	}
	before := ReceiptTotals{Amount: current.Amount, VAT: current.VAT, Lines: len(current.Goods) + len(current.Expenses)}
	sectionsChanged := map[string]bool{}
	seen := map[string]bool{}
	for _, lineChange := range change.ChangeLines {
		lines := result.Goods
		if lineChange.Section == advanceExpense {
			lines = result.Expenses
		} else if lineChange.Section != advanceGoods {
			return AdvanceReportWrite{}, errors.New("line section must be goods or expense")
		}
		key := fmt.Sprintf("%s:%d", lineChange.Section, lineChange.LineNumber)
		index := -1
		for i, line := range lines {
			if line.LineNumber == lineChange.LineNumber {
				index = i
			}
		}
		if index < 0 || seen[key] {
			return AdvanceReportWrite{}, fmt.Errorf("%s line %d does not exist or is changed twice", lineChange.Section, lineChange.LineNumber)
		}
		seen[key] = true
		line := &lines[index]
		input := ReceiptLineInput{ProductID: line.ProductID, CharacteristicID: line.CharacteristicID, Quantity: string(line.Quantity), Price: string(line.Price), VATRate: rateName(rates, line.VATRateID)}
		for target, value := range map[*string]*string{&input.Quantity: lineChange.Quantity, &input.Price: lineChange.Price, &input.VATRate: lineChange.VATRate, &input.Amount: lineChange.Amount, &input.VAT: lineChange.VAT} {
			if value != nil {
				*target = *value
			}
		}
		updated, err := computeReceiptLine(input, line.LineNumber, lineUnit{ID: line.UnitID, Type: line.UnitType}, result.VATMode, rates)
		if err != nil {
			return AdvanceReportWrite{}, fmt.Errorf("%s line %d: %w", lineChange.Section, lineChange.LineNumber, err)
		}
		line.ReceiptPreviewLine = updated
		sectionsChanged[lineChange.Section] = true
		changes = append(changes, key)
	}
	if len(result.Goods)+len(result.Expenses)+len(change.AddLines) > maxAdvanceReportLines {
		return AdvanceReportWrite{}, fmt.Errorf("an advance report holds at most %d lines", maxAdvanceReportLines)
	}
	for _, input := range change.AddLines {
		section := &result.Goods
		if input.Section == advanceExpense {
			section = &result.Expenses
		}
		next := 0
		for _, line := range *section {
			next = max(next, line.LineNumber)
		}
		preview, err := s.advanceLine(ctx, input, next+1, result.VATMode, rates)
		if err != nil {
			return AdvanceReportWrite{}, fmt.Errorf("new line: %w", err)
		}
		*section = append(*section, preview)
		sectionsChanged[preview.Section] = true
		changes = append(changes, fmt.Sprintf("new %s line %d", preview.Section, preview.LineNumber))
	}
	if len(change.AddAdvances) > 0 {
		used, err := s.advanceIssueUsage(ctx, current.ID)
		if err != nil {
			return AdvanceReportWrite{}, err
		}
		for _, input := range change.AddAdvances {
			linked, err := s.advanceIssue(ctx, input, len(result.Advances)+1, result.AdvanceReport, used)
			if err != nil {
				return AdvanceReportWrite{}, fmt.Errorf("new advance: %w", err)
			}
			result.Advances = append(result.Advances, linked)
			changes = append(changes, fmt.Sprintf("new advance %d", linked.LineNumber))
		}
		if err := uniqueAdvanceIssues(result.Advances); err != nil {
			return AdvanceReportWrite{}, err
		}
	}
	finishAdvanceTotals(&result.AdvanceReport)
	if len(sectionsChanged) == 0 {
		result.Taxation = current.Taxation
	}
	result.ID, result.Number, result.DataVersion, result.Posted, result.Before, result.Changes = current.ID, current.Number, current.DataVersion, current.Posted, &before, changes
	if err := s.advanceChecks(ctx, &result, current.ID); err != nil {
		return AdvanceReportWrite{}, err
	}
	result.Impact = advanceDraftImpact(result.AdvanceReport)
	if !change.Apply || len(changes) == 0 {
		return result, nil
	}
	if len(result.AccountedElsewhere) > 0 && !change.AllowDuplicate {
		return AdvanceReportWrite{}, errors.New("a new line's receipt number is already used by another document; review accounted_elsewhere or allow it explicitly")
	}
	writer, ok := s.OData.(odataWriter)
	if !ok {
		return AdvanceReportWrite{}, errors.New("OData client does not support writes")
	}
	fields := map[string]any{}
	if headerChanged {
		fields["Date"], fields["НазначениеАванса"], fields["Комментарий"] = result.Date, result.Purpose, result.Comment
	}
	if sectionsChanged[advanceGoods] {
		fields["Запасы"] = advanceLineRows(result.Goods, current.raw.goods)
	}
	if sectionsChanged[advanceExpense] {
		fields["Расходы"] = advanceLineRows(result.Expenses, current.raw.expenses)
	}
	if len(change.AddAdvances) > 0 {
		fields["ВыданныеАвансы"] = advanceIssueRows(result.Advances, current.raw.advances)
	}
	if len(sectionsChanged) > 0 {
		fields["СуммаДокумента"], fields["НалогообложениеНДС"] = json.Number(result.Amount), result.Taxation
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return AdvanceReportWrite{}, errors.New("could not encode advance report")
	}
	if _, err := writer.Write(ctx, "PATCH", advanceReportResource(current.ID), body, current.DataVersion); err != nil {
		return AdvanceReportWrite{}, err
	}
	saved, err := s.readAdvanceReport(ctx, current.ID)
	if err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("advance report was updated but could not be read back: %w", err)
	}
	if err := verifyAdvanceSaved(saved.AdvanceReport, result.AdvanceReport); err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("advance report was updated but differs from the request: %w", err)
	}
	result.Applied, result.DataVersion, result.Posted = true, saved.DataVersion, saved.Posted
	return result, nil
}

type AdvanceReportPosting struct {
	ID          string   `json:"id"`
	Number      string   `json:"number"`
	Action      string   `json:"action"`
	Applied     bool     `json:"applied"`
	PostedFrom  bool     `json:"posted_before"`
	Posted      bool     `json:"posted"`
	DataVersion string   `json:"data_version"`
	Amount      Decimal  `json:"amount"`
	Issued      Decimal  `json:"issued"`
	Remainder   Decimal  `json:"remainder"`
	Overspend   Decimal  `json:"overspend"`
	Impact      []string `json:"impact"`
}

// SetAdvanceReportPosting previews and, with Apply, posts or unposts an advance report through the 1C document actions.
func (s Service) SetAdvanceReportPosting(ctx context.Context, id, action, expectedDataVersion string, apply bool) (AdvanceReportPosting, error) {
	if action != "post" && action != "unpost" {
		return AdvanceReportPosting{}, errors.New("action must be post or unpost")
	}
	current, err := s.readAdvanceReport(ctx, id)
	if err != nil {
		return AdvanceReportPosting{}, err
	}
	result := AdvanceReportPosting{ID: current.ID, Number: current.Number, Action: action, PostedFrom: current.Posted, Posted: current.Posted, DataVersion: current.DataVersion, Amount: current.Amount, Issued: current.Issued, Remainder: current.Remainder, Overspend: current.Overspend}
	if action == "post" {
		result.Impact = []string{
			fmt.Sprintf("records the employee's spending of %s against accountable funds; goods lines add stock, expense lines add costs", current.Amount),
			"does not issue or return cash and does not create supplier receipts; a remainder or overspend is settled by separate cash documents",
		}
	} else {
		result.Impact = []string{"removes this report's accountable-funds, stock and expense movements until it is posted again"}
	}
	if expectedDataVersion != "" && expectedDataVersion != current.DataVersion {
		return AdvanceReportPosting{}, errors.New("advance report changed since the preview; preview again")
	}
	if !apply || current.Posted == (action == "post") {
		return result, nil
	}
	actor, ok := s.OData.(documentActor)
	if !ok {
		return AdvanceReportPosting{}, errors.New("OData client does not support document actions")
	}
	name := "Post"
	if action == "unpost" {
		name = "Unpost"
	}
	if _, err := actor.Action(ctx, advanceReportResource(current.ID), name); err != nil {
		return AdvanceReportPosting{}, fmt.Errorf("1C rejected %s; the advance report was not changed by this request or its state is uncertain, so read it before retrying: %w", action, err)
	}
	saved, err := s.readAdvanceReport(ctx, current.ID)
	if err != nil {
		return AdvanceReportPosting{}, fmt.Errorf("%s was sent but the advance report could not be read back: %w", action, err)
	}
	if saved.Posted != (action == "post") {
		return AdvanceReportPosting{}, fmt.Errorf("%s was accepted but the advance report posting flag did not change", action)
	}
	result.Applied, result.Posted, result.DataVersion = true, saved.Posted, saved.DataVersion
	return result, nil
}

func (s Service) validateAdvanceHeader(ctx context.Context, organizationID, employeeID, dateText, vatMode, purpose, comment string) (AdvanceReportWrite, error) {
	if !linkedGUID(organizationID) {
		return AdvanceReportWrite{}, errors.New("organization ID must be a nonzero GUID")
	}
	if !linkedGUID(employeeID) {
		return AdvanceReportWrite{}, errors.New("employee ID must be a nonzero GUID")
	}
	date, err := receiptDate(dateText, false)
	if err != nil {
		return AdvanceReportWrite{}, errors.New("date must be YYYY-MM-DD or YYYY-MM-DDTHH:MM:SS")
	}
	if vatMode != receiptVATIncluded && vatMode != receiptVATExcluded {
		return AdvanceReportWrite{}, errors.New("VAT mode must be inclusive or exclusive")
	}
	if purpose, err = counterpartyText(purpose, true, 150); err != nil {
		return AdvanceReportWrite{}, errors.New("purpose must be at most 150 characters without control characters")
	}
	if comment, err = documentComment(comment); err != nil {
		return AdvanceReportWrite{}, err
	}
	if _, err := s.activeRecord(ctx, "Catalog_Организации", organizationID, "Ref_Key,DeletionMark"); err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("organization: %w", err)
	}
	// Only the employee's existence is checked; no personal fields are read.
	if _, err := s.activeRecord(ctx, employeesResourceName, employeeID, "Ref_Key,DeletionMark"); err != nil {
		return AdvanceReportWrite{}, fmt.Errorf("employee: %w", err)
	}
	currencyID, err := s.baseCurrency(ctx)
	if err != nil {
		return AdvanceReportWrite{}, err
	}
	balance, err := s.GetAccountableBalance(ctx, organizationID, employeeID, date[:10])
	warnings := []string{}
	if err != nil {
		warnings = append(warnings, "accountable-funds balance could not be read: "+err.Error())
		balance = AccountableBalance{}
	}
	result := AdvanceReportWrite{
		AdvanceReport: AdvanceReport{
			OrganizationID: strings.ToLower(organizationID), EmployeeID: strings.ToLower(employeeID), CurrencyID: currencyID, Date: date,
			VATMode: vatMode, Purpose: purpose, Comment: comment,
			Goods: []AdvanceLine{}, Expenses: []AdvanceLine{}, Advances: []AdvanceIssue{}, Payments: []AdvanceSupplierPayment{},
		},
		Duplicates: []AdvanceReportSummary{}, AccountedElsewhere: []AdvanceConflict{}, Warnings: warnings,
	}
	if err == nil {
		result.EmployeeBalance = &balance
	}
	return result, nil
}

func documentComment(comment string) (string, error) {
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > 1024 || strings.IndexFunc(comment, func(r rune) bool { return r < 0x20 && r != '\n' || r == 0x7f }) >= 0 {
		return "", errors.New("comment must be at most 1024 characters")
	}
	return comment, nil
}

func (s Service) advanceLine(ctx context.Context, input AdvanceLineInput, number int, vatMode string, rates map[string]vatRate) (AdvanceLine, error) {
	if input.Section != advanceGoods && input.Section != advanceExpense {
		return AdvanceLine{}, errors.New("section must be goods or expense")
	}
	product, err := s.activeRecord(ctx, config.Products.Name, input.ProductID, "Ref_Key,DeletionMark,IsFolder,ТипНоменклатуры,ЕдиницаИзмерения_Key")
	if err != nil {
		return AdvanceLine{}, fmt.Errorf("product: %w", err)
	}
	kind := rawString(product["ТипНоменклатуры"])
	if string(product["IsFolder"]) != "false" {
		return AdvanceLine{}, errors.New("product must not be a group")
	}
	if input.Section == advanceGoods && kind != receiptStockProducts {
		return AdvanceLine{}, errors.New("goods lines need a stock product; put services and other costs in expense lines")
	}
	if input.Section == advanceExpense && !expenseProductTypes[kind] {
		return AdvanceLine{}, errors.New("expense lines need a service or work product; stock items belong in goods lines")
	}
	unitID := rawString(product["ЕдиницаИзмерения_Key"])
	if !linkedGUID(unitID) {
		return AdvanceLine{}, unsupported("product has no base unit")
	}
	unit := lineUnit{ID: strings.ToLower(unitID), Type: unitClassifierType}
	if input.UnitID != "" && !strings.EqualFold(input.UnitID, unitID) {
		if unit, err = s.productPackagingUnit(ctx, input.ProductID, input.UnitID); err != nil {
			return AdvanceLine{}, err
		}
	}
	if input.CharacteristicID != "" && input.CharacteristicID != emptyGUID {
		if input.Section == advanceExpense {
			return AdvanceLine{}, errors.New("expense lines have no characteristic")
		}
		characteristic, err := s.activeRecord(ctx, config.ProductCharacteristics.Name, input.CharacteristicID, "Ref_Key,DeletionMark,Owner,Недействителен")
		if err != nil {
			return AdvanceLine{}, fmt.Errorf("characteristic: %w", err)
		}
		if !strings.EqualFold(rawString(characteristic["Owner"]), input.ProductID) || string(characteristic["Недействителен"]) == "true" {
			return AdvanceLine{}, errors.New("characteristic must be active and belong to the product")
		}
	}
	line := AdvanceLine{Section: input.Section}
	if input.Section == advanceGoods {
		if err := s.structuralUnit(ctx, "warehouse", input.WarehouseID, "Склад"); err != nil {
			return AdvanceLine{}, err
		}
		line.WarehouseID = strings.ToLower(input.WarehouseID)
	} else {
		if err := s.structuralUnit(ctx, "department", input.DepartmentID, "Подразделение"); err != nil {
			return AdvanceLine{}, err
		}
		line.DepartmentID = strings.ToLower(input.DepartmentID)
		if input.BusinessLineID != "" {
			if _, err := s.activeRecord(ctx, businessLinesResourceName, input.BusinessLineID, "Ref_Key,DeletionMark"); err != nil {
				return AdvanceLine{}, fmt.Errorf("business line: %w", err)
			}
			line.BusinessLineID = strings.ToLower(input.BusinessLineID)
		}
	}
	if input.SupplierID != "" {
		supplier, err := s.readCounterpartyRecord(ctx, input.SupplierID)
		if err != nil {
			return AdvanceLine{}, fmt.Errorf("supplier: %w", err)
		}
		if supplier.IsFolder == nil || *supplier.IsFolder || supplier.Deleted == nil || *supplier.Deleted {
			return AdvanceLine{}, errors.New("supplier ID must identify an active counterparty")
		}
		line.SupplierID = strings.ToLower(input.SupplierID)
	}
	if line.IncomingNumber, err = counterpartyText(input.IncomingNumber, true, 30); err != nil {
		return AdvanceLine{}, errors.New("receipt or UPD number must be at most 30 characters without control characters")
	}
	if line.IncomingDate, err = receiptDate(input.IncomingDate, true); err != nil {
		return AdvanceLine{}, errors.New("receipt or UPD date must be YYYY-MM-DD")
	}
	if line.Content, err = counterpartyText(input.Content, true, 250); err != nil {
		return AdvanceLine{}, errors.New("content must be at most 250 characters without control characters")
	}
	computed, err := computeReceiptLine(ReceiptLineInput{ProductID: input.ProductID, CharacteristicID: input.CharacteristicID, Quantity: input.Quantity, Price: input.Price, VATRate: input.VATRate, Amount: input.Amount, VAT: input.VAT}, number, unit, vatMode, rates)
	if err != nil {
		return AdvanceLine{}, err
	}
	line.ReceiptPreviewLine = computed
	return line, nil
}

func (s Service) structuralUnit(ctx context.Context, label, id, kind string) error {
	if !linkedGUID(id) {
		return fmt.Errorf("%s ID must be a nonzero GUID", label)
	}
	unit, err := s.activeRecord(ctx, config.Warehouses.Name, id, "Ref_Key,DeletionMark,ТипСтруктурнойЕдиницы,Недействителен")
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if rawString(unit["ТипСтруктурнойЕдиницы"]) != kind || string(unit["Недействителен"]) == "true" {
		return fmt.Errorf("%s ID must identify an active %s", label, kind)
	}
	return nil
}

// advanceIssueUsage sums, per cash or bank document, what other active advance reports already account for.
func (s Service) advanceIssueUsage(ctx context.Context, exceptID string) (map[string]*big.Rat, error) {
	headers, err := s.advanceReportSummaries(ctx)
	if err != nil {
		return nil, err
	}
	active := make(map[string]bool, len(headers))
	for _, header := range headers {
		active[strings.ToLower(header.ID)] = !strings.EqualFold(header.ID, exceptID)
	}
	rows, err := s.scanRows(ctx, advanceReportResourceName+"_ВыданныеАвансы", "Ref_Key,LineNumber,Документ,Документ_Type,Сумма", "", 20000)
	if err != nil {
		return nil, fmt.Errorf("could not check advances already reported: %w", err)
	}
	used := map[string]*big.Rat{}
	for _, row := range rows {
		if !active[strings.ToLower(rawString(row["Ref_Key"]))] {
			continue
		}
		id := strings.ToLower(rawString(row["Документ"]))
		if used[id] == nil {
			used[id] = new(big.Rat)
		}
		used[id].Add(used[id], decimalRat(rawDecimal(row["Сумма"])))
	}
	return used, nil
}

func (s Service) advanceIssue(ctx context.Context, input AdvanceIssueInput, number int, report AdvanceReport, used map[string]*big.Rat) (AdvanceIssue, error) {
	resource, ok := advanceIssueKinds[input.Kind]
	if !ok {
		return AdvanceIssue{}, errors.New("advance kind must be cash-out or bank-out")
	}
	document, err := s.activeRecord(ctx, resource, input.DocumentID, "Ref_Key,DeletionMark,Posted,Number,Date,Организация_Key,Подотчетник_Key,СуммаДокумента")
	if err != nil {
		return AdvanceIssue{}, fmt.Errorf("document: %w", err)
	}
	if string(document["Posted"]) != "true" {
		return AdvanceIssue{}, errors.New("the cash or bank document is not posted, so it gave the employee no money yet")
	}
	if !strings.EqualFold(rawString(document["Организация_Key"]), report.OrganizationID) || !strings.EqualFold(rawString(document["Подотчетник_Key"]), report.EmployeeID) {
		return AdvanceIssue{}, errors.New("the cash or bank document must pay this employee from this organization")
	}
	issued := decimalRat(rawDecimal(document["СуммаДокумента"]))
	amount := new(big.Rat).Set(issued)
	usedBefore := new(big.Rat)
	if prior := used[strings.ToLower(input.DocumentID)]; prior != nil {
		usedBefore.Set(prior)
	}
	available := new(big.Rat).Sub(issued, usedBefore)
	if input.Amount != "" {
		if amount, err = exactDecimal(input.Amount, receiptAmountScale, false); err != nil {
			return AdvanceIssue{}, errors.New("amount must be positive with at most 2 decimals")
		}
	} else {
		amount = available
	}
	if amount.Sign() <= 0 || amount.Cmp(available) > 0 {
		return AdvanceIssue{}, fmt.Errorf("only %s of this advance is not yet reported by other advance reports", ratDecimal(available, receiptAmountScale))
	}
	date := rawString(document["Date"])
	return AdvanceIssue{
		LineNumber: number, Kind: input.Kind, DocumentID: strings.ToLower(input.DocumentID), Number: rawString(document["Number"]), Date: date[:min(10, len(date))],
		Issued: ratDecimal(issued, receiptAmountScale), UsedBefore: ratDecimal(usedBefore, receiptAmountScale), Amount: ratDecimal(amount, receiptAmountScale),
	}, nil
}

func uniqueAdvanceIssues(issues []AdvanceIssue) error {
	seen := map[string]bool{}
	for _, issue := range issues {
		if seen[issue.DocumentID] {
			return errors.New("the same cash or bank document is linked twice")
		}
		seen[issue.DocumentID] = true
	}
	return nil
}

func finishAdvanceTotals(report *AdvanceReport) {
	spent, vat, issued := &stockSum{}, &stockSum{}, &stockSum{}
	taxed := false
	for _, lines := range [][]AdvanceLine{report.Goods, report.Expenses} {
		for _, line := range lines {
			_ = spent.Add(line.Total)
			_ = vat.Add(line.VAT)
			if rateIsTaxed(line) {
				taxed = true
			}
		}
	}
	for _, payment := range report.Payments {
		_ = spent.Add(payment.Amount)
	}
	for _, issue := range report.Advances {
		_ = issued.Add(issue.Amount)
	}
	report.Amount, report.VAT, report.Spent, report.Issued = spent.Decimal(), vat.Decimal(), spent.Decimal(), issued.Decimal()
	difference := new(big.Rat).Sub(decimalRat(report.Issued), decimalRat(report.Spent))
	report.Remainder, report.Overspend = "0", "0"
	if difference.Sign() > 0 {
		report.Remainder = ratDecimal(difference, receiptAmountScale)
	} else if difference.Sign() < 0 {
		report.Overspend = ratDecimal(new(big.Rat).Neg(difference), receiptAmountScale)
	}
	report.Taxation = receiptNotTaxed
	if taxed {
		report.Taxation = receiptTaxed
	}
}

func rateIsTaxed(line AdvanceLine) bool {
	return line.VATRate != "" && line.VATRate != "БезНДС" || line.VATRate == "" && decimalRat(line.VAT).Sign() != 0
}

func advanceDraftImpact(report AdvanceReport) []string {
	impact := []string{
		"saved unposted: no accountable-funds, stock or expense movements until it is posted explicitly",
		fmt.Sprintf("posting would record %s spent by the employee: %d goods lines and %d expense lines", report.Spent, len(report.Goods), len(report.Expenses)),
		"this tool never issues or returns cash and never creates a supplier receipt",
	}
	if len(report.Advances) == 0 {
		impact = append(impact, "no issued advance is linked; the report is checked against the employee's overall balance only")
	} else if report.Overspend != "0" {
		impact = append(impact, "spending exceeds the linked advances by "+string(report.Overspend)+"; the organization would owe the employee")
	} else if report.Remainder != "0" {
		impact = append(impact, string(report.Remainder)+" of the linked advances stays with the employee to return or report later")
	}
	return impact
}

// advanceChecks fills likely duplicate reports and lines already accounted for by another document, and warnings for lines it cannot check.
func (s Service) advanceChecks(ctx context.Context, result *AdvanceReportWrite, exceptID string) error {
	summaries, err := s.advanceReportSummaries(ctx)
	if err != nil {
		return fmt.Errorf("could not check for duplicate advance reports: %w", err)
	}
	headers := make(map[string]AdvanceReportSummary, len(summaries))
	for _, summary := range summaries {
		headers[strings.ToLower(summary.ID)] = summary
		if strings.EqualFold(summary.ID, exceptID) || exceptID != "" {
			continue
		}
		if strings.EqualFold(summary.OrganizationID, result.OrganizationID) && strings.EqualFold(summary.EmployeeID, result.EmployeeID) && strings.HasPrefix(summary.Date, result.Date[:10]) && decimalEqual(summary.Amount, result.Amount) {
			result.Duplicates = append(result.Duplicates, summary)
		}
	}
	type key struct{ supplier, number string }
	wanted := map[key][]AdvanceLine{}
	for _, lines := range [][]AdvanceLine{result.Goods, result.Expenses} {
		for _, line := range lines {
			if line.Preserved {
				continue
			}
			if line.IncomingNumber == "" {
				result.Warnings = append(result.Warnings, fmt.Sprintf("%s line %d has no receipt or UPD number, so it cannot be checked against other documents", line.Section, line.LineNumber))
				continue
			}
			k := key{line.SupplierID, strings.ToLower(line.IncomingNumber)}
			wanted[k] = append(wanted[k], line)
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	matches := func(line AdvanceLine, supplier, number, date string) bool {
		if !strings.EqualFold(strings.TrimSpace(number), line.IncomingNumber) {
			return false
		}
		if line.SupplierID != "" && supplier != "" && supplier != emptyGUID && !strings.EqualFold(supplier, line.SupplierID) {
			return false
		}
		return line.IncomingDate == "" || strings.HasPrefix(date, "0001-01-01") || strings.HasPrefix(date, line.IncomingDate)
	}
	for section, resource := range map[string]string{advanceGoods: "_Запасы", advanceExpense: "_Расходы"} {
		rows, err := s.scanRows(ctx, advanceReportResourceName+resource, "Ref_Key,LineNumber,Поставщик_Key,НомерВходящегоДокумента,ДатаВходящегоДокумента", "", 50000)
		if err != nil {
			return fmt.Errorf("could not check other advance reports' %s lines: %w", section, err)
		}
		for _, row := range rows {
			owner := strings.ToLower(rawString(row["Ref_Key"]))
			header, ok := headers[owner]
			if !ok || owner == strings.ToLower(exceptID) {
				continue
			}
			for _, candidates := range wanted {
				for _, line := range candidates {
					if matches(line, rawString(row["Поставщик_Key"]), rawString(row["НомерВходящегоДокумента"]), rawString(row["ДатаВходящегоДокумента"])) {
						result.AccountedElsewhere = append(result.AccountedElsewhere, AdvanceConflict{Section: line.Section, LineNumber: line.LineNumber, Document: "advance report", ID: header.ID, Number: header.Number, Date: header.Date, Posted: header.Posted, Reason: "another advance report has a line with the same receipt number"})
					}
				}
			}
		}
	}
	plan := config.DocumentResource{Name: receiptResourceName, DateField: "Date", DeletedField: "DeletionMark", Fields: []config.FieldBinding{
		{Output: "id", Source: "Ref_Key"}, {Output: "number", Source: "Number"}, {Output: "date", Source: "Date"}, {Output: "posted", Source: "Posted"},
		{Output: "organization_id", Source: "Организация_Key"}, {Output: "supplier_id", Source: "Контрагент_Key"},
		{Output: "incoming_number", Source: "НомерВходящегоДокумента"}, {Output: "incoming_date", Source: "ДатаВходящегоДокумента"},
	}}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return fmt.Errorf("could not check supplier receipts: %w", err)
	}
	for _, row := range rows {
		if !strings.EqualFold(rawString(row["Организация_Key"]), result.OrganizationID) {
			continue
		}
		for _, candidates := range wanted {
			for _, line := range candidates {
				// A receipt always names its supplier, so only lines with a supplier are compared with receipts.
				if line.SupplierID == "" || !matches(line, rawString(row["Контрагент_Key"]), rawString(row["НомерВходящегоДокумента"]), rawString(row["ДатаВходящегоДокумента"])) {
					continue
				}
				result.AccountedElsewhere = append(result.AccountedElsewhere, AdvanceConflict{Section: line.Section, LineNumber: line.LineNumber, Document: "supplier receipt", ID: strings.ToLower(rawString(row["Ref_Key"])), Number: rawString(row["Number"]), Date: rawString(row["Date"]), Posted: string(row["Posted"]) == "true", Reason: "a supplier receipt from the same supplier has the same incoming number"})
			}
		}
	}
	sort.SliceStable(result.AccountedElsewhere, func(i, j int) bool {
		a, b := result.AccountedElsewhere[i], result.AccountedElsewhere[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		return a.LineNumber < b.LineNumber
	})
	return nil
}

func (s Service) advanceReportSummaries(ctx context.Context) ([]AdvanceReportSummary, error) {
	plan := config.DocumentResource{Name: advanceReportResourceName, DateField: "Date", DeletedField: "DeletionMark", Fields: []config.FieldBinding{
		{Output: "id", Source: "Ref_Key"}, {Output: "number", Source: "Number"}, {Output: "date", Source: "Date"}, {Output: "posted", Source: "Posted"},
		{Output: "organization_id", Source: "Организация_Key"}, {Output: "employee_id", Source: "Сотрудник_Key"}, {Output: "amount", Source: "СуммаДокумента"},
	}}
	rows, err := s.allDocumentRows(ctx, plan)
	if err != nil {
		return nil, err
	}
	items := make([]AdvanceReportSummary, 0, len(rows))
	for _, row := range rows {
		item, err := bindFields[AdvanceReportSummary](row, plan.Fields)
		if err != nil {
			return nil, errors.New("invalid OData advance report fields")
		}
		item.ID, item.OrganizationID, item.EmployeeID = strings.ToLower(item.ID), strings.ToLower(item.OrganizationID), strings.ToLower(item.EmployeeID)
		items = append(items, item)
	}
	return items, nil
}

// advanceBusinessOperation uses the predefined operation, or else the one every existing advance report uses.
func (s Service) advanceBusinessOperation(ctx context.Context) (string, error) {
	if operation, err := s.predefinedOperation(ctx, advanceReportOperation); err == nil {
		return operation, nil
	}
	rows, err := s.scanRows(ctx, advanceReportResourceName, "Ref_Key,DeletionMark,ХозяйственнаяОперация_Key", "", 20000)
	if err != nil {
		return "", err
	}
	found := ""
	for _, row := range rows {
		operation := strings.ToLower(rawString(row["ХозяйственнаяОперация_Key"]))
		if string(row["DeletionMark"]) == "true" || !linkedGUID(operation) {
			continue
		}
		if found != "" && found != operation {
			return "", errors.New("existing advance reports use different business operations; cannot choose one")
		}
		found = operation
	}
	if found == "" {
		return "", unsupported("the advance report business operation was not found")
	}
	return found, nil
}

type advanceRaw struct {
	goods, expenses, advances []map[string]any
}

type storedAdvanceReport struct {
	AdvanceReport
	raw advanceRaw
}

func (s Service) readAdvanceReport(ctx context.Context, id string) (storedAdvanceReport, error) {
	if !linkedGUID(id) {
		return storedAdvanceReport{}, errors.New("advance report ID must be a nonzero GUID")
	}
	data, err := s.OData.Get(ctx, advanceReportResource(id), url.Values{"$format": {"json"}}, 8<<20)
	if err != nil {
		return storedAdvanceReport{}, err
	}
	var row map[string]json.RawMessage
	if err := json.Unmarshal(data, &row); err != nil || !strings.EqualFold(rawString(row["Ref_Key"]), id) {
		return storedAdvanceReport{}, errors.New("invalid OData advance report response")
	}
	if string(row["DeletionMark"]) != "false" {
		return storedAdvanceReport{}, errors.New("advance report is marked for deletion")
	}
	if rawString(row["DataVersion"]) == "" {
		return storedAdvanceReport{}, errors.New("advance report has no data version for a safe update")
	}
	mode := receiptVATExcluded
	if string(row["СуммаВключаетНДС"]) == "true" {
		mode = receiptVATIncluded
	}
	report := storedAdvanceReport{AdvanceReport: AdvanceReport{
		ID: strings.ToLower(id), Number: rawString(row["Number"]), DataVersion: rawString(row["DataVersion"]), Posted: string(row["Posted"]) == "true",
		OrganizationID: strings.ToLower(rawString(row["Организация_Key"])), EmployeeID: strings.ToLower(rawString(row["Сотрудник_Key"])),
		CurrencyID: strings.ToLower(rawString(row["ВалютаДокумента_Key"])), Date: rawString(row["Date"]), VATMode: mode,
		Taxation: rawString(row["НалогообложениеНДС"]), Purpose: rawString(row["НазначениеАванса"]), Comment: rawString(row["Комментарий"]),
		Goods: []AdvanceLine{}, Expenses: []AdvanceLine{}, Advances: []AdvanceIssue{}, Payments: []AdvanceSupplierPayment{},
	}}
	sections := map[string]*[]map[string]any{"Запасы": &report.raw.goods, "Расходы": &report.raw.expenses, "ВыданныеАвансы": &report.raw.advances}
	var payments []map[string]any
	sections["Оплаты"] = &payments
	for name, target := range sections {
		raw, ok := row[name]
		if !ok {
			continue
		}
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if err := decoder.Decode(target); err != nil {
			return storedAdvanceReport{}, errors.New("invalid OData advance report lines")
		}
		for _, line := range *target {
			for key := range line {
				if strings.Contains(key, "@") {
					delete(line, key)
				}
			}
		}
	}
	lineNumber := func(line map[string]any) (int, error) {
		var number int
		if _, err := fmt.Sscan(anyString(line["LineNumber"]), &number); err != nil || number < 1 {
			return 0, errors.New("invalid OData advance report line number")
		}
		return number, nil
	}
	for _, pair := range []struct {
		section string
		rows    []map[string]any
		target  *[]AdvanceLine
	}{{advanceGoods, report.raw.goods, &report.Goods}, {advanceExpense, report.raw.expenses, &report.Expenses}} {
		for _, line := range pair.rows {
			number, err := lineNumber(line)
			if err != nil {
				return storedAdvanceReport{}, err
			}
			parsed := AdvanceLine{
				ReceiptPreviewLine: ReceiptPreviewLine{
					LineNumber: number, ProductID: strings.ToLower(anyString(line["Номенклатура_Key"])), CharacteristicID: strings.ToLower(anyString(line["Характеристика_Key"])),
					UnitID: strings.ToLower(anyString(line["ЕдиницаИзмерения"])), UnitType: anyString(line["ЕдиницаИзмерения_Type"]),
					Quantity: anyDecimal(line["Количество"]), Price: anyDecimal(line["Цена"]), VATRateID: strings.ToLower(anyString(line["СтавкаНДС_Key"])),
					Amount: anyDecimal(line["Сумма"]), VAT: anyDecimal(line["СуммаНДС"]), Total: anyDecimal(line["Всего"]),
				},
				Section: pair.section, SupplierID: nonEmptyGUID(anyString(line["Поставщик_Key"])),
				IncomingNumber: strings.TrimSpace(anyString(line["НомерВходящегоДокумента"])), Content: anyString(line["Содержание"]),
			}
			if parsed.CharacteristicID == emptyGUID {
				parsed.CharacteristicID = ""
			}
			if date := anyString(line["ДатаВходящегоДокумента"]); len(date) >= 10 && !strings.HasPrefix(date, "0001-01-01") {
				parsed.IncomingDate = date[:10]
			}
			unit := nonEmptyGUID(anyString(line["СтруктурнаяЕдиница_Key"]))
			if pair.section == advanceGoods {
				parsed.WarehouseID = unit
			} else {
				parsed.DepartmentID, parsed.BusinessLineID = unit, nonEmptyGUID(anyString(line["НаправлениеДеятельности_Key"]))
			}
			*pair.target = append(*pair.target, parsed)
		}
	}
	kinds := map[string]string{}
	for kind, resource := range advanceIssueKinds {
		kinds["StandardODATA."+resource] = kind
	}
	for _, line := range report.raw.advances {
		number, err := lineNumber(line)
		if err != nil {
			return storedAdvanceReport{}, err
		}
		kind := kinds[anyString(line["Документ_Type"])]
		if kind == "" {
			kind = "other"
		}
		report.Advances = append(report.Advances, AdvanceIssue{LineNumber: number, Kind: kind, DocumentID: strings.ToLower(anyString(line["Документ"])), Amount: anyDecimal(line["Сумма"])})
	}
	for _, line := range payments {
		number, err := lineNumber(line)
		if err != nil {
			return storedAdvanceReport{}, err
		}
		report.Payments = append(report.Payments, AdvanceSupplierPayment{LineNumber: number, SupplierID: strings.ToLower(anyString(line["Контрагент_Key"])), ContractID: strings.ToLower(anyString(line["Договор_Key"])), Amount: anyDecimal(line["СуммаПлатежа"])})
	}
	taxation := report.Taxation
	finishAdvanceTotals(&report.AdvanceReport)
	report.Taxation = taxation
	report.Amount = rawDecimal(row["СуммаДокумента"])
	return report, nil
}

func nonEmptyGUID(id string) string {
	if id == emptyGUID {
		return ""
	}
	return strings.ToLower(id)
}

func rateName(rates map[string]vatRate, id string) string {
	for name, rate := range rates {
		if strings.EqualFold(rate.ID, id) {
			return name
		}
	}
	return ""
}

func verifyAdvanceSaved(saved, want AdvanceReport) error {
	if saved.Posted {
		return errors.New("1C saved the advance report as posted")
	}
	if !strings.EqualFold(saved.OrganizationID, want.OrganizationID) || !strings.EqualFold(saved.EmployeeID, want.EmployeeID) || saved.Purpose != want.Purpose || saved.VATMode != want.VATMode {
		return errors.New("header fields do not match")
	}
	for _, pair := range [][2][]AdvanceLine{{saved.Goods, want.Goods}, {saved.Expenses, want.Expenses}} {
		if len(pair[0]) != len(pair[1]) {
			return errors.New("line count does not match")
		}
		for index, line := range pair[0] {
			expected := pair[1][index]
			if !strings.EqualFold(line.ProductID, expected.ProductID) || !decimalEqual(line.Quantity, expected.Quantity) || !decimalEqual(line.Price, expected.Price) || !decimalEqual(line.Total, expected.Total) {
				return fmt.Errorf("%s line %d does not match", line.Section, line.LineNumber)
			}
		}
	}
	if len(saved.Advances) != len(want.Advances) || !decimalEqual(saved.Issued, want.Issued) {
		return errors.New("linked advances do not match")
	}
	if !decimalEqual(saved.Amount, want.Amount) {
		return errors.New("document total " + string(saved.Amount) + " differs from " + string(want.Amount))
	}
	return nil
}

func advanceHeaderFields(report AdvanceReport) map[string]any {
	return map[string]any{
		"Date": report.Date, "НазначениеАванса": report.Purpose, "Комментарий": report.Comment, "СуммаВключаетНДС": report.VATMode == receiptVATIncluded,
		"НалогообложениеНДС": report.Taxation, "СуммаДокумента": json.Number(report.Amount),
	}
}

// advanceLineRows starts from the stored rows, so fields this tool does not manage survive an amendment.
func advanceLineRows(lines []AdvanceLine, original []map[string]any) []map[string]any {
	stored := make(map[int]map[string]any, len(original))
	for _, row := range original {
		var number int
		_, _ = fmt.Sscan(anyString(row["LineNumber"]), &number)
		stored[number] = row
	}
	rows := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		row := map[string]any{}
		for key, value := range stored[line.LineNumber] {
			row[key] = value
		}
		if line.Preserved {
			rows = append(rows, row)
			continue
		}
		row["LineNumber"] = fmt.Sprint(line.LineNumber)
		row["Номенклатура_Key"] = line.ProductID
		row["ЕдиницаИзмерения"] = line.UnitID
		row["ЕдиницаИзмерения_Type"] = line.UnitType
		row["Количество"] = json.Number(line.Quantity)
		row["Цена"] = json.Number(line.Price)
		row["Сумма"] = json.Number(line.Amount)
		row["СтавкаНДС_Key"] = line.VATRateID
		row["СуммаНДС"] = json.Number(line.VAT)
		row["Всего"] = json.Number(line.Total)
		if _, kept := stored[line.LineNumber]; !kept {
			row["Поставщик_Key"] = orEmptyGUID(line.SupplierID)
			row["НомерВходящегоДокумента"] = line.IncomingNumber
			row["ДатаВходящегоДокумента"] = "0001-01-01T00:00:00"
			if line.IncomingDate != "" {
				row["ДатаВходящегоДокумента"] = line.IncomingDate + "T00:00:00"
			}
			row["Содержание"] = line.Content
			if line.Section == advanceGoods {
				row["Характеристика_Key"] = orEmptyGUID(line.CharacteristicID)
				row["СтруктурнаяЕдиница_Key"] = line.WarehouseID
			} else {
				row["СтруктурнаяЕдиница_Key"] = line.DepartmentID
				row["НаправлениеДеятельности_Key"] = orEmptyGUID(line.BusinessLineID)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func advanceIssueRows(issues []AdvanceIssue, original []map[string]any) []map[string]any {
	rows := make([]map[string]any, 0, len(issues))
	for index, issue := range issues {
		if issue.Preserved && index < len(original) {
			rows = append(rows, original[index])
			continue
		}
		rows = append(rows, map[string]any{
			"LineNumber": fmt.Sprint(issue.LineNumber), "Документ": issue.DocumentID, "Документ_Type": "StandardODATA." + advanceIssueKinds[issue.Kind],
			"Сумма": json.Number(issue.Amount),
		})
	}
	return rows
}

func advanceReportResource(id string) string {
	return advanceReportResourceName + "(guid'" + strings.ToLower(id) + "')"
}
