package clash

import (
	"fmt"

	"github.com/sssilverhand/subforge/internal/subgen"
	"gopkg.in/yaml.v3"
)

// Generator produces Clash Meta / Mihomo YAML.
// Compatible with: Clash Verge Rev, Mihomo Party, Stash, Clash Meta for Android.
// Note: Hysteria2 requires Clash Meta (Mihomo) — not supported in original Clash.
type Generator struct{}

func New() *Generator { return &Generator{} }

func (g *Generator) ContentType() string { return "text/yaml; charset=utf-8" }

func (g *Generator) Generate(data subgen.SubData) ([]byte, error) {
	var proxies []map[string]any
	var proxyNames []string

	for _, ep := range data.Endpoints {
		name := fmt.Sprintf("%s [%s]", ep.Protocol, ep.NodeName)
		var p map[string]any

		switch ep.Protocol {
		case "vless-xhttp":
			p = vlessXHTTP(name, data, ep)
		case "vless-ws":
			p = vlessWS(name, data, ep)
		case "vless-reality":
			p = vlessReality(name, data, ep)
		case "hysteria2":
			p = hysteria2(name, data, ep)
		default:
			continue
		}

		proxies = append(proxies, p)
		proxyNames = append(proxyNames, name)
	}

	config := map[string]any{
		"mixed-port":       7890,
		"allow-lan":        false,
		"mode":             "rule",
		"log-level":        "info",
		"external-controller": "127.0.0.1:9090",
		"proxies":          proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    "Proxy",
				"type":    "select",
				"proxies": append(proxyNames, "DIRECT"),
			},
			{
				"name":    "Auto",
				"type":    "url-test",
				"proxies": proxyNames,
				"url":     "http://www.gstatic.com/generate_204",
				"interval": 300,
			},
		},
		"rules": []string{
			"GEOIP,private,DIRECT,no-resolve",
			"MATCH,Proxy",
		},
	}

	return yaml.Marshal(config)
}

func vlessXHTTP(name string, data subgen.SubData, ep subgen.Endpoint) map[string]any {
	return map[string]any{
		"name":    name,
		"type":    "vless",
		"server":  ep.PublicHost,
		"port":    ep.Port,
		"uuid":    data.UserUUID.String(),
		"tls":     true,
		"servername": ep.Settings.Host,
		"network": "xhttp",
		"xhttp-opts": map[string]any{
			"path": ep.Settings.Path,
			"host": ep.Settings.Host,
		},
	}
}

func vlessWS(name string, data subgen.SubData, ep subgen.Endpoint) map[string]any {
	return map[string]any{
		"name":    name,
		"type":    "vless",
		"server":  ep.PublicHost,
		"port":    ep.Port,
		"uuid":    data.UserUUID.String(),
		"tls":     true,
		"servername": ep.Settings.Host,
		"network": "ws",
		"ws-opts": map[string]any{
			"path": ep.Settings.Path,
			"headers": map[string]string{
				"Host": ep.Settings.Host,
			},
		},
	}
}

func vlessReality(name string, data subgen.SubData, ep subgen.Endpoint) map[string]any {
	p := map[string]any{
		"name":           name,
		"type":           "vless",
		"server":         ep.PublicHost,
		"port":           ep.Port,
		"uuid":           data.UserUUID.String(),
		"tls":            true,
		"servername":     ep.Settings.ServerName,
		"reality-opts": map[string]any{
			"public-key": ep.Settings.PublicKey,
			"short-id":   ep.Settings.ShortID,
		},
		"client-fingerprint": "chrome",
		"network":            "tcp",
	}
	if ep.Settings.Flow != "" {
		p["flow"] = ep.Settings.Flow
	}
	return p
}

func hysteria2(name string, data subgen.SubData, ep subgen.Endpoint) map[string]any {
	p := map[string]any{
		"name":     name,
		"type":     "hysteria2",
		"server":   ep.PublicHost,
		"port":     ep.Port,
		"password": data.Hy2Password,
		"sni":      ep.Settings.SNI,
		"skip-cert-verify": ep.Settings.Insecure,
	}
	if ep.Settings.Obfs == "salamander" {
		p["obfs"] = "salamander"
		p["obfs-password"] = ep.Settings.ObfsPassword
	}
	return p
}
