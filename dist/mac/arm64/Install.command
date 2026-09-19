#!/usr/bin/env bash
# Double-click launcher: macOS Finder runs .command files in Terminal
# automatically (unlike .sh, which just opens in the default text editor).
cd "$(dirname "${BASH_SOURCE[0]}")" || exit 1

chmod +x install.sh uninstall.sh agent watchdog 2>/dev/null
xattr -dr com.apple.quarantine . 2>/dev/null || true

./install.sh

echo ""
read -n 1 -s -r -p "Done. Press any key to close this window..."
echo ""
