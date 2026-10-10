# How to Release a New Version of WinSentinel Agent

Auto-update runs off one pair of files on the web server (`monitor-cloudd`):

- `public/downloads/agent.exe`, the binary agents download, and
- `public/downloads/agent-manifest.json`, `{"version": "1.0.6", "sha256": "..."}`, written by `build.bat`.

`build.bat` reads the version from the built `agent.exe` itself (`agent.exe -version`) and hashes that same file, so the
manifest always describes the binary next to it. **Deploying the pair is the whole release. There is no `.env` step.**
If the two ever disagree (an `agent.exe` replaced without its manifest, or the other way round), the server offers
nothing and logs a warning, instead of offering an update agents would refuse or repeat.

### Step 1: Bump the version, then build
In `internal/services/updater/updater.go`, update the `CurrentVersion` constant
**before** building (the version is compiled into the binary):
```go
const CurrentVersion = "1.0.7"
```
Never reuse a version number that already has a release or tag: two different binaries called 1.0.5 can never update
each other.

Run `.\build.bat` in PowerShell (or `cmd /c build.bat`). It builds everything, writes `dist\agent-manifest.json`, and
copies `agent.exe`, `agent-manifest.json`, `Installer.exe` and `uninstaller.exe` into
`monitor-cloudd\public\downloads`. Check the result:
```powershell
Get-Content ..\..\web\monitor-cloudd\public\downloads\agent-manifest.json   # the version you just set
```

### Step 2: Commit and deploy
In the Go repo, commit and tag (the tag triggers the GitHub Release workflow, which builds `Installer.exe` etc. for new
installs):
```bash
git add .
git commit -m "release: v1.0.7"
git push origin master
git tag v1.0.7
git push origin v1.0.7
```
In `monitor-cloudd`, commit the files `build.bat` copied to `public/downloads` (`agent.exe`, `agent-manifest.json`,
`Installer.exe`, `uninstaller.exe`) and deploy. Each online agent picks the release up within an hour.

### Pausing a rollout
Set `AGENT_UPDATES_ENABLED=false` in the web server's `.env` (and redeploy, or run `php artisan config:cache`).
Every agent is then told there is no update. Set it back to `true` (or remove it) to resume.

---

### 🔄 How Auto-Update Works:
- New users downloading from the web dashboard get the new version installer.
- Existing installed agents query `/api/agents/check-update` every hour. The server answers from the manifest. If the
  version is newer, the agent downloads `/downloads/agent-binary` and refuses it unless (1) it hashes to the SHA-256 the
  server sent, and (2) running it with `-version` prints the offered version. Then it swaps `agent.exe`, keeps the old
  one as `agent_old.exe`, and restarts.
- A refused update is reported to the dashboard (Device Issues) and is not downloaded again until the server offers
  something different.
- HTTPS certificates are verified, except for development hosts (`localhost`, `*.test`, `*.local`). To override, set
  `insecure_skip_verify: true|false` under `server:` in `agent.yaml`.
- Only `agent.exe` is replaced. `svc.exe`, `watchdog.exe` and `ui.exe` change only by reinstalling with the new
  `Installer.exe`.
- Agents older than 1.0.6 do not check what they download: they install whatever the server offers. Make sure the
  manifest and `agent.exe` you deploy are the ones you mean before an old fleet sees them.

### Checking which version a machine runs
- Dashboard: Administration → Users shows `Agent vX.Y.Z` under each machine, and flags machines behind the published
  version. (Agents older than 1.0.6 show "Agent version not reported".)
- On the machine: `agent.exe -version` (1.0.6 and newer), or `agent.log`, which has
  `Agent is up to date ... current_version` after every hourly check.
