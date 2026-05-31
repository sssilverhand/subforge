// Package web embeds the built React SPA into the Go binary.
package web

import "embed"

//go:embed dist
var FS embed.FS
