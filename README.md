<div align="center">

# ⚡ SubForge

**VPN subscription management panel**  
Works directly with xray-core and hysteria2 — no intermediate panels.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev)
[![Release](https://img.shields.io/github/v/release/sssilverhand/subforge)](https://github.com/sssilverhand/subforge/releases)

[🇷🇺 Читать на русском](README.ru.md) · [Issues](https://github.com/sssilverhand/subforge/issues) · [Releases](https://github.com/sssilverhand/subforge/releases)

</div>

---

## Quick Install

```bash
bash <(curl -Ls https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh)
```

No Go or Node.js required on the server — the script downloads a pre-built binary.

## Features

- **Mixed-protocol subscriptions** — VLESS (XHTTP / Reality / WS) + Hysteria2 in one link
- **Multi-node** — unlimited VPS nodes, clients get configs from all nodes at once
- **Subscription formats** — Sing-box JSON, Clash Meta YAML, raw base64 URI (`?client=singbox|clash|raw`)
- **Real-time access control** — xray via gRPC API (no restarts), hysteria2 via HTTP auth backend
- **Hard limits** — traffic and time limits with automatic cut-off, scheduler runs every 60s
- **Telegram bot** — admin panel + user portal (status, traffic, links per client)
- **Payments** — Cryptomus (crypto) and Telegram Stars; optional auto-creation mode
- **Node agent** — install and update xray/hysteria2 directly from the panel
- **Single binary** — Go backend + React admin UI embedded via `go:embed`, runs as systemd service

## Architecture

```
Browser / Telegram Bot
        │
   SubForge (Go)          ← JWT auth, REST API, subscription endpoint
        │
   PostgreSQL
        │
   Node Agent (Go)        ← on each VPS
   ├── xray gRPC API
   └── hysteria2 HTTP API
```

## First Run

After install, create your admin account:

```bash
curl -X POST http://localhost:8080/api/setup \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}'
```

Then set `super_admin_setup: false` in `/etc/subforge/config.yaml` and restart:

```bash
systemctl restart subforge
```

Open `http://your-server:8080` in your browser.

## Hysteria2 Setup

SubForge acts as the auth backend for hysteria2. Add to your hysteria2 config:

```yaml
auth:
  type: http
  http:
    url: http://127.0.0.1:8080/internal/hy2/auth

trafficStats:
  listen: 127.0.0.1:11451
  secret: your-hy2-secret
```

## Telegram Bot

1. Create a bot via [@BotFather](https://t.me/BotFather), copy the token
2. Set in `/etc/subforge/config.yaml`:

```yaml
bot:
  enabled: true
  token: "123456:ABC..."
```

3. In the panel → Users → set your Telegram chat ID  
4. Send `/start` — you'll get the admin menu

## Build from Source

Requires: Go 1.23+, Node.js 18+, PostgreSQL 14+

```bash
git clone https://github.com/sssilverhand/subforge
cd subforge
make build           # current platform
make build-linux     # cross-compile for Linux amd64 + arm64
```

## Management

```bash
systemctl status subforge      # status
journalctl -u subforge -f      # logs
systemctl restart subforge     # restart

# Update to latest version
bash <(curl -Ls https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh)
```

## Disclaimer

This software is provided **as is**, without warranty of any kind. The authors are not responsible for any damages, data loss, security incidents, or legal issues arising from the use of this software. Use at your own risk.

## License

[GNU Affero General Public License v3.0](LICENSE)  
Forks and derivatives must remain open source. Commercial use is allowed, but modifications must be published under the same license.
