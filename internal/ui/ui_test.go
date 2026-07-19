package ui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
)

func TestLegacyInstallationSummaryOffersMigration(t *testing.T) {
	state, detail := installationSummary(true, true)
	if state != "Previous version detected" {
		t.Fatalf("state = %q", state)
	}
	for _, expected := range []string{"Repair / Upgrade", "Wallpaper Identity", "configuration"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("detail %q does not contain %q", detail, expected)
		}
	}
}

func TestPresetSelectionCopyTracksInstallIntent(t *testing.T) {
	choices := []presetChoice{
		{Name: "identity", Title: "Identity", Description: "OS, build, IP and serial"},
		{Name: "balanced", Title: "Balanced", Description: "Hardware, capacity and health"},
		{Name: "operations", Title: "Operations", Description: "Resources, restart and failures"},
	}
	tests := []struct {
		name      string
		selected  int
		installed bool
		changed   bool
		want      string
	}{
		{name: "new install", selected: 1, want: "Selected: Balanced — Hardware, capacity and health"},
		{name: "existing layout", selected: 2, installed: true, want: "Current layout: Operations — Resources, restart and failures"},
		{name: "explicit maintenance change", selected: 0, installed: true, changed: true, want: "Selected for update: Identity — OS, build, IP and serial"},
		{name: "custom config", selected: -1, installed: true, want: "Current layout: Custom configuration. Select a preview to replace it."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := presetSelectionText(choices, test.selected, test.installed, test.changed); got != test.want {
				t.Fatalf("selection text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPresetButtonShowsOnlySelectedChoice(t *testing.T) {
	choice := presetChoice{Title: "Balanced", Description: "Hardware, capacity and health"}
	if got := presetButtonText(choice, false); got != "Balanced" {
		t.Fatalf("unselected button text = %q", got)
	}
	if got := presetButtonText(choice, true); got != "✓  Balanced" {
		t.Fatalf("selected button text = %q", got)
	}
}

func TestPresetIndexRecognizesNamedLayouts(t *testing.T) {
	choices := []presetChoice{{Name: "identity"}, {Name: "balanced"}, {Name: "operations"}}
	if got := presetIndex(choices, "operations"); got != 2 {
		t.Fatalf("operations index = %d, want 2", got)
	}
	if got := presetIndex(choices, "custom"); got != -1 {
		t.Fatalf("custom index = %d, want -1", got)
	}
}

func TestSuccessfulProgressEndsAtDone(t *testing.T) {
	headline, detail := operationCompletionText(nil)
	if headline != "Done" || !strings.Contains(detail, "completed successfully") {
		t.Fatalf("success completion = %q, %q", headline, detail)
	}

	wantErr := errors.New("policy verification failed")
	headline, detail = operationCompletionText(wantErr)
	if headline == "Done" || detail != wantErr.Error() {
		t.Fatalf("error completion = %q, %q", headline, detail)
	}
}

func TestPresetPreviewPixelsFollowDisplayDPI(t *testing.T) {
	tests := []struct {
		dpi                   int
		wantWidth, wantHeight int
	}{
		{dpi: 96, wantWidth: 238, wantHeight: 134},
		{dpi: 120, wantWidth: 298, wantHeight: 168},
		{dpi: 144, wantWidth: 357, wantHeight: 201},
		{dpi: 192, wantWidth: 476, wantHeight: 268},
	}
	for _, test := range tests {
		width, height := presetPreviewPixelSize(test.dpi)
		if width != test.wantWidth || height != test.wantHeight {
			t.Fatalf("DPI %d preview = %dx%d, want %dx%d", test.dpi, width, height, test.wantWidth, test.wantHeight)
		}
	}
}

func TestBackgroundSelectionCopyAndImageWell(t *testing.T) {
	appearance := config.Default()
	if got := backgroundSelectionText(appearance, "", false, false); got != "Selected: Azure · Dark" {
		t.Fatalf("new color selection = %q", got)
	}
	appearance.BackgroundColor = config.BackgroundTeal
	appearance.BackgroundMode = config.BackgroundLight
	if got := backgroundSelectionText(appearance, `C:\Images\room.png`, true, true); got != "Selected for update: Custom image · Light appearance" {
		t.Fatalf("custom selection = %q", got)
	}
	if got := backgroundImageWellText(""); !strings.Contains(got, "Drop a JPEG / PNG") {
		t.Fatalf("empty image well = %q", got)
	}
	path := filepath.Join(`C:\Images`, strings.Repeat("very-long-", 5)+"room.png")
	if got := backgroundImageWellText(path); !strings.HasPrefix(got, "✓  Custom image: ") || len([]rune(got)) > 50 {
		t.Fatalf("selected image well = %q", got)
	}
}

func TestBackgroundButtonsAndPresetNames(t *testing.T) {
	choice := backgroundChoice{Name: config.BackgroundSlate, Title: "Slate"}
	if got := backgroundButtonText(choice, true); got != "✓  Slate" {
		t.Fatalf("selected background button = %q", got)
	}
	if got := backgroundModeButtonText(config.BackgroundLight, config.BackgroundDark); got != "Light" {
		t.Fatalf("unselected mode = %q", got)
	}
	if got := backgroundModeButtonText(config.BackgroundDark, config.BackgroundDark); got != "✓  Dark" {
		t.Fatalf("selected mode = %q", got)
	}
	if got := selectedPresetName(-1); got != config.PresetBalanced {
		t.Fatalf("custom preview fallback = %q", got)
	}
	if got := presetIndexByName(config.PresetOperations); got != 2 {
		t.Fatalf("operations preset index = %d", got)
	}
}
