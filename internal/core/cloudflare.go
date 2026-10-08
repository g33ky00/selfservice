package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type cfClient struct {
	accountID string
	tunnelID  string
	token     string
	host      string
	httpc     *http.Client
}

func newCFClient(accountID, tunnelID, token, host string) *cfClient {
	return &cfClient{
		accountID: accountID,
		tunnelID:  tunnelID,
		token:     token,
		host:      host,
		httpc:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *cfClient) cfRequest(method, path string, body interface{}) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "https://api.cloudflare.com/client/v4/"+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Success bool            `json:"success"`
		Errors  json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("CF API decode: %w", err)
	}
	if !out.Success {
		return fmt.Errorf("CF API %s %s: %s", method, path, out.Errors)
	}
	return nil
}

// SetIngress activates a session route on the permanent tunnel.
func (c *cfClient) SetIngress(sessionToken string, port int) error {
	return c.cfRequest("PUT",
		fmt.Sprintf("accounts/%s/cfd_tunnel/%s/configurations", c.accountID, c.tunnelID),
		map[string]interface{}{
			"config": map[string]interface{}{
				"ingress": []interface{}{
					map[string]interface{}{
						"hostname": c.host,
						"path":     "/" + sessionToken,
						"service":  fmt.Sprintf("http://127.0.0.1:%d", port),
					},
					map[string]interface{}{"service": "http_status:404"},
				},
			},
		},
	)
}

// ResetIngress restores the 404 catch-all on teardown.
func (c *cfClient) ResetIngress() error {
	return c.cfRequest("PUT",
		fmt.Sprintf("accounts/%s/cfd_tunnel/%s/configurations", c.accountID, c.tunnelID),
		map[string]interface{}{
			"config": map[string]interface{}{
				"ingress": []interface{}{
					map[string]interface{}{"service": "http_status:404"},
				},
			},
		},
	)
}
