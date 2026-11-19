package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func TestCheckBankStatementCommandRunsOffline(t *testing.T) {
	const account = "40702810000000000001"
	path := filepath.Join(t.TempDir(), "statement.txt")
	t.Setenv("ONEC_ENV_FILE", filepath.Join(t.TempDir(), "missing.env"))
	const content = `1CClientBankExchange
ВерсияФормата=1.03
Кодировка=UTF-8
ДатаНачала=18.11.2026
ДатаКонца=18.11.2026
РасчСчет=` + account + `
СекцияРасчСчет
ДатаНачала=18.11.2026
РасчСчет=` + account + `
НачальныйОстаток=25.10
КонечныйОстаток=25.10
КонецРасчСчет
КонецФайла
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := []string{"check", "bank-statement", "--file", path, "--number", account, "--from", "2026-11-18", "--to", "2026-11-18", "--json"}
	if err := run(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var result service.BankStatement
	if err := json.NewDecoder(&output).Decode(&result); err != nil || result.AccountNumber != account || result.Opening != "25.1" ||
		result.Closing != "25.1" || len(result.Entries) != 0 || len(result.CheckedTotals) != 1 || result.CheckedTotals[0] != "closing" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}
