# Markmiru build script (Windows / PowerShell).
#
# Go-only build: the app is Go-SSR (Wails AssetServer.Handler). There is no frontend
# to install or bundle. wails build compiles the Go backend and packages the exe;
# -clean wipes build/bin so no stale artifact survives. Go's content-addressed cache tracks source.
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
# Name: Markmiru-windows-<arch>-<sha>-<yyyymmdd>.zip
#   - <arch>      = Go's name for the architecture (amd64 / arm64), same on all three OSes
#   - <sha>       = same git short SHA used as the version above (with -dirty if applicable)
#   - <yyyymmdd>  = the date this ZIP is produced
# dist/ is git-ignored (distributables are not committed). Runs only on a successful build
# (a build failure throws above and stops the script before reaching here).
#
# go env GOARCH returns the GOARCH environment variable when set, otherwise the host value.
# That is the same rule wails build uses to pick its target (cmd/wails/flags/build.go), so the
# name always matches what was actually built.
$arch = (& go env GOARCH | Out-String).Trim()
if (-not $arch) { throw 'go env GOARCH returned nothing' }
$dateStamp = Get-Date -Format 'yyyyMMdd'
$distDir = Join-Path $root 'dist'
New-Item -ItemType Directory -Force -Path $distDir | Out-Null
$zipPath = Join-Path $distDir "Markmiru-windows-$arch-$sha-$dateStamp.zip"
if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
Compress-Archive -Path (Join-Path $root 'build\bin\Markmiru.exe') -DestinationPath $zipPath
Write-Host "Packaged: $zipPath"
