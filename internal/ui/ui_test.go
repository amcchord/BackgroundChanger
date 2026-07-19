package ui

import (
	"strings"
	"testing"
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
