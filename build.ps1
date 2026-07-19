param(
    [string]$Version = "dev",
    [string]$Commit = "unknown"
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$distDir = Join-Path $projectRoot "dist"
$output = Join-Path $distDir "WallpaperIdentitySetup.exe"
$cliOutput = Join-Path $distDir "WallpaperIdentityCLI.exe"

if ($Commit -eq "unknown") {
    $Commit = (git rev-parse --short HEAD 2>$null)
    if (-not $Commit) { $Commit = "unknown" }
}

New-Item -ItemType Directory -Path $distDir -Force | Out-Null

Write-Host "Formatting and testing..." -ForegroundColor Cyan
gofmt -w (Get-ChildItem -Path $projectRoot -Filter "*.go" -Recurse | Select-Object -ExpandProperty FullName)
go test ./...
if ($LASTEXITCODE -ne 0) { throw "Tests failed" }

Write-Host "Building WallpaperIdentitySetup.exe..." -ForegroundColor Cyan
$versionFlags = "-s -w -X github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo.Version=$Version -X github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo.Commit=$Commit"
go build -trimpath -ldflags "-H=windowsgui $versionFlags" -o $output ./cmd/wallpaperidentity
if ($LASTEXITCODE -ne 0) { throw "Build failed" }

Write-Host "Building WallpaperIdentityCLI.exe..." -ForegroundColor Cyan
go build -trimpath -ldflags $versionFlags -o $cliOutput ./cmd/wallpaperidentitycli
if ($LASTEXITCODE -ne 0) { throw "CLI build failed" }

$setupHash = (Get-FileHash -Algorithm SHA256 $output).Hash.ToLowerInvariant()
$cliHash = (Get-FileHash -Algorithm SHA256 $cliOutput).Hash.ToLowerInvariant()
@(
    "$setupHash  WallpaperIdentitySetup.exe"
    "$cliHash  WallpaperIdentityCLI.exe"
) | Set-Content -Encoding ascii (Join-Path $distDir "SHA256SUMS.txt")

$setupSize = [math]::Round((Get-Item $output).Length / 1MB, 2)
$cliSize = [math]::Round((Get-Item $cliOutput).Length / 1MB, 2)
Write-Host "Built $output ($setupSize MB)" -ForegroundColor Green
Write-Host "SHA256 $setupHash"
Write-Host "Built $cliOutput ($cliSize MB)" -ForegroundColor Green
Write-Host "SHA256 $cliHash"
