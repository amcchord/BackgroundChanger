// Package loginscreen provides functionality for managing Windows login screen backgrounds.
package loginscreen

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

var (
	// BackupDir is the directory where we store the original background backup.
	// Uses PROGRAMDATA environment variable to support non-standard Windows installations.
	BackupDir = filepath.Join(os.Getenv("PROGRAMDATA"), "BgStatusService")
	// BackupFileName is the name of the backup file.
	BackupFileName = "original_background.jpg"
)

// GetBackupPath returns the full path to the backup file.
func GetBackupPath() string {
	return filepath.Join(BackupDir, BackupFileName)
}

// HasBackup checks if a backup of the original login screen exists.
func HasBackup() bool {
	_, err := os.Stat(GetBackupPath())
	return err == nil
}

// GetBackupImage returns the path to the backed-up original image if it exists.
func GetBackupImage() (string, error) {
	backupPath := GetBackupPath()
	if _, err := os.Stat(backupPath); err != nil {
		return "", fmt.Errorf("backup does not exist: %v", err)
	}
	return backupPath, nil
}

// BackupOriginalImage saves the given image as the original backup.
func BackupOriginalImage(imagePath string) error {
	// Create backup directory if it doesn't exist
	err := os.MkdirAll(BackupDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create backup directory: %v", err)
	}

	// Open source file
	src, err := os.Open(imagePath)
	if err != nil {
		return fmt.Errorf("failed to open source image: %v", err)
	}
	defer src.Close()

	// Create destination file
	backupPath := GetBackupPath()
	dst, err := os.Create(backupPath)
	if err != nil {
		return fmt.Errorf("failed to create backup file: %v", err)
	}
	defer dst.Close()

	// Copy the file
	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy image to backup: %v", err)
	}

	return nil
}

// InvalidateBackup removes the backup file so a new one will be created.
func InvalidateBackup() error {
	backupPath := GetBackupPath()
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		// Already doesn't exist, nothing to do
		return nil
	}
	return os.Remove(backupPath)
}

// GetCurrentLoginScreenImage finds the current login screen background image.
// It checks multiple locations in priority order.
func GetCurrentLoginScreenImage() (string, error) {
	// Priority 1: Check Group Policy registry for LockScreenImage
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Policies\Microsoft\Windows\Personalization`,
		registry.QUERY_VALUE,
	)
	if err == nil {
		defer key.Close()
		path, _, err := key.GetStringValue("LockScreenImage")
		if err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			}
		}
	}

	// Priority 2: Check OOBE backgrounds folder
	systemRoot := os.Getenv("SystemRoot")
	oobeBackgrounds := filepath.Join(systemRoot, "System32", "oobe", "info", "backgrounds")
	oobeDefault := filepath.Join(oobeBackgrounds, "backgroundDefault.jpg")
	if _, err := os.Stat(oobeDefault); err == nil {
		return oobeDefault, nil
	}

	// Priority 3: Check PersonalizationCSP registry
	cspKey, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\PersonalizationCSP`,
		registry.QUERY_VALUE,
	)
	if err == nil {
		defer cspKey.Close()
		path, _, err := cspKey.GetStringValue("LockScreenImagePath")
		if err == nil && path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				return path, nil
			}
		}
	}

	// Priority 4: Check Windows Spotlight assets
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		spotlightDir := filepath.Join(localAppData, "Packages", "Microsoft.Windows.ContentDeliveryManager_cw5n1h2txyewy", "LocalState", "Assets")
		if info, err := os.Stat(spotlightDir); err == nil && info.IsDir() {
			// Find the largest file (likely the landscape wallpaper)
			var largestFile string
			var largestSize int64
			entries, err := os.ReadDir(spotlightDir)
			if err == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						continue
					}
					info, err := entry.Info()
					if err != nil {
						continue
					}
					if info.Size() > largestSize {
						largestSize = info.Size()
						largestFile = filepath.Join(spotlightDir, entry.Name())
					}
				}
			}
			if largestFile != "" && largestSize > 100000 { // At least 100KB to be a wallpaper
				return largestFile, nil
			}
		}
	}

	// No existing login screen found
	return "", fmt.Errorf("no existing login screen image found")
}

// SetLoginScreenImage sets the given image as the Windows login screen background.
func SetLoginScreenImage(imagePath string) error {
	// Convert to absolute path
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Ensure the image exists
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("image file does not exist: %v", err)
	}

	// Try multiple methods - WinRT is the most reliable for immediate effect
	var anySuccess bool
	var lastError error

	// Method 1: WinRT API via PowerShell (PRIMARY - works immediately at user level)
	err = setLoginScreenViaWinRT(absPath)
	if err != nil {
		lastError = err
	} else {
		anySuccess = true
	}

	// Method 2: Group Policy Registry (fallback - may require reboot/gpupdate)
	err = setLoginScreenViaGroupPolicy(absPath)
	if err != nil {
		if lastError == nil {
			lastError = err
		}
	} else {
		anySuccess = true
	}

	// Method 3: OOBE background folder (fallback for older Windows versions)
	err = setLoginScreenViaOOBE(absPath)
	if err != nil {
		if lastError == nil {
			lastError = err
		}
	} else {
		anySuccess = true
	}

	if !anySuccess {
		return fmt.Errorf("all login screen methods failed, last error: %v", lastError)
	}

	return nil
}

// setLoginScreenViaGroupPolicy sets the login screen using Group Policy registry keys.
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

	return nil
}

// setLoginScreenViaOOBE copies the image to the OOBE backgrounds folder.
func setLoginScreenViaOOBE(absPath string) error {
	// Create the backgrounds directory if it doesn't exist
	systemRoot := os.Getenv("SystemRoot")
	backgroundsDir := filepath.Join(systemRoot, "System32", "oobe", "info", "backgrounds")
	err := os.MkdirAll(backgroundsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create backgrounds directory: %v", err)
	}

	// Load the source image
	srcFile, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("failed to open source image: %v", err)
	}
	defer srcFile.Close()

	// Decode the image to ensure it's valid and can be re-encoded as JPEG
	img, format, err := image.Decode(srcFile)
	if err != nil {
		return fmt.Errorf("failed to decode image: %v", err)
	}

	// The target file must be named backgroundDefault.jpg
	targetPath := filepath.Join(backgroundsDir, "backgroundDefault.jpg")

	// Create the target file
	dstFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create target file: %v", err)
	}
	defer dstFile.Close()

	// Encode as JPEG
	if format == "jpeg" || format == "jpg" {
		// Re-open and copy directly for JPEG
		srcFile.Seek(0, 0)
		_, err = io.Copy(dstFile, srcFile)
	} else {
		// Convert to JPEG
		err = jpeg.Encode(dstFile, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		return fmt.Errorf("failed to save image: %v", err)
	}

	// Enable OEM background in registry
	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI\Background`,
		registry.ALL_ACCESS,
	)
	if err != nil {
		return fmt.Errorf("failed to open LogonUI Background key: %v", err)
	}
	defer key.Close()

	err = key.SetDWordValue("OEMBackground", 1)
	if err != nil {
		return fmt.Errorf("failed to set OEMBackground: %v", err)
	}

	return nil
}

// setLoginScreenViaWinRT uses PowerShell and WinRT API to set the lock screen.
func setLoginScreenViaWinRT(absPath string) error {
	psScript := fmt.Sprintf(`
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Runtime.WindowsRuntime

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

[Windows.System.UserProfile.LockScreen,Windows.System.UserProfile,ContentType=WindowsRuntime] | Out-Null
[Windows.Storage.StorageFile,Windows.Storage,ContentType=WindowsRuntime] | Out-Null

$imagePath = '%s'
$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imagePath)) ([Windows.Storage.StorageFile])
AwaitAction ([Windows.System.UserProfile.LockScreen]::SetImageFileAsync($file))
`, absPath)

	cmd := exec.Command("powershell.exe",
		"-NoProfile",
		"-ExecutionPolicy", "Bypass",
		"-Command", psScript,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PowerShell WinRT failed: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// LoadImage loads an image from the given path.
func LoadImage(imagePath string) (image.Image, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	return img, nil
}

// SaveImage saves an image to the given path as JPEG.
func SaveImage(img image.Image, imagePath string) error {
	file, err := os.Create(imagePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(imagePath))
	if ext == ".png" {
		return png.Encode(file, img)
	}

	// Default to JPEG
	return jpeg.Encode(file, img, &jpeg.Options{Quality: 95})
}

// CreateDefaultBackground creates a solid dark background image.
func CreateDefaultBackground(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with dark gray (#1a1a1a)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, image.Black)
		}
	}
	return img
}

