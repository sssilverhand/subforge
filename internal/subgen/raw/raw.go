package raw

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/sssilverhand/subforge/internal/subgen"
)

// Generator produces a base64-encoded newline-separated URI list.
// Compatible with: v2rayN, v2rayNG, Shadowrocket, V2Box, NekoRay.
type Generator struct{}

func New() *Generator { return &Generator{} }

func (g *Generator) ContentType() string { return "text/plain; charset=utf-8" }

func (g *Generator) Generate(data subgen.SubData) ([]byte, error) {
	var lines []string
	for _, ep := range data.Endpoints {
		var uri string
		var err error
		switch ep.Protocol {
		case "vless-xhttp", "vless-ws", "vless-reality":
			uri, err = vlessURI(data, ep)
		case "hysteria2":
			uri, err = hy2URI(data, ep)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, uri)
	}

	raw := strings.Join(lines, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	return []byte(encoded), nil
}

func vlessURI(data subgen.SubData, ep subgen.Endpoint) (string, error) {
	q := url.Values{}

	switch ep.Protocol {
	case "vless-xhttp":
		q.Set("type", "xhttp")
		q.Set("security", "tls")
		q.Set("host", ep.Settings.Host)
		q.Set("path", ep.Settings.Path)
		if ep.Settings.SNI != "" {
			q.Set("sni", ep.Settings.SNI)
		}
	case "vless-ws":
		q.Set("type", "ws")
		q.Set("security", "tls")
		q.Set("host", ep.Settings.Host)
		q.Set("path", ep.Settings.Path)
	case "vless-reality":
		q.Set("type", "tcp")
		q.Set("security", "reality")
		q.Set("pbk", ep.Settings.PublicKey)
		q.Set("sid", ep.Settings.ShortID)
		q.Set("sni", ep.Settings.ServerName)
		q.Set("fp", "chrome")
		if ep.Settings.Flow != "" {
			q.Set("flow", ep.Settings.Flow)
		}
		if ep.Settings.SpiderX != "" {
			q.Set("spx", ep.Settings.SpiderX)
		}
	default:
		return "", fmt.Errorf("unknown vless variant: %s", ep.Protocol)
	}

	fragment := ep.NodeName
	if data.SubName != "" {
		fragment = fmt.Sprintf("%s - %s", data.SubName, ep.NodeName)
	}

	u := url.URL{
		Scheme:   "vless",
		User:     url.User(data.UserUUID.String()),
		Host:     fmt.Sprintf("%s:%d", ep.PublicHost, ep.Port),
		RawQuery: q.Encode(),
		Fragment: fragment,
	}
	return u.String(), nil
}

func hy2URI(data subgen.SubData, ep subgen.Endpoint) (string, error) {
	q := url.Values{}
	if ep.Settings.Insecure {
		q.Set("insecure", "1")
	}
	if ep.Settings.SNI != "" {
		q.Set("sni", ep.Settings.SNI)
	}
	if ep.Settings.Obfs == "salamander" {
		q.Set("obfs", "salamander")
		q.Set("obfs-password", ep.Settings.ObfsPassword)
	}

	fragment := ep.NodeName
	if data.SubName != "" {
		fragment = fmt.Sprintf("%s - %s", data.SubName, ep.NodeName)
	}

	u := url.URL{
		Scheme:   "hy2",
		User:     url.User(data.Hy2Password),
		Host:     fmt.Sprintf("%s:%d", ep.PublicHost, ep.Port),
		RawQuery: q.Encode(),
		Fragment: fragment,
	}
	return u.String(), nil
}
