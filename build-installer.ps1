# Build script for BgStatusService Installer
# This script builds the service executable and embeds it into the installer

param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"

$ProjectRoot = $PSScriptRoot
$EmbedDir = Join-Path $ProjectRoot "cmd\installer\embed"
$ServiceExe = Join-Path $ProjectRoot "bgStatusService.exe"
$InstallerExe = Join-Path $ProjectRoot "bgStatusServiceSetup.exe"
$EmbedExe = Join-Path $EmbedDir "bgStatusService.exe"
$EmbedGo = Join-Path $EmbedDir "embed.go"

Write-Host "=== BgStatusService Installer Build ===" -ForegroundColor Cyan
Write-Host ""

# Step 1: Build the service executable
Write-Host "[1/4] Building bgStatusService.exe..." -ForegroundColor Yellow
go build -o $ServiceExe ./cmd/statusservice
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to build bgStatusService.exe" -ForegroundColor Red
    exit 1
}
Write-Host "      Built successfully" -ForegroundColor Green

# Step 2: Copy service exe to embed directory
Write-Host "[2/4] Copying service exe to embed directory..." -ForegroundColor Yellow
if (-not (Test-Path $EmbedDir)) {
    New-Item -ItemType Directory -Path $EmbedDir -Force | Out-Null
}
Copy-Item $ServiceExe $EmbedExe -Force
Write-Host "      Copied successfully" -ForegroundColor Green

# Step 3: Update version in embed.go
Write-Host "[3/4] Updating embedded version to '$Version'..." -ForegroundColor Yellow
$embedContent = Get-Content $EmbedGo -Raw
$embedContent = $embedContent -replace 'var Version = "[^"]*"', "var Version = `"$Version`""
Set-Content $EmbedGo -Value $embedContent -NoNewline
Write-Host "      Version updated" -ForegroundColor Green

# Step 4: Build the installer
Write-Host "[4/4] Building bgStatusServiceSetup.exe..." -ForegroundColor Yellow
go build -o $InstallerExe ./cmd/installer
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to build installer" -ForegroundColor Red
    exit 1
}
Write-Host "      Built successfully" -ForegroundColor Green

# Summary
Write-Host ""
Write-Host "=== Build Complete ===" -ForegroundColor Cyan
$installerSize = [math]::Round((Get-Item $InstallerExe).Length / 1MB, 2)
$serviceSize = [math]::Round((Get-Item $ServiceExe).Length / 1MB, 2)
Write-Host "  Service:   $serviceSize MB ($ServiceExe)"
Write-Host "  Installer: $installerSize MB ($InstallerExe)"
Write-Host "  Version:   $Version"
Write-Host ""
Write-Host "The installer is now self-contained and works offline!" -ForegroundColor Green

