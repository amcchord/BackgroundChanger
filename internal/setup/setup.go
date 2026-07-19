// Package setup installs and removes the self-contained executable and service.
package setup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/amcchord/BackgroundChanger/internal/buildinfo"
	"github.com/amcchord/BackgroundChanger/internal/config"
	"github.com/amcchord/BackgroundChanger/internal/engine"
	"github.com/amcchord/BackgroundChanger/internal/loginscreen"
	"github.com/amcchord/BackgroundChanger/internal/paths"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type ProgressFunc func(percent int, message string)

func Install(progress ProgressFunc) error {
	progress = safeProgress(progress)
	if !IsAdministrator() {
		return errors.New("administrator privileges are required")
	}
	progress(5, "Preparing installation…")
	if err := os.MkdirAll(paths.InstallDir(), 0o755); err != nil {
		return fmt.Errorf("create install directory: %w", err)
	}
	if err := os.MkdirAll(paths.ImageDir(), 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := loginscreen.BackupPolicies(paths.PolicyBackupFile()); err != nil {
		return fmt.Errorf("back up existing lock-screen policy: %w", err)
	}
	if _, err := config.LoadOrCreate(paths.ConfigFile()); err != nil {
		return fmt.Errorf("create configuration: %w", err)
	}

	progress(15, "Removing the legacy task-based installation…")
	removeScheduledTask("BgStatusServiceBoot")
	removeScheduledTask("BgStatusServiceLock")
	_ = removeService("BgStatusService")
	_ = removeService(paths.ServiceName)

	progress(35, "Installing BackgroundChanger…")
	current, err := os.Executable()
	if err != nil {
		return err
	}
	if !samePath(current, paths.InstalledExe()) {
		if err := copyFile(current, paths.InstalledExe()); err != nil {
			return fmt.Errorf("copy application: %w", err)
		}
	}

	progress(55, "Registering the pre-login service…")
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to Service Control Manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(paths.ServiceName, paths.InstalledExe(), mgr.Config{
		DisplayName:      "BackgroundChanger pre-login status",
		Description:      "Renders current machine identity and health on the Windows lock and sign-in background before user logon.",
		StartType:        mgr.StartAutomatic,
		ErrorControl:     mgr.ErrorNormal,
		Dependencies:     []string{"winmgmt"},
		ServiceStartName: "LocalSystem",
		SidType:          windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}, "service")
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer service.Close()
	_ = service.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 2 * time.Minute},
	}, 86400)
	_ = service.SetRecoveryActionsOnNonCrashFailures(true)

	progress(70, "Adding Apps & features registration…")
	if err := registerUninstall(); err != nil {
		return fmt.Errorf("register uninstaller: %w", err)
	}
	_ = os.Remove(paths.StatusFile())

	progress(80, "Starting the service and generating the first image…")
	if err := service.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	if err := waitForService(service, svc.Running, 75*time.Second); err != nil {
		return err
	}
	if err := waitForFirstRefresh(60 * time.Second); err != nil {
		return err
	}
	progress(100, "Installed. The pre-login image is active.")
	return nil
}

func Uninstall(progress ProgressFunc, removeData bool) error {
	progress = safeProgress(progress)
	if !IsAdministrator() {
		return errors.New("administrator privileges are required")
	}
	progress(5, "Restoring the LocalSystem personalization policy...")
	mdmRestoreErr := requestMDMRestore()
	progress(10, "Stopping the BackgroundChanger service…")
	if err := removeService(paths.ServiceName); err != nil {
		return err
	}
	progress(35, "Removing legacy scheduled tasks…")
	removeScheduledTask("BgStatusServiceBoot")
	removeScheduledTask("BgStatusServiceLock")
	_ = removeService("BgStatusService")
	progress(55, "Restoring Windows lock-screen policy…")
	policyErrors := loginscreen.RestorePolicies(paths.PolicyBackupFile(), paths.DataDir())
	progress(70, "Removing Apps & features registration…")
	_ = registry.DeleteKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\BackgroundChanger`)
	progress(85, "Removing application files…")
	current, _ := os.Executable()
	if !samePath(current, paths.InstalledExe()) {
		_ = os.RemoveAll(paths.InstallDir())
	} else {
		_ = windows.MoveFileEx(windows.StringToUTF16Ptr(current), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
	if len(policyErrors) == 0 {
		_ = os.Remove(paths.PolicyBackupFile())
	}
	if mdmRestoreErr == nil {
		_ = os.Remove(paths.MDMBackupFile())
		_ = os.Remove(paths.MDMRestoreMarker())
	}
	if removeData {
		_ = os.RemoveAll(paths.DataDir())
	}
	if len(policyErrors) > 0 {
		return fmt.Errorf("uninstalled, but policy cleanup reported: %v", policyErrors[0])
	}
	if mdmRestoreErr != nil {
		return fmt.Errorf("uninstalled, but MDM policy cleanup reported: %v", mdmRestoreErr)
	}
	progress(100, "Uninstalled successfully.")
	return nil
}

func requestMDMRestore() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(paths.ServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	defer service.Close()
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State != svc.Running {
		if status.State == svc.Stopped {
			if err := service.Start(); err != nil {
				return err
			}
		}
		if err := waitForService(service, svc.Running, 75*time.Second); err != nil {
			return err
		}
	}
	_ = os.Remove(paths.MDMRestoreMarker())
	if _, err := service.Control(svc.ParamChange); err != nil {
		return err
	}
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		if result, err := os.ReadFile(paths.MDMRestoreMarker()); err == nil {
			message := strings.TrimSpace(string(result))
			if message == "ok" {
				return nil
			}
			return errors.New(message)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("timed out waiting for LocalSystem MDM policy cleanup")
}

func IsInstalled() bool {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(paths.ServiceName)
	if err != nil {
		return false
	}
	// A non-elevated setup window only needs to know whether the service is
	// present. mgr.OpenService requests broader rights and reports access denied
	// to a standard UAC token, which made installed systems look uninstalled.
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	windows.CloseServiceHandle(service)
	return true
}

func ReadStatus() (engine.Status, error) {
	b, err := os.ReadFile(paths.StatusFile())
	if err != nil {
		return engine.Status{}, err
	}
	var value engine.Status
	if err := json.Unmarshal(b, &value); err != nil {
		return engine.Status{}, err
	}
	return value, nil
}

func IsAdministrator() bool {
	token := windows.Token(0)
	adminSID, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	isMember, err := token.IsMember(adminSID)
	return err == nil && isMember
}

func RelaunchElevated(arguments ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(quoteArguments(arguments))
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	return windows.ShellExecute(0, verb, file, params, dir, windows.SW_SHOWNORMAL)
}

func removeService(name string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(name)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	status, _ := service.Query()
	if status.State != svc.Stopped {
		_, _ = service.Control(svc.Stop)
		_ = waitForService(service, svc.Stopped, 20*time.Second)
	}
	if err := service.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		service.Close()
		return fmt.Errorf("delete service %s: %w", name, err)
	}
	service.Close()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		probe, probeErr := manager.OpenService(name)
		if probeErr != nil {
			break
		}
		probe.Close()
		time.Sleep(200 * time.Millisecond)
	}
	return nil
}

func waitForService(service *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == want {
			return nil
		}
		if status.State == svc.Stopped && want != svc.Stopped {
			return fmt.Errorf("service stopped before reaching the running state")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for service state %d", want)
}

func waitForFirstRefresh(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := ReadStatus()
		if err == nil {
			if status.Success {
				return nil
			}
			if status.Error != "" {
				return fmt.Errorf("initial refresh failed: %s", status.Error)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("timed out waiting for the first background refresh")
}

func registerUninstall() error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\BackgroundChanger`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	values := map[string]string{
		"DisplayName": "BackgroundChanger", "DisplayVersion": buildinfo.Version,
		"Publisher": "BackgroundChanger", "InstallLocation": paths.InstallDir(),
		"DisplayIcon": paths.InstalledExe(), "UninstallString": fmt.Sprintf("\"%s\" --uninstall", paths.InstalledExe()),
		"QuietUninstallString": fmt.Sprintf("\"%s\" --uninstall --quiet", paths.InstalledExe()),
	}
	for name, value := range values {
		if err := key.SetStringValue(name, value); err != nil {
			return err
		}
	}
	if err := key.SetDWordValue("NoModify", 1); err != nil {
		return err
	}
	return key.SetDWordValue("NoRepair", 0)
}

func removeScheduledTask(name string) {
	cmd := exec.Command("schtasks.exe", "/Delete", "/TN", name, "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Run()
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := destination + ".new"
	dst, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(destination)
	return os.Rename(tmp, destination)
}

func samePath(a, b string) bool {
	aAbs, _ := filepath.Abs(a)
	bAbs, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(aAbs), filepath.Clean(bAbs))
}

func quoteArguments(arguments []string) string {
	quoted := make([]string, len(arguments))
	for i, argument := range arguments {
		quoted[i] = syscall.EscapeArg(argument)
	}
	return strings.Join(quoted, " ")
}

func safeProgress(progress ProgressFunc) ProgressFunc {
	if progress == nil {
		return func(int, string) {}
	}
	return progress
}
