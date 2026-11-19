package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const MaxBankStatementSize = 8 << 20

type BankStatementScope struct {
	AccountNumber string `json:"account_number"`
	From          string `json:"from"`
	To            string `json:"to"`
}

type BankStatementEntry struct {
	ID              string  `json:"id"`
	Line            int     `json:"line"`
	DocumentType    string  `json:"document_type"`
	Number          string  `json:"number"`
	DocumentDate    string  `json:"document_date"`
	BookedDate      string  `json:"booked_date"`
	Kind            string  `json:"kind"`
	Amount          Decimal `json:"amount"`
	CounterpartyINN string  `json:"counterparty_inn,omitempty"`
	Purpose         string  `json:"purpose,omitempty"`
}

type BankStatement struct {
	BankStatementScope
	Version       string               `json:"version"`
	Opening       Decimal              `json:"opening"`
	Incoming      Decimal              `json:"incoming"`
	Outgoing      Decimal              `json:"outgoing"`
	Closing       Decimal              `json:"closing"`
	CheckedTotals []string             `json:"checked_totals"`
	Entries       []BankStatementEntry `json:"entries"`
}

func ParseBankStatement(input io.Reader, scope BankStatementScope) (BankStatement, error) {
	if !bankAccountNumber(scope.AccountNumber) {
		return BankStatement{}, errors.New("account number must contain exactly 20 digits")
	}
	if _, _, err := documentDateRange(scope.From, scope.To); err != nil {
		return BankStatement{}, err
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxBankStatementSize+1))
	if err != nil {
		return BankStatement{}, fmt.Errorf("read statement: %w", err)
	}
	if len(data) > MaxBankStatementSize {
		return BankStatement{}, errors.New("statement exceeds 8 MiB")
	}
	data, err = decodeBankStatement(data)
	if err != nil {
		return BankStatement{}, err
	}

	result := BankStatement{BankStatementScope: scope, CheckedTotals: []string{}, Entries: []BankStatementEntry{}}
	header := map[string]string{}
	var account, section map[string]string
	var documentType string
	var accountClosed, finished bool
	seen := map[string]int{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNumber, documentLine := 0, 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if lineNumber == 1 {
			if line != "1CClientBankExchange" {
				return BankStatement{}, errors.New("missing 1CClientBankExchange signature")
			}
			continue
		}
		if line == "" {
			continue
		}
		key, value, hasValue := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		current := section
		if current == nil && account == nil {
			current = header
		}
		if current == nil && !accountClosed {
			current = account
		}
		switch {
		case finished:
			return BankStatement{}, fmt.Errorf("line %d: content after КонецФайла", lineNumber)
		case !hasValue && key == "СекцияРасчСчет" && account == nil && section == nil:
			account = map[string]string{}
		case !hasValue && key == "КонецРасчСчет" && account != nil && !accountClosed:
			accountClosed = true
		case hasValue && key == "СекцияДокумент" && accountClosed && section == nil && value != "":
			section, documentType, documentLine = map[string]string{}, value, lineNumber
		case !hasValue && key == "КонецДокумента" && section != nil:
			entry, err := bankStatementEntry(scope, documentType, section, documentLine)
			if err != nil {
				return BankStatement{}, fmt.Errorf("line %d: %w", documentLine, err)
			}
			if previous, ok := seen[entry.ID]; ok {
				return BankStatement{}, fmt.Errorf("lines %d and %d: duplicate bank document", previous, documentLine)
			}
			if len(result.Entries) == 5000 {
				return BankStatement{}, errors.New("statement exceeds 5000 documents")
			}
			seen[entry.ID] = documentLine
			result.Entries = append(result.Entries, entry)
			section = nil
		case !hasValue && key == "КонецФайла" && accountClosed && section == nil:
			finished = true
		case key == "СекцияРасчСчет" || key == "КонецРасчСчет" || key == "СекцияДокумент" || key == "КонецДокумента" || key == "КонецФайла":
			return BankStatement{}, fmt.Errorf("line %d: unexpected section marker", lineNumber)
		case hasValue && current != nil && key != "":
			if _, exists := current[key]; exists && !(account == nil && key == "Документ") {
				return BankStatement{}, fmt.Errorf("line %d: repeated field %s", lineNumber, key)
			}
			current[key] = value
		default:
			return BankStatement{}, fmt.Errorf("line %d: unexpected statement field or section", lineNumber)
		}
	}
	if err := scanner.Err(); err != nil {
		return BankStatement{}, fmt.Errorf("read statement line %d: %w", lineNumber+1, err)
	}
	if !finished {
		return BankStatement{}, errors.New("incomplete statement: missing account section or КонецФайла")
	}
	result.Version = header["ВерсияФормата"]
	switch result.Version {
	case "1.01", "1.02", "1.03":
	default:
		return BankStatement{}, errors.New("unsupported statement version; expected 1.01, 1.02 or 1.03")
	}
	switch strings.ToLower(header["Кодировка"]) {
	case "windows", "dos", "utf-8", "utf8":
	default:
		return BankStatement{}, errors.New("unsupported or missing statement encoding")
	}
	if header["РасчСчет"] != scope.AccountNumber || account["РасчСчет"] != scope.AccountNumber {
		return BankStatement{}, errors.New("statement account does not match the requested account")
	}
	for _, bound := range []struct{ key, expected string }{{"ДатаНачала", scope.From}, {"ДатаКонца", scope.To}} {
		if bankStatementDate(header[bound.key]) != bound.expected ||
			(account[bound.key] != "" || bound.key == "ДатаНачала") && bankStatementDate(account[bound.key]) != bound.expected {
			return BankStatement{}, errors.New("statement period does not match the requested period")
		}
	}
	if err := result.checkTotals(account); err != nil {
		return BankStatement{}, err
	}
	return result, nil
}

func decodeBankStatement(data []byte) ([]byte, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	if !utf8.Valid(data) {
		decoded, err := decodeLegacyBankStatement(data)
		if err != nil {
			return nil, err
		}
		data = decoded
	}
	for _, char := range string(data) {
		if char == utf8.RuneError || unicode.IsControl(char) && char != '\n' && char != '\r' && char != '\t' {
			return nil, errors.New("statement contains invalid characters")
		}
	}
	return data, nil
}

func decodeLegacyBankStatement(data []byte) ([]byte, error) {
	for _, encoding := range []struct {
		header  string
		charmap *charmap.Charmap
	}{{"Windows", charmap.Windows1251}, {"DOS", charmap.CodePage866}} {
		decoded, err := encoding.charmap.NewDecoder().Bytes(data)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(decoded), "\n") {
			key, value, _ := strings.Cut(line, "=")
			if strings.TrimSpace(key) == "Кодировка" && strings.EqualFold(strings.TrimSpace(value), encoding.header) {
				return decoded, nil
			}
		}
	}
	return nil, errors.New("could not decode statement as UTF-8, Windows-1251, or CP866")
}

func bankAccountNumber(value string) bool {
	return len(value) == 20 && strings.Trim(value, "0123456789") == ""
}

func bankStatementDate(value string) string {
	date, err := time.Parse("02.01.2006", value)
	if err != nil || date.Format("02.01.2006") != value {
		return ""
	}
	return date.Format("2006-01-02")
}

func bankStatementEntry(scope BankStatementScope, documentType string, fields map[string]string, line int) (BankStatementEntry, error) {
	payer, recipient := fields["ПлательщикСчет"], fields["ПолучательСчет"]
	incoming, outgoing := recipient == scope.AccountNumber, payer == scope.AccountNumber
	if incoming == outgoing {
		return BankStatementEntry{}, errors.New("exactly one side of the payment must be the statement account")
	}
	kind, dateField, partner, partnerSide := "bank-in", "ДатаПоступило", payer, "Плательщик"
	if outgoing {
		kind, dateField, partner, partnerSide = "bank-out", "ДатаСписано", recipient, "Получатель"
	}
	date, booked := bankStatementDate(fields["Дата"]), bankStatementDate(fields[dateField])
	if date == "" || booked == "" || date > booked || booked < scope.From || booked > scope.To {
		return BankStatementEntry{}, errors.New("invalid document or booking date, or booking date outside the statement period")
	}
	number := fields["Номер"]
	if number == "" || len(number) > 120 || len(documentType) > 250 {
		return BankStatementEntry{}, errors.New("invalid document number or type")
	}
	amount, err := exactDecimal(fields["Сумма"], 2, false)
	if err != nil {
		return BankStatementEntry{}, errors.New("invalid payment amount")
	}
	if partner == "" {
		partner = fields[partnerSide+"ИНН"]
	}
	// Number/date can repeat across unrelated payers and distinct bookings.
	identity := strings.Join([]string{scope.AccountNumber, kind, documentType, number, date, booked, partner}, "\x00")
	return BankStatementEntry{
		ID: fmt.Sprintf("%x", sha256.Sum256([]byte(identity))), Line: line, DocumentType: documentType, Number: number,
		DocumentDate: date, BookedDate: booked, Kind: kind, Amount: ratDecimal(amount, 2),
		CounterpartyINN: fields[partnerSide+"ИНН"], Purpose: fields["НазначениеПлатежа"],
	}, nil
}

func (s *BankStatement) checkTotals(fields map[string]string) error {
	incoming, outgoing := new(big.Rat), new(big.Rat)
	for _, entry := range s.Entries {
		amount, _ := new(big.Rat).SetString(string(entry.Amount))
		if entry.Kind == "bank-in" {
			incoming.Add(incoming, amount)
		} else {
			outgoing.Add(outgoing, amount)
		}
	}
	opening, err := bankStatementBalance(fields["НачальныйОстаток"])
	if err != nil {
		return errors.New("invalid or missing opening balance")
	}
	closing := new(big.Rat).Sub(new(big.Rat).Add(opening, incoming), outgoing)
	for _, control := range []struct {
		field, name string
		total       *big.Rat
	}{{"ВсегоПоступило", "incoming", incoming}, {"ВсегоСписано", "outgoing", outgoing}, {"КонечныйОстаток", "closing", closing}} {
		if value, exists := fields[control.field]; exists {
			declared, err := bankStatementBalance(value)
			if err != nil || declared.Cmp(control.total) != 0 {
				return fmt.Errorf("statement %s total does not match its documents", control.name)
			}
			s.CheckedTotals = append(s.CheckedTotals, control.name)
		}
	}
	s.Opening, s.Incoming = ratDecimal(opening, 2), ratDecimal(incoming, 2)
	s.Outgoing, s.Closing = ratDecimal(outgoing, 2), ratDecimal(closing, 2)
	return nil
}

func bankStatementBalance(value string) (*big.Rat, error) {
	text, negative := strings.CutPrefix(value, "-")
	amount, err := exactDecimal(text, 2, true)
	if err == nil && negative {
		amount.Neg(amount)
	}
	return amount, err
}
