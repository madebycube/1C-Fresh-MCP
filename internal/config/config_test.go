package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultEnvFileUsesProjectForLinkedBinary(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	launcher := filepath.Join(root, "launcher")
	elsewhere := filepath.Join(root, "elsewhere")
	for _, dir := range []string{bin, launcher, elsewhere} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(bin, "1c")
	if err := os.WriteFile(binary, nil, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(launcher, "1c")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(resolved), "..", ".env")
	if got := defaultEnvFile(elsewhere, link); got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
	local := filepath.Join(elsewhere, ".env")
	if err := os.WriteFile(local, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := defaultEnvFile(elsewhere, link); got != local {
		t.Fatalf("got %q; want %q", got, local)
	}
}
