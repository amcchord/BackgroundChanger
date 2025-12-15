// Package main implements a GUI installer for BgStatusService.
// It downloads the latest version from GitHub and installs/uninstalls it as a Windows service.
package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/backgroundchanger/internal/installer"
)

var (
	shell32          = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = shell32.NewProc("ShellExecuteW")
)

func main() {
	// Check if running as administrator
	if !isAdmin() {
		// Re-launch with elevation
		if !elevate() {
			installer.ShowError("BgStatusService Setup", "Administrator privileges are required to install the service.")
		}
		return
	}

	// Show main menu
	choice := installer.AskInstallOrUninstall()

	switch choice {
	case installer.ChoiceInstall:
		runInstall()
	case installer.ChoiceUninstall:
		runUninstall()
	case installer.ChoiceCancel:
		// User cancelled, just exit
		return
	}
}

// isAdmin checks if the current process has administrator privileges
func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}

	return member
}

// elevate re-launches the current process with administrator privileges
func elevate() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}

	verb, _ := syscall.UTF16PtrFromString("runas")
	exePath, _ := syscall.UTF16PtrFromString(exe)
	cwd, _ := syscall.UTF16PtrFromString("")
	args, _ := syscall.UTF16PtrFromString("")

	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exePath)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(cwd)),
		1, // SW_SHOWNORMAL
	)

	return ret > 32
}

// runInstall handles the installation flow with a progress window
func runInstall() {
	// Create progress window
	pw := installer.NewProgressWindow("BgStatusService Setup - Installing")

	// Run installation in a goroutine so we can update the UI
	go func() {
		var installError error
		var version string

		// Step 1: Check existing installation
		pw.SetStatus("Checking existing installation...")
		pw.SetProgress(5)
		processMessagesWithDelay(pw, 300)

		exists, err := installer.ServiceExists()
		if err != nil {
			installError = err
			pw.SetComplete(false, "Error: Failed to check service status")
			return
		}

		// Step 2: Stop and remove existing service if it exists
		if exists {
			pw.SetStatus("Stopping existing service...")
			pw.SetProgress(15)
			processMessagesWithDelay(pw, 200)

			_ = installer.StopService() // Ignore errors, service might not be running

			pw.SetStatus("Removing old service...")
			pw.SetProgress(25)
			processMessagesWithDelay(pw, 200)

			if err := installer.DeleteService(); err != nil {
				installError = err
				pw.SetComplete(false, "Error: Failed to remove existing service")
				return
			}
		} else {
			pw.SetProgress(25)
		}

		// Step 3: Download latest version with progress
		pw.SetStatus("Connecting to GitHub...")
		pw.SetProgress(35)
		pw.ProcessMessages()

		exePath, ver, err := installer.DownloadLatestServiceWithProgress(func(status string, percent int) {
			pw.SetStatus(status)
			pw.SetProgress(percent)
			pw.ProcessMessages()
		})
		if err != nil {
			installError = err
			pw.SetComplete(false, "Error: Failed to download - "+err.Error())
			return
		}
		version = ver
		defer os.Remove(exePath) // Clean up temp file

		// Step 4: Install service
		pw.SetStatus("Installing service...")
		pw.SetProgress(70)
		processMessagesWithDelay(pw, 200)

		err = installer.InstallService(exePath)
		if err != nil {
			installError = err
			pw.SetComplete(false, "Error: Failed to install service")
			return
		}

		// Step 5: Start service (creates the image)
		pw.SetStatus("Starting service...")
		pw.SetProgress(85)
		processMessagesWithDelay(pw, 200)

		err = installer.StartService()
		if err != nil {
			// Service installed but failed to start - still mark as success
			pw.SetComplete(true, "Installed "+version+" (service will start at next boot)")
			return
		}

		// Step 6: Wait for image to be created and apply it as user
		pw.SetStatus("Applying lock screen...")
		pw.SetProgress(95)
		processMessagesWithDelay(pw, 500) // Give service time to create image

		// Find the latest loginscreen image and apply it via WinRT (runs as current user)
		applyErr := applyLockScreenAsUser()
		if applyErr != nil {
			// Service worked but WinRT failed - still success, will work on reboot
			pw.SetComplete(true, "Installed "+version+"! Lock screen will update on next sign-in.")
			return
		}

		// Complete!
		if installError == nil {
			pw.SetComplete(true, "Successfully installed "+version+"! Press Win+L to see your new login screen.")
		}
	}()

	// Run message loop
	pw.RunMessageLoop()
}

// runUninstall handles the uninstallation flow with a progress window
func runUninstall() {
	// Check if service is installed first
	exists, err := installer.ServiceExists()
	if err != nil {
		installer.ShowError("Error", "Failed to check service status")
		return
	}

	if !exists {
		installer.ShowInfo("Not Installed", "BgStatusService is not currently installed.")
		return
	}

	// Create progress window
	pw := installer.NewProgressWindow("BgStatusService Setup - Uninstalling")

	// Run uninstallation in a goroutine
	go func() {
		// Step 1: Stop service
		pw.SetStatus("Stopping service...")
		pw.SetProgress(15)
		processMessagesWithDelay(pw, 300)

		_ = installer.StopService() // Ignore errors

		// Step 2: Remove service
		pw.SetStatus("Removing service...")
		pw.SetProgress(35)
		processMessagesWithDelay(pw, 300)

		if err := installer.DeleteService(); err != nil {
			pw.SetComplete(false, "Error: Failed to remove service")
			return
		}

		// Step 3: Remove event log source
		installer.RemoveEventLogSource()

		// Step 4: Remove files
		pw.SetStatus("Removing installation files...")
		pw.SetProgress(55)
		processMessagesWithDelay(pw, 300)

		_ = installer.RemoveInstallation() // Ignore errors

		// Step 5: Remove data directory
		pw.SetStatus("Removing data directory...")
		pw.SetProgress(65)
		processMessagesWithDelay(pw, 200)

		_ = installer.RemoveDataDirectory() // Ignore errors

		// Step 6: Clean registry (restore original background)
		pw.SetStatus("Restoring original login screen...")
		pw.SetProgress(80)
		processMessagesWithDelay(pw, 200)

		restoreOriginalBackground()

		// Complete!
		pw.SetProgress(100)
		pw.SetComplete(true, "Uninstalled successfully! Your login screen will be restored after a restart.")
	}()

	// Run message loop
	pw.RunMessageLoop()
}

// restoreOriginalBackground removes the custom login screen registry entries
func restoreOriginalBackground() {
	// Remove PersonalizationCSP registry entries
	cmd := exec.Command("reg", "delete",
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\PersonalizationCSP`,
		"/v", "LockScreenImagePath", "/f")
	cmd.Run()

	cmd = exec.Command("reg", "delete",
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\PersonalizationCSP`,
		"/v", "LockScreenImageStatus", "/f")
	cmd.Run()

	cmd = exec.Command("reg", "delete",
		`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\PersonalizationCSP`,
		"/v", "LockScreenImageUrl", "/f")
	cmd.Run()
}

// processMessagesWithDelay processes window messages and adds a small delay
func processMessagesWithDelay(pw *installer.ProgressWindow, delayMs int) {
	// Process any pending messages
	pw.ProcessMessages()
	// Add delay so user can see the progress
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	pw.ProcessMessages()
}

// applyLockScreenAsUser finds the latest loginscreen image and applies it via WinRT
// This runs as the current user (not SYSTEM) so WinRT works properly
func applyLockScreenAsUser() error {
	// Find the latest loginscreen_*.jpg file
	dataDir := installer.GetDataDir()
	imagePath, err := findLatestLoginScreenImage(dataDir)
	if err != nil {
		return err
	}

	// Run PowerShell WinRT command to set lock screen
	psScript := `
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
Function Await($WinRtTask, $ResultType) {
    $asTask = $asTaskGeneric.MakeGenericMethod($ResultType)
    $netTask = $asTask.Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
    $netTask.Result
}
Function AwaitAction($WinRtTask) {
    $asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and !$_.IsGenericMethod })[0]
    $netTask = $asTask.Invoke($null, @($WinRtTask))
    $netTask.Wait(-1) | Out-Null
}
[Windows.System.UserProfile.LockScreen,Windows.System.UserProfile,ContentType=WindowsRuntime] | Out-Null
[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime] | Out-Null
$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync('` + imagePath + `')) ([Windows.Storage.StorageFile])
AwaitAction ([Windows.System.UserProfile.LockScreen]::SetImageFileAsync($file))
`

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	return cmd.Run()
}

// findLatestLoginScreenImage finds the most recent loginscreen_*.jpg in the data directory
func findLatestLoginScreenImage(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	var latestPath string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Look for loginscreen_*.jpg files
		if len(name) > 12 && name[:12] == "loginscreen_" && name[len(name)-4:] == ".jpg" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestPath = dir + "\\" + name
			}
		}
	}

	if latestPath == "" {
		return "", os.ErrNotExist
	}
	return latestPath, nil
}
