package overlay

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/sysinfo"
)

func TestRenderUsesRequestedDimensions(t *testing.T) {
	snapshot := sysinfo.Snapshot{
		Hostname: "LAB-PC-042", OS: "Windows 11 Enterprise", Version: "25H2", Build: "26200.1234",
		CPU: "Virtual CPU · 4 logical", Memory: "2.1 / 8.0 GiB · 26%", GPU: "VirtualBox Graphics Adapter",
		Disk: "20.4 / 64.0 GiB · 32%", IPs: []string{"10.0.2.15"}, Serial: "0",
		Uptime: "0h 4m", ServicesRunning: 96, ServicesTotal: 142,
		CriticalServices: []sysinfo.ServiceState{{Name: "Defender", Running: true}},
		RefreshedAt:      time.Date(2026, 7, 18, 20, 0, 0, 0, time.UTC),
	}
	cfg := config.Default()
	cfg.Width, cfg.Height = 1280, 720
	got, err := Render(snapshot, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 1280, 720) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
}

func TestDimensionsUseDisplayAspectForFallbacks(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantWidth     int
		wantHeight    int
	}{
		{name: "actual display", width: 1366, height: 768, wantWidth: 1366, wantHeight: 768},
		{name: "four three fallback", width: 400, height: 300, wantWidth: 1600, wantHeight: 1200},
		{name: "wide fallback", width: 500, height: 281, wantWidth: 1920, wantHeight: 1080},
		{name: "portrait fallback", width: 300, height: 500, wantWidth: 1080, wantHeight: 1920},
		{name: "unknown fallback", wantWidth: 1920, wantHeight: 1080},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotWidth, gotHeight := Dimensions(config.Default(), sysinfo.Snapshot{DisplayWidth: test.width, DisplayHeight: test.height})
			if gotWidth != test.wantWidth || gotHeight != test.wantHeight {
				t.Fatalf("Dimensions = %dx%d, want %dx%d", gotWidth, gotHeight, test.wantWidth, test.wantHeight)
			}
		})
	}
}

func TestResponsiveLayoutsKeepContentInsideCanvas(t *testing.T) {
	resolutions := []struct{ width, height int }{
		{800, 600}, {1024, 768}, {1280, 1024}, {1366, 768}, {1920, 1080}, {2560, 1080}, {3440, 1440}, {1080, 1920},
	}
	for _, resolution := range resolutions {
		layout := calculateLayout(resolution.width, resolution.height, 6, 6, 2)
		for name, panel := range map[string]panelRect{"left": layout.Left, "right": layout.Right} {
			if panel.X < 0 || panel.Y < 0 || panel.Width <= 0 || panel.Height <= 0 || panel.X+panel.Width > float64(resolution.width)+0.01 || panel.Y+panel.Height > float64(resolution.height)+0.01 {
				t.Fatalf("%dx%d %s panel outside canvas: %#v", resolution.width, resolution.height, name, panel)
			}
		}
		lastValueBottom := layout.Left.Y + 82*layout.Scale + 5*layout.RowStep + 23*layout.Scale
		if lastValueBottom > layout.Left.Y+layout.Left.Height+0.01 {
			t.Fatalf("%dx%d system rows overflow: %.1f > %.1f", resolution.width, resolution.height, lastValueBottom, layout.Left.Y+layout.Left.Height)
		}
		lastHealthBottom := layout.Right.Y + 82*layout.Scale + 6*layout.RowStep + 2*25*layout.Scale
		if lastHealthBottom > layout.Right.Y+layout.Right.Height+0.01 {
			t.Fatalf("%dx%d health rows overflow: %.1f > %.1f", resolution.width, resolution.height, lastHealthBottom, layout.Right.Y+layout.Right.Height)
		}
	}
}

func TestAllTwelveColorVariantsRenderAtRepresentativeAspects(t *testing.T) {
	snapshot := sysinfo.Snapshot{
		Hostname: strings.Repeat("VERY-LONG-SERVER-NAME-", 4), OS: "Windows Server 2025 Datacenter Azure Edition with a deliberately long product name", Version: "24H2", Build: "26100.9999",
		CPU: strings.Repeat("Virtual processor family ", 8), GPU: strings.Repeat("Remote display adapter ", 7), Memory: "63.7 / 64.0 GiB · 99%", Disk: "4095.9 / 4096.0 GiB · 99%",
		IPs: []string{"10.0.2.15", "172.16.100.200", "192.168.255.254", "169.254.100.100"}, Serial: strings.Repeat("SERIAL", 16), Uptime: "999d 23h 59m",
		ServicesRunning: 999, ServicesTotal: 1000, FailedAutoServices: []string{"AnExtremelyLongAutomaticServiceNameOne", "AnotherVeryLongAutomaticServiceNameTwo", "ThirdService"},
		CriticalServices: []sysinfo.ServiceState{{Name: "Microsoft Defender Antivirus Service", Running: true}, {Name: "Dynamic Host Configuration Protocol Client", Running: true}, {Name: "Windows Event Log", Running: false}, {Name: "Windows Time Synchronization", Running: true}},
		RefreshedAt:      time.Date(2026, 7, 19, 12, 34, 56, 0, time.FixedZone("EDT", -4*60*60)),
	}
	for _, colorName := range config.BackgroundColors() {
		for _, mode := range []string{config.BackgroundDark, config.BackgroundLight} {
			cfg, _ := config.ForPreset(config.PresetOperations)
			cfg.BackgroundColor, cfg.BackgroundMode = colorName, mode
			cfg.Width, cfg.Height = 1366, 768
			img, err := Render(snapshot, cfg)
			if err != nil {
				t.Fatalf("%s/%s: %v", colorName, mode, err)
			}
			if img.Bounds() != image.Rect(0, 0, 1366, 768) {
				t.Fatalf("%s/%s bounds = %v", colorName, mode, img.Bounds())
			}
		}
	}
}

func TestMeasuredTruncationNeverExceedsWidth(t *testing.T) {
	measure := func(value string) float64 { return float64(len([]rune(value))) }
	got := truncateMeasured("abcdefghijklmnop", 6, measure)
	if got != "abcde…" || measure(got) > 6 {
		t.Fatalf("truncateMeasured = %q (%v)", got, measure(got))
	}
}

func TestPanelThemesStayOpaqueAndHighContrast(t *testing.T) {
	for _, mode := range []string{config.BackgroundDark, config.BackgroundLight} {
		cfg := config.Default()
		cfg.BackgroundMode = mode
		theme := themeFor(cfg)
		if theme.Panel.A != 255 || theme.Value.A != 255 {
			t.Fatalf("%s theme is translucent: panel=%v value=%v", mode, theme.Panel, theme.Value)
		}
		panelLuma := int(theme.Panel.R) + int(theme.Panel.G) + int(theme.Panel.B)
		valueLuma := int(theme.Value.R) + int(theme.Value.G) + int(theme.Value.B)
		if difference := panelLuma - valueLuma; difference > -300 && difference < 300 {
			t.Fatalf("%s panel/value contrast is too low: %d", mode, difference)
		}
	}
}

func TestPresetRowsAndGeneratedAtInvariant(t *testing.T) {
	snapshot := sysinfo.Snapshot{
		OS: "Windows 11 Enterprise", Version: "25H2", Build: "26200.1234",
		CPU: "CPU", GPU: "GPU", Memory: "memory", Disk: "disk", IPs: []string{"10.0.2.15"},
		Serial: "serial", Uptime: "1h", ServicesRunning: 10, ServicesTotal: 12,
		FailedAutoServices: []string{"ExampleSvc"},
		RefreshedAt:        time.Date(2026, 7, 18, 20, 1, 2, 0, time.FixedZone("EDT", -4*60*60)),
	}
	identity, _ := config.ForPreset(config.PresetIdentity)
	left, right := panelRows(snapshot, identity)
	if len(left) != 2 || len(right) != 2 {
		t.Fatalf("identity rows = %v / %v", left, right)
	}
	operations, _ := config.ForPreset(config.PresetOperations)
	_, right = panelRows(snapshot, operations)
	foundFailures := false
	for _, item := range right {
		foundFailures = foundFailures || item.label == "AUTO FAILURES"
	}
	if !foundFailures {
		t.Fatalf("operations rows = %v", right)
	}
	if got := generatedAtLabel(snapshot); got != "Generated at 2026-07-18 20:01:02 EDT" {
		t.Fatalf("generatedAtLabel = %q", got)
	}
}

func TestOverlayBrandLabel(t *testing.T) {
	if overlayBrandLabel != "W:ID  •  WALLPAPER IDENTITY" {
		t.Fatalf("overlayBrandLabel = %q", overlayBrandLabel)
	}
}
