package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

const statementAccountNo = "40702810000000000001"
const statementPeerINN = "7700000001"

var statementScope = BankStatementScope{AccountNumber: statementAccountNo, From: "2026-11-18", To: "2026-11-18"}

const statementHeader = `1CClientBankExchange
ВерсияФормата=1.03
Кодировка=Windows
ДатаНачала=18.11.2026
ДатаКонца=18.11.2026
РасчСчет=` + statementAccountNo + `
СекцияРасчСчет
ДатаНачала=18.11.2026
РасчСчет=` + statementAccountNo + `
НачальныйОстаток=100.20
ВсегоПоступило=0.00
ВсегоСписано=40.10
КонечныйОстаток=60.10
КонецРасчСчет
`

const statementPayment = `СекцияДокумент=Платежное поручение
Номер=42
Дата=17.11.2026
ДатаСписано=18.11.2026
Сумма=40.10
ПлательщикСчет=` + statementAccountNo + `
ПолучательСчет=40702810000000000002
ПолучательИНН=` + statementPeerINN + `
НазначениеПлатежа=Оплата по счёту = 42
КонецДокумента
`
const statementContents = statementHeader + statementPayment + "КонецФайла\n"

func TestParseBankStatement(t *testing.T) {
	windows, err := charmap.Windows1251.NewEncoder().String(strings.ReplaceAll(statementContents, "\n", "\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	dos, err := charmap.CodePage866.NewEncoder().String(strings.Replace(statementContents, "Кодировка=Windows", "Кодировка=DOS", 1))
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, input := range []string{"\ufeff" + statementContents, windows, dos} {
		got, err := ParseBankStatement(strings.NewReader(input), statementScope)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Entries) != 1 || len(got.CheckedTotals) != 3 || got.Opening != "100.2" || got.Incoming != "0" || got.Outgoing != "40.1" || got.Closing != "60.1" {
			t.Fatalf("statement: %+v", got)
		}
		entry := got.Entries[0]
		if entry.Kind != "bank-out" || entry.Purpose != "Оплата по счёту = 42" || entry.DocumentDate != "2026-11-17" || entry.BookedDate != statementScope.From || entry.CounterpartyINN != statementPeerINN {
			t.Fatalf("entry: %+v", entry)
		}
		if id != "" && entry.ID != id {
			t.Fatal("statement identity changed across encodings")
		}
		id = entry.ID
	}
}

func TestParseBankStatementRejectsInvalidFile(t *testing.T) {
	for _, test := range []struct{ name, input, want string }{
		{"duplicate despite amount change", statementHeader + statementPayment + strings.Replace(statementPayment, "Сумма=40.10", "Сумма=30.00", 1) + "КонецФайла", "duplicate"},
		{"bad account", strings.Replace(statementContents, "РасчСчет="+statementAccountNo, "РасчСчет=40702810000000000002", 1), "account"},
		{"bad period", strings.Replace(statementContents, "ДатаКонца=18", "ДатаКонца=19", 1), "period"},
		{"bad booking", strings.Replace(statementContents, "ДатаСписано=18", "ДатаСписано=19", 1), "booking date"},
		{"bad debit total", strings.Replace(statementContents, "ВсегоСписано=40.10", "ВсегоСписано=41.10", 1), "outgoing total"},
		{"bad closing", strings.Replace(statementContents, "КонечныйОстаток=60.10", "КонечныйОстаток=61.10", 1), "closing total"},
		{"excess precision", strings.Replace(statementContents, "Сумма=40.10", "Сумма=40.101", 1), "amount"},
		{"truncation", strings.TrimSuffix(statementContents, "КонецФайла\n"), "incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseBankStatement(strings.NewReader(test.input), statementScope)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

type statementWriter struct {
	*docWriter
	movementAmount string
	lostResponse   bool
}

func (w *statementWriter) Action(ctx context.Context, resource, action string) ([]byte, error) {
	response, err := w.docWriter.Action(ctx, resource, action)
	if w.lostResponse {
		return nil, errors.New("connection failed")
	}
	return response, err
}

func (w *statementWriter) GetRecordSet(_ context.Context, register, _, _ string, _ int64) ([]byte, bool, error) {
	if register != "AccumulationRegister_ДенежныеСредства" {
		return nil, false, nil
	}
	body, err := json.Marshal(map[string]any{"RecordSet": []map[string]any{{
		"БанковскийСчетКасса_Type": "StandardODATA.Catalog_БанковскиеСчета", "БанковскийСчетКасса": salesID(9),
		"Валюта_Key": salesID(80), "Active": true, "RecordType": "Expense", "Period": "2026-11-18T10:00:00",
		"Сумма": json.Number(w.movementAmount),
	}}})
	return body, true, err
}

const statementBankExpense = "Document_РасходСоСчета"

func newStatementWriter() *statementWriter {
	return &statementWriter{
		movementAmount: "40.1",
		docWriter: &docWriter{traceReader: &traceReader{rows: map[string][]map[string]any{
			"Catalog_БанковскиеСчета": {{
				"Ref_Key": salesID(9), "Owner": salesID(90), "Owner_Type": bankAccountOwnerType, "DeletionMark": false,
				"НомерСчета": statementAccountNo, "Недействителен": false,
			}},
			"Catalog_Валюты":      {{"Ref_Key": salesID(80), "Code": "643", "DeletionMark": false}},
			"Catalog_Контрагенты": {{"Ref_Key": salesID(60), "ИНН": statementPeerINN, "IsFolder": false, "DeletionMark": false, "Недействителен": false}},
			"Catalog_ДоговорыКонтрагентов": {{
				"Ref_Key": salesID(61), "Owner": salesID(60), "Организация_Key": salesID(90), "ВалютаРасчетов_Key": salesID(80),
				"DeletionMark": false, "Недействителен": false, "РасчетыВУсловныхЕдиницах": false,
			}},
			"Document_ПриходнаяНакладная": {{
				"Ref_Key": salesID(63), "Организация_Key": salesID(90), "Контрагент_Key": salesID(60), "Договор_Key": salesID(61),
				"DeletionMark": false, "Posted": true,
			}},
			statementBankExpense: {{
				"Ref_Key": salesID(50), "Number": "B-42", "Date": "2026-11-18T10:00:00", "Posted": false, "DeletionMark": false, "DataVersion": "v1",
				"СуммаДокумента": 40.1, "ВидОперации": "Поставщику", "Организация_Key": salesID(90), "БанковскийСчет_Key": salesID(9),
				"Контрагент_Key": salesID(60), "ВалютаДенежныхСредств_Key": salesID(80),
				"РасшифровкаПлатежа": []map[string]any{{
					"LineNumber": "1", "Договор_Key": salesID(61), "СтатьяДвиженияДенежныхСредств_Key": salesID(62),
					"СуммаПлатежа": 40.1, "СуммаРасчетов": 40.1, "Документ": salesID(63), "Документ_Type": receiptDocumentType,
				}},
			}},
		}}},
	}
}

func TestReviewBankStatementReportsCandidatesAndAmbiguity(t *testing.T) {
	fake := newStatementWriter()
	rows := fake.rows[statementBankExpense]
	for index, operation := range []string{"Поставщику", "Зарплата", "Поставщику"} {
		additional := map[string]any{}
		for key, value := range rows[0] {
			additional[key] = value
		}
		additional["Ref_Key"], additional["ВидОперации"] = salesID(51+index), operation
		if index == 2 {
			additional["БанковскийСчет_Key"] = salesID(10)
		}
		fake.rows[statementBankExpense] = append(fake.rows[statementBankExpense], additional)
	}
	svc := Service{OData: fake}
	report, err := svc.ReviewBankStatement(context.Background(), strings.NewReader(statementContents), statementScope, salesID(9))
	if err != nil || len(report.Rows) != 1 || report.Rows[0].Status != "ambiguous" || report.Rows[0].CandidateCount != 2 || report.Counts["ambiguous"] != 1 {
		t.Fatalf("report = %+v, err = %v", report, err)
	}
	if report.Rows[0].Candidates[1].ID != salesID(51) {
		t.Fatalf("candidates = %+v", report.Rows[0].Candidates)
	}
}

func TestPostBankStatementPaymentPreviewAndVerify(t *testing.T) {
	ctx := context.Background()
	fake := newStatementWriter()
	svc := Service{OData: fake}
	request := bankPostingRequest(t)
	post := func() (BankStatementPosting, error) {
		return svc.PostBankStatementPayment(ctx, strings.NewReader(statementContents), request)
	}
	preview, err := post()
	if err != nil || preview.Status != "ready" || preview.DataVersion != "v1" || len(fake.actions) != 0 {
		t.Fatalf("preview = %+v, err = %v, actions = %v", preview, err, fake.actions)
	}
	request.Apply, request.ExpectedDataVersion = true, preview.DataVersion
	result, err := post()
	if err != nil || result.Status != "verified" || result.Movements == nil || len(fake.actions) != 1 || fake.actions[0] != "Post "+statementBankExpense {
		t.Fatalf("result = %+v, err = %v, actions = %v", result, err, fake.actions)
	}
	fake.movementAmount = "40.2"
	result, err = post()
	if err != nil || result.Status != "verification_failed" || result.Error == "" || len(fake.actions) != 1 {
		t.Fatalf("verification = %+v, err = %v, actions = %v", result, err, fake.actions)
	}
}

func TestPostBankStatementPaymentReadsBackAfterLostResponse(t *testing.T) {
	fake := newStatementWriter()
	fake.lostResponse = true
	request := bankPostingRequest(t)
	request.Apply, request.ExpectedDataVersion = true, "v1"
	result, err := (Service{OData: fake}).PostBankStatementPayment(context.Background(), strings.NewReader(statementContents), request)
	if err != nil || result.Status != "verified" || len(fake.actions) != 1 {
		t.Fatalf("result = %+v, err = %v, actions = %v", result, err, fake.actions)
	}
}

func TestPostBankStatementPaymentChecksBeforeWriting(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*statementWriter, *BankStatementPostingRequest)
	}{
		{"stale version", func(_ *statementWriter, req *BankStatementPostingRequest) { req.ExpectedDataVersion = "stale" }},
		{"missing version", func(_ *statementWriter, req *BankStatementPostingRequest) { req.ExpectedDataVersion = "" }},
		{"wrong party", func(w *statementWriter, _ *BankStatementPostingRequest) {
			w.rows["Catalog_Контрагенты"][0]["ИНН"] = "1234567890"
		}},
		{"wrong basis", func(w *statementWriter, _ *BankStatementPostingRequest) {
			w.rows["Document_ПриходнаяНакладная"][0]["Контрагент_Key"] = salesID(64)
		}},
		{"fee unsupported", func(w *statementWriter, _ *BankStatementPostingRequest) {
			w.rows[statementBankExpense][0]["ВидОперации"] = "КомиссияБанка"
		}},
		{"excluded operation", func(w *statementWriter, _ *BankStatementPostingRequest) {
			w.rows[statementBankExpense][0]["ВидОперации"] = "Зарплата"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newStatementWriter()
			request := bankPostingRequest(t)
			request.Apply, request.ExpectedDataVersion = true, "v1"
			test.change(fake, &request)
			_, err := (Service{OData: fake}).PostBankStatementPayment(context.Background(), strings.NewReader(statementContents), request)
			if err == nil || len(fake.actions) != 0 || len(fake.writes) != 0 {
				t.Fatalf("err = %v, actions = %v, writes = %v", err, fake.actions, fake.writes)
			}
		})
	}
}

func bankPostingRequest(t *testing.T) BankStatementPostingRequest {
	t.Helper()
	statement, err := ParseBankStatement(strings.NewReader(statementContents), statementScope)
	if err != nil {
		t.Fatal(err)
	}
	return BankStatementPostingRequest{BankStatementScope: statementScope, AccountID: salesID(9), EntryID: statement.Entries[0].ID, DocumentID: salesID(50)}
}
