#!/usr/bin/env bash
# WinSentinel Monitoring Agent - Linux installer.
# Runs as the current (non-root) user; registers autostart via systemd --user.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${WINSENTINEL_INSTALL_DIR:-$HOME/.local/share/winsentinel}"
SERVER_URL_DEFAULT="http://monitor-cloudd.test/api"

echo "=================================================="
echo "   WinSentinel Monitoring Agent - Linux Installer  "
echo "=================================================="

if ! command -v xdotool >/dev/null 2>&1 || ! command -v xprintidle >/dev/null 2>&1; then
    echo "[INFO] Optional dependencies missing (xdotool/xprintidle)."
    echo "       Active-window and idle-time tracking need them. Attempting install..."
    if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update -qq && sudo apt-get install -y xdotool xprintidle || true
    elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y xdotool xprintidle || true
    elif command -v pacman >/dev/null 2>&1; then
        sudo pacman -Sy --noconfirm xdotool xprintidle || true
    else
        echo "[WARN] Could not detect package manager. Install xdotool + xprintidle manually for full tracking."
    fi
fi

mkdir -p "$INSTALL_DIR/configs" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"

cp "$SCRIPT_DIR/agent" "$INSTALL_DIR/agent"
cp "$SCRIPT_DIR/watchdog" "$INSTALL_DIR/watchdog"
chmod +x "$INSTALL_DIR/agent" "$INSTALL_DIR/watchdog"
cp "$SCRIPT_DIR/uninstall.sh" "$INSTALL_DIR/uninstall.sh"
chmod +x "$INSTALL_DIR/uninstall.sh"

SERVER_URL="$SERVER_URL_DEFAULT"
EMPLOYEE_ID=""
EMPLOYEE_NAME="Employee"

if [ -f "$SCRIPT_DIR/installer_config.json" ]; then
    SERVER_URL=$(sed -n 's/.*"server_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$SCRIPT_DIR/installer_config.json" | head -n1)
    EMPLOYEE_ID=$(sed -n 's/.*"employee_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$SCRIPT_DIR/installer_config.json" | head -n1)
    EMPLOYEE_NAME=$(sed -n 's/.*"employee_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$SCRIPT_DIR/installer_config.json" | head -n1)
    [ -z "$SERVER_URL" ] && SERVER_URL="$SERVER_URL_DEFAULT"
fi

if [ -z "$EMPLOYEE_ID" ]; then
    read -rp "Enter Server API URL (default: $SERVER_URL_DEFAULT): " input_url
    SERVER_URL="${input_url:-$SERVER_URL_DEFAULT}"
    read -rp "Enter Employee Key / Connection ID: " EMPLOYEE_ID
fi

MACHINE_NAME="$(hostname)"
CONFIG_PATH="$INSTALL_DIR/configs/agent.yaml"

cat > "$CONFIG_PATH" <<EOF
server:
  api_url: "$SERVER_URL"
  heartbeat_interval_sec: 15
  employee_id: "$EMPLOYEE_ID"
  machine_name: "$MACHINE_NAME"

web_server:
  enabled: true
  host: "0.0.0.0"
  port: 8080
  auto_open: false

database:
  path: "data/agent.db"
  max_open_conns: 1
  max_idle_conns: 1

logger:
  dir: "logs"
  level: "info"
  max_size_mb: 10
  max_backups: 5
  max_age_days: 30
  compress: true

app_tracker:
  enabled: true
  poll_interval_sec: 1

browser_tracker:
  enabled: true
  poll_interval_sec: 1

screenshot:
  enabled: true
  interval_sec: 30
  quality: 80
  storage_dir: "data/screenshots"

input:
  enabled: true
  poll_interval_sec: 1
  idle_threshold_sec: 60

sync:
  enabled: true
  interval_sec: 5
  batch_size: 50
EOF

echo "[INFO] Config written to $CONFIG_PATH"

# Register the agent for autostart (writes + enables the systemd --user unit
# via the agent's own -install flag, internal/autostart/autostart_linux.go).
"$INSTALL_DIR/agent" -config "$CONFIG_PATH" -install

# Register the watchdog as its own systemd --user unit (mirrors the second
# registry entry the Windows Installer.exe writes for watchdog.exe).
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"
mkdir -p "$SYSTEMD_USER_DIR"
cat > "$SYSTEMD_USER_DIR/winsentinel-watchdog.service" <<EOF
[Unit]
Description=WinSentinel Monitoring Agent Watchdog
After=graphical-session.target

[Service]
Type=simple
ExecStart="$INSTALL_DIR/watchdog" -config "$CONFIG_PATH" -agent "$INSTALL_DIR/agent"
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now winsentinel-agent.service
systemctl --user enable --now winsentinel-watchdog.service

echo "=================================================="
echo " WinSentinel Monitoring Agent installed and running!"
echo "   User:    $EMPLOYEE_NAME"
echo "   Key:     $EMPLOYEE_ID"
echo "   Machine: $MACHINE_NAME"
echo "   To uninstall: $INSTALL_DIR/uninstall.sh"
echo "=================================================="
