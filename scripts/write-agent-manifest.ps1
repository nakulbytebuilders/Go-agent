# Writes agent-manifest.json for a built agent.exe:
#
#   {"version":"1.0.6","sha256":"<hex sha-256 of agent.exe>"}
#
# The version is whatever the binary itself prints for `agent.exe -version`, and the hash is of that same file, so
# the manifest cannot describe a binary it does not belong to. The web server (monitor-cloudd) announces agent updates
# from this file, next to the agent.exe it serves: deploying the pair is the whole release, and a manifest that does not
# match the binary next to it makes the server offer nothing.
#
# Usage: write-agent-manifest.ps1 -Agent agent.exe -Out dist\agent-manifest.json[,other\agent-manifest.json]

param(
    [Parameter(Mandatory = $true)][string]$Agent,
    [Parameter(Mandatory = $true)][string[]]$Out
)

$ErrorActionPreference = 'Stop'

$agentPath = (Resolve-Path -LiteralPath $Agent).Path

# agent.exe is a GUI program (built with -H=windowsgui), so run it with stdout redirected and read the pipe.
$psi = New-Object System.Diagnostics.ProcessStartInfo $agentPath, '-version'
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true
$psi.UseShellExecute = $false
$psi.CreateNoWindow = $true
$process = [System.Diagnostics.Process]::Start($psi)
$version = $process.StandardOutput.ReadToEnd().Trim()
$process.WaitForExit()

if ($process.ExitCode -ne 0 -or $version -notmatch '^\d+\.\d+\.\d+$') {
    throw "'$agentPath -version' printed '$version' (exit code $($process.ExitCode)); expected a version like 1.0.6"
}

$sha256 = (Get-FileHash -LiteralPath $agentPath -Algorithm SHA256).Hash.ToLowerInvariant()
$json = '{"version":"' + $version + '","sha256":"' + $sha256 + '"}'

# UTF-8 without a byte order mark: PHP's json_decode rejects a BOM.
$utf8 = New-Object System.Text.UTF8Encoding $false
foreach ($path in $Out) {
    $dir = Split-Path -Parent $path
    if ($dir -and -not (Test-Path -LiteralPath $dir)) {
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
    }
    [System.IO.File]::WriteAllText($path, $json, $utf8)
    Write-Host "agent-manifest: $version $sha256 -> $path"
}
