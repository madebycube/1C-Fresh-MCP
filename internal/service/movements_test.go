package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

const movementTestID = "00000000-0000-0000-0000-000000000030"

type movementStub struct {
	doc   string
	reads *[]string
}

func (movementStub) Check(context.Context) (int, error) { return 1, nil }

func (m movementStub) Get(_ context.Context, resource string, _ url.Values, _ int64) ([]byte, error) {
	if strings.Contains(resource, "(guid'") {
		return []byte(m.doc), nil
	}
	return []byte(`{"odata.count":"0","value":[]}`), nil
}

func (m movementStub) GetRecordSet(_ context.Context, register, document, id string, _ int64) ([]byte, bool, error) {
	*m.reads = append(*m.reads, register+" "+document+" "+id)
	switch register {
	case "AccumulationRegister_ЗапасыНаСкладах":
		return []byte(`{"Recorder":"` + id + `","RecordSet":[{"Recorder":"` + id + `","Recorder_Type":"x","LineNumber":"1","RecordType":"Expense","Количество":3.5,"Номенклатура_Key":"00000000-0000-0000-0000-000000000004","Nested":{"a":1}}]}`), true, nil
	case "AccumulationRegister_Продажи":
		return []byte(`{"RecordSet":[]}`), true, nil
	}
	return nil, false, nil
}

func TestGetDocumentMovementsReadsAllowedRegisters(t *testing.T) {
	reads := []string{}
	svc := Service{OData: movementStub{reads: &reads, doc: `{"Ref_Key":"` + movementTestID + `","Number":"T1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"ВидОперации":"Перемещение","СтруктурнаяЕдиница_Key":"00000000-0000-0000-0000-000000000006","Запасы":[]}`}}
	result, err := svc.GetDocumentMovements(context.Background(), "transfer", movementTestID)
	if err != nil || result.Document != "Document_ПеремещениеЗапасов" || len(result.Registers) != 1 || len(reads) != len(movementRegisters) {
		t.Fatalf("movements: %+v, %v, %v", result, err, reads)
	}
	row := result.Registers[0].Rows[0]
	if _, ok := row["Recorder"]; ok || row["RecordType"] != "Expense" || row["Nested"] != nil || len(row) != 4 {
		t.Fatalf("row: %+v", row)
	}
	if !strings.HasSuffix(reads[0], " Document_ПеремещениеЗапасов "+movementTestID) {
		t.Fatalf("reads: %v", reads)
	}
}

func TestGetDocumentMovementsRefusesBeforeReadingRegisters(t *testing.T) {
	reads := []string{}
	svc := Service{OData: movementStub{reads: &reads, doc: `{"Ref_Key":"` + movementTestID + `","Number":"C1","Date":"2026-09-23T00:00:00","Posted":true,"DeletionMark":false,"ВидОперации":"Зарплата","СуммаДокумента":10}`}}
	for _, call := range [][2]string{{"payroll", movementTestID}, {"transfer", "bad"}, {"cash-out", movementTestID}} {
		if _, err := svc.GetDocumentMovements(context.Background(), call[0], call[1]); err == nil {
			t.Fatalf("expected error for %v", call)
		}
	}
	if len(reads) != 0 {
		t.Fatalf("refused documents read registers: %v", reads)
	}
}
