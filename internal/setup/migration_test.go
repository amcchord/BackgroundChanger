package setup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
)

func TestMigrateLegacyDataRenamesDirectoryAndLog(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	legacy := paths.LegacyDataDir()
	if err := os.MkdirAll(filepath.Join(legacy, "backgrounds"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, paths.LegacyConfigFile(), "preset: identity\n")
	mustWrite(t, paths.LegacyLogFile(), "legacy log\n")
	mustWrite(t, filepath.Join(legacy, "backgrounds", "old.jpg"), "image")

	if err := migrateLegacyData(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
	for _, path := range []string{paths.ConfigFile(), paths.LogFile(), filepath.Join(paths.ImageDir(), "old.jpg")} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("migrated path %q: %v", path, err)
		}
	}
}

func TestMigrateLegacyDataPreservesConflicts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	if err := os.MkdirAll(paths.LegacyDataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, paths.LegacyConfigFile(), "tuned v3 config")
	mustWrite(t, paths.ConfigFile(), "temporary v4 config")
	mustWrite(t, paths.LegacyStatusFile(), "legacy status")
	mustWrite(t, paths.StatusFile(), "new status")
	mustWrite(t, paths.LegacyLogFile(), "legacy log\n")
	mustWrite(t, paths.LogFile(), "new log\n")

	if err := migrateLegacyData(); err != nil {
		t.Fatal(err)
	}
	configData, _ := os.ReadFile(paths.ConfigFile())
	if string(configData) != "tuned v3 config" {
		t.Fatalf("tuned v3 config was not made authoritative: %q", configData)
	}
	preservedConfig, _ := os.ReadFile(paths.ConfigFile() + ".pre-v4")
	if string(preservedConfig) != "temporary v4 config" {
		t.Fatalf("temporary v4 config was not preserved: %q", preservedConfig)
	}
	preservedStatus, _ := os.ReadFile(paths.StatusFile() + ".from-v3")
	if string(preservedStatus) != "legacy status" {
		t.Fatalf("conflicting v3 status was not preserved: %q", preservedStatus)
	}
	logData, _ := os.ReadFile(paths.LogFile())
	if !strings.Contains(string(logData), "new log") || !strings.Contains(string(logData), "legacy log") {
		t.Fatalf("logs were not merged: %q", logData)
	}
	if _, err := os.Stat(paths.LegacyDataDir()); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still exists: %v", err)
	}
}

func TestSelectMDMRestoreTarget(t *testing.T) {
	tests := []struct {
		name                          string
		currentService, legacyService bool
		currentBackup, legacyBackup   bool
		wantService, wantMarker       string
		completed, wantErr            bool
	}{
		{name: "current service", currentService: true, currentBackup: true, wantService: paths.ServiceName, wantMarker: paths.MDMRestoreMarker()},
		{name: "legacy service", legacyService: true, legacyBackup: true, wantService: paths.LegacyServiceName, wantMarker: paths.LegacyMDMRestoreMarker()},
		{name: "nothing to restore"},
		{name: "backup without service", currentBackup: true, wantErr: true},
		{name: "completed restore without service", currentBackup: true, completed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, marker, err := selectMDMRestoreTarget(test.currentService, test.legacyService, test.currentBackup, test.legacyBackup, test.completed)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if service != test.wantService || marker != test.wantMarker {
				t.Fatalf("target = (%q, %q), want (%q, %q)", service, marker, test.wantService, test.wantMarker)
			}
		})
	}
}

func TestSetupLockRejectsConcurrentOperation(t *testing.T) {
	release, err := acquireSetupLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	secondRelease, err := acquireSetupLock()
	if secondRelease != nil {
		secondRelease()
	}
	if !errors.Is(err, ErrSetupRunning) {
		t.Fatalf("second lock error = %v", err)
	}
}

func TestMigrateLegacyDataPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	legacyImage := filepath.Join(paths.LegacyDataDir(), "wallpapers", "custom.jpg")
	want := filepath.Join(paths.DataDir(), "wallpapers", "custom.jpg")
	if got := migrateLegacyDataPath(legacyImage); got != want {
		t.Fatalf("migrated path = %q, want %q", got, want)
	}
	outside := filepath.Join(root, "Company", "custom.jpg")
	if got := migrateLegacyDataPath(outside); got != outside {
		t.Fatalf("unrelated path changed to %q", got)
	}
	if got := migrateLegacyDataPath("relative\\custom.jpg"); got != "relative\\custom.jpg" {
		t.Fatalf("relative path changed to %q", got)
	}
}

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}
