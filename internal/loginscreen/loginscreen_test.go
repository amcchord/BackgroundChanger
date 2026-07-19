package loginscreen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
)

func TestOwnedPath(t *testing.T) {
	data := filepath.Join(`C:\ProgramData`, "Wallpaper Identity")
	if !isOwnedPath(filepath.Join(data, "backgrounds", "status-1.jpg"), data) {
		t.Fatal("expected generated image to be owned")
	}
	if isOwnedPath(`C:\Windows\Web\Screen\img100.jpg`, data) {
		t.Fatal("must not claim a Windows image")
	}
}

func TestOwnedByAnyPathIncludesPreviousVersion(t *testing.T) {
	dataDirs := []string{
		filepath.Join(`C:\ProgramData`, "Wallpaper Identity"),
		filepath.Join(`C:\ProgramData`, "BackgroundChanger"),
	}
	legacyImage := filepath.Join(dataDirs[1], "backgrounds", "status-1.jpg")
	if !isOwnedByAnyPath(legacyImage, dataDirs) {
		t.Fatal("expected previous-version generated image to be owned during migration")
	}
}

func TestOwnedMDMImagePattern(t *testing.T) {
	pattern := regexp.MustCompile(ownedMDMImagePattern)
	for _, value := range []string{
		`file:///C:/ProgramData/Wallpaper%20Identity/backgrounds/status-1.jpg`,
		`C:\ProgramData\Wallpaper Identity\backgrounds\status-1.jpg`,
		`file:///C:/ProgramData/WallpaperIdentityCSP/status-1.jpg`,
		`file:///C:/ProgramData/BackgroundChanger/backgrounds/status-1.jpg`,
	} {
		if !pattern.MatchString(value) {
			t.Errorf("expected %q to be recognized as an owned MDM image", value)
		}
	}
	if pattern.MatchString(`file:///C:/Windows/Web/Screen/img100.jpg`) {
		t.Fatal("must not claim an unrelated MDM image")
	}
}

func TestStageMDMImageUsesSpaceFreeDeliveryDirectory(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	source := filepath.Join(t.TempDir(), "machine-status.jpg")
	if err := os.WriteFile(source, []byte("jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	staged, err := stageMDMImage(source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.Base(filepath.Dir(staged)), " ") {
		t.Fatalf("CSP delivery directory contains a space: %s", staged)
	}
	if b, err := os.ReadFile(staged); err != nil || string(b) != "jpeg" {
		t.Fatalf("staged content = %q, %v", b, err)
	}
}

func TestCleanupStagedMDMImagesRetainsCurrentAndThreePrevious(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ProgramData", root)
	var current string
	for index := 0; index < 7; index++ {
		path := filepath.Join(paths.CSPImageDir(), fmt.Sprintf("machine-%d.jpg", index))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := time.Unix(int64(index), 0)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		current = path
	}
	cleanupStagedMDMImages(current, 4)
	entries, err := os.ReadDir(paths.CSPImageDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatalf("retained %d CSP images, want 4", len(entries))
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current CSP image was removed: %v", err)
	}
}

func TestPowerShellEncoding(t *testing.T) {
	got := encodePowerShell("Write-Output 'ok'")
	if got == "" || strings.Contains(got, "Write") {
		t.Fatalf("unexpected encoding %q", got)
	}
}
