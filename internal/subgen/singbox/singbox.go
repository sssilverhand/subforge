package singbox

import (
	"encoding/json"
	"fmt"

	"github.com/sssilverhand/subforge/internal/subgen"
)

// Generator produces a sing-box JSON config (outbounds + selector).
// Compatible with: sing-box native, Hiddify, NekoBox, SFI/SFA.
type Generator struct{}

func New() *Generator { return &Generator{} }

func (g *Generator) ContentType() string { return "application/json; charset=utf-8" }

func (g *Generator) Generate(data subgen.SubData) ([]byte, error) {
	var outbounds []map[string]any
	var selectorTags []string

	for _, ep := range data.Endpoints {
		var ob map[string]any
		var err error
		tag := fmt.Sprintf("%s [%s]", ep.Protocol, ep.NodeName)

		switch ep.Protocol {
		case "vless-xhttp":
			ob, err = vlessXHTTP(tag, data, ep)
		case "vless-ws":
			ob, err = vlessWS(tag, data, ep)
		case "vless-reality":
			ob, err = vlessReality(tag, data, ep)
		case "hysteria2":
			ob, err = hysteria2(tag, data, ep)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		outbounds = append(outbounds, ob)
		selectorTags = append(selectorTags, tag)
	}

	// selector proxy — user picks active outbound
	selector := map[string]any{
		"type":     "selector",
		"tag":      "proxy",
		"outbounds": selectorTags,
	}
	if len(selectorTags) > 0 {
		selector["default"] = selectorTags[0]
	}

	outbounds = append([]map[string]any{selector}, outbounds...)
	outbounds = append(outbounds,
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	)

	config := map[string]any{
		"log":       map[string]any{"level": "info"},
		"dns":       defaultDNS(),
		"inbounds":  defaultInbounds(),
		"outbounds": outbounds,
		"route":     defaultRoute(),
	}

	return json.MarshalIndent(config, "", "  ")
}

func vlessXHTTP(tag string, data subgen.SubData, ep subgen.Endpoint) (map[string]any, error) {
	ob := map[string]any{
		"type":       "vless",
		"tag":        tag,
		"server":     ep.PublicHost,
		"server_port": ep.Port,
		"uuid":       data.UserUUID.String(),
		"tls": map[string]any{
			"enabled":     true,
			"server_name": ep.Settings.Host,
		},
		"transport": map[string]any{
			"type": "httpupgrade",
			"host": ep.Settings.Host,
			"path": ep.Settings.Path,
		},
	}
	return ob, nil
}

func vlessWS(tag string, data subgen.SubData, ep subgen.Endpoint) (map[string]any, error) {
	ob := map[string]any{
		"type":       "vless",
		"tag":        tag,
		"server":     ep.PublicHost,
		"server_port": ep.Port,
		"uuid":       data.UserUUID.String(),
		"tls": map[string]any{
			"enabled":     true,
			"server_name": ep.Settings.Host,
		},
		"transport": map[string]any{
			"type": "ws",
			"path": ep.Settings.Path,
			"headers": map[string]string{
				"Host": ep.Settings.Host,
			},
		},
	}
	return ob, nil
}

func vlessReality(tag string, data subgen.SubData, ep subgen.Endpoint) (map[string]any, error) {
	ob := map[string]any{
		"type":       "vless",
		"tag":        tag,
		"server":     ep.PublicHost,
		"server_port": ep.Port,
		"uuid":       data.UserUUID.String(),
		"flow":       ep.Settings.Flow,
		"tls": map[string]any{
			"enabled":     true,
			"server_name": ep.Settings.ServerName,
			"utls": map[string]any{
				"enabled":     true,
				"fingerprint": "chrome",
			},
			"reality": map[string]any{
				"enabled":    true,
				"public_key": ep.Settings.PublicKey,
				"short_id":   ep.Settings.ShortID,
			},
		},
	}
	return ob, nil
}

func hysteria2(tag string, data subgen.SubData, ep subgen.Endpoint) (map[string]any, error) {
	ob := map[string]any{
		"type":       "hysteria2",
		"tag":        tag,
		"server":     ep.PublicHost,
		"server_port": ep.Port,
		"password":   data.Hy2Password,
		"tls": map[string]any{
			"enabled":     true,
			"server_name": ep.Settings.SNI,
			"insecure":    ep.Settings.Insecure,
		},
	}
	if ep.Settings.Obfs == "salamander" {
		ob["obfs"] = map[string]any{
			"type":     "salamander",
			"password": ep.Settings.ObfsPassword,
		}
	}
	return ob, nil
}

func defaultDNS() map[string]any {
	return map[string]any{
		"servers": []map[string]any{
			{"tag": "google", "address": "tls://8.8.8.8"},
			{"tag": "local", "address": "local", "detour": "direct"},
		},
		"rules": []map[string]any{
			{"outbound": "any", "server": "local"},
		},
		"final": "google",
	}
}

func defaultInbounds() []map[string]any {
	return []map[string]any{
		{
			"type":        "tun",
			"tag":         "tun-in",
			"inet4_address": "172.19.0.1/30",
			"auto_route":  true,
			"strict_route": true,
		},
	}
}

func defaultRoute() map[string]any {
	return map[string]any{
		"rules": []map[string]any{
			{"protocol": "dns", "outbound": "dns-out"},
			{"geosite": "private", "outbound": "direct"},
			{"geoip": "private", "outbound": "direct"},
		},
		"final":               "proxy",
		"auto_detect_interface": true,
	}
}
