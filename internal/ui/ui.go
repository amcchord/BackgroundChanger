// Package ui implements the native Windows setup and maintenance window.
package ui

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"github.com/amcchord/WallpaperIdentity/v4/internal/branding"
	"github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/overlay"
	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"github.com/amcchord/WallpaperIdentity/v4/internal/setup"
	"github.com/amcchord/WallpaperIdentity/v4/internal/sysinfo"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

type presetChoice struct {
	Name        string
	Title       string
	Description string
	Preview     walk.Image
}

func Main() error {
	logo, err := branding.Logo()
	if err != nil {
		return err
	}
	appIcon, err := walk.NewIconFromImage(logo)
	if err != nil {
		return fmt.Errorf("create W:ID application icon: %w", err)
	}
	defer appIcon.Dispose()
	logoBitmap, err := walk.NewBitmapFromImage(logo)
	if err != nil {
		return fmt.Errorf("create W:ID logo bitmap: %w", err)
	}
	defer logoBitmap.Dispose()
	previews, err := createPresetPreviews()
	if err != nil {
		return fmt.Errorf("create preset previews: %w", err)
	}
	defer disposePreviews(previews)
	choices := []presetChoice{
		{Name: config.PresetIdentity, Title: "Identity", Description: "OS, build, IP and serial", Preview: previews[config.PresetIdentity]},
		{Name: config.PresetBalanced, Title: "Balanced", Description: "Hardware, capacity and health", Preview: previews[config.PresetBalanced]},
		{Name: config.PresetOperations, Title: "Operations", Description: "Resources, restart and failures", Preview: previews[config.PresetOperations]},
	}
	presetLabels := []string{"Identity — find the machine", "Balanced — everyday status", "Operations — service health"}

	var window *walk.MainWindow
	var stateLabel, detailLabel *walk.Label
	var installButton, uninstallButton, closeButton *walk.PushButton
	var presetBox *walk.ComboBox
	working := false
	installed := setup.IsInstalled()
	legacyInstalled := setup.IsLegacyInstalled()
	state, detail := installationSummary(installed, legacyInstalled)

	err = (MainWindow{
		AssignTo: &window,
		Title:    "Wallpaper Identity Setup",
		Icon:     appIcon,
		MinSize:  Size{Width: 810, Height: 555},
		Size:     Size{Width: 840, Height: 575},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 14}, Spacing: 6},
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 12}, Children: []Widget{
				ImageView{Image: logoBitmap, Mode: ImageViewModeShrink, MinSize: Size{Width: 64, Height: 64}, MaxSize: Size{Width: 64, Height: 64}},
				Composite{Layout: VBox{Spacing: 1}, Children: []Widget{
					Label{Text: "Wallpaper Identity", Font: Font{Family: "Segoe UI", PointSize: 23, Bold: true}},
					Label{Text: "W:ID  •  Machine identity and health, visible before sign-in", Font: Font{Family: "Segoe UI", PointSize: 11}},
				}},
			}},
			GroupBox{
				Title:  "Current status",
				Layout: VBox{Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 10}, Spacing: 3},
				Children: []Widget{
					Label{AssignTo: &stateLabel, Text: state, Font: Font{Family: "Segoe UI Semibold", PointSize: 12}},
					Label{AssignTo: &detailLabel, Text: detail, Font: Font{Family: "Segoe UI", PointSize: 9}},
				},
			},
			GroupBox{
				Title:  "Choose a starting layout",
				Layout: VBox{Margins: Margins{Left: 12, Top: 9, Right: 12, Bottom: 9}, Spacing: 6},
				Children: []Widget{
					Composite{Layout: HBox{Spacing: 12}, Children: presetPreviewWidgets(choices)},
					Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
						Label{Text: "Install preset:", Font: Font{Family: "Segoe UI Semibold", PointSize: 9}},
						ComboBox{AssignTo: &presetBox, Model: presetLabels, CurrentIndex: 1, Enabled: !installed, MinSize: Size{Width: 230}},
						Label{Text: "Advanced: edit config.yml in " + paths.DataDir() + ".", Font: Font{Family: "Segoe UI", PointSize: 8}},
					}},
				},
			},
			Label{
				Text: "One automatic LocalSystem service renders at boot, after session changes, and every five minutes. The Generated at timestamp is always visible.",
				Font: Font{Family: "Segoe UI", PointSize: 9},
			},
			VSpacer{},
			Composite{
				Layout: HBox{Spacing: 10},
				Children: []Widget{
					PushButton{AssignTo: &installButton, Text: installText(installed)},
					PushButton{AssignTo: &uninstallButton, Text: "Uninstall", Enabled: installed && !legacyInstalled},
					HSpacer{},
					PushButton{AssignTo: &closeButton, Text: "Close", OnClicked: func() { window.Close() }},
				},
			},
			Label{Text: fmt.Sprintf("W:ID  •  Version %s  •  Windows Enterprise, Education, IoT Enterprise, or Server", buildinfo.Version), Font: Font{Family: "Segoe UI", PointSize: 8}},
		},
	}).Create()
	if err != nil {
		return err
	}
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if working {
			*canceled = true
		}
	})
	centerInWorkArea(window)

	run := func(uninstall bool) {
		preset := ""
		if !uninstall && !setup.IsInstalled() {
			index := presetBox.CurrentIndex()
			if index < 0 || index >= len(choices) {
				index = 1
			}
			preset = choices[index].Name
		}
		args := []string{"--install"}
		if preset != "" {
			args = append(args, "--preset", preset)
		}
		if uninstall {
			args = []string{"--uninstall"}
		}
		if !setup.IsAdministrator() {
			if err := setup.RelaunchElevated(args...); err != nil {
				walk.MsgBox(window, "Wallpaper Identity Setup", "Administrator elevation failed:\n\n"+err.Error(), walk.MsgBoxIconError)
				return
			}
			window.Close()
			return
		}
		installButton.SetEnabled(false)
		uninstallButton.SetEnabled(false)
		closeButton.SetEnabled(false)
		presetBox.SetEnabled(false)
		working = true
		stateLabel.SetText("Working…")
		detailLabel.SetText("Preparing the requested change.")
		go func() {
			progress := func(_ int, message string) {
				window.Synchronize(func() { detailLabel.SetText(message) })
			}
			var opErr error
			if uninstall {
				opErr = setup.Uninstall(progress, false)
			} else {
				opErr = setup.InstallWithPreset(progress, preset)
			}
			window.Synchronize(func() {
				working = false
				installedNow := setup.IsInstalled()
				installButton.SetText(installText(installedNow))
				installButton.SetEnabled(true)
				uninstallButton.SetEnabled(installedNow && !setup.IsLegacyInstalled())
				closeButton.SetEnabled(true)
				presetBox.SetEnabled(!installedNow)
				if opErr != nil {
					stateLabel.SetText("The operation could not be completed")
					detailLabel.SetText(opErr.Error())
					walk.MsgBox(window, "Wallpaper Identity Setup", opErr.Error(), walk.MsgBoxIconError)
				} else {
					newState, newDetail := installationSummary(installedNow, setup.IsLegacyInstalled())
					stateLabel.SetText(newState)
					detailLabel.SetText(newDetail)
				}
			})
		}()
	}
	installButton.Clicked().Attach(func() { run(false) })
	uninstallButton.Clicked().Attach(func() {
		if walk.MsgBox(window, "Uninstall Wallpaper Identity", "Remove the W:ID service and its Windows lock-screen policy?\n\nGenerated images and configuration will be kept in "+paths.DataDir()+".", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			run(true)
		}
	})
	window.Run()
	return nil
}

func presetPreviewWidgets(choices []presetChoice) []Widget {
	widgets := make([]Widget, 0, len(choices))
	for _, choice := range choices {
		choice := choice
		widgets = append(widgets, Composite{
			Layout:  VBox{Spacing: 4},
			MinSize: Size{Width: 215},
			Children: []Widget{
				Label{Text: choice.Title, Font: Font{Family: "Segoe UI Semibold", PointSize: 10}},
				ImageView{Image: choice.Preview, Mode: ImageViewModeShrink, MinSize: Size{Width: 210, Height: 118}, MaxSize: Size{Width: 210, Height: 118}},
				Label{Text: choice.Description, Font: Font{Family: "Segoe UI", PointSize: 8}},
			},
		})
	}
	return widgets
}

func centerInWorkArea(window *walk.MainWindow) {
	const spiGetWorkArea = 0x0030
	var workArea win.RECT
	if !win.SystemParametersInfo(spiGetWorkArea, 0, unsafe.Pointer(&workArea), 0) {
		return
	}
	bounds := window.Bounds()
	workWidth := int(workArea.Right - workArea.Left)
	workHeight := int(workArea.Bottom - workArea.Top)
	bounds.X = int(workArea.Left) + (workWidth-bounds.Width)/2
	bounds.Y = int(workArea.Top) + (workHeight-bounds.Height)/2
	_ = window.SetBounds(bounds)
}

func createPresetPreviews() (map[string]walk.Image, error) {
	snapshot := sysinfo.Snapshot{
		Hostname: "LAB-RACK-07", OS: "Windows 11 Enterprise", Edition: "Enterprise", Version: "25H2", Build: "26200.6584",
		CPU: "Intel Core i7 • 8 logical", GPU: "Display adapter", Memory: "3.2 / 8.0 GiB • 40%", Disk: "22.4 / 64.0 GiB • 35%",
		IPs: []string{"10.0.2.15"}, Serial: "RACK07-2026", Uptime: "2d 4h 12m", ServicesRunning: 124, ServicesTotal: 132,
		CriticalServices: []sysinfo.ServiceState{
			{Name: "Defender", Running: true}, {Name: "DHCP", Running: true},
			{Name: "DNS Client", Running: true}, {Name: "Event Log", Running: true},
		},
		FailedAutoServices: []string{"ExampleSvc"}, PendingReboot: true,
		DisplayWidth: 1280, DisplayHeight: 720, RefreshedAt: time.Now(),
	}
	result := make(map[string]walk.Image, 3)
	for _, name := range []string{config.PresetIdentity, config.PresetBalanced, config.PresetOperations} {
		cfg, _ := config.ForPreset(name)
		cfg.Width, cfg.Height = 1280, 720
		img, err := overlay.Render(snapshot, cfg)
		if err != nil {
			disposePreviews(result)
			return nil, err
		}
		preview, err := walk.NewBitmapFromImage(img)
		if err != nil {
			disposePreviews(result)
			return nil, err
		}
		result[name] = preview
	}
	return result, nil
}

func disposePreviews(previews map[string]walk.Image) {
	for _, preview := range previews {
		preview.Dispose()
	}
}

func RunOperation(title string, operation func(setup.ProgressFunc) error) error {
	logo, err := branding.Logo()
	if err != nil {
		return err
	}
	appIcon, err := walk.NewIconFromImage(logo)
	if err != nil {
		return err
	}
	defer appIcon.Dispose()
	var window *walk.MainWindow
	var statusLabel *walk.Label
	var progressBar *walk.ProgressBar
	var closeButton *walk.PushButton
	err = (MainWindow{
		AssignTo: &window,
		Title:    title,
		Icon:     appIcon,
		MinSize:  Size{Width: 570, Height: 260},
		Size:     Size{Width: 570, Height: 260},
		Layout:   VBox{Margins: Margins{Left: 30, Top: 28, Right: 30, Bottom: 24}, Spacing: 14},
		Children: []Widget{
			Label{Text: title, Font: Font{Family: "Segoe UI", PointSize: 20, Bold: true}},
			Label{AssignTo: &statusLabel, Text: "Starting…", Font: Font{Family: "Segoe UI", PointSize: 10}},
			ProgressBar{AssignTo: &progressBar, MinValue: 0, MaxValue: 100},
			VSpacer{},
			Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &closeButton, Text: "Please wait…", Enabled: false, OnClicked: func() { window.Close() }}}},
		},
	}).Create()
	if err != nil {
		return err
	}
	working := true
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if working {
			*canceled = true
		}
	})
	var result error
	go func() {
		result = operation(func(percent int, message string) {
			window.Synchronize(func() {
				progressBar.SetValue(percent)
				statusLabel.SetText(message)
			})
		})
		window.Synchronize(func() {
			working = false
			closeButton.SetText("Close")
			closeButton.SetEnabled(true)
			if result != nil {
				statusLabel.SetText(result.Error())
				walk.MsgBox(window, title, result.Error(), walk.MsgBoxIconError)
			}
		})
	}()
	window.Run()
	return result
}

func installationSummary(installed, legacyInstalled bool) (string, string) {
	if !installed {
		return "Not installed", "Choose a starting layout, then select Install."
	}
	if legacyInstalled {
		return "Previous version detected", "Repair / Upgrade will migrate its service, configuration, generated images, and policy backups to Wallpaper Identity."
	}
	status, err := setup.ReadStatus()
	if err != nil {
		return "Installed", "The service is registered; its first refresh has not reported yet."
	}
	if !status.Success {
		return "Installed — attention required", status.Error
	}
	age := time.Since(status.CompletedAt).Round(time.Second)
	if !status.EditionSupported {
		return "Installed — Windows edition unsupported", fmt.Sprintf("The service refreshed %s ago on %s, but Windows Pro/Home ignores this policy without broader device-management settings.", age, status.Snapshot.Hostname)
	}
	return "Installed and active", fmt.Sprintf("Last refreshed %s ago on %s. Supported Windows edition.", age, status.Snapshot.Hostname)
}

func installText(installed bool) string {
	if installed {
		return "Repair / Upgrade"
	}
	return "Install"
}

// Keep os referenced in GUI-subsystem builds where platform linkers can be aggressive.
var _ = os.ErrNotExist
