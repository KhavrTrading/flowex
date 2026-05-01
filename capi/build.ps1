#!/usr/bin/env pwsh
# Builds the flowex C-ABI shared library.
#
# Output: ../flowex.dll + ../flowex.h at repo root.
#
# Requirements: Go 1.22+, CGO_ENABLED=1, a C toolchain on PATH (MinGW-w64 gcc
# on Windows, clang/gcc on Linux/macOS).

param(
    [string]$OutDir = (Resolve-Path (Join-Path $PSScriptRoot ".."))
)

$ErrorActionPreference = "Stop"
$env:CGO_ENABLED = "1"

$ext = switch ($PSVersionTable.Platform) {
    "Unix" { if ($IsMacOS) { ".dylib" } else { ".so" } }
    default { ".dll" }
}

$out = Join-Path $OutDir "flowex$ext"

Push-Location (Join-Path $PSScriptRoot "..")
try {
    # -ldflags="-s -w" strips symbols + DWARF. Required on Windows + TDM-GCC 10:
    # without it the LoadLibrary call fails with ERROR_BAD_EXE_FORMAT (193).
    go build -buildmode=c-shared -ldflags="-s -w" -o $out ./capi
    Write-Host "built $out"
} finally {
    Pop-Location
}
