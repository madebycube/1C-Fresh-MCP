package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var credentialKeys = []string{"ONEC_ODATA_BASE_URL", "ONEC_ODATA_USERNAME", "ONEC_ODATA_PASSWORD"}

func SaveEnvFile(path string, cfg Config) error {
	if path == "" {
		return errors.New("environment file path is empty")
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	values := map[string]string{
		credentialKeys[0]: cfg.BaseURL.String(),
		credentialKeys[1]: cfg.Username,
		credentialKeys[2]: cfg.Password,
	}
	for _, value := range values {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("credentials cannot contain line breaks")
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(existing), "\n"), "\n")
	if len(existing) == 0 {
		lines = nil
	}
	written := make(map[string]bool, len(values))
	output := make([]string, 0, len(lines)+len(values))
	for _, line := range lines {
		key, _, found := strings.Cut(line, "=")
		key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
		if found {
			if value, ok := values[key]; ok {
				if !written[key] {
					output = append(output, key+"="+envValue(value))
					written[key] = true
				}
				continue
			}
		}
		output = append(output, line)
	}
	for _, key := range credentialKeys {
		if !written[key] {
			output = append(output, key+"="+envValue(values[key]))
		}
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".1c-env-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(strings.Join(output, "\n") + "\n"); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func envValue(value string) string {
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\"'\\") {
		return strconv.Quote(value)
	}
	return value
}
