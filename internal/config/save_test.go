package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveEnvFilePreservesOtherSettingsAndRestrictsAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# local settings\nCUSTOM=yes\nONEC_ODATA_USERNAME=old\nONEC_ODATA_PASSWORD=old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := New("https://example.com/a/sbm/1", "new-user", " a'\"\\b ")
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveEnvFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	values, err := ReadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["CUSTOM"] != "yes" || values["ONEC_ODATA_PASSWORD"] != cfg.Password || values["ONEC_ODATA_USERNAME"] != cfg.Username || values["ONEC_ODATA_BASE_URL"] != cfg.BaseURL.String() {
		t.Fatal("saved values did not round-trip")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "# local settings\nCUSTOM=yes\n") {
		t.Fatal("unrelated settings changed")
	}
}
