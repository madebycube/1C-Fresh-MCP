package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

func TestLoginChecksBeforeWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	old := []byte("ONEC_ODATA_BASE_URL=https://example.com/a/sbm/old\nONEC_ODATA_USERNAME=old\nONEC_ODATA_PASSWORD=old-password\nCUSTOM=kept\n")
	if err := os.WriteFile(path, old, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	check := func(_ context.Context, cfg config.Config) error {
		if cfg.BaseURL.String() != "https://example.com/a/sbm/new" || cfg.Username != "new-user" || cfg.Password != "new-password" {
			t.Fatal("unexpected credentials passed to check")
		}
		return errors.New("OData returned HTTP 401")
	}
	input := strings.NewReader("https://example.com/a/sbm/new\nnew-user\n")
	if err := login(context.Background(), path, input, &output, func() (string, error) { return "new-password", nil }, check); err == nil {
		t.Fatal("login unexpectedly succeeded")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, old) || strings.Contains(output.String(), "new-password") {
		t.Fatal("failed login changed or exposed credentials")
	}
	output.Reset()
	input = strings.NewReader("https://example.com/a/sbm/new\nnew-user\n")
	if err := login(context.Background(), path, input, &output, func() (string, error) { return "new-password", nil }, func(context.Context, config.Config) error { return nil }); err != nil {
		t.Fatal(err)
	}
	values, err := config.ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["ONEC_ODATA_PASSWORD"] != "new-password" || values["CUSTOM"] != "kept" {
		t.Fatal("login did not save credentials and other settings")
	}
	for _, label := range []string{"1C Link/Ссылка на 1C", "User/Юзер", "Password/Пароль"} {
		if !strings.Contains(output.String(), label) {
			t.Fatalf("missing prompt %q", label)
		}
	}
}
