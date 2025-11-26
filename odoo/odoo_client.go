package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// Client is a minimal Odoo JSON-RPC client used by tools.
type Client struct {
	URL  string
	DB   string
	User string
	Key  string
	UID  int
	HTTP *http.Client
}

// New constructs a client with sensible defaults.
func New(url, db, user, key string) *Client {
	return &Client{
		URL:  url,
		DB:   db,
		User: user,
		Key:  key,
		HTTP: &http.Client{Timeout: 15 * time.Second},
	}
}

// rpc posts a JSON-RPC payload and returns raw body and decoded map.
func (c *Client) rpc(ctx context.Context, payload any) ([]byte, map[string]any, error) {
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", c.URL, bytes.NewReader(b))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	var out map[string]any
	_ = json.Unmarshal(body, &out)

	// If HTTP-level error
	if resp.StatusCode >= 400 {
		return body, out, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	// If JSON-RPC returned an error object, surface it as Go error with details
	if rpcErr, ok := out["error"]; ok {
		// try to extract message/data
		return body, out, fmt.Errorf("odoo rpc error: %v", rpcErr)
	}

	return body, out, nil
}

// Login authenticates and stores the returned UID. On failure returns error.
func (c *Client) Login() error {
	ctx := context.Background()
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"params": map[string]any{
			"service": "common",
			"method":  "authenticate",
			"args":    []any{c.DB, c.User, c.Key, map[string]any{}},
		},
	}

	_, out, err := c.rpc(ctx, payload)
	if err != nil {
		return err
	}

	// result can be false, number, or object depending on Odoo
	if res, ok := out["result"]; ok {
		switch v := res.(type) {
		case float64:
			c.UID = int(v)
			return nil
		case bool:
			if v == false {
				return fmt.Errorf("invalid credentials")
			}
		default:
			return fmt.Errorf("unexpected login result: %T", v)
		}
	}
	return fmt.Errorf("no result in login response")
}

// SearchRead performs a search_read RPC and returns the slice of records.
func (c *Client) SearchRead(model string, fields []string, domain []any) ([]map[string]any, error) {
	ctx := context.Background()
	var args []any
	if len(domain) == 0 {
		// Odoo may reject a domain list containing an empty item ([]). If the
		// caller provided an empty domain, omit the positional domain argument
		// so search_read runs without a domain (equivalent to searching all records).
		args = []any{}
	} else {
		args = []any{domain}
	}
	params := map[string]any{"fields": fields, "limit": 500}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"params": map[string]any{
			"service": "object",
			"method":  "execute_kw",
			"args":    []any{c.DB, c.UID, c.Key, model, "search_read", args, params},
		},
	}

	body, out, err := c.rpc(ctx, payload)
	if err != nil {
		return nil, err
	}

	res, ok := out["result"]
	if !ok {
		return nil, fmt.Errorf("no result in search_read response: %s", string(body))
	}

	// expected array of maps
	arr, ok := res.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type %T", res)
	}

	outArr := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			outArr = append(outArr, m)
		}
	}
	return outArr, nil
}

// Create creates a record in the given model with the provided values map
// and returns the created record id (int) or error.
func (c *Client) Create(model string, vals map[string]any) (int, error) {
	ctx := context.Background()
	// build payload: execute_kw(db, uid, key, model, 'create', [vals])
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "call",
		"params": map[string]any{
			"service": "object",
			"method":  "execute_kw",
			"args":    []any{c.DB, c.UID, c.Key, model, "create", []any{vals}},
		},
	}

	_, out, err := c.rpc(ctx, payload)
	if err != nil {
		return 0, err
	}

	res, ok := out["result"]
	if !ok {
		// If Odoo returned an error it should have been caught in rpc(),
		// but defensively include the raw response in the error.
		return 0, fmt.Errorf("no result from create, response=%v", out)
	}
	switch v := res.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected create result type %T value=%v", v, v)
	}
}
