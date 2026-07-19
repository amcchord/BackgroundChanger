// Package ui implements the native Windows setup and maintenance window.
package ui

import (
	"fmt"
	"image"
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
	"golang.org/x/image/draw"
)

const (
	defaultPresetIndex  = 1
	presetPreviewWidth  = 238
	presetPreviewHeight = 134
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

	var window *walk.Dialog
	var stateLabel, detailLabel, selectionLabel *walk.Label
	var installButton, uninstallButton, closeButton *walk.PushButton
	presetButtons := make([]*walk.PushButton, len(choices))
	working := false
	installed := setup.IsInstalled()
	legacyInstalled := setup.IsLegacyInstalled()
	selectedPreset := defaultPresetIndex
	presetChanged := false
	if installed {
		selectedPreset = -1
		if current, loadErr := config.Load(paths.ConfigFile()); loadErr == nil {
			selectedPreset = presetIndex(choices, current.Preset)
		}
	}
	state, detail := installationSummary(installed, legacyInstalled)
	platformNote := "Native policy support: Windows Enterprise, Education, IoT Enterprise, and Server."
	if sysinfo.IsProfessionalEdition(sysinfo.CurrentEdition()) {
		platformNote = "Windows Pro: compatibility turns off tips, advertising ID, and consumer experiences."
	}
	selectPreset := func(index int) {
		if index < 0 || index >= len(choices) {
			return
		}
		selectedPreset = index
		presetChanged = installed
		updatePresetButtons(presetButtons, choices, selectedPreset)
		if selectionLabel != nil {
			selectionLabel.SetText(presetSelectionText(choices, selectedPreset, installed, presetChanged))
		}
	}

	err = (Dialog{
		AssignTo:      &window,
		Title:         "Wallpaper Identity Setup",
		Icon:          appIcon,
		FixedSize:     true,
		Size:          Size{Width: 900, Height: 630},
		Layout:        VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 18}, Spacing: 10},
		DefaultButton: &installButton,
		CancelButton:  &closeButton,
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 12}, Children: []Widget{
				ImageView{Image: logoBitmap, Mode: ImageViewModeShrink, MinSize: Size{Width: 64, Height: 64}, MaxSize: Size{Width: 64, Height: 64}},
				Composite{Layout: VBox{Spacing: 1}, Children: []Widget{
					Label{Text: "Wallpaper Identity", Font: Font{Family: "Segoe UI", PointSize: 23, Bold: true}},
					Label{Text: "See this machine's identity and health before anyone signs in", Font: Font{Family: "Segoe UI", PointSize: 11}},
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
				Title:  "Choose what to show",
				Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
				Children: []Widget{
					Label{Text: "Click a preview to select its information mix. Hostname and Generated at are always included.", Font: Font{Family: "Segoe UI", PointSize: 9}},
					Composite{Layout: HBox{Spacing: 12}, Children: presetPreviewWidgets(choices, presetButtons, selectedPreset, selectPreset)},
					Label{AssignTo: &selectionLabel, Text: presetSelectionText(choices, selectedPreset, installed, presetChanged), Font: Font{Family: "Segoe UI Semibold", PointSize: 9}},
					Label{Text: "Power users can tune every field later in " + paths.ConfigFile(), Font: Font{Family: "Segoe UI", PointSize: 8}},
				},
			},
			Label{
				Text: "A small LocalSystem service updates the image at boot, after session changes, and every five minutes.",
				Font: Font{Family: "Segoe UI", PointSize: 9},
			},
			Label{Text: platformNote, Font: Font{Family: "Segoe UI", PointSize: 8}},
			VSpacer{},
			HSeparator{},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					Label{Text: fmt.Sprintf("W:ID  •  Version %s", buildinfo.Version), Font: Font{Family: "Segoe UI", PointSize: 8}},
					HSpacer{},
					PushButton{AssignTo: &uninstallButton, Text: "Uninstall", Enabled: installed && !legacyInstalled, MinSize: Size{Width: 96}},
					PushButton{AssignTo: &closeButton, Text: "Close", MinSize: Size{Width: 96}, OnClicked: func() { window.Cancel() }},
					PushButton{AssignTo: &installButton, Text: installText(installed), MinSize: Size{Width: 120}},
				},
			},
		},
	}).Create(nil)
	if err != nil {
		return err
	}
	_ = installButton.SetFocus()
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if working {
			*canceled = true
		}
	})
	centerInWorkArea(window)

	run := func(uninstall bool) {
		preset := ""
		applyPreset := !uninstall && (!setup.IsInstalled() || presetChanged)
		if applyPreset {
			index := selectedPreset
			if index < 0 || index >= len(choices) {
				index = defaultPresetIndex
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
			window.Cancel()
			return
		}
		installButton.SetEnabled(false)
		uninstallButton.SetEnabled(false)
		closeButton.SetEnabled(false)
		setPresetButtonsEnabled(presetButtons, false)
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
				installed = installedNow
				installButton.SetText(installText(installedNow))
				installButton.SetEnabled(true)
				uninstallButton.SetEnabled(installedNow && !setup.IsLegacyInstalled())
				closeButton.SetEnabled(true)
				setPresetButtonsEnabled(presetButtons, true)
				if opErr != nil {
					presetChanged = applyPreset && installedNow
					stateLabel.SetText("The operation could not be completed")
					detailLabel.SetText(opErr.Error())
					selectionLabel.SetText(presetSelectionText(choices, selectedPreset, installedNow, presetChanged))
					walk.MsgBox(window, "Wallpaper Identity Setup", opErr.Error(), walk.MsgBoxIconError)
				} else {
					presetChanged = false
					newState, newDetail := installationSummary(installedNow, setup.IsLegacyInstalled())
					stateLabel.SetText(newState)
					detailLabel.SetText(newDetail)
					selectionLabel.SetText(presetSelectionText(choices, selectedPreset, installedNow, presetChanged))
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

func presetPreviewWidgets(choices []presetChoice, buttons []*walk.PushButton, selected int, selectPreset func(int)) []Widget {
	widgets := make([]Widget, 0, len(choices))
	for index, choice := range choices {
		index := index
		choice := choice
		widgets = append(widgets, Composite{
			Layout:  VBox{Spacing: 3},
			MinSize: Size{Width: 246},
			Children: []Widget{
				PushButton{
					AssignTo:       &buttons[index],
					Text:           presetButtonText(choice, index == selected),
					Image:          choice.Preview,
					ImageAboveText: true,
					MinSize:        Size{Width: 246, Height: 154},
					ToolTipText:    "Select " + choice.Title + ": " + choice.Description,
					OnClicked:      func() { selectPreset(index) },
				},
				Label{Text: choice.Description, Font: Font{Family: "Segoe UI", PointSize: 8}},
			},
		})
	}
	return widgets
}

func presetButtonText(choice presetChoice, selected bool) string {
	prefix := ""
	if selected {
		prefix = "✓  "
	}
	return prefix + choice.Title
}

func presetSelectionText(choices []presetChoice, selected int, installed, changed bool) string {
	if selected < 0 || selected >= len(choices) {
		if installed {
			return "Current layout: Custom configuration. Select a preview to replace it."
		}
		selected = defaultPresetIndex
		if selected < 0 || selected >= len(choices) {
			return "Select a layout to continue."
		}
	}
	prefix := "Selected"
	if installed && changed {
		prefix = "Selected for update"
	} else if installed {
		prefix = "Current layout"
	}
	return fmt.Sprintf("%s: %s — %s", prefix, choices[selected].Title, choices[selected].Description)
}

func presetIndex(choices []presetChoice, name string) int {
	for index, choice := range choices {
		if choice.Name == name {
			return index
		}
	}
	return -1
}

func updatePresetButtons(buttons []*walk.PushButton, choices []presetChoice, selected int) {
	for index, button := range buttons {
		if button != nil {
			_ = button.SetText(presetButtonText(choices[index], index == selected))
		}
	}
}

func setPresetButtonsEnabled(buttons []*walk.PushButton, enabled bool) {
	for _, button := range buttons {
		if button != nil {
			button.SetEnabled(enabled)
		}
	}
}

func centerInWorkArea(window walk.Window) {
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
	dpi := primaryScreenDPI()
	pixelWidth, pixelHeight := presetPreviewPixelSize(dpi)
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
		thumbnail := image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
		draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, img.Bounds(), draw.Over, nil)
		preview, err := walk.NewBitmapFromImageForDPI(thumbnail, dpi)
		if err != nil {
			disposePreviews(result)
			return nil, err
		}
		result[name] = preview
	}
	return result, nil
}

func primaryScreenDPI() int {
	hdc := win.GetDC(0)
	if hdc == 0 {
		return 96
	}
	defer win.ReleaseDC(0, hdc)
	dpi := int(win.GetDeviceCaps(hdc, win.LOGPIXELSX))
	if dpi < 96 {
		return 96
	}
	return dpi
}

func presetPreviewPixelSize(dpi int) (int, int) {
	if dpi < 96 {
		dpi = 96
	}
	return walk.IntFrom96DPI(presetPreviewWidth, dpi), walk.IntFrom96DPI(presetPreviewHeight, dpi)
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
	var window *walk.Dialog
	var statusLabel, detailLabel *walk.Label
	var progressBar *walk.ProgressBar
	var closeButton *walk.PushButton
	err = (Dialog{
		AssignTo:      &window,
		Title:         title,
		Icon:          appIcon,
		FixedSize:     true,
		Size:          Size{Width: 610, Height: 300},
		Layout:        VBox{Margins: Margins{Left: 30, Top: 26, Right: 30, Bottom: 22}, Spacing: 12},
		DefaultButton: &closeButton,
		CancelButton:  &closeButton,
		Children: []Widget{
			Label{Text: title, Font: Font{Family: "Segoe UI", PointSize: 20, Bold: true}},
			Label{AssignTo: &statusLabel, Text: "Starting…", Font: Font{Family: "Segoe UI Semibold", PointSize: 12}},
			ProgressBar{AssignTo: &progressBar, MinValue: 0, MaxValue: 100},
			Label{AssignTo: &detailLabel, Text: "Please wait while Windows applies and verifies the requested change.", Font: Font{Family: "Segoe UI", PointSize: 9}},
			VSpacer{},
			HSeparator{},
			Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &closeButton, Text: "Please wait…", Enabled: false, MinSize: Size{Width: 100}, OnClicked: func() { window.Cancel() }}}},
		},
	}).Create(nil)
	if err != nil {
		return err
	}
	centerInWorkArea(window)
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
			headline, detail := operationCompletionText(result)
			statusLabel.SetText(headline)
			detailLabel.SetText(detail)
			if result != nil {
				walk.MsgBox(window, title, result.Error(), walk.MsgBoxIconError)
			} else {
				progressBar.SetValue(100)
			}
			_ = closeButton.SetFocus()
		})
	}()
	window.Run()
	return result
}

func operationCompletionText(result error) (string, string) {
	if result != nil {
		return "Could not complete the operation", result.Error()
	}
	return "Done", "The requested change completed successfully. You can close this window."
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
		return "Installed — policy unverified", fmt.Sprintf("The saved status from %s does not confirm an effective lock-screen policy. Run Repair / Upgrade.", status.CompletedAt.Format(time.RFC3339))
	}
	return "Installed — policy verified", fmt.Sprintf("Last refreshed %s ago on %s. Windows confirmed the managed lock-screen image.", age, status.Snapshot.Hostname)
}

func installText(installed bool) string {
	if installed {
		return "Repair / Upgrade"
	}
	return "Install"
}

// Keep os referenced in GUI-subsystem builds where platform linkers can be aggressive.
var _ = os.ErrNotExist
