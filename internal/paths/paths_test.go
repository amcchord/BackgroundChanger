package paths

import (
	"path/filepath"
	"testing"
)

func TestBrandedPathsAndLegacyMigrationPaths(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\PF`)
	t.Setenv("ProgramData", `C:\PD`)

	if got, want := InstalledExe(), filepath.Join(`C:\PF`, "Wallpaper Identity", "WallpaperIdentity.exe"); got != want {
		t.Fatalf("InstalledExe = %q, want %q", got, want)
	}
	if got, want := DataDir(), filepath.Join(`C:\PD`, "Wallpaper Identity"); got != want {
		t.Fatalf("DataDir = %q, want %q", got, want)
	}
	if got, want := LegacyInstalledExe(), filepath.Join(`C:\PF`, "BackgroundChanger", "BackgroundChanger.exe"); got != want {
		t.Fatalf("LegacyInstalledExe = %q, want %q", got, want)
	}
	if ShortName != "W:ID" {
		t.Fatalf("ShortName = %q", ShortName)
	}
}
