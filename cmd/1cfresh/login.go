package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
	"github.com/madebycube/1C-Fresh-MCP/internal/odata"
	"golang.org/x/term"
)

func runLogin(ctx context.Context, args []string, out io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: 1c login")
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("login needs an interactive terminal")
	}
	return login(ctx, config.EnvFilePath(), os.Stdin, out, func() (string, error) {
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(password), err
	}, func(ctx context.Context, cfg config.Config) error {
		_, err := odata.New(cfg).Check(ctx)
		return err
	})
}

func login(ctx context.Context, path string, in io.Reader, out io.Writer, readPassword func() (string, error), check func(context.Context, config.Config) error) error {
	previous, err := config.ReadEnvFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read environment file: %w", err)
	}
	reader := bufio.NewReader(in)
	base, err := prompt(reader, out, "1C Link/Ссылка на 1C", previous["ONEC_ODATA_BASE_URL"])
	if err != nil {
		return err
	}
	username, err := prompt(reader, out, "User/Юзер", previous["ONEC_ODATA_USERNAME"])
	if err != nil {
		return err
	}
	passwordLabel := "Password/Пароль"
	if previous["ONEC_ODATA_PASSWORD"] != "" {
		passwordLabel += " [enter to keep saved]"
	}
	if _, err := fmt.Fprintf(out, "%s: ", passwordLabel); err != nil {
		return err
	}
	password, err := readPassword()
	if _, writeErr := fmt.Fprintln(out); writeErr != nil {
		return writeErr
	}
	if err != nil {
		return errors.New("could not read password")
	}
	if password == "" {
		password = previous["ONEC_ODATA_PASSWORD"]
	}
	cfg, err := config.New(base, username, password)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Checking OData access..."); err != nil {
		return err
	}
	if err := check(ctx, cfg); err != nil {
		return fmt.Errorf("login failed; .env was not changed: %w", err)
	}
	if err := config.SaveEnvFile(path, cfg); err != nil {
		return fmt.Errorf("could not save .env: %w", err)
	}
	_, err = fmt.Fprintf(out, "Connected. Credentials saved to %s\n", path)
	return err
}

func prompt(reader *bufio.Reader, out io.Writer, label, previous string) (string, error) {
	if previous == "" {
		if _, err := fmt.Fprintf(out, "%s: ", label); err != nil {
			return "", err
		}
	} else if _, err := fmt.Fprintf(out, "%s [%s]: ", label, previous); err != nil {
		return "", err
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", errors.New("input ended before login was complete")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return previous, nil
	}
	return value, nil
}
