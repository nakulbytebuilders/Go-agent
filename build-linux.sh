#!/usr/bin/env bash
# Cross-compiles the Linux agent + watchdog binaries and copies the raw
# files (agent, watchdog, install.sh, uninstall.sh) next to the Windows
# downloads. monitor-cloudd zips them on demand per-download, the same way
# it already does for Installer.exe/uninstaller.exe (routes/web.php) - no
# pre-built archive needed here.
# Does not touch build.bat / the Windows build in any way.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

echo "=================================================="
echo "   Building Linux Agent Binaries                   "
echo "=================================================="

echo "Building agent (linux/amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/linux/agent ./cmd/agent

echo "Building watchdog (linux/amd64)..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/linux/watchdog ./cmd/watchdog

cp packaging/linux/install.sh dist/linux/install.sh
cp packaging/linux/uninstall.sh dist/linux/uninstall.sh
chmod +x dist/linux/install.sh dist/linux/uninstall.sh dist/linux/agent dist/linux/watchdog

for downloads_dir in "../../web/monitor-cloudd/public/downloads" "/c/Projects/web/monitor-cloudd/public/downloads"; do
    if [ -d "$downloads_dir" ]; then
        mkdir -p "$downloads_dir/linux"
        cp dist/linux/agent dist/linux/watchdog dist/linux/install.sh dist/linux/uninstall.sh "$downloads_dir/linux/"
    fi
done

echo "Done. Output: dist/linux/{agent,watchdog,install.sh,uninstall.sh}"
