package main

import (
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows API constants
const (
	SPI_SETDESKWALLPAPER       = 0x0014
	SPI_SETLOCKSCREENWALLPAPER = 0x0115
	SPIF_UPDATEINIFILE         = 0x01
	SPIF_SENDCHANGE            = 0x02
)

// Supported image extensions
var supportedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".bmp":  true,
}

// isAdmin checks if the current process is running with administrator privileges
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
	isMember, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return isMember
}

// runElevated re-launches the current process with administrator privileges
func runElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	// Build arguments string (skip the first arg which is the program name)
	args := ""
	if len(os.Args) > 1 {
		args = strings.Join(os.Args[1:], " ")
	}

	// Convert strings to UTF16 for Windows API
	verb, _ := syscall.UTF16PtrFromString("runas")
	exePath, _ := syscall.UTF16PtrFromString(exe)
	argsPtr, _ := syscall.UTF16PtrFromString(args)
	workDir, _ := syscall.UTF16PtrFromString("")

	// ShellExecute with "runas" verb to trigger UAC
	ret, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exePath)),
		uintptr(unsafe.Pointer(argsPtr)),
		uintptr(unsafe.Pointer(workDir)),
		1, // SW_SHOWNORMAL
	)

	// ShellExecute returns > 32 on success
	if ret <= 32 {
		return fmt.Errorf("ShellExecute failed with code %d", ret)
	}

	return nil
}

// setLoginScreenViaWinRT sets the lock/login screen using PowerShell and the Windows Runtime API
func setLoginScreenViaWinRT(absPath string) error {
	// PowerShell script to use Windows Runtime LockScreen API
	// This is the official Windows 10/11 way to set lock screen images
	psScript := fmt.Sprintf(`
$ErrorActionPreference = "Stop"

# Load Windows Runtime assemblies
Add-Type -AssemblyName System.Runtime.WindowsRuntime

# Helper function to await async operations
$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object { $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`+"`"+`1' })[0]

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

# Load the LockScreen and StorageFile types
[Windows.System.UserProfile.LockScreen,Windows.System.UserProfile,ContentType=WindowsRuntime] | Out-Null
[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime] | Out-Null

# Get the image file
$imagePath = '%s'
$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])

# Set the lock screen image
AwaitAction ([Windows.System.UserProfile.LockScreen]::SetImageFileAsync($file))

Write-Host "Lock screen image set successfully via WinRT API"
`, absPath)

	// Run PowerShell with execution policy bypass
	cmd := exec.Command("powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", psScript,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PowerShell WinRT failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("- WinRT output: %s\n", strings.TrimSpace(string(output)))
	return nil
}

// setLoginScreenViaGroupPolicy sets the login screen using Group Policy registry keys
func setLoginScreenViaGroupPolicy(absPath string) error {
	// Open or create the Personalization policy key
	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Policies\Microsoft\Windows\Personalization`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("failed to open Personalization policy key: %v", err)
	}
	defer key.Close()

	// Set LockScreenImage to the image path
	err = key.SetStringValue("LockScreenImage", absPath)
	if err != nil {
		return fmt.Errorf("failed to set LockScreenImage: %v", err)
	}

	// Also need to ensure DisableLogonBackgroundImage is set to 0 in the System key
	sysKey, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Policies\Microsoft\Windows\System`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("failed to open System policy key: %v", err)
	}
	defer sysKey.Close()

	// Set DisableLogonBackgroundImage to 0 (enable custom background)
	err = sysKey.SetDWordValue("DisableLogonBackgroundImage", 0)
	if err != nil {
		return fmt.Errorf("failed to set DisableLogonBackgroundImage: %v", err)
	}

	fmt.Println("- Group Policy registry keys set successfully")
	return nil
}

// Sets the desktop wallpaper using Windows API
func setDesktopWallpaper(path string) error {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW").Call(
		uintptr(SPI_SETDESKWALLPAPER),
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(SPIF_UPDATEINIFILE|SPIF_SENDCHANGE),
	)

	if err != nil && err != syscall.Errno(0) {
		return err
	}
	return nil
}

// Sets the lock screen wallpaper for Windows 10/11
func setLockScreenWallpaper(path string) error {
	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	// Try all methods one by one, continuing if one fails
	methods := []struct {
		name string
		fn   func(string) error
	}{
		{"Registry (HKCU)", setLockScreenWallpaperViaRegistry},
		{"Assets folder", setLockScreenWallpaperViaAssets},
		{"System Data folder", setLockScreenWallpaperViaSystemData},
		{"Registry (HKLM)", setLockScreenWallpaperViaHKLM},
	}

	var anySuccess bool
	var lastError error
	for _, method := range methods {
		fmt.Printf("Trying method: %s\n", method.name)
		err := method.fn(absPath)
		if err != nil {
			fmt.Printf("- Method failed: %v\n", err)
			lastError = err
		} else {
			fmt.Printf("- Method succeeded\n")
			anySuccess = true
		}
	}

	// If all methods failed, return the last error
	if !anySuccess {
		return fmt.Errorf("all methods failed, last error: %v", lastError)
	}

	return nil
}

// Sets the login screen background (sign-in screen) for Windows 10/11
func setLoginScreenBackground(path string) error {
	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	fmt.Println("Setting login screen background using modern methods...")

	// Try methods in order of reliability
	// 1. WinRT API via PowerShell (works on all Windows 10/11 editions)
	// 2. Group Policy registry (works on Pro/Enterprise)
	methods := []struct {
		name string
		fn   func(string) error
	}{
		{"Windows Runtime API (PowerShell)", setLoginScreenViaWinRT},
		{"Group Policy Registry", setLoginScreenViaGroupPolicy},
	}

	var anySuccess bool
	var lastError error
	for _, method := range methods {
		fmt.Printf("Trying method: %s\n", method.name)
		err := method.fn(absPath)
		if err != nil {
			fmt.Printf("- Method failed: %v\n", err)
			lastError = err
		} else {
			fmt.Printf("- Method succeeded\n")
			anySuccess = true
		}
	}

	// If all methods failed, return the last error
	if !anySuccess {
		return fmt.Errorf("all login screen methods failed, last error: %v", lastError)
	}

	return nil
}

// Sets lock screen wallpaper using registry
func setLockScreenWallpaperViaRegistry(absPath string) error {
	// Create a key for the lock screen
	keyPathPtr, err := syscall.UTF16PtrFromString("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\PersonalizationCSP")
	if err != nil {
		return err
	}

	key, _, err := syscall.NewLazyDLL("advapi32.dll").NewProc("RegCreateKeyExW").Call(
		uintptr(syscall.HKEY_CURRENT_USER),
		uintptr(unsafe.Pointer(keyPathPtr)),
		0,
		0,
		0,
		uintptr(syscall.KEY_WRITE),
		0,
		0,
		0,
	)
	if err != nil && err != syscall.Errno(0) {
		return err
	}
	defer syscall.RegCloseKey(syscall.Handle(key))

	// Set the LockScreenImagePath value
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return err
	}

	valueNamePtr, err := syscall.UTF16PtrFromString("LockScreenImagePath")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key,
		uintptr(unsafe.Pointer(valueNamePtr)),
		0,
		uintptr(syscall.REG_SZ),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(2*(len(absPath)+1)),
	)
	if err != nil && err != syscall.Errno(0) {
		return err
	}

	// Set the LockScreenImageStatus value
	statusPtr, err := syscall.UTF16PtrFromString("1")
	if err != nil {
		return err
	}

	statusNamePtr, err := syscall.UTF16PtrFromString("LockScreenImageStatus")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key,
		uintptr(unsafe.Pointer(statusNamePtr)),
		0,
		uintptr(syscall.REG_SZ),
		uintptr(unsafe.Pointer(statusPtr)),
		uintptr(4),
	)
	if err != nil && err != syscall.Errno(0) {
		return err
	}

	return nil
}

// Sets lock screen wallpaper by copying to the Assets folder
func setLockScreenWallpaperViaAssets(absPath string) error {
	// Get user's local app data path
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return fmt.Errorf("could not determine LOCALAPPDATA path")
	}

	// Create the destination directory if it doesn't exist
	assetsDir := filepath.Join(localAppData, "Packages", "Microsoft.Windows.ContentDeliveryManager_cw5n1h2txyewy", "LocalState", "Assets")
	err := os.MkdirAll(assetsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create assets directory: %v", err)
	}

	// Generate a unique destination filename
	destFile := filepath.Join(assetsDir, fmt.Sprintf("LockScreen_%d%s", time.Now().UnixNano(), filepath.Ext(absPath)))

	// Copy the image file to the assets directory
	sourceData, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read source image: %v", err)
	}

	err = os.WriteFile(destFile, sourceData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write to destination: %v", err)
	}

	// Try also the direct Windows API method
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return err
	}

	_, _, _ = syscall.NewLazyDLL("user32.dll").NewProc("SystemParametersInfoW").Call(
		uintptr(SPI_SETLOCKSCREENWALLPAPER),
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(SPIF_UPDATEINIFILE|SPIF_SENDCHANGE),
	)

	// Don't return error from this call as it may not be supported on all Windows versions

	return nil
}

// Sets lock screen wallpaper via HKEY_LOCAL_MACHINE (requires admin privileges)
func setLockScreenWallpaperViaHKLM(absPath string) error {
	// Disable logon background image
	systemKeyPtr, err := syscall.UTF16PtrFromString("SOFTWARE\\Policies\\Microsoft\\Windows\\System")
	if err != nil {
		return err
	}

	key, _, err := syscall.NewLazyDLL("advapi32.dll").NewProc("RegCreateKeyExW").Call(
		uintptr(syscall.HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(systemKeyPtr)),
		0,
		0,
		0,
		uintptr(syscall.KEY_WRITE),
		0,
		0,
		0,
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to open HKLM System key: %v", err)
	}
	defer syscall.RegCloseKey(syscall.Handle(key))

	// Set DisableLogonBackgroundImage to 0
	valPtr, err := syscall.UTF16PtrFromString("0")
	if err != nil {
		return err
	}

	disableLogonPtr, err := syscall.UTF16PtrFromString("DisableLogonBackgroundImage")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key,
		uintptr(unsafe.Pointer(disableLogonPtr)),
		0,
		uintptr(syscall.REG_DWORD),
		uintptr(unsafe.Pointer(valPtr)),
		uintptr(4),
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to set DisableLogonBackgroundImage: %v", err)
	}

	// Now set the PersonalizationCSP keys in HKEY_LOCAL_MACHINE
	personalizationPtr, err := syscall.UTF16PtrFromString("SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\PersonalizationCSP")
	if err != nil {
		return err
	}

	key2, _, err := syscall.NewLazyDLL("advapi32.dll").NewProc("RegCreateKeyExW").Call(
		uintptr(syscall.HKEY_LOCAL_MACHINE),
		uintptr(unsafe.Pointer(personalizationPtr)),
		0,
		0,
		0,
		uintptr(syscall.KEY_WRITE),
		0,
		0,
		0,
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to open HKLM PersonalizationCSP key: %v", err)
	}
	defer syscall.RegCloseKey(syscall.Handle(key2))

	// Set LockScreenImagePath
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return err
	}

	lockScreenPathPtr, err := syscall.UTF16PtrFromString("LockScreenImagePath")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key2,
		uintptr(unsafe.Pointer(lockScreenPathPtr)),
		0,
		uintptr(syscall.REG_SZ),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(2*(len(absPath)+1)),
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to set LockScreenImagePath: %v", err)
	}

	// Set LockScreenImageUrl
	lockScreenUrlPtr, err := syscall.UTF16PtrFromString("LockScreenImageUrl")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key2,
		uintptr(unsafe.Pointer(lockScreenUrlPtr)),
		0,
		uintptr(syscall.REG_SZ),
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(2*(len(absPath)+1)),
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to set LockScreenImageUrl: %v", err)
	}

	// Set LockScreenImageStatus
	statusPtr, err := syscall.UTF16PtrFromString("1")
	if err != nil {
		return err
	}

	lockScreenStatusPtr, err := syscall.UTF16PtrFromString("LockScreenImageStatus")
	if err != nil {
		return err
	}

	_, _, err = syscall.NewLazyDLL("advapi32.dll").NewProc("RegSetValueExW").Call(
		key2,
		uintptr(unsafe.Pointer(lockScreenStatusPtr)),
		0,
		uintptr(syscall.REG_DWORD),
		uintptr(unsafe.Pointer(statusPtr)),
		uintptr(4),
	)
	if err != nil && err != syscall.Errno(0) {
		return fmt.Errorf("failed to set LockScreenImageStatus: %v", err)
	}

	return nil
}

// Sets lock screen wallpaper by copying to the SystemData folder
func setLockScreenWallpaperViaSystemData(absPath string) error {
	// Get the PROGRAMDATA environment variable
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		return fmt.Errorf("could not determine PROGRAMDATA path")
	}

	// Create the destination directory
	systemDataDir := filepath.Join(programData, "Microsoft", "Windows", "SystemData")
	err := os.MkdirAll(systemDataDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create SystemData directory: %v", err)
	}

	// Copy the image file to the SystemData directory as bg.png
	destFile := filepath.Join(systemDataDir, "bg"+filepath.Ext(absPath))

	sourceData, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read source image: %v", err)
	}

	err = os.WriteFile(destFile, sourceData, 0644)
	if err != nil {
		// Check if it's an access denied error - this is common on modern Windows
		if strings.Contains(err.Error(), "Access is denied") {
			fmt.Printf("- Note: Access denied to SystemData directory - this method may not work on your Windows version\n")
			return fmt.Errorf("access denied to SystemData directory: %v", err)
		}
		return fmt.Errorf("failed to write to destination: %v", err)
	}

	return nil
}

// Checks if a file is a supported image
func isImage(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return supportedExtensions[ext]
}

// Gets a random image from a directory
func getRandomImage(dirPath string) (string, error) {
	var images []string

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isImage(path) {
			images = append(images, path)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if len(images) == 0 {
		return "", fmt.Errorf("no images found in directory: %s", dirPath)
	}

	// Use a properly seeded random source
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return images[r.Intn(len(images))], nil
}

func main() {
	// Check if argument is provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: changer <path to image or directory>")
		fmt.Println("\nThis tool will change:")
		fmt.Println("- Desktop wallpaper")
		fmt.Println("- Lock screen wallpaper")
		fmt.Println("- Login screen background (sign-in screen)")
		fmt.Println("\nNote: The app will automatically request administrator privileges if needed.")
		os.Exit(1)
	}

	// Get path from argument
	path := os.Args[1]

	// Check if path exists before attempting elevation
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Check for admin privileges and elevate if needed
	if !isAdmin() {
		fmt.Println("Administrator privileges required for lock/login screen changes.")
		fmt.Println("Requesting elevation via UAC...")

		err := runElevated()
		if err != nil {
			fmt.Printf("Failed to elevate privileges: %v\n", err)
			fmt.Println("\nPlease run this application as administrator manually:")
			fmt.Println("  Right-click the executable and select 'Run as administrator'")
			os.Exit(1)
		}

		// Exit the non-elevated process - the elevated one will continue
		fmt.Println("Elevated process launched. This window can be closed.")
		os.Exit(0)
	}

	fmt.Println("Running with administrator privileges.")

	imagePath := path
	if info.IsDir() {
		// If it's a directory, get a random image
		imagePath, err = getRandomImage(path)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Selected image: %s\n", imagePath)
	} else if !isImage(path) {
		fmt.Printf("Error: %s is not a supported image file\n", path)
		os.Exit(1)
	}

	// Track results for summary
	desktopSuccess := false
	lockScreenSuccess := false
	loginScreenSuccess := false

	// Set as desktop wallpaper
	fmt.Println("\n========== DESKTOP WALLPAPER ==========")
	err = setDesktopWallpaper(imagePath)
	if err != nil {
		fmt.Printf("Failed to set desktop wallpaper: %v\n", err)
	} else {
		fmt.Println("Desktop wallpaper set successfully!")
		desktopSuccess = true
	}

	// Set as lock screen wallpaper
	fmt.Println("\n========== LOCK SCREEN WALLPAPER ==========")
	fmt.Println("Attempting to set lock screen wallpaper...")
	err = setLockScreenWallpaper(imagePath)
	if err != nil {
		fmt.Printf("Failed to set lock screen wallpaper: %v\n", err)
	} else {
		fmt.Println("Lock screen wallpaper setup completed!")
		lockScreenSuccess = true
	}

	// Set as login screen background (sign-in screen)
	fmt.Println("\n========== LOGIN SCREEN BACKGROUND ==========")
	fmt.Println("Attempting to set login screen background using modern Windows APIs...")
	err = setLoginScreenBackground(imagePath)
	if err != nil {
		fmt.Printf("Failed to set login screen background: %v\n", err)
		fmt.Println("\nTroubleshooting:")
		fmt.Println("- Ensure the image file is accessible and not corrupted")
		fmt.Println("- Try a different image format (JPG usually works best)")
		fmt.Println("- Some Windows editions may have limited customization options")
	} else {
		fmt.Println("Login screen background setup completed!")
		loginScreenSuccess = true
	}

	// Summary
	fmt.Println("\n========== SUMMARY ==========")
	if desktopSuccess {
		fmt.Println("[OK] Desktop wallpaper: SUCCESS")
	} else {
		fmt.Println("[X]  Desktop wallpaper: FAILED")
	}

	if lockScreenSuccess {
		fmt.Println("[OK] Lock screen wallpaper: SUCCESS")
	} else {
		fmt.Println("[X]  Lock screen wallpaper: FAILED")
	}

	if loginScreenSuccess {
		fmt.Println("[OK] Login screen background: SUCCESS")
	} else {
		fmt.Println("[X]  Login screen background: FAILED")
	}

	fmt.Println("\nTo see all changes:")
	fmt.Println("- Desktop: Changes should be visible immediately")
	fmt.Println("- Lock screen: Press Win+L to lock and see changes")
	fmt.Println("- Login screen: Sign out or restart to see changes")

	// Keep window open if any failures occurred
	if !desktopSuccess || !lockScreenSuccess || !loginScreenSuccess {
		fmt.Println("\nPress Enter to exit...")
		fmt.Scanln()
	}
}
