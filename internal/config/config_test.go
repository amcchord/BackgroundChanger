package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	got, err := LoadOrCreate(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != Default() {
		t.Fatalf("got %#v, want %#v", got, Default())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	got.RefreshMinutes = 17
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.RefreshMinutes != 17 {
		t.Fatalf("refresh_minutes = %d", reloaded.RefreshMinutes)
	}
}

func TestValidation(t *testing.T) {
	tests := []Config{
		{RefreshMinutes: 0},
		{RefreshMinutes: 5, Width: 1920},
		{RefreshMinutes: 5, Width: 320, Height: 200},
	}
	for _, tc := range tests {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected %#v to be invalid", tc)
		}
	}
}
