package hysteria2

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config holds parameters for generating a hysteria2 config.yaml.
type Config struct {
	ListenPort      int
	TLSCert         string
	TLSKey          string
	AuthBackendURL  string // SubForge's /internal/hy2/auth endpoint
	TrafficStatsPort int   // management API port, default 11451
	TrafficStatsSecret string
	// Optional obfuscation
	Obfs         string // "salamander" or ""
	ObfsPassword string
	// Masquerade — makes traffic look like HTTPS to a real site
	MasqueradeURL string // e.g. "https://news.ycombinator.com/"
	// Bandwidth limits (optional)
	UpMbps   int
	DownMbps int
}

// Generate produces a hysteria2 config.yaml as bytes.
func Generate(c Config) ([]byte, error) {
	if c.TrafficStatsPort == 0 {
		c.TrafficStatsPort = 11451
	}
	if c.MasqueradeURL == "" {
		c.MasqueradeURL = "https://news.ycombinator.com/"
	}
	if c.AuthBackendURL == "" {
		return nil, fmt.Errorf("auth backend URL is required")
	}

	cfg := map[string]any{
		"listen": fmt.Sprintf(":%d", c.ListenPort),
		"tls": map[string]any{
			"cert": c.TLSCert,
			"key":  c.TLSKey,
		},
		"auth": map[string]any{
			"type": "http",
			"http": map[string]any{
				"url":      c.AuthBackendURL,
				"insecure": false,
			},
		},
		"trafficStats": map[string]any{
			"listen": fmt.Sprintf("127.0.0.1:%d", c.TrafficStatsPort),
			"secret": c.TrafficStatsSecret,
		},
		"masquerade": map[string]any{
			"type": "proxy",
			"proxy": map[string]any{
				"url":         c.MasqueradeURL,
				"rewriteHost": true,
			},
		},
	}

	if c.Obfs == "salamander" && c.ObfsPassword != "" {
		cfg["obfs"] = map[string]any{
			"type": "salamander",
			"salamander": map[string]any{
				"password": c.ObfsPassword,
			},
		}
	}

	if c.UpMbps > 0 || c.DownMbps > 0 {
		cfg["bandwidth"] = map[string]any{
			"up":   fmt.Sprintf("%d mbps", c.UpMbps),
			"down": fmt.Sprintf("%d mbps", c.DownMbps),
		}
	}

	return yaml.Marshal(cfg)
}
