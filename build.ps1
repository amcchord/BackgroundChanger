param(
    [string]$Version = "dev",
    [string]$Commit = "unknown"
)

$ErrorActionPreference = "Stop"
$projectRoot = $PSScriptRoot
$distDir = Join-Path $projectRoot "dist"
$output = Join-Path $distDir "BackgroundChangerSetup.exe"

if ($Commit -eq "unknown") {
    $Commit = (git rev-parse --short HEAD 2>$null)
    if (-not $Commit) { $Commit = "unknown" }
}

New-Item -ItemType Directory -Path $distDir -Force | Out-Null

Write-Host "Formatting and testing..." -ForegroundColor Cyan
gofmt -w (Get-ChildItem -Path $projectRoot -Filter "*.go" -Recurse | Select-Object -ExpandProperty FullName)
go test ./...
if ($LASTEXITCODE -ne 0) { throw "Tests failed" }

Write-Host "Building BackgroundChangerSetup.exe..." -ForegroundColor Cyan
$ldflags = "-H=windowsgui -s -w -X github.com/amcchord/BackgroundChanger/internal/buildinfo.Version=$Version -X github.com/amcchord/BackgroundChanger/internal/buildinfo.Commit=$Commit"
go build -trimpath -ldflags $ldflags -o $output ./cmd/backgroundchanger
if ($LASTEXITCODE -ne 0) { throw "Build failed" }

$hash = (Get-FileHash -Algorithm SHA256 $output).Hash.ToLowerInvariant()
"$hash  BackgroundChangerSetup.exe" | Set-Content -Encoding ascii (Join-Path $distDir "SHA256SUMS.txt")

$size = [math]::Round((Get-Item $output).Length / 1MB, 2)
Write-Host "Built $output ($size MB)" -ForegroundColor Green
Write-Host "SHA256 $hash"
