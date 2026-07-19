# Compatibility wrapper for older build instructions.
param([string]$Version = "dev")

& (Join-Path $PSScriptRoot "build.ps1") -Version $Version
