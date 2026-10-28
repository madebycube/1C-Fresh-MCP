package odata

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/madebycube/1C-Fresh-MCP/internal/config"
)

const maxMetadataBytes = 16 << 20

var guidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

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
	return c.get(ctx, resource, params, maxBytes)
}

func (c *Client) GetStockBalance(ctx context.Context, productID string, params url.Values, maxBytes int64) ([]byte, error) {
	if !guidPattern.MatchString(productID) {
		return nil, errors.New("product ID must be a GUID")
	}
	plan := config.WarehouseStock
	resource := plan.Name + "/Balance(Condition='" + plan.ProductField + " eq guid''" + strings.ToLower(productID) + "''')"
	return c.get(ctx, resource, params, maxBytes)
}

var (
	registerNamePattern = regexp.MustCompile(`^AccumulationRegister_[\p{L}\p{N}_]+$`)
	registerListPattern = regexp.MustCompile(`^[\p{L}\p{N}_]+(,[\p{L}\p{N}_]+)*$`)
	registerTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$`)
)

// GetRegisterTable reads a Balance or Turnovers virtual table. Every argument is validated so callers cannot inject OData syntax.
func (c *Client) GetRegisterTable(ctx context.Context, query config.RegisterQuery, params url.Values, maxBytes int64) ([]byte, error) {
	resource, err := RegisterTableResource(query)
	if err != nil {
		return nil, err
	}
	return c.get(ctx, resource, params, maxBytes)
}

func RegisterTableResource(query config.RegisterQuery) (string, error) {
	if !registerNamePattern.MatchString(query.Register) {
		return "", errors.New("invalid accumulation register")
	}
	var args []string
	periods := [][2]string{}
	switch query.Table {
	case "Balance":
		if query.StartPeriod != "" || query.EndPeriod != "" {
			return "", errors.New("balance takes only a period")
		}
		periods = append(periods, [2]string{"Period", query.Period})
	case "Turnovers":
		if query.Period != "" {
			return "", errors.New("turnovers take a start and end period")
		}
		periods = append(periods, [2]string{"StartPeriod", query.StartPeriod}, [2]string{"EndPeriod", query.EndPeriod})
	default:
		return "", errors.New("unsupported register table")
	}
	for _, period := range periods {
		if period[1] == "" {
			continue
		}
		if !registerTimePattern.MatchString(period[1]) {
			return "", errors.New("invalid register period")
		}
		args = append(args, period[0]+"=datetime'"+period[1]+"'")
	}
	if query.Dimensions != "" {
		if !registerListPattern.MatchString(query.Dimensions) {
			return "", errors.New("invalid register dimensions")
		}
		args = append(args, "Dimensions='"+query.Dimensions+"'")
	}
	return query.Register + "/" + query.Table + "(" + strings.Join(args, ",") + ")", nil
}

func (c *Client) get(ctx context.Context, resource string, params url.Values, maxBytes int64) ([]byte, error) {
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

// Action runs a 1C document action such as Post or Unpost on one document by GUID.
func (c *Client) Action(ctx context.Context, resource, action string) ([]byte, error) {
	name, id, ok := strings.Cut(strings.TrimSuffix(resource, "')"), "(guid'")
	if !ok || !strings.HasPrefix(name, "Document_") || strings.ContainsAny(name, "/()'") || !guidPattern.MatchString(id) {
		return nil, errors.New("invalid OData document action target")
	}
	query := url.Values{}
	switch action {
	case "Post":
		query.Set("PostingModeOperational", "false")
	case "Unpost":
	default:
		return nil, errors.New("unsupported OData document action")
	}
	u := c.base
	u.Path = strings.TrimRight(u.Path, "/") + "/odata/standard.odata/" + resource + "/" + action
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return nil, errors.New("could not create OData action request")
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("OData connection failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return nil, errors.New("could not read OData response")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("OData returned HTTP %d%s", resp.StatusCode, odataErrorMessage(data))
	}
	return data, nil
}

// odataErrorMessage extracts 1C's own explanation, which says why posting failed.
func odataErrorMessage(data []byte) string {
	var response struct {
		Error struct {
			Message struct {
				Value string `json:"value"`
			} `json:"message"`
		} `json:"odata.error"`
	}
	if json.Unmarshal(data, &response) != nil || response.Error.Message.Value == "" {
		return ""
	}
	message := []rune(strings.TrimSpace(response.Error.Message.Value))
	if len(message) > 500 {
		message = message[:500]
	}
	return ": " + string(message)
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
