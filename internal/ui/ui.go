// Package ui implements the small native Windows setup and maintenance window.
package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/amcchord/BackgroundChanger/internal/buildinfo"
	"github.com/amcchord/BackgroundChanger/internal/paths"
	"github.com/amcchord/BackgroundChanger/internal/setup"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func Main() error {
	var window *walk.MainWindow
	var stateLabel, detailLabel *walk.Label
	var installButton, uninstallButton *walk.PushButton
	installed := setup.IsInstalled()
	state, detail := installationSummary(installed)

	err := (MainWindow{
		AssignTo: &window,
		Title:    "BackgroundChanger Setup",
		MinSize:  Size{Width: 650, Height: 440},
		Size:     Size{Width: 650, Height: 440},
		Layout:   VBox{Margins: Margins{Left: 34, Top: 30, Right: 34, Bottom: 26}, Spacing: 12},
		Children: []Widget{
			Label{Text: "BackgroundChanger", Font: Font{Family: "Segoe UI", PointSize: 23, Bold: true}},
			Label{Text: "Machine identity and health, visible before sign-in", Font: Font{Family: "Segoe UI", PointSize: 11}},
			VSpacer{Size: 6},
			GroupBox{
				Title:  "Current status",
				Layout: VBox{Margins: Margins{Left: 18, Top: 18, Right: 18, Bottom: 18}, Spacing: 8},
				Children: []Widget{
					Label{AssignTo: &stateLabel, Text: state, Font: Font{Family: "Segoe UI Semibold", PointSize: 13}},
					Label{AssignTo: &detailLabel, Text: detail, Font: Font{Family: "Segoe UI", PointSize: 9}},
				},
			},
			Label{
				Text: "The installer registers one automatic LocalSystem service. It renders at boot, after session changes, and every five minutes. No network connection is required.",
				Font: Font{Family: "Segoe UI", PointSize: 9},
			},
			VSpacer{},
			Composite{
				Layout: HBox{Spacing: 10},
				Children: []Widget{
					PushButton{AssignTo: &installButton, Text: installText(installed)},
					PushButton{AssignTo: &uninstallButton, Text: "Uninstall", Enabled: installed},
					HSpacer{},
					PushButton{Text: "Close", OnClicked: func() { window.Close() }},
				},
			},
			Label{Text: fmt.Sprintf("Version %s  •  Windows Enterprise, Education, IoT Enterprise, or Server", buildinfo.Version), Font: Font{Family: "Segoe UI", PointSize: 8}},
		},
	}).Create()
	if err != nil {
		return err
	}

	run := func(uninstall bool) {
		args := []string{"--install"}
		if uninstall {
			args = []string{"--uninstall"}
		}
		if !setup.IsAdministrator() {
			if err := setup.RelaunchElevated(args...); err != nil {
				walk.MsgBox(window, "BackgroundChanger Setup", "Administrator elevation failed:\n\n"+err.Error(), walk.MsgBoxIconError)
				return
			}
			window.Close()
			return
		}
		installButton.SetEnabled(false)
		uninstallButton.SetEnabled(false)
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
				opErr = setup.Install(progress)
			}
			window.Synchronize(func() {
				installedNow := setup.IsInstalled()
				installButton.SetText(installText(installedNow))
				installButton.SetEnabled(true)
				uninstallButton.SetEnabled(installedNow)
				if opErr != nil {
					stateLabel.SetText("The operation could not be completed")
					detailLabel.SetText(opErr.Error())
					walk.MsgBox(window, "BackgroundChanger Setup", opErr.Error(), walk.MsgBoxIconError)
				} else {
					newState, newDetail := installationSummary(installedNow)
					stateLabel.SetText(newState)
					detailLabel.SetText(newDetail)
				}
			})
		}()
	}
	installButton.Clicked().Attach(func() { run(false) })
	uninstallButton.Clicked().Attach(func() {
		if walk.MsgBox(window, "Uninstall BackgroundChanger", "Remove the service and its Windows lock-screen policy?\n\nGenerated images and configuration will be kept in "+paths.DataDir()+".", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			run(true)
		}
	})
	window.Run()
	return nil
}

func RunOperation(title string, operation func(setup.ProgressFunc) error) error {
	var window *walk.MainWindow
	var statusLabel *walk.Label
	var progressBar *walk.ProgressBar
	var closeButton *walk.PushButton
	err := (MainWindow{
		AssignTo: &window,
		Title:    title,
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
	var result error
	go func() {
		result = operation(func(percent int, message string) {
			window.Synchronize(func() {
				progressBar.SetValue(percent)
				statusLabel.SetText(message)
			})
		})
		window.Synchronize(func() {
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

func installationSummary(installed bool) (string, string) {
	if !installed {
		return "Not installed", "Select Install to activate the boot-time pre-login status background."
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
