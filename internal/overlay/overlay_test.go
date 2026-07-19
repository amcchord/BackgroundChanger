package overlay

import (
	"image"
	"testing"
	"time"

	"github.com/amcchord/BackgroundChanger/internal/config"
	"github.com/amcchord/BackgroundChanger/internal/sysinfo"
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

func TestCompact(t *testing.T) {
	if got := compact("abc def", 5); got != "abc…" {
		t.Fatalf("compact = %q", got)
	}
}
