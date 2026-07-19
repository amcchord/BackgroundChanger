package setup

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
)

func TestInstallStandardBackgroundUsesDecodedFormatAndOneStablePath(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	sourcePNG := filepath.Join(t.TempDir(), "source-with-any-name.dat")
	writeTestImage(t, sourcePNG, "png")
	destination, err := installStandardBackground(sourcePNG)
	if err != nil {
		t.Fatal(err)
	}
	if destination != paths.StandardBackgroundPNG() || paths.ResolveBackgroundImage("") != destination {
		t.Fatalf("PNG destination = %q, resolved = %q", destination, paths.ResolveBackgroundImage(""))
	}

	sourceJPEG := filepath.Join(t.TempDir(), "replacement.png")
	writeTestImage(t, sourceJPEG, "jpeg")
	destination, err = installStandardBackground(sourceJPEG)
	if err != nil {
		t.Fatal(err)
	}
	if destination != paths.StandardBackgroundJPEG() || paths.ResolveBackgroundImage("") != destination {
		t.Fatalf("JPEG destination = %q, resolved = %q", destination, paths.ResolveBackgroundImage(""))
	}
	if _, err := os.Stat(paths.StandardBackgroundPNG()); !os.IsNotExist(err) {
		t.Fatalf("obsolete PNG still exists: %v", err)
	}

	if err := removeStandardBackgrounds(); err != nil {
		t.Fatal(err)
	}
	if got := paths.ResolveBackgroundImage(""); got != "" {
		t.Fatalf("background remained after color reset: %q", got)
	}
}

func TestInstallStandardBackgroundAcceptsItsExistingStablePath(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	source := filepath.Join(t.TempDir(), "source.png")
	writeTestImage(t, source, "png")
	destination, err := installStandardBackground(source)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := installStandardBackground(destination); err != nil || repeated != destination {
		t.Fatalf("reinstall stable path = %q, %v", repeated, err)
	}
}

func TestStandardBackgroundBackupRestoresPreviousFiles(t *testing.T) {
	t.Setenv("ProgramData", t.TempDir())
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(t.TempDir(), "original.png")
	replacement := filepath.Join(t.TempDir(), "replacement.jpg")
	writeTestImage(t, original, "png")
	writeTestImage(t, replacement, "jpeg")
	if _, err := installStandardBackground(original); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(paths.StandardBackgroundPNG())
	if err != nil {
		t.Fatal(err)
	}
	backup, err := backupStandardBackgrounds()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installStandardBackground(replacement); err != nil {
		t.Fatal(err)
	}
	if err := backup.restore(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(paths.StandardBackgroundPNG())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("restored PNG differs from the original")
	}
	if _, err := os.Stat(paths.StandardBackgroundJPEG()); !os.IsNotExist(err) {
		t.Fatalf("replacement JPEG remained after rollback: %v", err)
	}
}

func TestCurrentWindowsBackgroundIsOnlyTheTrueFreshInstallDefault(t *testing.T) {
	tests := []struct {
		name      string
		options   InstallOptions
		installed bool
		saved     bool
		want      bool
	}{
		{name: "fresh install", want: true},
		{name: "fresh preset", options: InstallOptions{Preset: "balanced"}, want: true},
		{name: "saved configuration", saved: true},
		{name: "installed", installed: true},
		{name: "explicit image", options: InstallOptions{BackgroundImage: `C:\RMM\room.jpg`}},
		{name: "explicit colors", options: InstallOptions{UseColors: true}},
		{name: "complete config", options: InstallOptions{ConfigFile: `C:\RMM\config.yml`}},
		{name: "explicit current with saved config", options: InstallOptions{UseCurrentBackground: true}, saved: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldUseCurrentBackground(test.options, test.installed, test.saved); got != test.want {
				t.Fatalf("shouldUseCurrentBackground = %v, want %v", got, test.want)
			}
		})
	}
	if err := validateBackgroundSourceOptions(InstallOptions{UseCurrentBackground: true, UseColors: true}); err == nil {
		t.Fatal("expected mutually exclusive background sources to fail")
	}
}

func writeTestImage(t *testing.T, path, format string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{20, 80, 160, 255})
	if format == "png" {
		err = png.Encode(file, img)
	} else {
		err = jpeg.Encode(file, img, &jpeg.Options{Quality: 90})
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}
