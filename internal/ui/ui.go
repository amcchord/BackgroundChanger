// Package ui implements the native Windows setup and maintenance window.
package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/amcchord/WallpaperIdentity/v4/internal/branding"
	"github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/loginscreen"
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
	defaultPresetIndex      = 1
	presetPreviewWidth      = 238
	presetPreviewHeight     = 134
	appearancePreviewWidth  = 400
	appearancePreviewHeight = 225
	colorSwatchWidth        = 92
	colorSwatchHeight       = 22
)

type presetChoice struct {
	Name        string
	Title       string
	Description string
	Preview     walk.Image
}

type backgroundChoice struct {
	Name  string
	Title string
}

var backgroundChoices = []backgroundChoice{
	{Name: config.BackgroundBlue, Title: "Azure"},
	{Name: config.BackgroundTeal, Title: "Teal"},
	{Name: config.BackgroundGreen, Title: "Forest"},
	{Name: config.BackgroundPurple, Title: "Indigo"},
	{Name: config.BackgroundSlate, Title: "Slate"},
	{Name: config.BackgroundCopper, Title: "Copper"},
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
	installed := setup.IsInstalled()
	legacyInstalled := setup.IsLegacyInstalled()
	appearance := config.Default()
	selectedPreset := defaultPresetIndex
	hasSavedConfiguration := fileExists(paths.ConfigFile()) || fileExists(paths.LegacyConfigFile())
	if current, loadErr := config.Load(paths.ConfigFile()); loadErr == nil {
		appearance = current
		appearance.BaseImage = paths.ResolveBackgroundImage(current.BaseImage)
		if index := presetIndexByName(current.Preset); index >= 0 {
			selectedPreset = index
		} else if installed {
			selectedPreset = -1
		}
	} else if installed {
		selectedPreset = -1
	}
	freshInstall := !installed && !legacyInstalled && !hasSavedConfiguration
	currentWindowsBackground := loginscreen.CurrentBackgroundImage{}
	currentWindowsBackgroundFound := false
	if freshInstall {
		currentWindowsBackground, currentWindowsBackgroundFound = loginscreen.FindCurrentBackgroundImage()
		if currentWindowsBackgroundFound {
			appearance.BaseImage = currentWindowsBackground.Path
		} else {
			appearance.BaseImage = ""
		}
	}
	initialAppearance := appearance
	defaultBackgroundSelected := true
	selectedBackgroundImage := ""
	pendingBackgroundImage := ""
	backgroundChanged := false
	useColors := false

	previews, err := createPresetPreviews(appearance)
	if err != nil {
		return fmt.Errorf("create preset previews: %w", err)
	}
	choices := []presetChoice{
		{Name: config.PresetIdentity, Title: "Identity", Description: "OS, build, IP and serial", Preview: previews[config.PresetIdentity]},
		{Name: config.PresetBalanced, Title: "Balanced", Description: "Hardware, capacity and health", Preview: previews[config.PresetBalanced]},
		{Name: config.PresetOperations, Title: "Operations", Description: "Resources, restart and failures", Preview: previews[config.PresetOperations]},
	}
	swatches, err := createBackgroundSwatches(appearance.BackgroundMode)
	if err != nil {
		disposePreviews(previews)
		return fmt.Errorf("create background swatches: %w", err)
	}
	appearancePreview, err := createAppearancePreview(appearance, selectedPresetName(selectedPreset))
	if err != nil {
		disposePreviews(previews)
		disposePreviews(swatches)
		return fmt.Errorf("create background preview: %w", err)
	}
	defer func() {
		disposePreviews(previews)
		disposePreviews(swatches)
		if appearancePreview != nil {
			appearancePreview.Dispose()
		}
	}()

	var window *walk.Dialog
	var layoutPage, backgroundPage *walk.Composite
	var stateLabel, detailLabel, selectionLabel, backgroundSelectionLabel *walk.Label
	var appearanceImageView *walk.ImageView
	var defaultBackgroundButton, imageWell, advanceButton, backButton, uninstallButton, closeButton *walk.PushButton
	presetButtons := make([]*walk.PushButton, len(choices))
	colorButtons := make([]*walk.PushButton, len(backgroundChoices))
	modeButtons := make([]*walk.PushButton, 2)
	working := false
	presetChanged := false
	page := 0
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
		if appearanceImageView != nil {
			newPreview, previewErr := createAppearancePreview(appearance, selectedPresetName(selectedPreset))
			if previewErr == nil {
				_ = appearanceImageView.SetImage(newPreview)
				appearancePreview.Dispose()
				appearancePreview = newPreview
			}
		}
	}
	refreshAppearance := func(refreshSwatches bool) error {
		newPreviews, refreshErr := createPresetPreviews(appearance)
		if refreshErr != nil {
			return refreshErr
		}
		newAppearance, refreshErr := createAppearancePreview(appearance, selectedPresetName(selectedPreset))
		if refreshErr != nil {
			disposePreviews(newPreviews)
			return refreshErr
		}
		var newSwatches map[string]walk.Image
		if refreshSwatches {
			newSwatches, refreshErr = createBackgroundSwatches(appearance.BackgroundMode)
			if refreshErr != nil {
				disposePreviews(newPreviews)
				newAppearance.Dispose()
				return refreshErr
			}
		}
		for index, choice := range choices {
			_ = presetButtons[index].SetImage(newPreviews[choice.Name])
			choices[index].Preview = newPreviews[choice.Name]
		}
		if appearanceImageView != nil {
			_ = appearanceImageView.SetImage(newAppearance)
		}
		if refreshSwatches {
			for index, choice := range backgroundChoices {
				_ = colorButtons[index].SetImage(newSwatches[choice.Name])
			}
			disposePreviews(swatches)
			swatches = newSwatches
		}
		disposePreviews(previews)
		previews = newPreviews
		appearancePreview.Dispose()
		appearancePreview = newAppearance
		return nil
	}
	updateBackgroundControls := func() {
		replacementSelected := !defaultBackgroundSelected
		updateBackgroundButtons(colorButtons, backgroundChoices, appearance.BackgroundColor, !replacementSelected || selectedBackgroundImage != "")
		updateModeButtons(modeButtons, appearance.BackgroundMode)
		if defaultBackgroundButton != nil {
			_ = defaultBackgroundButton.SetText(defaultBackgroundButtonText(defaultBackgroundSelected, freshInstall))
		}
		if imageWell != nil {
			_ = imageWell.SetText(backgroundImageWellText(selectedBackgroundImage))
		}
		if backgroundSelectionLabel != nil {
			backgroundSelectionLabel.SetText(backgroundSelectionText(appearance, selectedBackgroundImage, installed, backgroundChanged, defaultBackgroundSelected, freshInstall, currentWindowsBackgroundFound))
		}
	}
	selectDefaultBackground := func() {
		appearance.BackgroundColor = initialAppearance.BackgroundColor
		appearance.BackgroundMode = initialAppearance.BackgroundMode
		appearance.BaseImage = initialAppearance.BaseImage
		defaultBackgroundSelected = true
		selectedBackgroundImage = ""
		pendingBackgroundImage = ""
		useColors = false
		backgroundChanged = false
		updateBackgroundControls()
		if refreshErr := refreshAppearance(true); refreshErr != nil && window != nil {
			walk.MsgBox(window, "Wallpaper Identity Setup", refreshErr.Error(), walk.MsgBoxIconError)
		}
	}
	selectBackgroundColor := func(name string) {
		appearance.BackgroundColor = name
		appearance.BaseImage = ""
		defaultBackgroundSelected = false
		selectedBackgroundImage = ""
		pendingBackgroundImage = ""
		useColors = true
		backgroundChanged = true
		updateBackgroundControls()
		if refreshErr := refreshAppearance(false); refreshErr != nil && window != nil {
			walk.MsgBox(window, "Wallpaper Identity Setup", refreshErr.Error(), walk.MsgBoxIconError)
		}
	}
	selectBackgroundMode := func(mode string) {
		if appearance.BackgroundMode == mode {
			return
		}
		appearance.BackgroundMode = mode
		backgroundChanged = true
		updateBackgroundControls()
		if refreshErr := refreshAppearance(true); refreshErr != nil && window != nil {
			walk.MsgBox(window, "Wallpaper Identity Setup", refreshErr.Error(), walk.MsgBoxIconError)
		}
	}
	selectBackgroundImage := func(path string) {
		absolute, pathErr := filepath.Abs(path)
		if pathErr != nil {
			walk.MsgBox(window, "Choose a background image", pathErr.Error(), walk.MsgBoxIconError)
			return
		}
		probe := appearance
		probe.BaseImage = absolute
		if validateErr := config.ValidateAssets(probe); validateErr != nil {
			walk.MsgBox(window, "Choose a background image", "Use a readable JPEG or PNG file.\n\n"+validateErr.Error(), walk.MsgBoxIconError)
			return
		}
		appearance.BaseImage = absolute
		defaultBackgroundSelected = false
		selectedBackgroundImage = absolute
		pendingBackgroundImage = absolute
		useColors = false
		backgroundChanged = true
		updateBackgroundControls()
		if refreshErr := refreshAppearance(false); refreshErr != nil {
			walk.MsgBox(window, "Wallpaper Identity Setup", refreshErr.Error(), walk.MsgBoxIconError)
		}
	}
	browseBackground := func() {
		dialog := walk.FileDialog{
			Title: "Choose a Wallpaper Identity background", Filter: "Image files (*.jpg;*.jpeg;*.png)|*.jpg;*.jpeg;*.png|All files (*.*)|*.*", FilterIndex: 1,
		}
		accepted, dialogErr := dialog.ShowOpen(window)
		if dialogErr != nil {
			walk.MsgBox(window, "Choose a background image", dialogErr.Error(), walk.MsgBoxIconError)
			return
		}
		if accepted {
			selectBackgroundImage(dialog.FilePath)
		}
	}
	showPage := func(index int) {
		page = index
		if layoutPage != nil {
			layoutPage.SetVisible(page == 0)
			backgroundPage.SetVisible(page == 1)
			backButton.SetEnabled(page == 1)
			if page == 0 {
				_ = advanceButton.SetText("Next")
			} else {
				_ = advanceButton.SetText(installText(installed))
			}
			_ = advanceButton.SetFocus()
		}
	}

	err = (Dialog{
		AssignTo:      &window,
		Title:         "Wallpaper Identity Setup",
		Icon:          appIcon,
		FixedSize:     true,
		Size:          Size{Width: 900, Height: 700},
		Layout:        VBox{Margins: Margins{Left: 24, Top: 20, Right: 24, Bottom: 18}, Spacing: 10},
		DefaultButton: &advanceButton,
		CancelButton:  &closeButton,
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 12}, Children: []Widget{
				ImageView{Image: logoBitmap, Mode: ImageViewModeShrink, MinSize: Size{Width: 64, Height: 64}, MaxSize: Size{Width: 64, Height: 64}},
				Composite{Layout: VBox{Spacing: 1}, Children: []Widget{
					Label{Text: "Wallpaper Identity", Font: Font{Family: "Segoe UI", PointSize: 23, Bold: true}},
					Label{Text: "See this machine's identity and health before anyone signs in", Font: Font{Family: "Segoe UI", PointSize: 11}},
				}},
			}},
			Composite{
				AssignTo: &layoutPage,
				Layout:   VBox{Spacing: 10},
				Children: []Widget{
					GroupBox{
						Title:  "Current status",
						Layout: VBox{Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 10}, Spacing: 3},
						Children: []Widget{
							Label{AssignTo: &stateLabel, Text: state, Font: Font{Family: "Segoe UI Semibold", PointSize: 12}},
							Label{AssignTo: &detailLabel, Text: detail, Font: Font{Family: "Segoe UI", PointSize: 9}},
						},
					},
					GroupBox{
						Title:  "1 of 2 — Choose what to show",
						Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
						Children: []Widget{
							Label{Text: "Click a preview to select its information mix. Hostname and Generated at are always included.", Font: Font{Family: "Segoe UI", PointSize: 9}},
							Composite{Layout: HBox{Spacing: 12}, Children: presetPreviewWidgets(choices, presetButtons, selectedPreset, selectPreset)},
							Label{AssignTo: &selectionLabel, Text: presetSelectionText(choices, selectedPreset, installed, presetChanged), Font: Font{Family: "Segoe UI Semibold", PointSize: 9}},
							Label{Text: "Power users can tune every field later in " + paths.ConfigFile(), Font: Font{Family: "Segoe UI", PointSize: 8}},
						},
					},
				},
			},
			Composite{
				AssignTo: &backgroundPage,
				Visible:  false,
				Layout:   VBox{Spacing: 10},
				Children: []Widget{
					GroupBox{
						Title:  "2 of 2 — Choose the background",
						Layout: VBox{Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 10}, Spacing: 8},
						Children: []Widget{
							Label{Text: "W:ID starts with the background Windows already uses. Choose a replacement only when you want one.", Font: Font{Family: "Segoe UI", PointSize: 9}},
							Composite{Layout: HBox{Spacing: 18}, Children: []Widget{
								ImageView{AssignTo: &appearanceImageView, Image: appearancePreview, Mode: ImageViewModeShrink, MinSize: Size{Width: appearancePreviewWidth, Height: appearancePreviewHeight}, MaxSize: Size{Width: appearancePreviewWidth, Height: appearancePreviewHeight}},
								Composite{Layout: VBox{Spacing: 6}, MinSize: Size{Width: 325}, Children: []Widget{
									PushButton{AssignTo: &defaultBackgroundButton, Text: defaultBackgroundButtonText(defaultBackgroundSelected, freshInstall), MinSize: Size{Width: 320, Height: 36}, ToolTipText: defaultBackgroundTooltip(freshInstall, currentWindowsBackground, currentWindowsBackgroundFound), OnClicked: selectDefaultBackground},
									Label{Text: "Replacement color", Font: Font{Family: "Segoe UI Semibold", PointSize: 9}},
									Composite{Layout: Grid{Columns: 3, Spacing: 6}, Children: backgroundColorWidgets(backgroundChoices, colorButtons, swatches, appearance.BackgroundColor, defaultBackgroundSelected || selectedBackgroundImage != "", selectBackgroundColor)},
									Composite{Layout: HBox{Spacing: 6}, Children: []Widget{
										Label{Text: "Appearance:", Font: Font{Family: "Segoe UI Semibold", PointSize: 9}},
										PushButton{AssignTo: &modeButtons[0], Text: backgroundModeButtonText(config.BackgroundDark, appearance.BackgroundMode), MinSize: Size{Width: 86}, OnClicked: func() { selectBackgroundMode(config.BackgroundDark) }},
										PushButton{AssignTo: &modeButtons[1], Text: backgroundModeButtonText(config.BackgroundLight, appearance.BackgroundMode), MinSize: Size{Width: 86}, OnClicked: func() { selectBackgroundMode(config.BackgroundLight) }},
									}},
									PushButton{AssignTo: &imageWell, Text: backgroundImageWellText(selectedBackgroundImage), MinSize: Size{Width: 320, Height: 42}, ToolTipText: "Drop one JPEG or PNG here, or click to open the file browser.", OnClicked: browseBackground},
									Label{AssignTo: &backgroundSelectionLabel, Text: backgroundSelectionText(appearance, selectedBackgroundImage, installed, backgroundChanged, defaultBackgroundSelected, freshInstall, currentWindowsBackgroundFound), Font: Font{Family: "Segoe UI Semibold", PointSize: 8}},
								}},
							}},
							Label{Text: "Future changes: replace background.jpg or background.png in " + paths.DataDir() + ".", Font: Font{Family: "Segoe UI", PointSize: 8}},
						},
					},
					Label{Text: "A small LocalSystem service updates the image at boot, after session changes, and every five minutes.", Font: Font{Family: "Segoe UI", PointSize: 9}},
					Label{Text: platformNote, Font: Font{Family: "Segoe UI", PointSize: 8}},
				},
			},
			VSpacer{},
			HSeparator{},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					Label{Text: fmt.Sprintf("W:ID  •  Version %s", buildinfo.Version), Font: Font{Family: "Segoe UI", PointSize: 8}},
					HSpacer{},
					PushButton{AssignTo: &uninstallButton, Text: "Uninstall", Enabled: installed && !legacyInstalled, MinSize: Size{Width: 96}},
					PushButton{AssignTo: &closeButton, Text: "Close", MinSize: Size{Width: 96}, OnClicked: func() { window.Cancel() }},
					PushButton{AssignTo: &backButton, Text: "Back", Enabled: false, MinSize: Size{Width: 96}, OnClicked: func() { showPage(0) }},
					PushButton{AssignTo: &advanceButton, Text: "Next", MinSize: Size{Width: 120}},
				},
			},
		},
	}).Create(nil)
	if err != nil {
		return err
	}
	_ = advanceButton.SetFocus()
	updateBackgroundControls()
	imageWell.DropFiles().Attach(func(files []string) {
		if len(files) != 1 {
			walk.MsgBox(window, "Choose a background image", "Drop exactly one JPEG or PNG file.", walk.MsgBoxIconWarning)
			return
		}
		selectBackgroundImage(files[0])
	})
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
		installOptions := setup.InstallOptions{Preset: preset}
		if preset != "" {
			args = append(args, "--preset", preset)
		}
		applyBackground := !uninstall && (backgroundChanged || freshInstall)
		if applyBackground {
			installOptions.BackgroundColor = appearance.BackgroundColor
			installOptions.BackgroundMode = appearance.BackgroundMode
			installOptions.BackgroundImage = pendingBackgroundImage
			installOptions.UseColors = useColors
			installOptions.UseCurrentBackground = defaultBackgroundSelected && freshInstall
			args = append(args, "--background-color", appearance.BackgroundColor, "--background-mode", appearance.BackgroundMode)
			if pendingBackgroundImage != "" {
				args = append(args, "--background-image", pendingBackgroundImage)
			}
			if useColors {
				args = append(args, "--use-colors")
			}
			if installOptions.UseCurrentBackground {
				args = append(args, "--use-current-background")
			}
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
		advanceButton.SetEnabled(false)
		backButton.SetEnabled(false)
		uninstallButton.SetEnabled(false)
		closeButton.SetEnabled(false)
		setPresetButtonsEnabled(presetButtons, false)
		setBackgroundButtonsEnabled(colorButtons, modeButtons, defaultBackgroundButton, imageWell, false)
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
				opErr = setup.InstallWithOptions(progress, installOptions)
			}
			window.Synchronize(func() {
				working = false
				installedNow := setup.IsInstalled()
				installed = installedNow
				advanceButton.SetText(installText(installedNow))
				advanceButton.SetEnabled(true)
				backButton.SetEnabled(page == 1)
				uninstallButton.SetEnabled(installedNow && !setup.IsLegacyInstalled())
				closeButton.SetEnabled(true)
				setPresetButtonsEnabled(presetButtons, true)
				setBackgroundButtonsEnabled(colorButtons, modeButtons, defaultBackgroundButton, imageWell, true)
				if opErr != nil {
					presetChanged = applyPreset && installedNow
					stateLabel.SetText("The operation could not be completed")
					detailLabel.SetText(opErr.Error())
					selectionLabel.SetText(presetSelectionText(choices, selectedPreset, installedNow, presetChanged))
					walk.MsgBox(window, "Wallpaper Identity Setup", opErr.Error(), walk.MsgBoxIconError)
				} else {
					presetChanged = false
					backgroundChanged = false
					useColors = false
					freshInstall = false
					defaultBackgroundSelected = true
					pendingBackgroundImage = ""
					selectedBackgroundImage = ""
					if current, loadErr := config.Load(paths.ConfigFile()); loadErr == nil {
						appearance.BackgroundColor = current.BackgroundColor
						appearance.BackgroundMode = current.BackgroundMode
						appearance.BaseImage = paths.ResolveBackgroundImage(current.BaseImage)
						initialAppearance = appearance
					}
					newState, newDetail := installationSummary(installedNow, setup.IsLegacyInstalled())
					stateLabel.SetText(newState)
					detailLabel.SetText(newDetail)
					selectionLabel.SetText(presetSelectionText(choices, selectedPreset, installedNow, presetChanged))
				}
				updateBackgroundControls()
				showPage(page)
			})
		}()
	}
	advanceButton.Clicked().Attach(func() {
		if page == 0 {
			showPage(1)
			return
		}
		run(false)
	})
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

func backgroundColorWidgets(choices []backgroundChoice, buttons []*walk.PushButton, swatches map[string]walk.Image, selected string, customImage bool, selectColor func(string)) []Widget {
	widgets := make([]Widget, 0, len(choices))
	for index, choice := range choices {
		index := index
		choice := choice
		widgets = append(widgets, PushButton{
			AssignTo: &buttons[index], Text: backgroundButtonText(choice, !customImage && choice.Name == selected),
			Image: swatches[choice.Name], ImageAboveText: true, MinSize: Size{Width: 100, Height: 46},
			ToolTipText: "Use the " + choice.Title + " color family", OnClicked: func() { selectColor(choice.Name) },
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

func presetIndexByName(name string) int {
	switch name {
	case config.PresetIdentity:
		return 0
	case config.PresetBalanced:
		return 1
	case config.PresetOperations:
		return 2
	default:
		return -1
	}
}

func selectedPresetName(index int) string {
	switch index {
	case 0:
		return config.PresetIdentity
	case 2:
		return config.PresetOperations
	default:
		return config.PresetBalanced
	}
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

func backgroundButtonText(choice backgroundChoice, selected bool) string {
	if selected {
		return "✓  " + choice.Title
	}
	return choice.Title
}

func backgroundModeButtonText(mode, selected string) string {
	title := strings.ToUpper(mode[:1]) + mode[1:]
	if mode == selected {
		return "✓  " + title
	}
	return title
}

func backgroundTitle(name string) string {
	for _, choice := range backgroundChoices {
		if choice.Name == name {
			return choice.Title
		}
	}
	return "Azure"
}

func defaultBackgroundButtonText(selected, freshInstall bool) string {
	label := "Keep current W:ID background"
	if freshInstall {
		label = "Use current Windows login background"
	}
	if selected {
		return "✓  " + label
	}
	return label
}

func defaultBackgroundTooltip(freshInstall bool, current loginscreen.CurrentBackgroundImage, found bool) string {
	if !freshInstall {
		return "Keep the backdrop already configured for Wallpaper Identity."
	}
	if !found {
		return "Windows did not expose a readable image. W:ID will use Azure Dark as a safe fallback."
	}
	return fmt.Sprintf("Snapshot the current image from %s.\n%s", current.Source, current.Path)
}

func backgroundSelectionText(appearance config.Config, imagePath string, installed, changed, defaultSelected, freshInstall, currentFound bool) string {
	prefix := "Selected"
	if installed && changed {
		prefix = "Selected for update"
	} else if installed {
		prefix = "Current background"
	}
	mode := strings.ToUpper(appearance.BackgroundMode[:1]) + appearance.BackgroundMode[1:]
	if defaultSelected {
		if freshInstall {
			if currentFound {
				return fmt.Sprintf("%s: Current Windows login background · %s appearance", prefix, mode)
			}
			return fmt.Sprintf("%s: Current Windows background when available · Azure fallback · %s", prefix, mode)
		}
		return fmt.Sprintf("%s: Keep current W:ID backdrop · %s appearance", prefix, mode)
	}
	if imagePath != "" {
		return fmt.Sprintf("%s: Custom image · %s appearance", prefix, mode)
	}
	return fmt.Sprintf("%s: %s · %s", prefix, backgroundTitle(appearance.BackgroundColor), mode)
}

func backgroundImageWellText(path string) string {
	if path == "" {
		return "Drop a JPEG / PNG here, or click to browse…"
	}
	name := filepath.Base(path)
	if len([]rune(name)) > 30 {
		runes := []rune(name)
		name = string(runes[:29]) + "…"
	}
	return "✓  Custom image: " + name
}

func updateBackgroundButtons(buttons []*walk.PushButton, choices []backgroundChoice, selected string, customImage bool) {
	for index, button := range buttons {
		if button != nil {
			_ = button.SetText(backgroundButtonText(choices[index], !customImage && choices[index].Name == selected))
		}
	}
}

func updateModeButtons(buttons []*walk.PushButton, selected string) {
	modes := []string{config.BackgroundDark, config.BackgroundLight}
	for index, button := range buttons {
		if button != nil {
			_ = button.SetText(backgroundModeButtonText(modes[index], selected))
		}
	}
}

func setBackgroundButtonsEnabled(colors, modes []*walk.PushButton, defaultButton, imageWell *walk.PushButton, enabled bool) {
	for _, button := range append(colors, modes...) {
		if button != nil {
			button.SetEnabled(enabled)
		}
	}
	if imageWell != nil {
		imageWell.SetEnabled(enabled)
	}
	if defaultButton != nil {
		defaultButton.SetEnabled(enabled)
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

func createPresetPreviews(appearance config.Config) (map[string]walk.Image, error) {
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
		cfg.BackgroundColor = appearance.BackgroundColor
		cfg.BackgroundMode = appearance.BackgroundMode
		cfg.BaseImage = appearance.BaseImage
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

func createAppearancePreview(appearance config.Config, preset string) (walk.Image, error) {
	dpi := primaryScreenDPI()
	pixelWidth := walk.IntFrom96DPI(appearancePreviewWidth, dpi)
	pixelHeight := walk.IntFrom96DPI(appearancePreviewHeight, dpi)
	cfg, err := config.ForPreset(preset)
	if err != nil {
		cfg, _ = config.ForPreset(config.PresetBalanced)
	}
	cfg.BackgroundColor = appearance.BackgroundColor
	cfg.BackgroundMode = appearance.BackgroundMode
	cfg.BaseImage = appearance.BaseImage
	cfg.Width, cfg.Height = 1280, 720
	snapshot := previewSnapshot()
	img, err := overlay.Render(snapshot, cfg)
	if err != nil {
		return nil, err
	}
	thumbnail := image.NewRGBA(image.Rect(0, 0, pixelWidth, pixelHeight))
	draw.CatmullRom.Scale(thumbnail, thumbnail.Bounds(), img, img.Bounds(), draw.Over, nil)
	return walk.NewBitmapFromImageForDPI(thumbnail, dpi)
}

func createBackgroundSwatches(mode string) (map[string]walk.Image, error) {
	dpi := primaryScreenDPI()
	pixelWidth := walk.IntFrom96DPI(colorSwatchWidth, dpi)
	pixelHeight := walk.IntFrom96DPI(colorSwatchHeight, dpi)
	result := make(map[string]walk.Image, len(backgroundChoices))
	for _, choice := range backgroundChoices {
		cfg := config.Default()
		cfg.BackgroundColor, cfg.BackgroundMode = choice.Name, mode
		img, err := overlay.BackgroundPreview(cfg, pixelWidth, pixelHeight)
		if err != nil {
			disposePreviews(result)
			return nil, err
		}
		bitmap, err := walk.NewBitmapFromImageForDPI(img, dpi)
		if err != nil {
			disposePreviews(result)
			return nil, err
		}
		result[choice.Name] = bitmap
	}
	return result, nil
}

func previewSnapshot() sysinfo.Snapshot {
	return sysinfo.Snapshot{
		Hostname: "LAB-RACK-07", OS: "Windows Server 2025 Datacenter", Edition: "Server", Version: "24H2", Build: "26100.6584",
		CPU: "Intel Xeon · 8 logical", GPU: "Remote display adapter", Memory: "3.2 / 8.0 GiB · 40%", Disk: "22.4 / 64.0 GiB · 35%",
		IPs: []string{"10.0.2.15"}, Serial: "RACK07-2026", Uptime: "2d 4h 12m", ServicesRunning: 124, ServicesTotal: 132,
		CriticalServices: []sysinfo.ServiceState{
			{Name: "Defender", Running: true}, {Name: "DHCP", Running: true},
			{Name: "DNS Client", Running: true}, {Name: "Event Log", Running: true},
		},
		FailedAutoServices: []string{"ExampleSvc"}, PendingReboot: true,
		DisplayWidth: 1280, DisplayHeight: 720, RefreshedAt: time.Now(),
	}
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
		return "Not installed", "Choose what to show, then review the current Windows login background."
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
