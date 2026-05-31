package subgen

import (
	"github.com/google/uuid"
)

// SubData is everything the generators need to build a subscription.
type SubData struct {
	SubName     string
	UserUUID    uuid.UUID
	Hy2Password string
	Endpoints   []Endpoint
}

// Endpoint is one protocol on one node.
type Endpoint struct {
	NodeName string
	PublicHost string
	Protocol   string // vless-xhttp | vless-reality | vless-ws | hysteria2
	Port       int
	Settings   EndpointSettings
}

// EndpointSettings holds all protocol-specific parameters.
type EndpointSettings struct {
	// VLESS common
	Flow string // xtls-rprx-vision or empty

	// XHTTP / WS
	Path    string
	Host    string // SNI / Host header
	Network string // xhttp | ws | tcp

	// Reality
	PublicKey string
	ShortID   string
	SpiderX   string
	ServerName string // reality SNI

	// Hysteria2
	Obfs       string
	ObfsPassword string
	Insecure   bool
	SNI        string
}

// Generator produces a subscription payload for a specific client.
type Generator interface {
	// ContentType returns the MIME type of the output (text/plain, application/json, text/yaml)
	ContentType() string
	// Generate returns the subscription body bytes.
	Generate(data SubData) ([]byte, error)
}
