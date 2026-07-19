package loginscreen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnedPath(t *testing.T) {
	data := filepath.Join(`C:\ProgramData`, "BackgroundChanger")
	if !isOwnedPath(filepath.Join(data, "backgrounds", "status-1.jpg"), data) {
		t.Fatal("expected generated image to be owned")
	}
	if isOwnedPath(`C:\Windows\Web\Screen\img100.jpg`, data) {
		t.Fatal("must not claim a Windows image")
	}
}

func TestPowerShellEncoding(t *testing.T) {
	got := encodePowerShell("Write-Output 'ok'")
	if got == "" || strings.Contains(got, "Write") {
		t.Fatalf("unexpected encoding %q", got)
	}
}
