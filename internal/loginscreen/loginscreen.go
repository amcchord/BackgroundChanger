// Package loginscreen applies the generated image to Windows machine policy.
package loginscreen

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/amcchord/BackgroundChanger/internal/paths"
	"golang.org/x/sys/windows/registry"
)

const personalizationPolicy = `SOFTWARE\Policies\Microsoft\Windows\Personalization`
const systemPolicy = `SOFTWARE\Policies\Microsoft\Windows\System`

type policyValue struct {
	Exists bool   `json:"exists"`
	String string `json:"string,omitempty"`
	DWORD  uint32 `json:"dword,omitempty"`
}

type policyBackup struct {
	LockScreenImage                 policyValue `json:"lock_screen_image"`
	NoChangingLockScreen            policyValue `json:"no_changing_lock_screen"`
	LockScreenOverlaysDisabled      policyValue `json:"lock_screen_overlays_disabled"`
	DisableAcrylicBackgroundOnLogon policyValue `json:"disable_acrylic_background_on_logon"`
}

type ApplyResult struct {
	GroupPolicyApplied bool     `json:"group_policy_applied"`
	MDMBridgeApplied   bool     `json:"mdm_bridge_applied"`
	LockAppRefreshed   bool     `json:"lock_app_refreshed"`
	Warnings           []string `json:"warnings,omitempty"`
}

func Apply(imagePath string) ApplyResult {
	result := ApplyResult{}
	abs, err := filepath.Abs(imagePath)
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
		return result
	}
	if err := applyGroupPolicy(abs); err != nil {
		result.Warnings = append(result.Warnings, "group policy: "+err.Error())
	} else {
		result.GroupPolicyApplied = true
	}
	if err := applyMDMBridge(abs); err != nil {
		result.Warnings = append(result.Warnings, "MDM bridge: "+err.Error())
	} else {
		result.MDMBridgeApplied = true
	}
	// LockApp owns only the lock-screen surface. If it is currently visible,
	// Windows will recreate it and read the new versioned image path. We never
	// terminate LogonUI, which owns the security-sensitive credential surface.
	if refreshed, err := RefreshLockApp(); err != nil {
		result.Warnings = append(result.Warnings, "LockApp refresh: "+err.Error())
	} else {
		result.LockAppRefreshed = refreshed
	}
	return result
}

func applyGroupPolicy(imagePath string) error {
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("LockScreenImage", imagePath); err != nil {
		return err
	}
	if err := key.SetDWordValue("NoChangingLockScreen", 1); err != nil {
		return err
	}
	if err := key.SetDWordValue("LockScreenOverlaysDisabled", 1); err != nil {
		return err
	}
	logonKey, _, err := registry.CreateKey(registry.LOCAL_MACHINE, systemPolicy, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer logonKey.Close()
	// Microsoft's "Show clear logon background" policy keeps the status text
	// readable after the user advances from the lock screen to credentials.
	return logonKey.SetDWordValue("DisableAcrylicBackgroundOnLogon", 1)
}

func applyMDMBridge(imagePath string) error {
	fileURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(imagePath)}).String()
	fileURL = strings.Replace(fileURL, "file:////", "file:///", 1)
	quotedURL := strings.ReplaceAll(fileURL, "'", "''")
	quotedBackup := strings.ReplaceAll(paths.MDMBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_Personalization'
$url='%s'
$backup='%s'
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
if (-not (Test-Path -LiteralPath $backup)) {
  $record=[pscustomobject]@{ Existed=($null -ne $instance); Url=$(if ($null -eq $instance) { '' } else { [string]$instance.LockScreenImageUrl }) }
  $record | ConvertTo-Json -Compress | Set-Content -LiteralPath $backup -Encoding UTF8
}
if ($null -eq $instance) {
  New-CimInstance -Namespace $ns -ClassName $class -Property @{ ParentID='./Vendor/MSFT'; InstanceID='Personalization'; LockScreenImageUrl=$url } | Out-Null
} else {
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=$url } | Out-Null
}`, quotedURL, quotedBackup)
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(powershell); err != nil {
		powershell = "powershell.exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(script))
	cmd.SysProcAttr = hiddenProcessAttributes()
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("timed out")
	}
	if err != nil {
		message := compactOutput(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

// RestoreMDMBridge rolls back the LocalSystem-only Personalization CSP value.
// It intentionally leaves a value alone if another administrator or MDM has
// replaced BackgroundChanger's URL since installation.
func RestoreMDMBridge() error {
	quotedBackup := strings.ReplaceAll(paths.MDMBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_Personalization'
$backup='%s'
if (-not (Test-Path -LiteralPath $backup)) { return }
$record=Get-Content -LiteralPath $backup -Raw | ConvertFrom-Json
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
if ($null -eq $instance) { return }
$current=[string]$instance.LockScreenImageUrl
if ($current -notmatch '(?i)BackgroundChanger[/\\]backgrounds[/\\]') { return }
if ([bool]$record.Existed) {
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=[string]$record.Url } | Out-Null
} elseif ([string]::IsNullOrEmpty([string]$instance.DesktopImageUrl)) {
  Remove-CimInstance -CimInstance $instance
} else {
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=$null } | Out-Null
}`, quotedBackup)
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(powershell); err != nil {
		powershell = "powershell.exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(script))
	cmd.SysProcAttr = hiddenProcessAttributes()
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("timed out")
	}
	if err != nil {
		message := compactOutput(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s", message)
	}
	return nil
}

func RefreshLockApp() (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	check := exec.CommandContext(ctx, "tasklist.exe", "/FI", "IMAGENAME eq LockApp.exe", "/FO", "CSV", "/NH")
	check.SysProcAttr = hiddenProcessAttributes()
	output, err := check.Output()
	if err != nil || !strings.Contains(strings.ToLower(string(output)), "lockapp.exe") {
		return false, nil
	}
	kill := exec.CommandContext(ctx, "taskkill.exe", "/F", "/IM", "LockApp.exe")
	kill.SysProcAttr = hiddenProcessAttributes()
	if output, err := kill.CombinedOutput(); err != nil {
		return false, fmt.Errorf("%s", compactOutput(string(output)))
	}
	return true, nil
}

func BackupPolicies(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	backup := policyBackup{}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.QUERY_VALUE); err == nil {
		backup.LockScreenImage = readStringValue(key, "LockScreenImage")
		backup.NoChangingLockScreen = readDWORDValue(key, "NoChangingLockScreen")
		backup.LockScreenOverlaysDisabled = readDWORDValue(key, "LockScreenOverlaysDisabled")
		key.Close()
	}
	// Values left behind by the pre-v3 application are not useful restoration
	// targets; treating them as absent completes the migration cleanly.
	if strings.Contains(strings.ToLower(backup.LockScreenImage.String), `\bgstatusservice\`) {
		backup.LockScreenImage = policyValue{}
		backup.NoChangingLockScreen = policyValue{}
		backup.LockScreenOverlaysDisabled = policyValue{}
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, systemPolicy, registry.QUERY_VALUE); err == nil {
		backup.DisableAcrylicBackgroundOnLogon = readDWORDValue(key, "DisableAcrylicBackgroundOnLogon")
		key.Close()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func RestorePolicies(backupPath, ownedDataDir string) []error {
	var errors []error
	backup := policyBackup{}
	if b, err := os.ReadFile(backupPath); err == nil {
		if err := json.Unmarshal(b, &backup); err != nil {
			errors = append(errors, err)
		}
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.QUERY_VALUE|registry.SET_VALUE)
	if err == nil {
		if current, _, valueErr := key.GetStringValue("LockScreenImage"); valueErr == nil && isOwnedPath(current, ownedDataDir) {
			errors = append(errors, restoreStringValue(key, "LockScreenImage", backup.LockScreenImage)...)
			errors = append(errors, restoreDWORDValue(key, "NoChangingLockScreen", backup.NoChangingLockScreen)...)
			errors = append(errors, restoreDWORDValue(key, "LockScreenOverlaysDisabled", backup.LockScreenOverlaysDisabled)...)
		}
		key.Close()
	} else if err != registry.ErrNotExist {
		errors = append(errors, err)
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, systemPolicy, registry.QUERY_VALUE|registry.SET_VALUE); err == nil {
		if current, _, valueErr := key.GetIntegerValue("DisableAcrylicBackgroundOnLogon"); valueErr == nil && current == 1 {
			errors = append(errors, restoreDWORDValue(key, "DisableAcrylicBackgroundOnLogon", backup.DisableAcrylicBackgroundOnLogon)...)
		}
		key.Close()
	} else if err != registry.ErrNotExist {
		errors = append(errors, err)
	}
	return errors
}

func readStringValue(key registry.Key, name string) policyValue {
	value, _, err := key.GetStringValue(name)
	return policyValue{Exists: err == nil, String: value}
}

func readDWORDValue(key registry.Key, name string) policyValue {
	value, _, err := key.GetIntegerValue(name)
	return policyValue{Exists: err == nil, DWORD: uint32(value)}
}

func restoreStringValue(key registry.Key, name string, value policyValue) []error {
	if value.Exists {
		if err := key.SetStringValue(name, value.String); err != nil {
			return []error{err}
		}
		return nil
	}
	if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return []error{err}
	}
	return nil
}

func restoreDWORDValue(key registry.Key, name string, value policyValue) []error {
	if value.Exists {
		if err := key.SetDWordValue(name, value.DWORD); err != nil {
			return []error{err}
		}
		return nil
	}
	if err := key.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return []error{err}
	}
	return nil
}

func isOwnedPath(value, dataDir string) bool {
	valueAbs, err1 := filepath.Abs(value)
	dataAbs, err2 := filepath.Abs(dataDir)
	if err1 != nil || err2 != nil {
		return false
	}
	valueLower := strings.ToLower(filepath.Clean(valueAbs))
	dataLower := strings.ToLower(filepath.Clean(dataAbs)) + string(os.PathSeparator)
	return strings.HasPrefix(valueLower, dataLower)
}

func encodePowerShell(script string) string {
	encoded := utf16.Encode([]rune(script))
	b := make([]byte, len(encoded)*2)
	for i, value := range encoded {
		binary.LittleEndian.PutUint16(b[i*2:], value)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func compactOutput(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 500 {
		return value[:497] + "..."
	}
	return value
}
