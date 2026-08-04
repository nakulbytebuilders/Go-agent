# How to Release a New Version of WinSentinel Agent

### Step 1: Bump Version in Go-agent
In `internal/services/updater/updater.go`, update the `CurrentVersion` constant:
```go
const CurrentVersion = "1.0.1"
```

### Step 2: Build Binaries & Tag GitHub Release
Run `build.bat` in `Go-agent` root directory:
```cmd
build.bat
```

Commit changes and push a new Release Tag:
```bash
git add .
git commit -m "release: bump agent version to v1.0.1"
git push origin master

# Trigger GitHub Release Workflow
git tag v1.0.1
git push origin v1.0.1
```

---

### Step 3: Update Live Web Server (.env)
On the live web server (`monitor-cloudd`), update `.env`:
```env
AGENT_LATEST_VERSION=1.0.1
```

Clear Laravel config cache:
```bash
php artisan config:clear
```

---

### 🔄 How Auto-Update Works:
- New users downloading from the web dashboard get the new version installer.
- Existing installed agents query `/api/agents/check-update` every 1 hour, detect `update_available: true`, silently download `v1.0.1` in the background, and restart.
