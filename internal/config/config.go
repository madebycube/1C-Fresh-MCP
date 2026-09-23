package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	BaseURL  *url.URL
	Username string
	Password string
}

func Load() (Config, error) {
	filePath := os.Getenv("ONEC_ENV_FILE")
	if filePath == "" {
		filePath = ".env"
	}
	fileValues, err := readEnvFile(filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || os.Getenv("ONEC_ENV_FILE") != "" {
			return Config{}, fmt.Errorf("read environment file: %w", err)
		}
	}
	value := func(key string) string {
		if v, ok := os.LookupEnv(key); ok {
			return v
		}
		return fileValues[key]
	}
	base := strings.TrimSpace(value("ONEC_ODATA_BASE_URL"))
	username := value("ONEC_ODATA_USERNAME")
	password := value("ONEC_ODATA_PASSWORD")
	if base == "" || username == "" || password == "" {
		return Config{}, errors.New("ONEC_ODATA_BASE_URL, ONEC_ODATA_USERNAME, and ONEC_ODATA_PASSWORD are required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, errors.New("ONEC_ODATA_BASE_URL must be an HTTPS application URL without credentials, query, or fragment")
	}
	if strings.Contains(parsed.Path, "/odata/") {
		return Config{}, errors.New("ONEC_ODATA_BASE_URL must be the application URL, not an OData endpoint")
	}
	return Config{BaseURL: parsed, Username: username, Password: password}, nil
}

func readEnvFile(path string) (map[string]string, error) {
	values := make(map[string]string)
	file, err := os.Open(path)
	if err != nil {
		return values, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			return nil, errors.New("invalid environment file line")
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		raw = strings.TrimSpace(raw)
		if len(raw) >= 2 && (raw[0] == '\'' && raw[len(raw)-1] == '\'' || raw[0] == '"' && raw[len(raw)-1] == '"') {
			raw = raw[1 : len(raw)-1]
		}
		values[key] = raw
	}
	return values, scanner.Err()
}
