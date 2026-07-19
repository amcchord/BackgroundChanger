// Package paths owns the on-disk layout shared by setup and the service.
package paths

import (
	"os"
	"path/filepath"
)

const (
	ProductName = "Wallpaper Identity"
	ProductID   = "WallpaperIdentity"
	ShortName   = "W:ID"
	ServiceName = "WallpaperIdentity"
	ExeName     = "WallpaperIdentity.exe"

	LegacyProductName = "BackgroundChanger"
	LegacyServiceName = "BackgroundChanger"
	LegacyExeName     = "BackgroundChanger.exe"
)

func InstallDir() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, ProductName)
}

func InstalledExe() string { return filepath.Join(InstallDir(), ExeName) }

func LegacyInstallDir() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, LegacyProductName)
}

func LegacyInstalledExe() string { return filepath.Join(LegacyInstallDir(), LegacyExeName) }

func DataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, ProductName)
}

func LegacyDataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, LegacyProductName)
}

func ImageDir() string { return filepath.Join(DataDir(), "backgrounds") }
func CSPImageDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "WallpaperIdentityCSP")
}
func ConfigFile() string       { return filepath.Join(DataDir(), "config.yml") }
func StatusFile() string       { return filepath.Join(DataDir(), "status.json") }
func LogFile() string          { return filepath.Join(DataDir(), "WallpaperIdentity.log") }
func PolicyBackupFile() string { return filepath.Join(DataDir(), "policy-backup.json") }
func MDMBackupFile() string    { return filepath.Join(DataDir(), "mdm-policy-backup.json") }
func ProCompatibilityBackupFile() string {
	return filepath.Join(DataDir(), "pro-compatibility-backup.json")
}
func MDMRestoreMarker() string { return filepath.Join(DataDir(), "mdm-policy-restore.txt") }
func PreLoginRefreshMarker() string {
	return filepath.Join(DataDir(), "pre-login-refresh.json")
}

func LegacyConfigFile() string       { return filepath.Join(LegacyDataDir(), "config.yml") }
func LegacyStatusFile() string       { return filepath.Join(LegacyDataDir(), "status.json") }
func LegacyLogFile() string          { return filepath.Join(LegacyDataDir(), "BackgroundChanger.log") }
func LegacyPolicyBackupFile() string { return filepath.Join(LegacyDataDir(), "policy-backup.json") }
func LegacyMDMBackupFile() string    { return filepath.Join(LegacyDataDir(), "mdm-policy-backup.json") }
func LegacyMDMRestoreMarker() string { return filepath.Join(LegacyDataDir(), "mdm-policy-restore.txt") }

const UninstallRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\WallpaperIdentity`
const LegacyUninstallRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\BackgroundChanger`
