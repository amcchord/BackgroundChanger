<#
.SYNOPSIS
    Uninstalls the BgStatusService Windows service.

.DESCRIPTION
    This script removes the BgStatusService Windows service and optionally
    restores the original login screen background.

.NOTES
    Will automatically request Administrator privileges if needed.
#>

param(
    [switch]$KeepBackup
)

$ErrorActionPreference = "Stop"

$ServiceName = "BgStatusService"
$InstallDir = Join-Path $env:ProgramFiles "BgStatusService"
$DataDir = Join-Path $env:ProgramData "BgStatusService"
$BackupFile = Join-Path $DataDir "original_background.jpg"

# Check if running as administrator
$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "Requesting Administrator privileges..." -ForegroundColor Yellow
    
    # Build the argument list to pass to the elevated process
    $scriptPath = $MyInvocation.MyCommand.Path
    $argList = "-NoProfile -ExecutionPolicy Bypass -File `"$scriptPath`""
    
    if ($KeepBackup) {
        $argList += " -KeepBackup"
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
Write-Host "BgStatusService Uninstaller" -ForegroundColor Cyan
Write-Host "================================" -ForegroundColor Cyan
Write-Host ""

# Check if service exists
$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if (-not $service) {
    Write-Host "Service '$ServiceName' is not installed." -ForegroundColor Yellow
}
else {
    # Stop the service if running
    if ($service.Status -eq "Running") {
        Write-Host "Stopping service..." -ForegroundColor Cyan
        Stop-Service -Name $ServiceName -Force
        Start-Sleep -Seconds 2
    }

    # Remove the service
    Write-Host "Removing service..." -ForegroundColor Cyan
    sc.exe delete $ServiceName | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Service removed successfully." -ForegroundColor Green
    }
    else {
        Write-Host "Warning: Could not remove service. It may be removed after restart." -ForegroundColor Yellow
    }
}

# Restore original login screen from backup
if (Test-Path $BackupFile) {
    Write-Host ""
    Write-Host "Found original login screen backup." -ForegroundColor Cyan
    
    $restore = Read-Host "Would you like to restore the original login screen? (Y/n)"
    if ($restore -ne "n" -and $restore -ne "N") {
        Write-Host "Restoring original login screen..." -ForegroundColor Cyan
        
        # Try to restore via Group Policy registry
        $oobeDir = Join-Path $env:SystemRoot "System32\oobe\info\backgrounds"
        if (Test-Path $oobeDir) {
            $targetFile = Join-Path $oobeDir "backgroundDefault.jpg"
            try {
                Copy-Item -Path $BackupFile -Destination $targetFile -Force -ErrorAction Stop
                Write-Host "Restored to OOBE backgrounds folder." -ForegroundColor Green
            }
            catch {
                Write-Host "Note: Could not restore to OOBE folder (may require additional permissions)" -ForegroundColor Yellow
            }
        }
        
        # Also try to set via registry
        $regPath = "HKLM:\SOFTWARE\Policies\Microsoft\Windows\Personalization"
        if (Test-Path $regPath) {
            try {
                Set-ItemProperty -Path $regPath -Name "LockScreenImage" -Value $BackupFile -ErrorAction Stop
                Write-Host "Updated registry LockScreenImage." -ForegroundColor Green
            }
            catch {
                Write-Host "Note: Could not update registry (may require additional permissions)" -ForegroundColor Yellow
            }
        }
        
        Write-Host "Original login screen restored. Changes will take effect after restart." -ForegroundColor Yellow
    }
}

# Remove event log source
Write-Host "Removing event log source..." -ForegroundColor Cyan
try {
    $logExists = [System.Diagnostics.EventLog]::SourceExists($ServiceName)
    if ($logExists) {
        [System.Diagnostics.EventLog]::DeleteEventSource($ServiceName)
        Write-Host "Event log source removed." -ForegroundColor Green
    }
}
catch {
    Write-Host "Note: Could not remove event log source (non-critical)" -ForegroundColor Yellow
}

# Remove program files
if (Test-Path $InstallDir) {
    Write-Host "Removing program files from $InstallDir..." -ForegroundColor Cyan
    try {
        Remove-Item -Path $InstallDir -Recurse -Force
        Write-Host "Program files removed." -ForegroundColor Green
    }
    catch {
        Write-Host "Warning: Could not remove program files: $_" -ForegroundColor Yellow
    }
}

# Remove data directory (unless -KeepBackup is specified)
if (Test-Path $DataDir) {
    if ($KeepBackup) {
        Write-Host "Keeping backup data in $DataDir (use without -KeepBackup to remove)" -ForegroundColor Yellow
    }
    else {
        Write-Host "Removing data directory $DataDir..." -ForegroundColor Cyan
        try {
            Remove-Item -Path $DataDir -Recurse -Force
            Write-Host "Data directory removed." -ForegroundColor Green
        }
        catch {
            Write-Host "Warning: Could not remove data directory: $_" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "================================" -ForegroundColor Green
Write-Host "Uninstallation Complete!" -ForegroundColor Green
Write-Host "================================" -ForegroundColor Green
Write-Host ""
Write-Host "The BgStatusService has been removed from your system." -ForegroundColor White
Write-Host ""
if (-not $KeepBackup) {
    Write-Host "Note: To fully restore the login screen, you may need to restart your computer." -ForegroundColor Yellow
}

Write-Host ""
Read-Host "Press Enter to exit"
