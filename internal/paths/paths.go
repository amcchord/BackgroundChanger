// Package paths owns the on-disk layout shared by setup and the service.
package paths

import (
	"os"
	"path/filepath"
)

const (
	ProductName = "BackgroundChanger"
	ServiceName = "BackgroundChanger"
	ExeName     = "BackgroundChanger.exe"
)

func InstallDir() string {
	base := os.Getenv("ProgramFiles")
	if base == "" {
		base = `C:\Program Files`
	}
	return filepath.Join(base, ProductName)
}

func InstalledExe() string { return filepath.Join(InstallDir(), ExeName) }

func DataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, ProductName)
}

func ImageDir() string         { return filepath.Join(DataDir(), "backgrounds") }
func ConfigFile() string       { return filepath.Join(DataDir(), "config.yml") }
func StatusFile() string       { return filepath.Join(DataDir(), "status.json") }
func LogFile() string          { return filepath.Join(DataDir(), "BackgroundChanger.log") }
func PolicyBackupFile() string { return filepath.Join(DataDir(), "policy-backup.json") }
func MDMBackupFile() string    { return filepath.Join(DataDir(), "mdm-policy-backup.json") }
func MDMRestoreMarker() string { return filepath.Join(DataDir(), "mdm-policy-restore.txt") }
