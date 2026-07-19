package loginscreen

import (
	"image"
	"image/color"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
)

func TestChooseCurrentBackgroundResolvesFileURLAndSkipsInvalidImages(t *testing.T) {
	valid := filepath.Join(t.TempDir(), "lock screen.png")
	writeBackgroundTestPNG(t, valid)
	reference := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(valid)}).String()
	got, ok := chooseCurrentBackground([]currentBackgroundCandidate{
		{reference: filepath.Join(t.TempDir(), "missing.jpg"), source: "missing"},
		{reference: reference, source: "Windows lock screen"},
	})
	if !ok || !samePath(got.Path, valid) || got.Source != "Windows lock screen" {
		t.Fatalf("current background = %#v, %v", got, ok)
	}
}

func TestChooseCurrentBackgroundSkipsWIDOwnedOutput(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	if err := os.MkdirAll(paths.ImageDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(paths.ImageDir(), "machine-status.jpg")
	fallback := filepath.Join(t.TempDir(), "windows-default.png")
	writeBackgroundTestPNG(t, owned)
	writeBackgroundTestPNG(t, fallback)
	got, ok := chooseCurrentBackground([]currentBackgroundCandidate{
		{reference: owned, source: "W:ID"},
		{reference: fallback, source: "Windows default"},
	})
	if !ok || !samePath(got.Path, fallback) {
		t.Fatalf("owned output was not skipped: %#v, %v", got, ok)
	}
}

func TestLocalBackgroundPathExpandsWindowsEnvironmentReferences(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WID_LOCK_ROOT", root)
	want := filepath.Join(root, "room.jpg")
	if got := localBackgroundPath(`%WID_LOCK_ROOT%\room.jpg`); !samePath(got, want) {
		t.Fatalf("expanded path = %q, want %q", got, want)
	}
	if got := localBackgroundPath("https://example.test/lock.jpg"); got != "" {
		t.Fatalf("remote URL should not be used directly: %q", got)
	}
}

func writeBackgroundTestPNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{20, 80, 160, 255})
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
