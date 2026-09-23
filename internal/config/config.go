package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	BaseURL  *url.URL
	Username string
	Password string
}

func Load() (Config, error) {
	filePath := EnvFilePath()
	fileValues, err := ReadEnvFile(filePath)
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
	return New(value("ONEC_ODATA_BASE_URL"), value("ONEC_ODATA_USERNAME"), value("ONEC_ODATA_PASSWORD"))
}

func EnvFilePath() string {
	if path := os.Getenv("ONEC_ENV_FILE"); path != "" {
		return path
	}
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	return defaultEnvFile(cwd, executable)
}

func New(base, username, password string) (Config, error) {
	base = strings.TrimSpace(base)
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

func defaultEnvFile(cwd, executable string) string {
	local := filepath.Join(cwd, ".env")
	if _, err := os.Stat(local); !errors.Is(err, os.ErrNotExist) {
		return local
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return local
	}
	return filepath.Join(filepath.Dir(resolved), "..", ".env")
}

func ReadEnvFile(path string) (map[string]string, error) {
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
		if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
			decoded, err := strconv.Unquote(raw)
			if err != nil {
				return nil, errors.New("invalid quoted environment value")
			}
			raw = decoded
		} else if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
			raw = raw[1 : len(raw)-1]
		}
		values[key] = raw
	}
	return values, scanner.Err()
}
