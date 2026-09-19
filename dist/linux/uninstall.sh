#!/usr/bin/env bash
# WinSentinel Monitoring Agent - Linux uninstaller.
set -uo pipefail

INSTALL_DIR="${WINSENTINEL_INSTALL_DIR:-$HOME/.local/share/winsentinel}"
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"

echo "Uninstalling WinSentinel Monitoring Agent..."

systemctl --user disable --now winsentinel-watchdog.service 2>/dev/null || true
systemctl --user disable --now winsentinel-agent.service 2>/dev/null || true

rm -f "$SYSTEMD_USER_DIR/winsentinel-watchdog.service"
rm -f "$SYSTEMD_USER_DIR/winsentinel-agent.service"
systemctl --user daemon-reload 2>/dev/null || true

rm -rf "$INSTALL_DIR"

echo "WinSentinel Monitoring Agent has been completely uninstalled."
