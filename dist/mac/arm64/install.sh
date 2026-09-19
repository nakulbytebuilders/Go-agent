#!/usr/bin/env bash
# WinSentinel Monitoring Agent - macOS installer.
# Runs as the current (non-root) user; registers autostart via LaunchAgent.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${WINSENTINEL_INSTALL_DIR:-$HOME/Library/Application Support/WinSentinel}"
SERVER_URL_DEFAULT="http://monitor-cloudd.test/api"

echo "=================================================="
echo "   WinSentinel Monitoring Agent - macOS Installer  "
echo "=================================================="

mkdir -p "$INSTALL_DIR/configs" "$INSTALL_DIR/data" "$INSTALL_DIR/logs"

cp "$SCRIPT_DIR/agent" "$INSTALL_DIR/agent"
cp "$SCRIPT_DIR/watchdog" "$INSTALL_DIR/watchdog"
chmod +x "$INSTALL_DIR/agent" "$INSTALL_DIR/watchdog"
cp "$SCRIPT_DIR/uninstall.sh" "$INSTALL_DIR/uninstall.sh"
chmod +x "$INSTALL_DIR/uninstall.sh"

# Remove the com.apple.quarantine flag so Gatekeeper doesn't block an
# unsigned binary that was extracted from a downloaded zip. If the agent is
# ever code-signed & notarized, this becomes a no-op.
xattr -dr com.apple.quarantine "$INSTALL_DIR/agent" 2>/dev/null || true
xattr -dr com.apple.quarantine "$INSTALL_DIR/watchdog" 2>/dev/null || true

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

MACHINE_NAME="$(scutil --get ComputerName 2>/dev/null || hostname)"
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

# Register the agent for autostart (writes + loads the LaunchAgent plist,
# via the agent's own -install flag, internal/autostart/autostart_darwin.go).
"$INSTALL_DIR/agent" -config "$CONFIG_PATH" -install

# Register the watchdog as its own LaunchAgent (mirrors the second registry
# entry the Windows Installer.exe writes for watchdog.exe).
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
mkdir -p "$LAUNCH_AGENTS_DIR"
WATCHDOG_PLIST="$LAUNCH_AGENTS_DIR/com.winsentinel.watchdog.plist"
cat > "$WATCHDOG_PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.winsentinel.watchdog</string>
	<key>ProgramArguments</key>
	<array>
		<string>$INSTALL_DIR/watchdog</string>
		<string>-config</string>
		<string>$CONFIG_PATH</string>
		<string>-agent</string>
		<string>$INSTALL_DIR/agent</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>
EOF

launchctl bootout "gui/$(id -u)" "$WATCHDOG_PLIST" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$WATCHDOG_PLIST" 2>/dev/null || launchctl load -w "$WATCHDOG_PLIST"

echo "=================================================="
echo " WinSentinel Monitoring Agent installed and running!"
echo "   User:    $EMPLOYEE_NAME"
echo "   Key:     $EMPLOYEE_ID"
echo "   Machine: $MACHINE_NAME"
echo ""
echo " IMPORTANT: macOS will ask you to grant 'Screen Recording' and"
echo " 'Accessibility' permission to the agent the first time it runs"
echo " (System Settings > Privacy & Security). These can't be granted"
echo " silently - please approve them for screenshots and window/app"
echo " tracking to work."
echo ""
echo "   To uninstall: \"$INSTALL_DIR/uninstall.sh\""
echo "=================================================="
