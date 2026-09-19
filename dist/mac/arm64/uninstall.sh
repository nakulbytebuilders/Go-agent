#!/usr/bin/env bash
# WinSentinel Monitoring Agent - macOS uninstaller.
set -uo pipefail

INSTALL_DIR="${WINSENTINEL_INSTALL_DIR:-$HOME/Library/Application Support/WinSentinel}"
LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
UID_NUM="$(id -u)"

echo "Uninstalling WinSentinel Monitoring Agent..."

launchctl bootout "gui/$UID_NUM" "$LAUNCH_AGENTS_DIR/com.winsentinel.watchdog.plist" 2>/dev/null || true
launchctl bootout "gui/$UID_NUM" "$LAUNCH_AGENTS_DIR/com.winsentinel.agent.plist" 2>/dev/null || true
launchctl unload -w "$LAUNCH_AGENTS_DIR/com.winsentinel.watchdog.plist" 2>/dev/null || true
launchctl unload -w "$LAUNCH_AGENTS_DIR/com.winsentinel.agent.plist" 2>/dev/null || true

rm -f "$LAUNCH_AGENTS_DIR/com.winsentinel.watchdog.plist"
rm -f "$LAUNCH_AGENTS_DIR/com.winsentinel.agent.plist"

rm -rf "$INSTALL_DIR"

echo "WinSentinel Monitoring Agent has been completely uninstalled."
