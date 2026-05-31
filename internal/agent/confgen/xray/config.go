package xray

import (
	"encoding/json"
	"fmt"
)

// InboundSpec describes one xray inbound to include in the generated config.
type InboundSpec struct {
	Tag      string          // e.g. "vless-xhttp-in"
	Protocol string          // vless-xhttp | vless-reality | vless-ws
	Listen   string          // "127.0.0.1" (behind nginx) or "0.0.0.0" (reality)
	Port     int
	Settings json.RawMessage // EndpointSettings JSON from DB
}

// Config holds all parameters needed to generate a full xray config.json.
type Config struct {
	APIPort   int           // gRPC management port, default 10085
	LogLevel  string        // warning | info | debug
	LogFile   string        // path for error log, empty = /var/log/xray/error.log
	Inbounds  []InboundSpec
}

// Generate produces a complete xray config.json as bytes.
func Generate(c Config) ([]byte, error) {
	if c.APIPort == 0 {
		c.APIPort = 10085
	}
	if c.LogLevel == "" {
		c.LogLevel = "warning"
	}
	if c.LogFile == "" {
		c.LogFile = "/var/log/xray/error.log"
	}

	inbounds := []map[string]any{
		// Management inbound — must have tag "api"
		{
			"tag":      "api",
			"listen":   "127.0.0.1",
			"port":     c.APIPort,
			"protocol": "dokodemo-door",
			"settings": map[string]any{"address": "127.0.0.1"},
		},
	}

	for _, spec := range c.Inbounds {
		ib, err := buildInbound(spec)
		if err != nil {
			return nil, fmt.Errorf("inbound %s: %w", spec.Tag, err)
		}
		inbounds = append(inbounds, ib)
	}

	cfg := map[string]any{
		"log": map[string]any{
			"loglevel": c.LogLevel,
			"error":    c.LogFile,
			"access":   "none",
		},
		"api": map[string]any{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService"},
		},
		"stats": map[string]any{},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
		},
		"inbounds": inbounds,
		"outbounds": []map[string]any{
			{"protocol": "freedom", "tag": "direct"},
			{"protocol": "blackhole", "tag": "blocked"},
		},
		"routing": map[string]any{
			"rules": []map[string]any{
				{"inboundTag": []string{"api"}, "outboundTag": "api"},
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func buildInbound(spec InboundSpec) (map[string]any, error) {
	var s EndpointSettings
	if len(spec.Settings) > 0 {
		if err := json.Unmarshal(spec.Settings, &s); err != nil {
			return nil, fmt.Errorf("unmarshal settings: %w", err)
		}
	}

	ib := map[string]any{
		"tag":      spec.Tag,
		"listen":   spec.Listen,
		"port":     spec.Port,
		"protocol": "vless",
		"settings": map[string]any{
			"clients":    []any{},
			"decryption": "none",
		},
	}

	switch spec.Protocol {
	case "vless-xhttp":
		ib["streamSettings"] = map[string]any{
			"network": "xhttp",
			"xhttpSettings": map[string]any{
				"path": s.Path,
				"host": s.Host,
			},
		}

	case "vless-ws":
		ib["streamSettings"] = map[string]any{
			"network":  "ws",
			"security": "tls",
			"tlsSettings": map[string]any{
				"certificates": []map[string]any{{
					"certificateFile": s.CertFile,
					"keyFile":         s.KeyFile,
				}},
			},
			"wsSettings": map[string]any{
				"path": s.Path,
				"headers": map[string]string{
					"Host": s.Host,
				},
			},
		}

	case "vless-reality":
		ib["streamSettings"] = map[string]any{
			"network":  "tcp",
			"security": "reality",
			"realitySettings": map[string]any{
				"show":        false,
				"dest":        s.RealityDest,
				"xver":        0,
				"serverNames": []string{s.ServerName},
				"privateKey":  s.PrivateKey,
				"shortIds":    []string{s.ShortID},
			},
		}
		// Reality with vision flow
		ib["settings"] = map[string]any{
			"clients":    []any{},
			"decryption": "none",
		}

	default:
		return nil, fmt.Errorf("unknown protocol: %s", spec.Protocol)
	}

	return ib, nil
}

// EndpointSettings mirrors subgen.EndpointSettings but lives here
// to avoid import cycles between agent and main server packages.
type EndpointSettings struct {
	// XHTTP / WS
	Path     string `json:"path"`
	Host     string `json:"host"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
	// Reality
	ServerName  string `json:"server_name"`
	PrivateKey  string `json:"private_key"`
	PublicKey   string `json:"public_key"`
	ShortID     string `json:"short_id"`
	RealityDest string `json:"reality_dest"` // e.g. "google.com:443"
	SpiderX     string `json:"spider_x"`
	// Hysteria2
	Obfs         string `json:"obfs"`
	ObfsPassword string `json:"obfs_password"`
	Insecure     bool   `json:"insecure"`
	SNI          string `json:"sni"`
	Flow         string `json:"flow"`
}
