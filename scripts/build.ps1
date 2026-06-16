# Markmiru build script (Windows / PowerShell).
#
# Produces a fully fresh build where the frontend, the Go backend, and the final
# executable all reflect the CURRENT source state in one shot:
#   - The previously generated frontend bundle (frontend/dist) is removed first, so the
#     embedded assets (//go:embed all:frontend/dist) cannot carry over stale files.
#   - wails build then re-runs "npm run build" to regenerate frontend/dist from source,
#     and -clean wipes build/bin so no old artifact is left behind.
#   - Go is recompiled by wails build (its content-addressed cache always tracks source).
#
# Embeds the git short SHA as the version:
#   - Runtime (shown at the end of the "About Markmiru" tab): Go ldflags (-X main.version=<sha>)
#   - OS properties (Explorer > Properties > Details > Product version):
#       temporarily injected into wails.json "info.productVersion".
# wails.json is always restored after the build (no SHA diff is left in the repo).
#
# NOTE: keep this file ASCII-only. PowerShell 5.1 misreads BOM-less UTF-8 with
#       multibyte comments, which can break command parsing. ASCII avoids that.
#
# Usage (from the repo root):  & .\scripts\build.ps1
$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
$wailsJson = Join-Path $root 'wails.json'
$wails = Join-Path $env:USERPROFILE 'go\bin\wails.exe'

# Verify the active Node/npm match the versions pinned in frontend/package.json (volta field).
# With Volta installed these are selected automatically; without it this stops early with a
# clear message instead of a cryptic engine-strict error during the frontend install.
$pkg = Get-Content (Join-Path $root 'frontend\package.json') -Raw | ConvertFrom-Json
$wantNode = $pkg.volta.node
$wantNpm = $pkg.volta.npm
# Probe node/npm from inside frontend/ so the check reflects the toolchain the frontend
# install actually uses. Volta (and other cwd-based managers) pick the version from the
# nearest package.json carrying a volta/engines pin, which lives in frontend/ (not the repo root).
Push-Location (Join-Path $root 'frontend')
try {
  $haveNode = ((& node -v) -replace '^v', '').Trim()
  $haveNpm = (& npm -v).Trim()
}
finally {
  Pop-Location
}
if ($haveNode.Split('.')[0] -ne $wantNode.Split('.')[0] -or $haveNpm.Split('.')[0] -ne $wantNpm.Split('.')[0]) {
  throw "Node $wantNode / npm $wantNpm required (found node $haveNode / npm $haveNpm). Install Volta (https://volta.sh) so the pinned versions are used automatically, or install matching versions manually."
}

# Version = git short SHA, with "-dirty" if the working tree is not clean.
$sha = (& git rev-parse --short HEAD | Out-String).Trim()
if (-not $sha) { throw 'git rev-parse failed (empty SHA)' }
if (& git status --porcelain) { $sha = "$sha-dirty" }
Write-Host "Markmiru version: $sha"

# [System.IO.File]::WriteAllText (2-arg) writes UTF-8 without BOM (Go's JSON parser dislikes a BOM).
$pvRegex = '("productVersion"\s*:\s*)"[^"]*"'
$original = [System.IO.File]::ReadAllText($wailsJson)
try {
  # Replace only the value of info.productVersion (keep the rest of the file as-is).
  $patched = [regex]::Replace($original, $pvRegex, '${1}"' + $sha + '"')
  [System.IO.File]::WriteAllText($wailsJson, $patched)

  # Remove the previously generated frontend bundle so embedded assets are always rebuilt
  # from the current source (wails build re-runs "npm run build" to regenerate it).
  $dist = Join-Path $root 'frontend\dist'
  if (Test-Path $dist) { Remove-Item -Recurse -Force $dist }

  # -clean wipes build/bin first so no stale executable/artifact survives the build.
  & $wails build -clean -ldflags "-X main.version=$sha"
  if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }
}
finally {
  # Always reset productVersion back to "dev" (self-healing even if a previous run was killed
  # mid-build and left a SHA behind), while preserving any other edits in the file.
  $restored = [regex]::Replace($original, $pvRegex, '${1}"dev"')
  [System.IO.File]::WriteAllText($wailsJson, $restored)
}

# Package the executable into a distributable ZIP under dist/ (created if missing).
# Distribution policy: ship a simple ZIP containing just the .exe (Windows).
# Name: Markmiru-<sha>-windows-amd64-<yyyymmdd>.zip
#   - <sha>       = same git short SHA used as the version above (with -dirty if applicable)
#   - <yyyymmdd>  = the date this ZIP is produced
# dist/ is git-ignored (distributables are not committed). Runs only on a successful build
# (a build failure throws above and stops the script before reaching here).
$dateStamp = Get-Date -Format 'yyyyMMdd'
$distDir = Join-Path $root 'dist'
New-Item -ItemType Directory -Force -Path $distDir | Out-Null
$zipPath = Join-Path $distDir "Markmiru-$sha-windows-amd64-$dateStamp.zip"
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path (Join-Path $root 'build\bin\Markmiru.exe') -DestinationPath $zipPath
Write-Host "Packaged: $zipPath"
