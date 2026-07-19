package loginscreen

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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

func TestPowerShellEncoding(t *testing.T) {
	got := encodePowerShell("Write-Output 'ok'")
	if got == "" || strings.Contains(got, "Write") {
		t.Fatalf("unexpected encoding %q", got)
	}
}
