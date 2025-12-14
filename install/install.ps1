<#
.SYNOPSIS
    Installs the BgStatusService Windows service.

.DESCRIPTION
    This script installs bgStatusService.exe as a Windows service that runs at boot
    to display system information on the login screen.

.NOTES
    Will automatically request Administrator privileges if needed.
#>

param(
    [string]$ExePath = ".\bgStatusService.exe"
)

$ErrorActionPreference = "Stop"

$ServiceName = "BgStatusService"
$DisplayName = "Background Status Service"
$Description = "Displays system information on the Windows login screen background."
$InstallDir = Join-Path $env:ProgramFiles "BgStatusService"

# Check if running as administrator
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Requesting Administrator privileges..." -ForegroundColor Yellow
    
    # Build the argument list to pass to the elevated process
    $scriptPath = $MyInvocation.MyCommand.Path
    $argList = "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`""
    
    if ($ExePath -ne ".\bgStatusService.exe") {
        $argList += " -ExePath `"$ExePath`""
    }
    
    # Re-launch with elevation
    try {
        Start-Process PowerShell.exe -Verb RunAs -ArgumentList $argList -Wait
    }
    catch {
        Write-Host "ERROR: Failed to elevate privileges. Please run as Administrator." -ForegroundColor Red
        exit 1
    }
    exit 0
}

Write-Host "================================" -ForegroundColor Cyan
Write-Host "BgStatusService Installer" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# Find the executable
if (-not (Test-Path $ExePath)) {
    # Try looking in the parent directory
    $ParentExePath = Join-Path (Split-Path $PSScriptRoot -Parent) "bgStatusService.exe"
    if (Test-Path $ParentExePath) {
        $ExePath = $ParentExePath
    }
    else {
        Write-Host "ERROR: Could not find bgStatusService.exe" -ForegroundColor Red
        Write-Host "Please ensure the executable is in the current directory or specify the path with -ExePath" -ForegroundColor Yellow
        Read-Host "Press Enter to exit"
        exit 1
    }
}

$ExePath = Resolve-Path $ExePath
Write-Host "Found executable: $ExePath" -ForegroundColor Green

# Check if service already exists
$existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($existingService) {
    Write-Host "Service already exists. Updating..." -ForegroundColor Yellow
    
    # Stop the service if running
    if ($existingService.Status -eq "Running") {
        Write-Host "Stopping existing service..." -ForegroundColor Yellow
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }
    
    # Remove the existing service
    Write-Host "Removing existing service..." -ForegroundColor Yellow
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
}

# Create installation directory
Write-Host "Creating installation directory..." -ForegroundColor Cyan
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# Copy executable to installation directory
Write-Host "Copying executable to $InstallDir..." -ForegroundColor Cyan
Copy-Item -Path $ExePath -Destination (Join-Path $InstallDir "bgStatusService.exe") -Force

$ServiceExePath = Join-Path $InstallDir "bgStatusService.exe"

# Create the Windows service
Write-Host "Creating Windows service..." -ForegroundColor Cyan

# Use sc.exe to create the service with more control over settings
$scArgs = @(
    "create"
    $ServiceName
    "binPath=$ServiceExePath"
    "DisplayName=$DisplayName"
    "start=auto"
    "obj=LocalSystem"
)

$result = & sc.exe $scArgs 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Failed to create service: $result" -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# Set the service description
sc.exe description $ServiceName $Description | Out-Null

# Configure the service to not restart on failure (since it's a one-shot service)
sc.exe failure $ServiceName reset=0 actions=none | Out-Null

# Create the data directory for backups
$DataDir = Join-Path $env:ProgramData "BgStatusService"
if (-not (Test-Path $DataDir)) {
    Write-Host "Creating data directory: $DataDir" -ForegroundColor Cyan
    New-Item -ItemType Directory -Path $DataDir -Force | Out-Null
}

# Register event log source
Write-Host "Registering event log source..." -ForegroundColor Cyan
try {
    $logExists = [System.Diagnostics.EventLog]::SourceExists($ServiceName)
    if (-not $logExists) {
        [System.Diagnostics.EventLog]::CreateEventSource($ServiceName, "Application")
    }
}
catch {
    Write-Host "Note: Could not register event log source (non-critical)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "================================" -ForegroundColor Green
Write-Host "Installation Complete!" -ForegroundColor Green
Write-Host "================================" -ForegroundColor Green
Write-Host ""
Write-Host "Service Name: $ServiceName" -ForegroundColor White
Write-Host "Install Path: $ServiceExePath" -ForegroundColor White
Write-Host "Data Path:    $DataDir" -ForegroundColor White
Write-Host ""
Write-Host "The service will run automatically at the next boot." -ForegroundColor Yellow
Write-Host ""
Write-Host "To run immediately (for testing):" -ForegroundColor Cyan
Write-Host "  Start-Service $ServiceName" -ForegroundColor White
Write-Host ""
Write-Host "To run in interactive mode (for debugging):" -ForegroundColor Cyan
Write-Host "  & '$ServiceExePath'" -ForegroundColor White
Write-Host ""

# Ask if user wants to start the service now
$response = Read-Host "Would you like to start the service now? (y/N)"
if ($response -eq "y" -or $response -eq "Y") {
    Write-Host "Starting service..." -ForegroundColor Cyan
    Start-Service -Name $ServiceName
    Start-Sleep -Seconds 3
    
    $service = Get-Service -Name $ServiceName
    if ($service.Status -eq "Running" -or $service.Status -eq "Stopped") {
        Write-Host "Service executed successfully!" -ForegroundColor Green
        Write-Host "Lock your screen (Win+L) or restart to see the changes." -ForegroundColor Yellow
    }
    else {
        Write-Host "Service status: $($service.Status)" -ForegroundColor Yellow
    }
}

Write-Host ""
Read-Host "Press Enter to exit"
