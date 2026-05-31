package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	xraygen "github.com/sssilverhand/subforge/internal/agent/confgen/xray"
	hy2gen "github.com/sssilverhand/subforge/internal/agent/confgen/hysteria2"
)

// Client calls the SubForge node agent HTTP API.
type Client struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

func New(agentURL, secret string) *Client {
	return &Client{
		baseURL: strings.TrimRight(agentURL, "/"),
		secret:  secret,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // installs take time
		},
	}
}

// StatusResponse is the agent /status response.
type StatusResponse struct {
	Xray struct {
		Running bool   `json:"running"`
		Version string `json:"version"`
		Service string `json:"service"`
	} `json:"xray"`
	Hysteria2 struct {
		Running bool   `json:"running"`
		Version string `json:"version"`
		Service string `json:"service"`
	} `json:"hysteria2"`
}

func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	var resp StatusResponse
	if err := c.get(ctx, "/status", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

type InstallResult struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

func (c *Client) InstallXray(ctx context.Context, version string) (*InstallResult, error) {
	var res InstallResult
	err := c.post(ctx, "/xray/install", map[string]string{"version": version}, &res)
	return &res, err
}

func (c *Client) WriteXrayConfig(ctx context.Context, cfg xraygen.Config) error {
	return c.post(ctx, "/xray/config", cfg, nil)
}

func (c *Client) RestartXray(ctx context.Context) error {
	return c.post(ctx, "/xray/restart", nil, nil)
}

func (c *Client) InstallHysteria2(ctx context.Context, version string) (*InstallResult, error) {
	var res InstallResult
	err := c.post(ctx, "/hysteria2/install", map[string]string{"version": version}, &res)
	return &res, err
}

func (c *Client) WriteHysteria2Config(ctx context.Context, cfg hy2gen.Config) error {
	return c.post(ctx, "/hysteria2/config", cfg, nil)
}

func (c *Client) RestartHysteria2(ctx context.Context) error {
	return c.post(ctx, "/hysteria2/restart", nil, nil)
}

// ─── internal ────────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent request %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode >= 400 {
		var e struct{ Error string `json:"error"` }
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("agent %s: HTTP %d: %s", req.URL.Path, resp.StatusCode, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
