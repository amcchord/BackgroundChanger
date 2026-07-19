package config

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadOrCreateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yml")
	got, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Default()) {
		t.Fatalf("got %#v, want %#v", got, Default())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Generated at timestamp") {
		t.Fatalf("generated config lacks invariant timestamp guidance:\n%s", b)
	}
	if !strings.Contains(string(b), "Wallpaper Identity (W:ID)") {
		t.Fatalf("generated config lacks W:ID branding:\n%s", b)
	}
	got.Preset = PresetCustom
	got.RefreshMinutes = 17
	got.Show.GPU = false
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.RefreshMinutes != 17 || reloaded.Show.GPU {
		t.Fatalf("reloaded = %#v", reloaded)
	}
}

func TestRepositoryExampleIsValid(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "config.example.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Preset != PresetCustom || !cfg.Show.OS || cfg.Show.FailedAutoServices {
		t.Fatalf("unexpected example config: %#v", cfg)
	}
}

func TestRejectsRelativeBaseImage(t *testing.T) {
	_, err := decode([]byte("preset: balanced\nbase_image: relative.jpg\n"), false)
	if err == nil || !strings.Contains(err.Error(), "absolute local path") {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateAssets(t *testing.T) {
	validPath := filepath.Join(t.TempDir(), "base.png")
	file, err := os.Create(validPath)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAssets(Config{BaseImage: validPath}); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(t.TempDir(), "not-an-image.png")
	if err := os.WriteFile(invalidPath, []byte("not an image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAssets(Config{BaseImage: invalidPath}); err == nil || !strings.Contains(err.Error(), "decode base_image") {
		t.Fatalf("invalid image error = %v", err)
	}
}

func TestPresetsHaveDistinctFocus(t *testing.T) {
	identity, _ := ForPreset(PresetIdentity)
	balanced, _ := ForPreset(PresetBalanced)
	operations, _ := ForPreset(PresetOperations)
	if identity.Show.CPU || !identity.Show.Serial {
		t.Fatalf("identity preset = %#v", identity.Show)
	}
	if !balanced.Show.GPU || balanced.Show.FailedAutoServices {
		t.Fatalf("balanced preset = %#v", balanced.Show)
	}
	if operations.Show.Serial || !operations.Show.FailedAutoServices {
		t.Fatalf("operations preset = %#v", operations.Show)
	}
}

func TestNamedPresetOverridesStoredShowValues(t *testing.T) {
	cfg, err := decode([]byte("preset: identity\nrefresh_minutes: 5\nshow:\n  cpu: true\n  serial: false\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Show.CPU || !cfg.Show.Serial {
		t.Fatalf("identity preset did not remain authoritative: %#v", cfg.Show)
	}
}

func TestUnknownYAMLFieldIsRejected(t *testing.T) {
	if _, err := decode([]byte("preset: balanced\nrefresh_minutes: 5\nshow:\n  memroy: true\n"), false); err == nil {
		t.Fatal("expected misspelled YAML field to be rejected")
	}
}

func TestMigrateLegacyJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"refresh_minutes":9,"width":1280,"height":720}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadOrCreate(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Preset != PresetBalanced || cfg.RefreshMinutes != 9 || cfg.Width != 1280 || !cfg.Show.CPU {
		t.Fatalf("migrated config = %#v", cfg)
	}
}

func TestValidation(t *testing.T) {
	tests := []Config{
		{Preset: PresetBalanced, RefreshMinutes: 0},
		{Preset: PresetBalanced, RefreshMinutes: 5, Width: 1920},
		{Preset: PresetBalanced, RefreshMinutes: 5, Width: 320, Height: 200},
		{Preset: "unknown", RefreshMinutes: 5},
	}
	for _, tc := range tests {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected %#v to be invalid", tc)
		}
	}
}
