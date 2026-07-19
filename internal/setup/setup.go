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

	"github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/engine"
	"github.com/amcchord/WallpaperIdentity/v4/internal/loginscreen"
	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type ProgressFunc func(percent int, message string)

var ErrSetupRunning = errors.New("another Wallpaper Identity setup operation is already running")

type InstallOptions struct {
	Preset     string
	ConfigFile string
}

// OperationResult reports setup work that an unattended caller may need to
// act on after the process exits.
type OperationResult struct {
	RebootRequired bool
	RemovedData    bool
}

func Install(progress ProgressFunc) error {
	return InstallWithOptions(progress, InstallOptions{})
}

// InstallWithPreset writes a selected starting layout only for an explicit
// first-install choice. An empty preset preserves an existing power-user file.
func InstallWithPreset(progress ProgressFunc, preset string) error {
	return InstallWithOptions(progress, InstallOptions{Preset: preset})
}

// InstallWithOptions performs a clean install, repair, or in-place upgrade.
// ConfigFile is validated before any installation state is changed and is
// mutually exclusive with Preset.
func InstallWithOptions(progress ProgressFunc, options InstallOptions) error {
	_, err := InstallWithOptionsResult(progress, options)
	return err
}

// InstallWithOptionsResult performs the installation and reports whether
// cleanup of a running previous-version executable was deferred until reboot.
func InstallWithOptionsResult(progress ProgressFunc, options InstallOptions) (OperationResult, error) {
	result := OperationResult{}
	progress = safeProgress(progress)
	if !IsAdministrator() {
		return result, errors.New("administrator privileges are required")
	}
	if options.Preset != "" && options.ConfigFile != "" {
		return result, errors.New("preset and config file are mutually exclusive")
	}
	var suppliedConfig *config.Config
	if options.ConfigFile != "" {
		cfg, err := config.Load(options.ConfigFile)
		if err != nil {
			return result, fmt.Errorf("validate supplied configuration: %w", err)
		}
		if err := config.ValidateAssets(cfg); err != nil {
			return result, fmt.Errorf("validate supplied configuration assets: %w", err)
		}
		suppliedConfig = &cfg
	}
	release, err := acquireSetupLock()
	if err != nil {
		return result, err
	}
	defer release()
	progress(5, "Preparing Wallpaper Identity…")
	if err := removeService(paths.LegacyServiceName); err != nil {
		return result, fmt.Errorf("stop the previous-version service: %w", err)
	}
	if err := migrateLegacyData(); err != nil {
		return result, fmt.Errorf("migrate previous-version data: %w", err)
	}
	if err := os.MkdirAll(paths.InstallDir(), 0o755); err != nil {
		return result, fmt.Errorf("create install directory: %w", err)
	}
	if err := os.MkdirAll(paths.ImageDir(), 0o755); err != nil {
		return result, fmt.Errorf("create data directory: %w", err)
	}
	if err := loginscreen.BackupPolicies(paths.PolicyBackupFile()); err != nil {
		return result, fmt.Errorf("back up existing lock-screen policy: %w", err)
	}
	if suppliedConfig != nil {
		if err := config.Save(paths.ConfigFile(), *suppliedConfig); err != nil {
			return result, fmt.Errorf("write supplied configuration: %w", err)
		}
	} else if options.Preset != "" {
		cfg, err := config.ForPreset(options.Preset)
		if err != nil {
			return result, err
		}
		if err := config.Save(paths.ConfigFile(), cfg); err != nil {
			return result, fmt.Errorf("write preset configuration: %w", err)
		}
	} else {
		cfg, err := config.LoadOrCreate(paths.ConfigFile())
		if err != nil {
			return result, fmt.Errorf("create configuration: %w", err)
		}
		cfg.BaseImage = migrateLegacyDataPath(cfg.BaseImage)
		// Saving an imported v3 file preserves its values while updating the
		// comments and schema presentation to the Wallpaper Identity brand.
		if err := config.Save(paths.ConfigFile(), cfg); err != nil {
			return result, fmt.Errorf("normalize configuration: %w", err)
		}
	}

	progress(15, "Removing earlier installation services…")
	removeScheduledTask("BgStatusServiceBoot")
	removeScheduledTask("BgStatusServiceLock")
	_ = removeService("BgStatusService")
	if err := removeService(paths.LegacyServiceName); err != nil {
		return result, fmt.Errorf("remove previous-version service: %w", err)
	}
	if err := removeService(paths.ServiceName); err != nil {
		return result, fmt.Errorf("remove active service: %w", err)
	}

	progress(35, "Installing Wallpaper Identity…")
	current, err := os.Executable()
	if err != nil {
		return result, err
	}
	if !samePath(current, paths.InstalledExe()) {
		if err := copyFile(current, paths.InstalledExe()); err != nil {
			return result, fmt.Errorf("copy application: %w", err)
		}
	}
	if err := deleteRegistryKey(paths.LegacyUninstallRegistryPath); err != nil {
		return result, fmt.Errorf("remove previous-version registration: %w", err)
	}
	result.RebootRequired, err = cleanupInstallDir(paths.LegacyInstallDir(), current)
	if err != nil {
		return result, fmt.Errorf("remove previous-version application files: %w", err)
	}

	progress(55, "Registering the pre-login service…")
	manager, err := mgr.Connect()
	if err != nil {
		return result, fmt.Errorf("connect to Service Control Manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(paths.ServiceName, paths.InstalledExe(), mgr.Config{
		DisplayName:      "Wallpaper Identity pre-login status",
		Description:      "W:ID renders current machine identity and health on the Windows lock and sign-in background before user logon.",
		StartType:        mgr.StartAutomatic,
		ErrorControl:     mgr.ErrorNormal,
		Dependencies:     []string{"winmgmt"},
		ServiceStartName: "LocalSystem",
		SidType:          windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}, "service")
	if err != nil {
		return result, fmt.Errorf("create service: %w", err)
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
		return result, fmt.Errorf("register uninstaller: %w", err)
	}
	refreshStarted := time.Now()
	if err := os.Remove(paths.StatusFile()); err != nil && !os.IsNotExist(err) {
		return result, fmt.Errorf("remove stale service status: %w", err)
	}

	progress(80, "Starting the service and generating the first image…")
	if err := service.Start(); err != nil {
		return result, fmt.Errorf("start service: %w", err)
	}
	if err := waitForService(service, svc.Running, 75*time.Second); err != nil {
		return result, err
	}
	if err := waitForFirstRefresh(60*time.Second, refreshStarted); err != nil {
		return result, err
	}
	progress(100, "Installed. The pre-login image is active.")
	return result, nil
}

func Uninstall(progress ProgressFunc, removeData bool) error {
	_, err := UninstallResult(progress, removeData)
	return err
}

// UninstallResult performs removal and reports when the running installed
// executable or its directory must be deleted during the next reboot.
func UninstallResult(progress ProgressFunc, removeData bool) (OperationResult, error) {
	result := OperationResult{}
	progress = safeProgress(progress)
	if !IsAdministrator() {
		return result, errors.New("administrator privileges are required")
	}
	if IsLegacyInstalled() {
		return result, errors.New("upgrade the previous version before uninstalling so LocalSystem policy rollback can be serialized safely")
	}
	release, err := acquireSetupLock()
	if err != nil {
		return result, err
	}
	defer release()
	progress(5, "Restoring the LocalSystem personalization policy...")
	mdmRestoreErr := requestMDMRestore()
	if mdmRestoreErr != nil {
		return result, fmt.Errorf("restore MDM policy before removal: %w", mdmRestoreErr)
	}
	progress(10, "Stopping the Wallpaper Identity service…")
	if err := removeService(paths.ServiceName); err != nil {
		return result, err
	}
	if err := removeService(paths.LegacyServiceName); err != nil {
		return result, err
	}
	// The service has acknowledged restoration, stopped, and been deleted, so
	// it cannot reapply W:ID after the authoritative backup is consumed. The
	// "ok" marker lets a retry recognize this completed state if cleanup below
	// is interrupted after service deletion.
	for _, path := range []string{paths.LegacyMDMRestoreMarker(), paths.LegacyMDMBackupFile(), paths.MDMBackupFile(), paths.MDMRestoreMarker()} {
		if err := removeFileIfExists(path); err != nil {
			return result, fmt.Errorf("consume restored MDM recovery state: %w", err)
		}
	}
	progress(35, "Removing legacy scheduled tasks…")
	removeScheduledTask("BgStatusServiceBoot")
	removeScheduledTask("BgStatusServiceLock")
	_ = removeService("BgStatusService")
	progress(55, "Restoring Windows lock-screen policy…")
	backupPath := paths.PolicyBackupFile()
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		backupPath = paths.LegacyPolicyBackupFile()
	}
	policyErrors := loginscreen.RestorePolicies(backupPath, paths.DataDir(), paths.LegacyDataDir())
	progress(70, "Removing Apps & features registration…")
	if err := deleteRegistryKey(paths.UninstallRegistryPath); err != nil {
		return result, fmt.Errorf("remove Apps & features registration: %w", err)
	}
	if err := deleteRegistryKey(paths.LegacyUninstallRegistryPath); err != nil {
		return result, fmt.Errorf("remove previous-version registration: %w", err)
	}
	progress(85, "Removing application files…")
	current, _ := os.Executable()
	activeReboot, err := cleanupInstallDir(paths.InstallDir(), current)
	result.RebootRequired = activeReboot
	if err != nil {
		return result, fmt.Errorf("remove application files: %w", err)
	}
	legacyReboot, err := cleanupInstallDir(paths.LegacyInstallDir(), current)
	result.RebootRequired = legacyReboot || result.RebootRequired
	if err != nil {
		return result, fmt.Errorf("remove previous-version application files: %w", err)
	}
	if len(policyErrors) == 0 {
		_ = os.Remove(backupPath)
	}
	if removeData && len(policyErrors) == 0 {
		if err := os.RemoveAll(paths.DataDir()); err != nil {
			return result, fmt.Errorf("remove application data: %w", err)
		}
		if err := os.RemoveAll(paths.LegacyDataDir()); err != nil {
			return result, fmt.Errorf("remove previous-version data: %w", err)
		}
		result.RemovedData = true
	} else if err := migrateLegacyData(); err != nil {
		return result, fmt.Errorf("uninstalled, but previous-version data migration failed: %w", err)
	}
	if len(policyErrors) > 0 {
		return result, fmt.Errorf("uninstalled, but policy cleanup reported: %v", policyErrors[0])
	}
	progress(100, "Uninstalled successfully.")
	return result, nil
}

func requestMDMRestore() error {
	serviceName, markerPath, err := selectMDMRestoreTarget(
		isServicePresent(paths.ServiceName),
		isServicePresent(paths.LegacyServiceName),
		fileExists(paths.MDMBackupFile()),
		fileExists(paths.LegacyMDMBackupFile()),
		restoreMarkerCompleted(paths.MDMRestoreMarker()) || restoreMarkerCompleted(paths.LegacyMDMRestoreMarker()),
	)
	if err != nil || serviceName == "" {
		return err
	}
	return requestMDMRestoreFor(serviceName, markerPath)
}

func selectMDMRestoreTarget(currentService, legacyService, currentBackup, legacyBackup, completed bool) (string, string, error) {
	if currentService {
		return paths.ServiceName, paths.MDMRestoreMarker(), nil
	}
	if legacyService {
		return paths.LegacyServiceName, paths.LegacyMDMRestoreMarker(), nil
	}
	if currentBackup || legacyBackup {
		if completed {
			return "", "", nil
		}
		return "", "", errors.New("cannot restore the LocalSystem MDM policy because the service is missing; the policy backup will be retained")
	}
	return "", "", nil
}

func restoreMarkerCompleted(path string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(b)) == "ok"
}

func requestMDMRestoreFor(serviceName, markerPath string) error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return fmt.Errorf("service %s disappeared before the LocalSystem MDM policy could be restored", serviceName)
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
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale MDM restore marker: %w", err)
	}
	if _, err := service.Control(svc.ParamChange); err != nil {
		return err
	}
	// A restore is serialized behind any in-flight refresh. A refresh may spend
	// about 40 seconds in bounded MDM/LockApp subprocesses, and the restore has
	// its own 30-second bound, so allow a conservative RMM-safe margin.
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		if result, err := os.ReadFile(markerPath); err == nil {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func acquireSetupLock() (func(), error) {
	name, err := windows.UTF16PtrFromString(`Global\WallpaperIdentity.Setup`)
	if err != nil {
		return nil, fmt.Errorf("create setup lock name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return nil, ErrSetupRunning
	}
	if err != nil {
		return nil, fmt.Errorf("create setup lock: %w", err)
	}
	return func() { _ = windows.CloseHandle(handle) }, nil
}

func IsInstalled() bool {
	return isServicePresent(paths.ServiceName) || isServicePresent(paths.LegacyServiceName)
}

func IsLegacyInstalled() bool {
	return !isServicePresent(paths.ServiceName) && isServicePresent(paths.LegacyServiceName)
}

// ServiceState returns an RMM-friendly state for the active W:ID service, or
// for the previous-version service while an upgrade is still pending.
func ServiceState() (string, error) {
	serviceName := paths.ServiceName
	if !isServicePresent(serviceName) {
		if !isServicePresent(paths.LegacyServiceName) {
			return "NotInstalled", nil
		}
		serviceName = paths.LegacyServiceName
	}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return "Unknown", err
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(serviceName)
	if err != nil {
		return "Unknown", err
	}
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return "Unknown", err
	}
	defer windows.CloseServiceHandle(service)
	var status windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(service, &status); err != nil {
		return "Unknown", err
	}
	switch status.CurrentState {
	case windows.SERVICE_STOPPED:
		return "Stopped", nil
	case windows.SERVICE_START_PENDING:
		return "StartPending", nil
	case windows.SERVICE_STOP_PENDING:
		return "StopPending", nil
	case windows.SERVICE_RUNNING:
		return "Running", nil
	case windows.SERVICE_CONTINUE_PENDING:
		return "ContinuePending", nil
	case windows.SERVICE_PAUSE_PENDING:
		return "PausePending", nil
	case windows.SERVICE_PAUSED:
		return "Paused", nil
	default:
		return fmt.Sprintf("Unknown(%d)", status.CurrentState), nil
	}
}

func isServicePresent(serviceName string) bool {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(manager)
	name, err := windows.UTF16PtrFromString(serviceName)
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
	if os.IsNotExist(err) && IsLegacyInstalled() {
		b, err = os.ReadFile(paths.LegacyStatusFile())
	}
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
	status, err := service.Query()
	if err != nil {
		service.Close()
		return fmt.Errorf("query service %s: %w", name, err)
	}
	if status.State != svc.Stopped {
		if _, err := service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			service.Close()
			return fmt.Errorf("stop service %s: %w", name, err)
		}
		if err := waitForService(service, svc.Stopped, 75*time.Second); err != nil {
			service.Close()
			return fmt.Errorf("wait for service %s to stop: %w", name, err)
		}
	}
	if err := service.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		service.Close()
		return fmt.Errorf("delete service %s: %w", name, err)
	}
	service.Close()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		probe, probeErr := manager.OpenService(name)
		if errors.Is(probeErr, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		if probeErr != nil && !errors.Is(probeErr, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
			return fmt.Errorf("verify service %s removal: %w", name, probeErr)
		}
		if probeErr == nil {
			probe.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("service %s remained registered after deletion", name)
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

func waitForFirstRefresh(timeout time.Duration, notBefore time.Time) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := ReadStatus()
		if err == nil {
			if status.Success && status.Version == buildinfo.Version && status.CompletedAt.After(notBefore) {
				return nil
			}
			if status.Version == buildinfo.Version && status.CompletedAt.After(notBefore) && status.Error != "" {
				return fmt.Errorf("initial refresh failed: %s", status.Error)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("timed out waiting for the first background refresh")
}

func registerUninstall() error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, paths.UninstallRegistryPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	values := map[string]string{
		"DisplayName": paths.ProductName, "DisplayVersion": buildinfo.Version,
		"Publisher": paths.ProductName + " (" + paths.ShortName + ")", "InstallLocation": paths.InstallDir(),
		"DisplayIcon": paths.InstalledExe(), "UninstallString": fmt.Sprintf("\"%s\" --uninstall --interactive", paths.InstalledExe()),
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

func migrateLegacyData() error {
	source, destination := paths.LegacyDataDir(), paths.DataDir()
	if samePath(source, destination) {
		return nil
	}
	if _, err := os.Stat(source); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(destination); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.Rename(source, destination); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := mergeLegacyDirectory(source, destination, source); err != nil {
		return err
	}
	return migrateLegacyLog()
}

func mergeLegacyDirectory(source, destination, sourceRoot string) error {
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			if err := mergeLegacyDirectory(sourcePath, destinationPath, sourceRoot); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Stat(destinationPath); os.IsNotExist(err) {
			if err := copyFile(sourcePath, destinationPath); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			relative, relErr := filepath.Rel(sourceRoot, sourcePath)
			if relErr != nil {
				return relErr
			}
			if legacyFileIsAuthoritative(relative) {
				preserved, err := availableMigrationPath(destinationPath, ".pre-v4")
				if err != nil {
					return err
				}
				if err := copyFile(destinationPath, preserved); err != nil {
					return err
				}
				if err := copyFile(sourcePath, destinationPath); err != nil {
					return err
				}
			} else {
				preserved, err := availableMigrationPath(destinationPath, ".from-v3")
				if err != nil {
					return err
				}
				if err := copyFile(sourcePath, preserved); err != nil {
					return err
				}
			}
		}
		if err := os.Remove(sourcePath); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func legacyFileIsAuthoritative(relative string) bool {
	relative = strings.ToLower(filepath.Clean(relative))
	return relative == "config.yml" || relative == "policy-backup.json" || relative == "mdm-policy-backup.json"
}

func availableMigrationPath(path, suffix string) (string, error) {
	for index := 0; ; index++ {
		candidate := path + suffix
		if index > 0 {
			candidate = fmt.Sprintf("%s%s-%d", path, suffix, index+1)
		}
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

func migrateLegacyDataPath(value string) string {
	if strings.TrimSpace(value) == "" || !filepath.IsAbs(value) {
		return value
	}
	legacy, err := filepath.Abs(paths.LegacyDataDir())
	if err != nil {
		return value
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return value
	}
	relative, err := filepath.Rel(legacy, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return value
	}
	return filepath.Join(paths.DataDir(), relative)
}

func migrateLegacyLog() error {
	legacyLog := filepath.Join(paths.DataDir(), filepath.Base(paths.LegacyLogFile()))
	if _, err := os.Stat(legacyLog); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(paths.LogFile()); os.IsNotExist(err) {
		return os.Rename(legacyLog, paths.LogFile())
	}
	source, err := os.Open(legacyLog)
	if err != nil {
		return err
	}
	destination, err := os.OpenFile(paths.LogFile(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		source.Close()
		return err
	}
	if _, err := io.Copy(destination, source); err != nil {
		destination.Close()
		source.Close()
		return err
	}
	if err := destination.Close(); err != nil {
		source.Close()
		return err
	}
	if err := source.Close(); err != nil {
		return err
	}
	return os.Remove(legacyLog)
}

func cleanupInstallDir(directory, runningExe string) (bool, error) {
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if isWithinPath(runningExe, directory) {
		exeErr := windows.MoveFileEx(windows.StringToUTF16Ptr(runningExe), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
		dirErr := windows.MoveFileEx(windows.StringToUTF16Ptr(directory), nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
		if exeErr != nil {
			return dirErr == nil, fmt.Errorf("schedule executable deletion: %w", exeErr)
		}
		if dirErr != nil {
			return true, fmt.Errorf("schedule install-directory deletion: %w", dirErr)
		}
		return true, nil
	}
	if err := os.RemoveAll(directory); err != nil {
		return false, err
	}
	return false, nil
}

func deleteRegistryKey(path string) error {
	err := registry.DeleteKey(registry.LOCAL_MACHINE, path)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func removeFileIfExists(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func isWithinPath(value, directory string) bool {
	valueAbs, valueErr := filepath.Abs(value)
	directoryAbs, directoryErr := filepath.Abs(directory)
	if valueErr != nil || directoryErr != nil {
		return false
	}
	prefix := strings.ToLower(filepath.Clean(directoryAbs)) + string(os.PathSeparator)
	return strings.HasPrefix(strings.ToLower(filepath.Clean(valueAbs)), prefix)
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
