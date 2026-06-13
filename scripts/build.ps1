# Markmiru build script (Windows / PowerShell).
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

  & $wails build -ldflags "-X main.version=$sha"
  if ($LASTEXITCODE -ne 0) { throw "wails build failed (exit $LASTEXITCODE)" }
}
finally {
  # Always reset productVersion back to "dev" (self-healing even if a previous run was killed
  # mid-build and left a SHA behind), while preserving any other edits in the file.
  $restored = [regex]::Replace($original, $pvRegex, '${1}"dev"')
  [System.IO.File]::WriteAllText($wailsJson, $restored)
}
