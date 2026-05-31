#!/usr/bin/env bash
# SubForge installer
# Usage: bash <(curl -Ls https://raw.githubusercontent.com/sssilverhand/subforge/main/install.sh)

set -euo pipefail

REPO="sssilverhand/subforge"
INSTALL_DIR="/opt/subforge"
CONFIG_DIR="/etc/subforge"
SERVICE_NAME="subforge"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; NC='\033[0m'

info()  { echo -e "${CYAN}[SubForge]${NC} $*"; }
ok()    { echo -e "${GREEN}[✓]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
die()   { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

# ─── Checks ──────────────────────────────────────────────────────────────────

[[ $EUID -eq 0 ]] || die "Run as root: sudo bash install.sh"
[[ "$(uname -s)" == "Linux" ]] || die "Linux only"

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  *) die "Unsupported architecture: $ARCH" ;;
esac

# ─── Package manager ─────────────────────────────────────────────────────────

if command -v apt-get &>/dev/null; then
  PKG_INSTALL="apt-get install -y -q"
  PKG_UPDATE="apt-get update -qq"
elif command -v yum &>/dev/null; then
  PKG_INSTALL="yum install -y -q"
  PKG_UPDATE="yum check-update -q || true"
else
  die "Unsupported package manager (need apt or yum)"
fi

# ─── Install PostgreSQL ───────────────────────────────────────────────────────

install_postgres() {
  if command -v psql &>/dev/null; then
    ok "PostgreSQL already installed"
    return
  fi
  info "Installing PostgreSQL..."
  $PKG_UPDATE
  $PKG_INSTALL postgresql postgresql-contrib
  systemctl enable --now postgresql
  ok "PostgreSQL installed"
}

setup_postgres() {
  local DB_PASS
  DB_PASS=$(openssl rand -hex 16)

  sudo -u postgres psql -c "CREATE USER subforge WITH PASSWORD '$DB_PASS';" 2>/dev/null || true
  sudo -u postgres psql -c "CREATE DATABASE subforge OWNER subforge;" 2>/dev/null || true
  sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE subforge TO subforge;" 2>/dev/null || true

  echo "postgres://subforge:${DB_PASS}@localhost:5432/subforge?sslmode=disable"
}

# ─── Download binary ─────────────────────────────────────────────────────────

latest_version() {
  curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | cut -d'"' -f4
}

download_binary() {
  local VERSION="$1"
  local BIN_URL="https://github.com/${REPO}/releases/download/${VERSION}/subforge-linux-${ARCH}"

  info "Downloading SubForge ${VERSION} (${ARCH})..."
  mkdir -p "$INSTALL_DIR"
  curl -fL "$BIN_URL" -o "${INSTALL_DIR}/subforge"
  chmod +x "${INSTALL_DIR}/subforge"
  ok "Binary downloaded to ${INSTALL_DIR}/subforge"
}

# ─── Config ──────────────────────────────────────────────────────────────────

generate_config() {
  local DSN="$1"
  local EXTERNAL_URL="$2"
  local JWT_SECRET
  JWT_SECRET=$(openssl rand -hex 32)

  mkdir -p "$CONFIG_DIR"
  cat > "${CONFIG_DIR}/config.yaml" <<EOF
server:
  host: "127.0.0.1"
  port: 8080
  external_url: "${EXTERNAL_URL}"

database:
  dsn: "${DSN}"

auth:
  jwt_secret: "${JWT_SECRET}"
  token_expiry: "24h"
  super_admin_setup: true

bot:
  enabled: false
  token: ""

scheduler:
  traffic_poll_interval: "60s"
  expiry_check_interval: "5m"
EOF
  chmod 600 "${CONFIG_DIR}/config.yaml"
  ok "Config written to ${CONFIG_DIR}/config.yaml"
}

# ─── Systemd ─────────────────────────────────────────────────────────────────

install_service() {
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=SubForge VPN Subscription Manager
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/subforge -config ${CONFIG_DIR}/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=subforge

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME"
  systemctl restart "$SERVICE_NAME"
  ok "Service ${SERVICE_NAME}.service installed and started"
}

# ─── Main ─────────────────────────────────────────────────────────────────────

echo ""
echo -e "${CYAN}  ⚡ SubForge Installer${NC}"
echo "  VPN Subscription Manager"
echo ""

# Ask for external URL
read -rp "  Enter your domain (e.g. https://sub.example.com): " EXTERNAL_URL
[[ -n "$EXTERNAL_URL" ]] || die "Domain is required"
# Strip trailing slash
EXTERNAL_URL="${EXTERNAL_URL%/}"

VERSION=$(latest_version)
[[ -n "$VERSION" ]] || die "Could not fetch latest version from GitHub"
info "Latest version: $VERSION"

install_postgres
DSN=$(setup_postgres)
download_binary "$VERSION"
generate_config "$DSN" "$EXTERNAL_URL"
install_service

# ─── Done ─────────────────────────────────────────────────────────────────────

PUBLIC_IP=$(curl -fsSL https://api.ipify.org 2>/dev/null || echo "your-server-ip")

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}  ✓ SubForge ${VERSION} installed successfully!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo "  Admin panel:  http://${PUBLIC_IP}:8080"
echo "  Config:       ${CONFIG_DIR}/config.yaml"
echo ""
echo "  Next step — create your admin account:"
echo ""
echo -e "  ${CYAN}curl -X POST http://localhost:8080/api/setup \\${NC}"
echo -e "  ${CYAN}    -H 'Content-Type: application/json' \\${NC}"
echo -e "  ${CYAN}    -d '{\"username\":\"admin\",\"password\":\"YOUR_PASSWORD\"}'${NC}"
echo ""
echo "  Then set ${YELLOW}super_admin_setup: false${NC} in config and:"
echo "  systemctl restart subforge"
echo ""
echo "  Logs: journalctl -u subforge -f"
echo ""
