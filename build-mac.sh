#!/usr/bin/env bash
# Cross-compiles the macOS agent + watchdog binaries (Intel + Apple Silicon)
# and copies the raw files next to the Windows/Linux downloads.
# monitor-cloudd zips them on demand per-download, same as Windows/Linux.
# Does not touch build.bat / build-linux.sh / the other builds in any way.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

echo "=================================================="
echo "   Building macOS Agent Binaries                   "
echo "=================================================="

for arch in amd64 arm64; do
    echo "Building agent (darwin/$arch)..."
    GOOS=darwin GOARCH=$arch go build -ldflags="-s -w" -o dist/mac/$arch/agent ./cmd/agent

    echo "Building watchdog (darwin/$arch)..."
    GOOS=darwin GOARCH=$arch go build -ldflags="-s -w" -o dist/mac/$arch/watchdog ./cmd/watchdog

    cp packaging/mac/install.sh dist/mac/$arch/install.sh
    cp packaging/mac/uninstall.sh dist/mac/$arch/uninstall.sh
    cp packaging/mac/Install.command dist/mac/$arch/Install.command
    rm -rf dist/mac/$arch/Install.app
    cp -R packaging/mac/Install.app dist/mac/$arch/Install.app
    chmod +x dist/mac/$arch/install.sh dist/mac/$arch/uninstall.sh dist/mac/$arch/Install.command dist/mac/$arch/agent dist/mac/$arch/watchdog
    chmod +x dist/mac/$arch/Install.app/Contents/MacOS/Install
done

for downloads_dir in "../../web/monitor-cloudd/public/downloads" "/c/Projects/web/monitor-cloudd/public/downloads"; do
    if [ -d "$downloads_dir" ]; then
        for arch in amd64 arm64; do
            mkdir -p "$downloads_dir/mac/$arch"
            cp dist/mac/$arch/agent dist/mac/$arch/watchdog dist/mac/$arch/install.sh dist/mac/$arch/uninstall.sh dist/mac/$arch/Install.command "$downloads_dir/mac/$arch/"
            rm -rf "$downloads_dir/mac/$arch/Install.app"
            cp -R dist/mac/$arch/Install.app "$downloads_dir/mac/$arch/Install.app"
        done
    fi
done

echo "Done. Output: dist/mac/{amd64,arm64}/{agent,watchdog,install.sh,uninstall.sh,Install.command,Install.app}"
