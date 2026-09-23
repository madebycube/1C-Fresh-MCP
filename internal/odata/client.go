package odata

import (
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
