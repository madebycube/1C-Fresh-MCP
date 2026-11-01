package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/madebycube/1C-Fresh-MCP/internal/operations"
	"github.com/madebycube/1C-Fresh-MCP/internal/service"
)

func parseAdvanceLine(section, value string) (service.AdvanceLineInput, error) {
	usage := errors.New("line must be PRODUCT_GUID:QUANTITY:PRICE:VAT_CODE[:key=value]... with keys unit, amount, vat, characteristic, warehouse, department, business-line, supplier, doc, doc-date, content")
	parts := strings.Split(value, ":")
	if len(parts) < 4 {
		return service.AdvanceLineInput{}, usage
	}
	line := service.AdvanceLineInput{Section: section, ProductID: parts[0], Quantity: parts[1], Price: parts[2], VATRate: parts[3]}
	targets := map[string]*string{
		"unit": &line.UnitID, "amount": &line.Amount, "vat": &line.VAT, "characteristic": &line.CharacteristicID, "warehouse": &line.WarehouseID,
		"department": &line.DepartmentID, "business-line": &line.BusinessLineID, "supplier": &line.SupplierID, "doc": &line.IncomingNumber,
		"doc-date": &line.IncomingDate, "content": &line.Content,
	}
	seen := map[string]bool{}
	for _, part := range parts[4:] {
		key, option, ok := strings.Cut(part, "=")
		target, known := targets[key]
		if !ok || !known || seen[key] {
			return service.AdvanceLineInput{}, usage
		}
		seen[key] = true
		*target = option
	}
	return line, nil
}

func parseAdvanceIssue(value string) (service.AdvanceIssueInput, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return service.AdvanceIssueInput{}, errors.New("advance must be cash-out|bank-out:DOC_GUID[:AMOUNT]")
	}
	issue := service.AdvanceIssueInput{Kind: parts[0], DocumentID: parts[1]}
	if len(parts) == 3 {
		issue.Amount = parts[2]
	}
	return issue, nil
}

func parseAdvanceLineChange(value string) (service.AdvanceLineChange, error) {
	usage := errors.New("set-line must be goods|expense:N:quantity=Q,price=P,vat=CODE,amount=A,vat-amount=V")
	section, rest, ok := strings.Cut(value, ":")
	if !ok {
		return service.AdvanceLineChange{}, usage
	}
	receiptChange, err := parseLineChange(rest)
	if err != nil {
		return service.AdvanceLineChange{}, usage
	}
	return service.AdvanceLineChange{Section: section, LineNumber: receiptChange.LineNumber, Quantity: receiptChange.Quantity, Price: receiptChange.Price, VATRate: receiptChange.VATRate, Amount: receiptChange.Amount, VAT: receiptChange.VAT}, nil
}

func runAdvanceReportList(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.ListAdvanceReports
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	employee := flags.String("employee", "", "optional employee GUID")
	from := flags.String("from", "", "first date (YYYY-MM-DD)")
	to := flags.String("to", "", "last date (YYYY-MM-DD)")
	limit := flags.Int("limit", 20, "maximum results (1-100)")
	offset := flags.Int("offset", 0, "offset within matching documents")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	page, err := svc.ListAdvanceReports(ctx, *employee, *from, *to, *limit, *offset)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(page)
	}
	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "NUMBER\tDATE\tPOSTED\tAMOUNT\tEMPLOYEE ID\tID"); err != nil {
		return err
	}
	for _, item := range page.Items {
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%t\t%s\t%s\t%s\n", flat(item.Number), flat(item.Date), item.Posted, item.Amount, item.EmployeeID, item.ID); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	if page.NextOffset != nil {
		_, err = fmt.Fprintf(out, "Next offset: %d\n", *page.NextOffset)
	}
	return err
}

func runAdvanceReportGet(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetAdvanceReport
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	report, err := svc.GetAdvanceReport(ctx, flags.Arg(0))
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	if _, err := fmt.Fprintf(out, "Advance report %s %s, posted %t\nData version: %s\n", flat(report.Number), report.Date, report.Posted, report.DataVersion); err != nil {
		return err
	}
	return printAdvanceBody(out, report)
}

func printAdvanceBody(out io.Writer, report service.AdvanceReport) error {
	if _, err := fmt.Fprintf(out, "Organization ID: %s\nEmployee ID: %s\nPurpose: %s\nVAT: %s, %s\nSpent: %s, VAT %s\nIssued: %s\nRemainder: %s, overspend: %s\n", report.OrganizationID, report.EmployeeID, flat(report.Purpose), report.VATMode, report.Taxation, report.Spent, report.VAT, report.Issued, report.Remainder, report.Overspend); err != nil {
		return err
	}
	for _, lines := range [][]service.AdvanceLine{report.Goods, report.Expenses} {
		for _, line := range lines {
			place := line.WarehouseID
			if line.Section == "expense" {
				place = line.DepartmentID
			}
			if _, err := fmt.Fprintf(out, "  %s %d  %s  %s × %s  VAT %s  total %s  at %s  doc %s %s\n", line.Section, line.LineNumber, line.ProductID, line.Quantity, line.Price, line.VAT, line.Total, place, flat(line.IncomingNumber), line.IncomingDate); err != nil {
				return err
			}
		}
	}
	for _, issue := range report.Advances {
		if _, err := fmt.Fprintf(out, "  advance %d  %s %s  %s\n", issue.LineNumber, issue.Kind, issue.DocumentID, issue.Amount); err != nil {
			return err
		}
	}
	for _, payment := range report.Payments {
		if _, err := fmt.Fprintf(out, "  supplier payment %d  %s  %s\n", payment.LineNumber, payment.SupplierID, payment.Amount); err != nil {
			return err
		}
	}
	return nil
}

func runAccountableBalance(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.GetAccountableBalance
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	employee := flags.String("employee", "", "employee GUID")
	organization := flags.String("organization", "", "optional organization GUID")
	asOf := flags.String("as-of", "", "inclusive date (YYYY-MM-DD); today by default")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 || *employee == "" {
		return errors.New("usage: " + spec.Usage)
	}
	balance, err := svc.GetAccountableBalance(ctx, *organization, *employee, *asOf)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(balance)
	}
	_, err = fmt.Fprintf(out, "Employee ID: %s\nAs of: %s\nBalance: %s (%s)\nSource: %s\n", balance.EmployeeID, balance.AsOf, balance.Balance, balance.State, balance.Source)
	return err
}

func runAdvanceReportCreate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.CreateAdvanceReport
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var input service.AdvanceReportDraft
	var goods, expenses, advances repeated
	flags.StringVar(&input.OrganizationID, "organization", "", "organization GUID")
	flags.StringVar(&input.EmployeeID, "employee", "", "employee GUID")
	flags.StringVar(&input.Date, "date", "", "report date")
	flags.StringVar(&input.VATMode, "vat", "", "inclusive or exclusive prices")
	flags.StringVar(&input.WarehouseID, "warehouse", "", "default warehouse for goods")
	flags.StringVar(&input.DepartmentID, "department", "", "default department for expenses")
	flags.StringVar(&input.BusinessLineID, "business-line", "", "default business line for expenses")
	flags.StringVar(&input.Purpose, "purpose", "", "advance purpose")
	flags.StringVar(&input.Comment, "comment", "", "comment")
	flags.Var(&goods, "goods", "goods line")
	flags.Var(&expenses, "expense", "expense line")
	flags.Var(&advances, "advance", "issued advance")
	flags.BoolVar(&input.AllowDuplicate, "allow-duplicate", false, "save despite likely duplicates")
	flags.BoolVar(&input.Apply, "apply", false, "save the draft")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: " + spec.Usage)
	}
	for _, group := range []struct {
		section string
		values  repeated
	}{{"goods", goods}, {"expense", expenses}} {
		for _, value := range group.values {
			line, err := parseAdvanceLine(group.section, value)
			if err != nil {
				return err
			}
			input.Lines = append(input.Lines, line)
		}
	}
	for _, value := range advances {
		issue, err := parseAdvanceIssue(value)
		if err != nil {
			return err
		}
		input.Advances = append(input.Advances, issue)
	}
	result, err := svc.CreateAdvanceReportDraft(ctx, input)
	if err != nil {
		return err
	}
	return printAdvanceWrite(out, result, *asJSON)
}

func runAdvanceReportUpdate(ctx context.Context, svc service.Service, args []string, out io.Writer) error {
	spec := operations.UpdateAdvanceReport
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var change service.AdvanceReportAmendment
	var setLines, addGoods, addExpenses, addAdvances repeated
	text := map[string]**string{"date": &change.Date, "purpose": &change.Purpose, "comment": &change.Comment}
	values := map[string]*string{}
	for name := range text {
		values[name] = flags.String(name, "", name)
	}
	flags.Var(&setLines, "set-line", "line change")
	flags.Var(&addGoods, "add-goods", "new goods line")
	flags.Var(&addExpenses, "add-expense", "new expense line")
	flags.Var(&addAdvances, "add-advance", "new issued advance")
	flags.StringVar(&change.ExpectedDataVersion, "expect-version", "", "data version from the preview")
	flags.BoolVar(&change.AllowDuplicate, "allow-duplicate", false, "save despite lines accounted elsewhere")
	flags.BoolVar(&change.Apply, "apply", false, "save the change")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	flags.Visit(func(option *flag.Flag) {
		if target, ok := text[option.Name]; ok {
			*target = values[option.Name]
		}
	})
	for _, value := range setLines {
		lineChange, err := parseAdvanceLineChange(value)
		if err != nil {
			return err
		}
		change.ChangeLines = append(change.ChangeLines, lineChange)
	}
	for _, group := range []struct {
		section string
		values  repeated
	}{{"goods", addGoods}, {"expense", addExpenses}} {
		for _, value := range group.values {
			line, err := parseAdvanceLine(group.section, value)
			if err != nil {
				return err
			}
			change.AddLines = append(change.AddLines, line)
		}
	}
	for _, value := range addAdvances {
		issue, err := parseAdvanceIssue(value)
		if err != nil {
			return err
		}
		change.AddAdvances = append(change.AddAdvances, issue)
	}
	result, err := svc.AmendAdvanceReport(ctx, flags.Arg(0), change)
	if err != nil {
		return err
	}
	return printAdvanceWrite(out, result, *asJSON)
}

func printAdvanceWrite(out io.Writer, result service.AdvanceReportWrite, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview (nothing saved)"
	if result.Applied {
		state = "Saved unposted advance report " + flat(result.Number) + " " + result.ID
	}
	if _, err := fmt.Fprintf(out, "%s\nDate: %s\n", state, result.Date); err != nil {
		return err
	}
	if result.Before != nil {
		if _, err := fmt.Fprintf(out, "Before: total %s, VAT %s, %d lines\nChanges: %s\nData version: %s\n", result.Before.Amount, result.Before.VAT, result.Before.Lines, strings.Join(result.Changes, ", "), result.DataVersion); err != nil {
			return err
		}
	}
	if err := printAdvanceBody(out, result.AdvanceReport); err != nil {
		return err
	}
	if result.EmployeeBalance != nil {
		if _, err := fmt.Fprintf(out, "Employee balance before this report: %s (%s)\n", result.EmployeeBalance.Balance, result.EmployeeBalance.State); err != nil {
			return err
		}
	}
	for _, duplicate := range result.Duplicates {
		if _, err := fmt.Fprintf(out, "Possible duplicate: %s %s posted %t amount %s (%s)\n", flat(duplicate.Number), duplicate.Date, duplicate.Posted, duplicate.Amount, duplicate.ID); err != nil {
			return err
		}
	}
	for _, conflict := range result.AccountedElsewhere {
		if _, err := fmt.Fprintf(out, "Already accounted: %s line %d in %s %s %s (%s): %s\n", conflict.Section, conflict.LineNumber, conflict.Document, flat(conflict.Number), conflict.Date, conflict.ID, conflict.Reason); err != nil {
			return err
		}
	}
	for _, warning := range result.Warnings {
		if _, err := fmt.Fprintln(out, "Warning: "+warning); err != nil {
			return err
		}
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied {
		_, err := fmt.Fprintln(out, "Run again with --apply to save.")
		return err
	}
	return nil
}

func runAdvanceReportPosting(ctx context.Context, svc service.Service, args []string, out io.Writer, action string) error {
	spec := operations.PostAdvanceReport
	if action == "unpost" {
		spec = operations.UnpostAdvanceReport
	}
	flags := flag.NewFlagSet(spec.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	expected := flags.String("expect-version", "", "data version from the preview")
	apply := flags.Bool("apply", false, "run the action")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := parseFlags(flags, args); err != nil || flags.NArg() != 1 {
		return errors.New("usage: " + spec.Usage)
	}
	result, err := svc.SetAdvanceReportPosting(ctx, flags.Arg(0), action, *expected, *apply)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	state := "Preview"
	if result.Applied {
		state = "Done"
	}
	if _, err := fmt.Fprintf(out, "%s: %s advance report %s %s\nPosted: %t -> %t\nSpent: %s, issued: %s, remainder: %s, overspend: %s\nData version: %s\n", state, action, flat(result.Number), result.ID, result.PostedFrom, result.Posted, result.Amount, result.Issued, result.Remainder, result.Overspend, result.DataVersion); err != nil {
		return err
	}
	for _, impact := range result.Impact {
		if _, err := fmt.Fprintln(out, "Impact: "+impact); err != nil {
			return err
		}
	}
	if !result.Applied && result.PostedFrom != (action == "post") {
		_, err := fmt.Fprintln(out, "Run again with --apply to "+action+".")
		return err
	}
	return nil
}
