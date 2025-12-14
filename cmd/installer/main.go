// Package main implements a GUI installer for BgStatusService.
// It downloads the latest version from GitHub and installs/uninstalls it as a Windows service.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
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

// runInstall handles the installation flow
func runInstall() {
	// Check if service already exists
	exists, err := installer.ServiceExists()
	if err != nil {
		installer.ShowError("Error", fmt.Sprintf("Failed to check service status:\n%v", err))
		return
	}

	if exists {
		// Ask if user wants to upgrade
		if !installer.AskYesNo("Service Already Installed",
			"BgStatusService is already installed.\n\n"+
				"Would you like to upgrade to the latest version?") {
			return
		}

		// Stop and remove existing service
		installer.ShowInfo("Upgrading", "Stopping existing service...")

		if err := installer.StopService(); err != nil {
			installer.ShowWarning("Warning", fmt.Sprintf("Could not stop service:\n%v\n\nContinuing anyway...", err))
		}

		if err := installer.DeleteService(); err != nil {
			installer.ShowError("Error", fmt.Sprintf("Failed to remove existing service:\n%v", err))
			return
		}
	}

	// Download the latest version
	installer.ShowInfo("Downloading", "Downloading the latest version from GitHub...\n\nThis may take a moment.")

	exePath, version, err := installer.DownloadLatestService()
	if err != nil {
		installer.ShowError("Download Failed", fmt.Sprintf("Failed to download the latest version:\n%v", err))
		return
	}

	// Install the service
	err = installer.InstallService(exePath)
	if err != nil {
		installer.ShowError("Installation Failed", fmt.Sprintf("Failed to install service:\n%v", err))
		// Clean up downloaded file
		os.Remove(exePath)
		return
	}

	// Clean up downloaded temp file
	os.Remove(exePath)

	// Ask if user wants to start the service now
	startNow := installer.AskYesNo("Installation Complete",
		fmt.Sprintf("BgStatusService %s has been installed successfully!\n\n"+
			"The service will run automatically at next boot.\n\n"+
			"Would you like to run it now?\n"+
			"(You can then press Win+L to see the result)", version))

	if startNow {
		err = installer.StartService()
		if err != nil {
			installer.ShowWarning("Warning", fmt.Sprintf("Service installed but failed to start:\n%v\n\n"+
				"The service will run at next boot.", err))
			return
		}

		installer.ShowInfo("Success", "Service started successfully!\n\n"+
			"Press Win+L (lock screen) to see your login screen with system info.")
	} else {
		installer.ShowInfo("Success", "Installation complete!\n\n"+
			"The service will run at next boot.\n"+
			"You can also start it manually from Services (services.msc).")
	}
}

// runUninstall handles the uninstallation flow
func runUninstall() {
	// Check if service exists
	exists, err := installer.ServiceExists()
	if err != nil {
		installer.ShowError("Error", fmt.Sprintf("Failed to check service status:\n%v", err))
		return
	}

	if !exists {
		installer.ShowInfo("Not Installed", "BgStatusService is not currently installed.")
		return
	}

	// Confirm uninstallation
	if !installer.AskYesNo("Confirm Uninstall",
		"Are you sure you want to uninstall BgStatusService?\n\n"+
			"This will:\n"+
			"• Stop and remove the Windows service\n"+
			"• Remove installed files") {
		return
	}

	// Stop the service
	installer.ShowInfo("Uninstalling", "Stopping service...")

	if err := installer.StopService(); err != nil {
		installer.ShowWarning("Warning", fmt.Sprintf("Could not stop service:\n%v\n\nContinuing anyway...", err))
	}

	// Delete the service
	if err := installer.DeleteService(); err != nil {
		installer.ShowError("Error", fmt.Sprintf("Failed to remove service:\n%v", err))
		return
	}

	// Remove event log source
	installer.RemoveEventLogSource()

	// Ask about removing data (original background backup)
	removeData := installer.AskYesNo("Remove Data?",
		"Would you like to remove the data directory?\n\n"+
			"This includes the backup of your original login screen background.\n"+
			"If you keep it, the original background will remain modified.\n\n"+
			"Remove data directory?")

	// Remove installation files
	if err := installer.RemoveInstallation(); err != nil {
		installer.ShowWarning("Warning", fmt.Sprintf("Some files could not be removed:\n%v", err))
	}

	// Optionally remove data directory
	if removeData {
		if err := installer.RemoveDataDirectory(); err != nil {
			installer.ShowWarning("Warning", fmt.Sprintf("Data directory could not be removed:\n%v", err))
		}
	}

	// Offer to restore original background
	restoreOriginal := installer.AskYesNo("Restore Original Background?",
		"Would you like to try to restore the original login screen background?\n\n"+
			"This will run the restore command to reset your login screen.")

	if restoreOriginal {
		// Try to restore the original background using reg.exe
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

		installer.ShowInfo("Uninstall Complete",
			"BgStatusService has been uninstalled.\n\n"+
				"The registry entries for the custom login screen have been removed.\n"+
				"Your original login screen should be restored after a restart.")
	} else {
		installer.ShowInfo("Uninstall Complete",
			"BgStatusService has been uninstalled.\n\n"+
				"Note: Your login screen background may still show the modified image.\n"+
				"Restart your computer to see any changes.")
	}
}

