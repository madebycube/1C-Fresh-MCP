package odata

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const maxMetadataBytes = 16 << 20

type Client struct {
	base     url.URL
	username string
	password string
	http     *http.Client
}

func New(cfg config.Config) *Client {
	return &Client{
		base:     *cfg.BaseURL,
		username: cfg.Username,
		password: cfg.Password,
		http: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) Get(ctx context.Context, resource string, params url.Values, maxBytes int64) ([]byte, error) {
	if resource == "" || strings.Contains(resource, "/") || strings.Contains(resource, "..") {
		return nil, errors.New("invalid OData resource")
	}
	u := c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/odata/standard.odata/" + resource
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("could not create OData request")
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json, application/xml;q=0.9")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("OData connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OData returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, errors.New("could not read OData response")
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("OData response exceeded size limit")
	}
	return data, nil
}

func (c *Client) Write(ctx context.Context, method, resource string, body []byte, ifMatch string) ([]byte, error) {
	if method != http.MethodPost && method != http.MethodPatch {
		return nil, errors.New("unsupported OData write method")
	}
	if resource == "" || strings.Contains(resource, "/") || strings.Contains(resource, "..") || len(body) == 0 || len(body) > 1<<20 {
		return nil, errors.New("invalid OData write request")
	}
	if method == http.MethodPatch && ifMatch == "" {
		return nil, errors.New("OData update requires a data version")
	}
	u := c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/odata/standard.odata/" + resource
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not create OData write request")
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("OData connection failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil, errors.New("OData record changed since it was read")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("OData returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil {
		return nil, errors.New("could not read OData response")
	}
	if len(data) > 1<<20 {
		return nil, errors.New("OData response exceeded size limit")
	}
	return data, nil
}

func (c *Client) Check(ctx context.Context) (int, error) {
	data, err := c.Get(ctx, "$metadata", nil, maxMetadataBytes)
	if err != nil {
		return 0, err
	}
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	count := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, errors.New("invalid OData metadata")
		}
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "EntitySet" {
			count++
		}
	}
	if count == 0 {
		return 0, errors.New("OData metadata contained no resources")
	}
	return count, nil
}
