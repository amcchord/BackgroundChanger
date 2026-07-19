package paths

import (
	"os"
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

func TestResolveBackgroundImageUsesDocumentedStandardFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ProgramData", dir)
	if got := ResolveBackgroundImage(""); got != "" {
		t.Fatalf("empty lookup = %q", got)
	}
	if err := os.MkdirAll(DataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StandardBackgroundPNG(), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveBackgroundImage(""); got != StandardBackgroundPNG() {
		t.Fatalf("PNG lookup = %q, want %q", got, StandardBackgroundPNG())
	}
	explicit := filepath.Join(dir, "managed.jpg")
	if got := ResolveBackgroundImage(explicit); got != explicit {
		t.Fatalf("explicit lookup = %q", got)
	}
}
