// Package loginscreen applies the generated image to Windows machine policy.
package loginscreen

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"golang.org/x/sys/windows/registry"
)

const ownedMDMImagePattern = `(?i)((Wallpaper(?:[ ]|%20)Identity|BackgroundChanger)[/\\]backgrounds|WallpaperIdentityCSP)[/\\]`

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
	GroupPolicyApplied      bool     `json:"group_policy_applied"`
	MDMBridgeApplied        bool     `json:"mdm_bridge_applied"`
	ProCompatibilityApplied bool     `json:"pro_compatibility_applied"`
	LockAppRefreshed        bool     `json:"lock_app_refreshed"`
	Warnings                []string `json:"warnings,omitempty"`
}

type ApplyOptions struct {
	ProfessionalEdition    bool
	EnableProCompatibility bool
}

func Apply(imagePath string, options ApplyOptions) ApplyResult {
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
	if options.ProfessionalEdition {
		if options.EnableProCompatibility {
			if err := ensureProCompatibility(); err != nil {
				result.Warnings = append(result.Warnings, "Windows Pro compatibility: "+err.Error())
			} else {
				result.ProCompatibilityApplied = true
			}
		} else if fileExists(paths.ProCompatibilityBackupFile()) {
			if err := RestoreProCompatibility(); err != nil {
				result.Warnings = append(result.Warnings, "disable Windows Pro compatibility: "+err.Error())
			}
		}
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
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.QUERY_VALUE|registry.SET_VALUE)
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
	if current, _, err := key.GetStringValue("LockScreenImage"); err != nil || !samePath(current, imagePath) {
		return fmt.Errorf("LockScreenImage read-back mismatch: got %q", current)
	}
	if current, _, err := key.GetIntegerValue("NoChangingLockScreen"); err != nil || current != 1 {
		return fmt.Errorf("NoChangingLockScreen read-back mismatch: got %d", current)
	}
	if current, _, err := key.GetIntegerValue("LockScreenOverlaysDisabled"); err != nil || current != 1 {
		return fmt.Errorf("LockScreenOverlaysDisabled read-back mismatch: got %d", current)
	}
	logonKey, _, err := registry.CreateKey(registry.LOCAL_MACHINE, systemPolicy, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer logonKey.Close()
	// Microsoft's "Show clear logon background" policy keeps the status text
	// readable after the user advances from the lock screen to credentials.
	if err := logonKey.SetDWordValue("DisableAcrylicBackgroundOnLogon", 1); err != nil {
		return err
	}
	if current, _, err := logonKey.GetIntegerValue("DisableAcrylicBackgroundOnLogon"); err != nil || current != 1 {
		return fmt.Errorf("DisableAcrylicBackgroundOnLogon read-back mismatch: got %d", current)
	}
	return nil
}

func applyMDMBridge(imagePath string) error {
	stagedPath, err := stageMDMImage(imagePath)
	if err != nil {
		return fmt.Errorf("stage CSP image: %w", err)
	}
	defer cleanupStagedMDMImages(stagedPath, 4)
	fileURL := (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(stagedPath)}).String()
	fileURL = strings.Replace(fileURL, "file:////", "file:///", 1)
	quotedURL := strings.ReplaceAll(fileURL, "'", "''")
	quotedBackup := strings.ReplaceAll(paths.MDMBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_Personalization'
$url='%s'
$backup='%s'
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
if (-not (Test-Path -LiteralPath $backup)) {
  $previousUrl=$(if ($null -eq $instance) { '' } else { [string]$instance.LockScreenImageUrl })
  $record=[pscustomobject]@{ Existed=(-not [string]::IsNullOrWhiteSpace($previousUrl)); Url=$previousUrl }
  $record | ConvertTo-Json -Compress | Set-Content -LiteralPath $backup -Encoding UTF8
} else {
  $record=Get-Content -LiteralPath $backup -Raw | ConvertFrom-Json
  if (([bool]$record.Existed -and [string]$record.Url -match '%s') -or [string]::IsNullOrWhiteSpace([string]$record.Url)) {
    [pscustomobject]@{ Existed=$false; Url='' } | ConvertTo-Json -Compress | Set-Content -LiteralPath $backup -Encoding UTF8
  }
}
if ($null -eq $instance) {
  New-CimInstance -Namespace $ns -ClassName $class -Property @{ ParentID='./Vendor/MSFT'; InstanceID='Personalization'; LockScreenImageUrl=$url } | Out-Null
} else {
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=$url } | Out-Null
}
$deadline=(Get-Date).AddSeconds(25)
$verify=$null
do {
  Start-Sleep -Milliseconds 250
  $verify=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
  if ($null -ne $verify -and [string]$verify.LockScreenImageUrl -eq $url -and [int]$verify.LockScreenImageStatus -eq 1) { return }
} while ((Get-Date) -lt $deadline)
$actualUrl=$(if ($null -eq $verify) { '<missing>' } else { [string]$verify.LockScreenImageUrl })
$actualStatus=$(if ($null -eq $verify) { -1 } else { [int]$verify.LockScreenImageStatus })
throw "Personalization CSP did not confirm the image (url=$actualUrl, status=$actualStatus; expected status=1)"`, quotedURL, quotedBackup, ownedMDMImagePattern)
	if err := runPowerShell(script, 40*time.Second); err != nil {
		return err
	}
	return nil
}

func ensureProCompatibility() error {
	quotedBackup := strings.ReplaceAll(paths.ProCompatibilityBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_SharedPC'
$backup='%s'
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not (Test-Path -LiteralPath $backup)) {
  $record=[pscustomobject]@{ Existed=($null -ne $instance); Enabled=$(if ($null -eq $instance) { $false } else { [bool]$instance.SetEduPolicies }) }
  $record | ConvertTo-Json -Compress | Set-Content -LiteralPath $backup -Encoding UTF8
}
if ($null -eq $instance) {
  New-CimInstance -Namespace $ns -ClassName $class -Property @{ ParentID='./Vendor/MSFT/Policy/Config'; InstanceID='SharedPC'; SetEduPolicies=$true } | Out-Null
} elseif (-not [bool]$instance.SetEduPolicies) {
  Set-CimInstance -CimInstance $instance -Property @{ SetEduPolicies=$true } | Out-Null
}
$verify=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $verify -or -not [bool]$verify.SetEduPolicies) { throw 'SetEduPolicies did not verify as enabled' }`, quotedBackup)
	return runPowerShell(script, 30*time.Second)
}

// RestoreProCompatibility returns SetEduPolicies to its pre-W:ID value. The
// SharedPC provider owns the associated policy cleanup; W:ID never enables full
// shared-PC mode, account cleanup, power policy, or storage restrictions.
func RestoreProCompatibility() error {
	quotedBackup := strings.ReplaceAll(paths.ProCompatibilityBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_SharedPC'
$backup='%s'
if (-not (Test-Path -LiteralPath $backup)) { return }
$record=Get-Content -LiteralPath $backup -Raw | ConvertFrom-Json
$target=[bool]$record.Enabled
$existed=[bool]$record.Existed
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $instance) {
  if ($target) { throw 'the previous SetEduPolicies state was enabled, but the SharedPC instance is missing' }
  return
}
if ([bool]$instance.SetEduPolicies -ne $target) {
  Set-CimInstance -CimInstance $instance -Property @{ SetEduPolicies=$target } | Out-Null
}
if (-not $existed) {
  $verify=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($null -eq $verify -or [bool]$verify.SetEduPolicies) { throw 'SetEduPolicies cleanup did not verify' }
  return
}
$verify=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $verify -or [bool]$verify.SetEduPolicies -ne $target) { throw 'SetEduPolicies rollback did not verify' }`, quotedBackup)
	return runPowerShell(script, 30*time.Second)
}

// RestoreDevicePolicies performs every LocalSystem-only rollback as one
// serialized service job so uninstall cannot race a queued refresh.
func RestoreDevicePolicies() error {
	return errors.Join(RestoreMDMBridge(), RestoreProCompatibility())
}

// RestoreMDMBridge rolls back the LocalSystem-only Personalization CSP value.
// It intentionally leaves a value alone if another administrator or MDM has
// replaced Wallpaper Identity's URL since installation.
func RestoreMDMBridge() error {
	quotedBackup := strings.ReplaceAll(paths.MDMBackupFile(), "'", "''")
	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
$ProgressPreference='SilentlyContinue'
$ns='root\cimv2\mdm\dmmap'
$class='MDM_Personalization'
$backup='%s'
if (-not (Test-Path -LiteralPath $backup)) { return }
$record=Get-Content -LiteralPath $backup -Raw | ConvertFrom-Json
$instance=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
if ($null -eq $instance) { return }
$current=[string]$instance.LockScreenImageUrl
if ($current -notmatch '%s') { return }
$previousUrl=[string]$record.Url
$hadLockScreenUrl=[bool]$record.Existed -and -not [string]::IsNullOrWhiteSpace($previousUrl)
if ($hadLockScreenUrl) {
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=$previousUrl } | Out-Null
} else {
  # DMWmiBridge accepts a null leaf value as the CSP Delete operation. The
  # Personalization class itself is a singleton and rejects Remove-CimInstance
  # on current Windows 11 builds; keeping its empty shell also preserves any
  # unrelated DesktopImageUrl value.
  Set-CimInstance -CimInstance $instance -Property @{ LockScreenImageUrl=$null } | Out-Null
}
$verify=Get-CimInstance -Namespace $ns -ClassName $class -ErrorAction SilentlyContinue | Where-Object { $_.ParentID -eq './Vendor/MSFT' -and $_.InstanceID -eq 'Personalization' } | Select-Object -First 1
if ($hadLockScreenUrl) {
  if ($null -eq $verify -or [string]$verify.LockScreenImageUrl -ne $previousUrl) { throw 'Personalization CSP rollback did not verify' }
} elseif ($null -ne $verify -and -not [string]::IsNullOrEmpty([string]$verify.LockScreenImageUrl)) {
  throw 'Personalization CSP cleanup did not verify'
}`, quotedBackup, ownedMDMImagePattern)
	return runPowerShell(script, 30*time.Second)
}

func runPowerShell(script string, timeout time.Duration) error {
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(powershell); err != nil {
		powershell = "powershell.exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
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

func stageMDMImage(source string) (string, error) {
	if err := os.MkdirAll(paths.CSPImageDir(), 0o755); err != nil {
		return "", err
	}
	destination := filepath.Join(paths.CSPImageDir(), filepath.Base(source))
	if _, err := os.Stat(destination); err == nil {
		return destination, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	b, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	temporary := destination + ".tmp"
	if err := os.WriteFile(temporary, b, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return "", err
	}
	return destination, nil
}

func cleanupStagedMDMImages(current string, keep int) {
	entries, err := os.ReadDir(paths.CSPImageDir())
	if err != nil {
		return
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	files := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jpg") {
			continue
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, candidate{path: filepath.Join(paths.CSPImageDir(), entry.Name()), mod: info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	retained := 0
	for _, file := range files {
		if samePath(file.path, current) {
			continue
		}
		if retained < keep-1 {
			retained++
			continue
		}
		_ = os.Remove(file.path)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func samePath(a, b string) bool {
	aAbs, errA := filepath.Abs(a)
	bAbs, errB := filepath.Abs(b)
	return errA == nil && errB == nil && strings.EqualFold(filepath.Clean(aAbs), filepath.Clean(bAbs))
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
	// Values owned by an earlier W:ID version are not useful restoration
	// targets; treating them as absent completes the migration cleanly.
	legacyImage := strings.ToLower(backup.LockScreenImage.String)
	if strings.Contains(legacyImage, `\bgstatusservice\`) || strings.Contains(legacyImage, `\backgroundchanger\backgrounds\`) {
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

func RestorePolicies(backupPath string, ownedDataDirs ...string) []error {
	var errors []error
	backup := policyBackup{}
	if b, err := os.ReadFile(backupPath); err == nil {
		if err := json.Unmarshal(b, &backup); err != nil {
			errors = append(errors, err)
		}
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, personalizationPolicy, registry.QUERY_VALUE|registry.SET_VALUE)
	if err == nil {
		if current, _, valueErr := key.GetStringValue("LockScreenImage"); valueErr == nil && isOwnedByAnyPath(current, ownedDataDirs) {
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

func isOwnedByAnyPath(value string, dataDirs []string) bool {
	for _, dataDir := range dataDirs {
		if isOwnedPath(value, dataDir) {
			return true
		}
	}
	return false
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
		return "..." + value[len(value)-497:]
	}
	return value
}
