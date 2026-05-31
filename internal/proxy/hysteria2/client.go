package hysteria2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to hysteria2's built-in management HTTP API.
//
// Required hysteria2 server config:
//
//	trafficStats:
//	  listen: 127.0.0.1:11451
//	  secret: <hy2_api_secret>
//
//	auth:
//	  type: http
//	  http:
//	    url: http://127.0.0.1:<subforge_port>/internal/hy2/auth
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

func NewClient(apiURL, secret string) *Client {
	return &Client{
		baseURL: strings.TrimRight(apiURL, "/"),
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// UserTraffic holds per-user TX/RX counters.
// The map key is the user ID returned by SubForge's auth endpoint ("sub_<token>").
type UserTraffic struct {
	TX int64 `json:"tx"`
	RX int64 `json:"rx"`
}

// GetTraffic returns per-user traffic stats.
// If clear=true, hysteria2 resets counters after reading (prevents double-counting).
func (c *Client) GetTraffic(ctx context.Context, clear bool) (map[string]UserTraffic, error) {
	url := c.baseURL + "/traffic"
	if clear {
		url += "?clear=1"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hy2 get traffic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hy2 get traffic: status %d", resp.StatusCode)
	}

	var stats map[string]UserTraffic
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("hy2 decode traffic: %w", err)
	}
	return stats, nil
}

// KickUsers disconnects active sessions by their user IDs.
// User ID = "sub_<token>" as returned by SubForge's auth backend.
func (c *Client) KickUsers(ctx context.Context, userIDs []string) error {
	if len(userIDs) == 0 {
		return nil
	}
	body, _ := json.Marshal(userIDs)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/kick",
		strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("hy2 kick: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hy2 kick: status %d", resp.StatusCode)
	}
	return nil
}

// Online returns currently connected user IDs and their connection counts.
func (c *Client) Online(ctx context.Context) (map[string]int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/online", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hy2 online: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("hy2 decode online: %w", err)
	}
	return result, nil
}

// UserID returns the ID SubForge sends to hysteria2 for a given subscription token.
// hysteria2 stats and kick API use this ID as the key.
func UserID(subscriptionToken string) string {
	return "sub_" + subscriptionToken
}
