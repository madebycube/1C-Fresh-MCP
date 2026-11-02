package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

// advanceWriter stores written advance reports so reads after a write see what 1C would return.
type advanceWriter struct {
	*traceReader
	writes   []string
	bodies   []map[string]any
	ifMatch  []string
	actions  []string
	failPost bool
	balance  []map[string]any
}

func (w *advanceWriter) Write(_ context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	w.writes = append(w.writes, method+" "+resource)
	w.ifMatch = append(w.ifMatch, ifMatch)
	var fields map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return nil, err
	}
	w.bodies = append(w.bodies, fields)
	if method == "POST" {
		if w.failPost {
			return nil, errors.New("OData connection failed")
		}
		stored := map[string]any{}
		for key, value := range fields {
			stored[key] = value
		}
		stored["Ref_Key"], stored["Number"], stored["DataVersion"], stored["Posted"], stored["DeletionMark"] = salesID(500), "AO-1", "v1", false, false
		w.rows[advanceReportResourceName] = append(w.rows[advanceReportResourceName], stored)
		return json.Marshal(map[string]any{"Ref_Key": salesID(500)})
	}
	for _, row := range w.rows[advanceReportResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			if row["DataVersion"] != ifMatch {
				return nil, errors.New("OData record changed since it was read")
			}
			for key, value := range fields {
				row[key] = value
			}
			row["DataVersion"] = "v2"
		}
	}
	return nil, nil
}

func (w *advanceWriter) Action(_ context.Context, resource, action string) ([]byte, error) {
	w.actions = append(w.actions, action)
	for _, row := range w.rows[advanceReportResourceName] {
		if strings.Contains(resource, row["Ref_Key"].(string)) {
			row["Posted"] = action == "Post"
		}
	}
	return nil, nil
}

func (w *advanceWriter) GetRegisterTable(_ context.Context, query config.RegisterQuery, _ url.Values, _ int64) ([]byte, error) {
	if query.Register != accountableRegister || query.Table != "Balance" {
		return nil, errors.New("unexpected register")
	}
	return json.Marshal(map[string]any{"odata.count": "2", "value": w.balance})
}

const (
	advOrg        = 80
	advEmployee   = 81
	advOther      = 82
	advSupplier   = 50
	advWarehouse  = 62
	advDepartment = 63
	advStock      = 64
	advService    = 66
	advCash       = 30
	advLine       = 67
)

func advanceFixture() *advanceWriter {
	return &advanceWriter{
		balance: []map[string]any{
			{"Организация_Key": salesID(advOrg), "Сотрудник_Key": salesID(advEmployee), "СуммаBalance": 1500},
			{"Организация_Key": salesID(advOrg), "Сотрудник_Key": salesID(advOther), "СуммаBalance": 99},
		},
		traceReader: &traceReader{rows: map[string][]map[string]any{
			"Catalog_Организации":             {{"Ref_Key": salesID(advOrg), "DeletionMark": false}},
			"Catalog_Сотрудники":              {{"Ref_Key": salesID(advEmployee), "DeletionMark": false}, {"Ref_Key": salesID(advOther), "DeletionMark": false}},
			"Catalog_Валюты":                  {{"Ref_Key": salesID(61), "DeletionMark": false, "Code": "643"}},
			"Catalog_Контрагенты":             {{"Ref_Key": salesID(advSupplier), "Description": "Example shop", "IsFolder": false, "DeletionMark": false, "Поставщик": true, "Покупатель": false}},
			"Catalog_НаправленияДеятельности": {{"Ref_Key": salesID(advLine), "DeletionMark": false}},
			"Catalog_СтруктурныеЕдиницы": {
				{"Ref_Key": salesID(advWarehouse), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Склад", "Недействителен": false},
				{"Ref_Key": salesID(advDepartment), "DeletionMark": false, "ТипСтруктурнойЕдиницы": "Подразделение", "Недействителен": false},
			},
			"Catalog_Номенклатура": {
				{"Ref_Key": salesID(advStock), "DeletionMark": false, "IsFolder": false, "ТипНоменклатуры": "Запас", "ЕдиницаИзмерения_Key": salesID(65)},
				{"Ref_Key": salesID(advService), "DeletionMark": false, "IsFolder": false, "ТипНоменклатуры": "Услуга", "ЕдиницаИзмерения_Key": salesID(65)},
			},
			"Catalog_СтавкиНДС": {
				{"Ref_Key": salesID(70), "DeletionMark": false, "Ставка": 22, "НеОблагается": false, "Расчетная": false, "ПеречислениеСтавкаНДС": "НДС22"},
				{"Ref_Key": salesID(72), "DeletionMark": false, "Ставка": 0, "НеОблагается": true, "Расчетная": false, "ПеречислениеСтавкаНДС": "БезНДС"},
			},
			"Catalog_ХозяйственныеОперации": {{"Ref_Key": salesID(76), "DeletionMark": false, "PredefinedDataName": "АвансовыйОтчет"}},
			"Document_РасходИзКассы": {
				{"Ref_Key": salesID(advCash), "DeletionMark": false, "Posted": true, "Number": "RK-1", "Date": "2026-10-01T09:00:00", "Организация_Key": salesID(advOrg), "Подотчетник_Key": salesID(advEmployee), "СуммаДокумента": 1000},
				{"Ref_Key": salesID(31), "DeletionMark": false, "Posted": false, "Number": "RK-2", "Date": "2026-10-01T09:00:00", "Организация_Key": salesID(advOrg), "Подотчетник_Key": salesID(advEmployee), "СуммаДокумента": 500},
				{"Ref_Key": salesID(32), "DeletionMark": false, "Posted": true, "Number": "RK-3", "Date": "2026-10-01T09:00:00", "Организация_Key": salesID(advOrg), "Подотчетник_Key": salesID(advOther), "СуммаДокумента": 500},
			},
			advanceReportResourceName: {
				{"Ref_Key": salesID(90), "Number": "AO-OLD", "Date": "2026-09-20T00:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": salesID(advOrg), "Сотрудник_Key": salesID(advEmployee), "СуммаДокумента": 300},
				{"Ref_Key": salesID(91), "Number": "AO-DEL", "Date": "2026-09-21T00:00:00", "Posted": false, "DeletionMark": true, "Организация_Key": salesID(advOrg), "Сотрудник_Key": salesID(advEmployee), "СуммаДокумента": 900},
			},
			advanceReportResourceName + "_ВыданныеАвансы": {
				{"Ref_Key": salesID(90), "LineNumber": "1", "Документ": salesID(advCash), "Документ_Type": "StandardODATA.Document_РасходИзКассы", "Сумма": 300},
				{"Ref_Key": salesID(91), "LineNumber": "1", "Документ": salesID(advCash), "Документ_Type": "StandardODATA.Document_РасходИзКассы", "Сумма": 700},
			},
			advanceReportResourceName + "_Запасы": {
				{"Ref_Key": salesID(90), "LineNumber": "1", "Поставщик_Key": salesID(advSupplier), "НомерВходящегоДокумента": "CHK-7", "ДатаВходящегоДокумента": "2026-09-19T00:00:00"},
			},
			advanceReportResourceName + "_Расходы": {},
			receiptResourceName: {
				{"Ref_Key": salesID(95), "Number": "PN-1", "Date": "2026-09-01T00:00:00", "Posted": true, "DeletionMark": false, "Организация_Key": salesID(advOrg), "Контрагент_Key": salesID(advSupplier), "НомерВходящегоДокумента": "UPD-5", "ДатаВходящегоДокумента": "2026-09-01T00:00:00"},
			},
		}},
	}
}

func advanceDraftInput() AdvanceReportDraft {
	return AdvanceReportDraft{
		OrganizationID: salesID(advOrg), EmployeeID: salesID(advEmployee), Date: "2026-10-05", VATMode: receiptVATIncluded,
		WarehouseID: salesID(advWarehouse), DepartmentID: salesID(advDepartment), BusinessLineID: salesID(advLine), Purpose: "Хозяйственные нужды",
		Advances: []AdvanceIssueInput{{Kind: "cash-out", DocumentID: salesID(advCash)}},
		Lines: []AdvanceLineInput{
			{Section: advanceGoods, ProductID: salesID(advStock), Quantity: "2", Price: "122", VATRate: "НДС22", SupplierID: salesID(advSupplier), IncomingNumber: "CHK-8", IncomingDate: "2026-10-04"},
			{Section: advanceExpense, ProductID: salesID(advService), Quantity: "1", Price: "250.50", VATRate: "БезНДС", Content: "Такси"},
		},
	}
}

func TestAdvanceReportPreviewComputesBalanceWithoutWriting(t *testing.T) {
	fake := advanceFixture()
	preview, err := Service{OData: fake}.CreateAdvanceReportDraft(context.Background(), advanceDraftInput())
	if err != nil {
		t.Fatal(err)
	}
	goods, expense := preview.Goods[0], preview.Expenses[0]
	// 2 × 122 = 244 including VAT; 244 × 22 / 122 = 44.
	if goods.Total != "244" || goods.VAT != "44" || goods.WarehouseID != salesID(advWarehouse) || expense.Total != "250.5" || expense.DepartmentID != salesID(advDepartment) || expense.BusinessLineID != salesID(advLine) {
		t.Fatalf("lines: %+v %+v", goods, expense)
	}
	// The deleted report's 700 does not count; the posted one used 300 of the 1000 issued.
	if len(preview.Advances) != 1 || preview.Advances[0].Amount != "700" || preview.Advances[0].UsedBefore != "300" {
		t.Fatalf("advances: %+v", preview.Advances)
	}
	if preview.Amount != "494.5" || preview.Issued != "700" || preview.Remainder != "205.5" || preview.Overspend != "0" || preview.Taxation != receiptTaxed {
		t.Fatalf("totals: %+v", preview.AdvanceReport)
	}
	if preview.EmployeeBalance == nil || preview.EmployeeBalance.Balance != "1500" {
		t.Fatalf("balance must cover only this employee: %+v", preview.EmployeeBalance)
	}
	if len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "expense line 1") || len(preview.AccountedElsewhere) != 0 || len(preview.Duplicates) != 0 {
		t.Fatalf("checks: %+v", preview)
	}
	if preview.Applied || len(fake.writes) != 0 {
		t.Fatalf("preview wrote: %v", fake.writes)
	}
}

func TestAdvanceReportOverspendAndApply(t *testing.T) {
	fake := advanceFixture()
	input := advanceDraftInput()
	input.Advances[0].Amount = "400"
	input.Apply = true
	saved, err := Service{OData: fake}.CreateAdvanceReportDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Applied || saved.ID != salesID(500) || saved.Number != "AO-1" || saved.Overspend != "94.5" || saved.Remainder != "0" {
		t.Fatalf("saved: %+v", saved)
	}
	if len(fake.writes) != 1 || fake.writes[0] != "POST "+advanceReportResourceName {
		t.Fatalf("writes: %v", fake.writes)
	}
	body := fake.bodies[0]
	if body["Сотрудник_Key"] != salesID(advEmployee) || body["ХозяйственнаяОперация_Key"] != salesID(76) || body["СуммаДокумента"] != json.Number("494.5") || body["Posted"] != nil {
		t.Fatalf("header: %v", body)
	}
	goods := body["Запасы"].([]any)[0].(map[string]any)
	expense := body["Расходы"].([]any)[0].(map[string]any)
	issue := body["ВыданныеАвансы"].([]any)[0].(map[string]any)
	if goods["СтруктурнаяЕдиница_Key"] != salesID(advWarehouse) || goods["НомерВходящегоДокумента"] != "CHK-8" || goods["Поставщик_Key"] != salesID(advSupplier) {
		t.Fatalf("goods row: %v", goods)
	}
	if expense["СтруктурнаяЕдиница_Key"] != salesID(advDepartment) || expense["НаправлениеДеятельности_Key"] != salesID(advLine) || expense["Характеристика_Key"] != nil {
		t.Fatalf("expense row: %v", expense)
	}
	if issue["Документ_Type"] != "StandardODATA.Document_РасходИзКассы" || issue["Сумма"] != json.Number("400") {
		t.Fatalf("advance row: %v", issue)
	}
	// A retry of the same report is flagged as a duplicate and blocked.
	again := advanceDraftInput()
	again.Advances = nil
	again.Apply = true
	if _, err := (Service{OData: fake}).CreateAdvanceReportDraft(context.Background(), again); err == nil || len(fake.writes) != 1 {
		t.Fatalf("duplicate retry: %v %v", err, fake.writes)
	}
}

func TestAdvanceReportRefusesBadAdvancesAndLines(t *testing.T) {
	for name, mutate := range map[string]func(*AdvanceReportDraft){
		"over issued":     func(d *AdvanceReportDraft) { d.Advances[0].Amount = "700.01" },
		"unposted cash":   func(d *AdvanceReportDraft) { d.Advances[0].DocumentID = salesID(31) },
		"other employee":  func(d *AdvanceReportDraft) { d.Advances[0].DocumentID = salesID(32) },
		"unknown kind":    func(d *AdvanceReportDraft) { d.Advances[0].Kind = "payroll" },
		"linked twice":    func(d *AdvanceReportDraft) { d.Advances = append(d.Advances, d.Advances[0]) },
		"service as good": func(d *AdvanceReportDraft) { d.Lines[0].ProductID = salesID(advService) },
		"stock expense":   func(d *AdvanceReportDraft) { d.Lines[1].ProductID = salesID(advStock) },
		"bad section":     func(d *AdvanceReportDraft) { d.Lines[0].Section = "payroll" },
		"no warehouse":    func(d *AdvanceReportDraft) { d.WarehouseID = "" },
		"unknown worker":  func(d *AdvanceReportDraft) { d.EmployeeID = salesID(99) },
		"bad vat mode":    func(d *AdvanceReportDraft) { d.VATMode = "none" },
	} {
		fake := advanceFixture()
		input := advanceDraftInput()
		mutate(&input)
		input.Apply = true
		if _, err := (Service{OData: fake}).CreateAdvanceReportDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
			t.Fatalf("%s: err %v, writes %v", name, err, fake.writes)
		}
	}
}

func TestAdvanceReportFlagsLinesAccountedElsewhere(t *testing.T) {
	fake := advanceFixture()
	input := advanceDraftInput()
	input.Lines[0].IncomingNumber, input.Lines[0].IncomingDate = "UPD-5", ""
	input.Lines[1].SupplierID, input.Lines[1].IncomingNumber = salesID(advSupplier), "chk-7"
	preview, err := Service{OData: fake}.CreateAdvanceReportDraft(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.AccountedElsewhere) != 2 || preview.AccountedElsewhere[0].Document != "advance report" || preview.AccountedElsewhere[0].ID != salesID(90) || preview.AccountedElsewhere[1].Document != "supplier receipt" || preview.AccountedElsewhere[1].ID != salesID(95) {
		t.Fatalf("conflicts: %+v", preview.AccountedElsewhere)
	}
	input.Apply = true
	if _, err := (Service{OData: fake}).CreateAdvanceReportDraft(context.Background(), input); err == nil || len(fake.writes) != 0 {
		t.Fatalf("conflict must block apply: %v", err)
	}
	input.AllowDuplicate = true
	if saved, err := (Service{OData: fake}).CreateAdvanceReportDraft(context.Background(), input); err != nil || !saved.Applied {
		t.Fatalf("allowed: %v", err)
	}
}

func storedAdvanceFixture(posted bool) *advanceWriter {
	fake := advanceFixture()
	fake.rows[advanceReportResourceName] = append(fake.rows[advanceReportResourceName], map[string]any{
		"Ref_Key": salesID(92), "Number": "AO-2", "DataVersion": "v1", "Date": "2026-10-02T00:00:00", "Posted": posted, "DeletionMark": false,
		"Организация_Key": salesID(advOrg), "Сотрудник_Key": salesID(advEmployee), "ВалютаДокумента_Key": salesID(61), "СуммаВключаетНДС": true,
		"НалогообложениеНДС": receiptTaxed, "НазначениеАванса": "", "Комментарий": "keep", "СуммаДокумента": 244,
		"Запасы": []map[string]any{{"LineNumber": "1", "Номенклатура_Key": salesID(advStock), "Характеристика_Key": emptyGUID, "ЕдиницаИзмерения": salesID(65), "ЕдиницаИзмерения_Type": unitClassifierType,
			"Количество": 2, "Цена": 122, "Сумма": 244, "СтавкаНДС_Key": salesID(70), "СуммаНДС": 44, "Всего": 244, "СтруктурнаяЕдиница_Key": salesID(advWarehouse),
			"Партия_Key": salesID(77), "НомерВходящегоДокумента": "CHK-9"}},
		"Расходы":        []map[string]any{},
		"ВыданныеАвансы": []map[string]any{{"LineNumber": "1", "Документ": salesID(advCash), "Документ_Type": "StandardODATA.Document_РасходИзКассы", "Сумма": 300}},
		"Оплаты":         []map[string]any{},
	})
	return fake
}

func TestAdvanceReportGetAndAmendKeepUntouchedFields(t *testing.T) {
	fake := storedAdvanceFixture(false)
	svc := Service{OData: fake}
	report, err := svc.GetAdvanceReport(context.Background(), salesID(92))
	if err != nil {
		t.Fatal(err)
	}
	if report.Amount != "244" || report.Issued != "300" || report.Remainder != "56" || len(report.Goods) != 1 || report.Goods[0].IncomingNumber != "CHK-9" || report.Advances[0].Kind != "cash-out" {
		t.Fatalf("report: %+v", report)
	}
	quantity := "3"
	change := AdvanceReportAmendment{ChangeLines: []AdvanceLineChange{{Section: advanceGoods, LineNumber: 1, Quantity: &quantity}}, ExpectedDataVersion: "v1", Apply: true}
	saved, err := svc.AmendAdvanceReport(context.Background(), salesID(92), change)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Applied || saved.Amount != "366" || saved.Overspend != "66" || len(fake.writes) != 1 || fake.ifMatch[0] != "v1" {
		t.Fatalf("saved: %+v %v", saved.AdvanceReport, fake.ifMatch)
	}
	body := fake.bodies[0]
	if _, ok := body["Комментарий"]; ok {
		t.Fatalf("unchanged header fields must not be sent: %v", body)
	}
	if _, ok := body["ВыданныеАвансы"]; ok {
		t.Fatalf("unchanged sections must not be sent: %v", body)
	}
	row := body["Запасы"].([]any)[0].(map[string]any)
	if row["Партия_Key"] != salesID(77) || row["НомерВходящегоДокумента"] != "CHK-9" || row["Количество"] != json.Number("3") || row["Всего"] != json.Number("366") {
		t.Fatalf("row: %v", row)
	}
	if _, err := svc.AmendAdvanceReport(context.Background(), salesID(92), AdvanceReportAmendment{ExpectedDataVersion: "v1", Apply: true}); err == nil {
		t.Fatal("a stale data version must be refused")
	}
}

func TestAdvanceReportPostedNeedsExplicitUnpost(t *testing.T) {
	fake := storedAdvanceFixture(true)
	svc := Service{OData: fake}
	comment := "new"
	if _, err := svc.AmendAdvanceReport(context.Background(), salesID(92), AdvanceReportAmendment{Comment: &comment, Apply: true}); err == nil || len(fake.writes) != 0 {
		t.Fatalf("posted report amended: %v", err)
	}
	preview, err := svc.SetAdvanceReportPosting(context.Background(), salesID(92), "unpost", "", false)
	if err != nil || preview.Applied || len(fake.actions) != 0 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	if _, err := svc.SetAdvanceReportPosting(context.Background(), salesID(92), "delete", "", true); err == nil {
		t.Fatal("unknown action accepted")
	}
	done, err := svc.SetAdvanceReportPosting(context.Background(), salesID(92), "unpost", "v1", true)
	if err != nil || !done.Applied || done.Posted || len(fake.actions) != 1 || fake.actions[0] != "Unpost" {
		t.Fatalf("unpost: %+v %v %v", done, err, fake.actions)
	}
}

func TestAccountableBalanceAndList(t *testing.T) {
	fake := advanceFixture()
	svc := Service{OData: fake}
	balance, err := svc.GetAccountableBalance(context.Background(), "", salesID(advOther), "2026-10-05")
	if err != nil || balance.Balance != "99" || !strings.Contains(balance.State, "unreported") {
		t.Fatalf("balance: %+v %v", balance, err)
	}
	if _, err := svc.GetAccountableBalance(context.Background(), "", "", ""); err == nil {
		t.Fatal("balance without an employee must be refused")
	}
	page, err := svc.ListAdvanceReports(context.Background(), salesID(advEmployee), "2026-09-01", "2026-09-30", 20, 0)
	if err != nil || page.Total != 1 || page.Items[0].Number != "AO-OLD" {
		t.Fatalf("page: %+v %v", page, err)
	}
	if _, err := svc.ListAdvanceReports(context.Background(), "", "2026-01-01", "2026-09-30", 20, 0); err == nil {
		t.Fatal("long range accepted")
	}
}
