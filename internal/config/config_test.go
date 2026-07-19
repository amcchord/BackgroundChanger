package config

import (
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
