// Package main implements a GUI installer for BgStatusService.
// The service executable is embedded in this installer for offline installation.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/backgroundchanger/cmd/installer/embed"
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
		// Recover from any panics and display error
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				errMsg := fmt.Sprintf("Unexpected error: %v\n\nPlease report this issue.", r)
				// Log stack trace to temp file for debugging
				logCrash(r, stackTrace)
				pw.SetComplete(false, errMsg)
			}
		}()

		// Give the UI a moment to fully initialize
		time.Sleep(100 * time.Millisecond)
		pw.ProcessMessages()

		// Step 1: Check existing installation
		pw.SetStatus("Checking existing installation...")
		pw.SetProgress(5)
		processMessagesWithDelay(pw, 300)

		// Check for old Windows service (with timeout protection)
		serviceExists := false
		serviceCheckDone := make(chan bool, 1)
		go func() {
			exists, _ := installer.ServiceExists()
			serviceExists = exists
			serviceCheckDone <- true
		}()
		
		select {
		case <-serviceCheckDone:
			// Success
		case <-time.After(15 * time.Second):
			pw.SetStatus("Warning: Service check timed out, continuing...")
			pw.ProcessMessages()
		}

		if serviceExists {
			pw.SetStatus("Removing old Windows service...")
			pw.SetProgress(10)
			processMessagesWithDelay(pw, 200)
			_ = installer.StopService()
			_ = installer.DeleteService()
		}

		// Check for existing scheduled tasks
		pw.SetStatus("Checking for existing scheduled tasks...")
		pw.SetProgress(12)
		pw.ProcessMessages()

		taskCheckDone := make(chan bool, 1)
		taskExists := false
		go func() {
			taskExists = installer.ScheduledTaskExists()
			taskCheckDone <- true
		}()

		select {
		case <-taskCheckDone:
			// Success
		case <-time.After(15 * time.Second):
			pw.SetStatus("Warning: Task check timed out, continuing...")
			pw.ProcessMessages()
		}

		if taskExists {
			pw.SetStatus("Removing existing scheduled tasks...")
			pw.SetProgress(15)
			processMessagesWithDelay(pw, 200)
			installer.DeleteScheduledTasks()
		}

		pw.SetProgress(20)

		// Step 2: Extract embedded service executable
		pw.SetStatus("Extracting service executable...")
		pw.SetProgress(25)
		pw.ProcessMessages()

		exePath, err := embed.ExtractServiceExe()
		if err != nil {
			pw.SetComplete(false, "Failed to extract service:\n"+err.Error())
			return
		}
		version := embed.Version
		defer os.Remove(exePath) // Clean up temp file

		pw.SetProgress(40)
		processMessagesWithDelay(pw, 100)

		// Step 3: Install scheduled tasks
		pw.SetStatus("Installing scheduled tasks...")
		pw.SetProgress(70)
		processMessagesWithDelay(pw, 200)

		err = installer.InstallScheduledTasks(exePath)
		if err != nil {
			pw.SetComplete(false, "Failed to install scheduled tasks:\n"+err.Error())
			return
		}

		// Step 4: Run the executable to generate initial image
		pw.SetStatus("Generating login screen image...")
		pw.SetProgress(85)
		processMessagesWithDelay(pw, 200)

		err = installer.RunExecutableDirectly()
		if err != nil {
			// Task installed but initial run failed - still mark as success
			pw.SetComplete(true, "Installed "+version+" (login screen will update on next boot)")
			return
		}

		// Step 5: Apply lock screen for current user
		pw.SetStatus("Applying lock screen...")
		pw.SetProgress(95)
		processMessagesWithDelay(pw, 500)

		// Find the latest loginscreen image and apply it via WinRT (runs as current user)
		applyErr := applyLockScreenAsUser()
		if applyErr != nil {
			// Task worked but WinRT failed - still success, will work on reboot
			pw.SetComplete(true, "Installed "+version+"! Login screen will update on next boot.")
			return
		}

		// Complete!
		pw.SetComplete(true, "Successfully installed "+version+"! Press Win+L to see your new login screen.")
	}()

	// Run message loop
	pw.RunMessageLoop()
}

// logCrash writes crash information to a temp file for debugging
func logCrash(err interface{}, stackTrace string) {
	tempDir := os.TempDir()
	logPath := tempDir + "\\bgstatus_crash.log"
	logContent := fmt.Sprintf("Time: %s\nError: %v\n\nStack Trace:\n%s", 
		time.Now().Format(time.RFC3339), err, stackTrace)
	os.WriteFile(logPath, []byte(logContent), 0644)
}

// runUninstall handles the uninstallation flow with a progress window
func runUninstall() {
	// Check if anything is installed (tasks or old service) with timeout
	serviceExists := false
	taskExists := false

	checkDone := make(chan bool, 1)
	go func() {
		serviceExists, _ = installer.ServiceExists()
		taskExists = installer.ScheduledTaskExists()
		checkDone <- true
	}()

	select {
	case <-checkDone:
		// Success
	case <-time.After(15 * time.Second):
		// Timeout - proceed with uninstall attempt anyway
	}

	if !serviceExists && !taskExists {
		installer.ShowInfo("Not Installed", "BgStatusService is not currently installed.")
		return
	}

	// Create progress window
	pw := installer.NewProgressWindow("BgStatusService Setup - Uninstalling")

	// Run uninstallation in a goroutine
	go func() {
		// Recover from any panics and display error
		defer func() {
			if r := recover(); r != nil {
				stackTrace := string(debug.Stack())
				errMsg := fmt.Sprintf("Unexpected error: %v\n\nPlease report this issue.", r)
				logCrash(r, stackTrace)
				pw.SetComplete(false, errMsg)
			}
		}()

		// Give the UI a moment to fully initialize
		time.Sleep(100 * time.Millisecond)
		pw.ProcessMessages()

		// Step 1: Remove scheduled tasks
		pw.SetStatus("Removing scheduled tasks...")
		pw.SetProgress(15)
		processMessagesWithDelay(pw, 300)

		installer.DeleteScheduledTasks()

		// Step 2: Remove old Windows service if present
		if serviceExists {
			pw.SetStatus("Removing old Windows service...")
			pw.SetProgress(25)
			processMessagesWithDelay(pw, 300)

			_ = installer.StopService()
			_ = installer.DeleteService()
		}

		// Step 3: Remove event log source
		pw.SetStatus("Cleaning up...")
		pw.SetProgress(40)
		processMessagesWithDelay(pw, 200)
		installer.RemoveEventLogSource()

		// Step 4: Remove files
		pw.SetStatus("Removing installation files...")
		pw.SetProgress(55)
		processMessagesWithDelay(pw, 300)

		_ = installer.RemoveInstallation()

		// Step 5: Remove data directory
		pw.SetStatus("Removing data directory...")
		pw.SetProgress(70)
		processMessagesWithDelay(pw, 200)

		_ = installer.RemoveDataDirectory()

		// Step 6: Clean registry (restore original background)
		pw.SetStatus("Restoring original login screen...")
		pw.SetProgress(85)
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
