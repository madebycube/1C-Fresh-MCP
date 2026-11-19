package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func runBankStatement(ctx context.Context, svc service.Service, spec operations.Spec, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var scope service.BankStatementScope
	path := flags.String("file", "", "1CClientBankExchange statement file")
	flags.StringVar(&scope.AccountNumber, "number", "", "expected 20-digit bank account number")
	flags.StringVar(&scope.From, "from", "", "first statement day")
	flags.StringVar(&scope.To, "to", "", "last statement day")
	asJSON := flags.Bool("json", false, "print JSON")
	var account, journal string
	var request service.BankStatementPostingRequest
	if spec.Command != operations.CheckBankStatement.Command {
		flags.StringVar(&account, "account", "", "1C bank account GUID")
	}
	if spec.Command == operations.PostBankStatementPayment.Command {
		flags.StringVar(&request.EntryID, "entry", "", "statement entry ID")
		flags.StringVar(&request.DocumentID, "document", "", "reviewed 1C bank document GUID")
		flags.StringVar(&request.ExpectedDataVersion, "expect-version", "", "version from the posting preview")
		flags.BoolVar(&request.Apply, "apply", false, "post the reviewed payment")
		flags.StringVar(&journal, "journal", "", "append action records to this JSON Lines file")
	}
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *path == "" || scope.AccountNumber == "" ||
		scope.From == "" || scope.To == "" || spec.Command != operations.CheckBankStatement.Command && account == "" {
		return errors.New("usage: " + spec.Usage)
	}
	file, err := os.Open(*path)
	if err != nil {
		return fmt.Errorf("open statement: %w", err)
	}
	defer file.Close()
	switch spec.Command {
	case operations.CheckBankStatement.Command:
		statement, err := service.ParseBankStatement(file, scope)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(statement)
		}
		return printBankStatement(out, statement, nil)
	case operations.ReviewBankStatement.Command:
		review, err := svc.ReviewBankStatement(ctx, file, scope, account)
		if err != nil {
			return err
		}
		if *asJSON {
			return json.NewEncoder(out).Encode(review)
		}
		return printBankStatement(out, review.Statement, review.Rows)
	case operations.PostBankStatementPayment.Command:
		if request.EntryID == "" || request.DocumentID == "" {
			return errors.New("usage: " + spec.Usage)
		}
		request.BankStatementScope, request.AccountID = scope, account
		return runBankStatementPosting(ctx, svc, file, request, journal, *asJSON, out)
	}
	return errors.New("unknown bank statement command")
}

func printBankStatement(out io.Writer, statement service.BankStatement, rows []service.BankStatementReviewRow) error {
	totals := strings.Join(statement.CheckedTotals, ", ")
	if totals == "" {
		totals = "none supplied"
	}
	if _, err := fmt.Fprintf(out, "%s to %s | %d documents | opening %s | in %s | out %s | closing %s\nBank control totals checked: %s\n",
		statement.From, statement.To, len(statement.Entries), statement.Opening, statement.Incoming, statement.Outgoing, statement.Closing, totals); err != nil {
		return err
	}
	for index, entry := range statement.Entries {
		status := ""
		if rows != nil {
			status = fmt.Sprintf(" %s (%d)", rows[index].Status, rows[index].CandidateCount)
		}
		if _, err := fmt.Fprintf(out, "%s %s %s %s [%s]%s\n", entry.BookedDate, entry.Kind, entry.Amount, flat(entry.Number), entry.ID, status); err != nil {
			return err
		}
		if rows == nil {
			continue
		}
		for _, candidate := range rows[index].Candidates {
			if _, err := fmt.Fprintf(out, "  %s %s %s posted=%t\n", candidate.ID, flat(candidate.Number), flat(candidate.Operation), candidate.Posted); err != nil {
				return err
			}
		}
	}
	return nil
}

func runBankStatementPosting(ctx context.Context, svc service.Service, file *os.File, request service.BankStatementPostingRequest, journalPath string, asJSON bool, out io.Writer) error {
	var journal *os.File
	if journalPath != "" {
		var err error
		journal, err = os.OpenFile(journalPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return fmt.Errorf("open posting journal: %w", err)
		}
		defer journal.Close()
		statementInfo, inputErr := file.Stat()
		journalInfo, outputErr := journal.Stat()
		if inputErr != nil || outputErr != nil || !journalInfo.Mode().IsRegular() || os.SameFile(statementInfo, journalInfo) {
			return errors.New("posting journal must be a separate regular file")
		}
		if err := writeBankJournal(journal, request, "started"); err != nil {
			return err
		}
	}
	result, err := svc.PostBankStatementPayment(ctx, file, request)
	if err != nil {
		if journal != nil {
			return errors.Join(err, writeBankJournal(journal, request, "rejected"))
		}
		return err
	}
	if journal != nil {
		err = writeBankJournal(journal, request, result.Status)
	}
	var reportErr error
	if asJSON {
		reportErr = json.NewEncoder(out).Encode(result)
	} else {
		_, reportErr = fmt.Fprintf(out, "%s | %s | %s | data version %s\n", result.Status, result.Document.ID, result.Entry.Amount, result.DataVersion)
		if reportErr == nil && result.Error != "" {
			_, reportErr = fmt.Fprintln(out, flat(result.Error))
		}
	}
	if result.Error != "" {
		return errors.Join(err, reportErr, errors.New(result.Status+"; see the posting report"))
	}
	return errors.Join(err, reportErr)
}

func writeBankJournal(journal *os.File, request service.BankStatementPostingRequest, status string) error {
	record := struct {
		Time       string `json:"time"`
		EntryID    string `json:"entry_id"`
		DocumentID string `json:"document_id"`
		Apply      bool   `json:"apply"`
		Status     string `json:"status"`
	}{time.Now().UTC().Format(time.RFC3339Nano), request.EntryID, request.DocumentID, request.Apply, status}
	if err := json.NewEncoder(journal).Encode(record); err != nil {
		return fmt.Errorf("write posting journal: %w", err)
	}
	return journal.Sync()
}
