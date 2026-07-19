// Package application owns the shared graphical and headless command surface.
package application

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/engine"
	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"github.com/amcchord/WallpaperIdentity/v4/internal/setup"
	"github.com/amcchord/WallpaperIdentity/v4/internal/ui"
	"github.com/amcchord/WallpaperIdentity/v4/internal/winservice"
)

const (
	exitOK             = 0
	exitUsage          = 2
	exitNotInstalled   = 3
	exitUnhealthy      = 4
	exitAdminRequired  = 5
	exitUpgradeNeeded  = 6
	exitOperationError = 10
	exitSetupRunning   = 1618
	exitRebootRequired = 3010
)

type commandOptions struct {
	Name        string
	Preset      string
	ConfigFile  string
	Output      string
	Headless    bool
	Quiet       bool
	JSON        bool
	ResultFile  string
	RemoveData  bool
	Interactive bool
}

type commandResult struct {
	SchemaVersion   int            `json:"schema_version"`
	Operation       string         `json:"command"`
	Action          string         `json:"action,omitempty"`
	Changed         bool           `json:"changed"`
	RebootRequired  bool           `json:"reboot_required"`
	Success         bool           `json:"success"`
	ExitCode        int            `json:"exit_code"`
	Version         string         `json:"version"`
	Commit          string         `json:"commit"`
	Message         string         `json:"message,omitempty"`
	Error           *resultError   `json:"error,omitempty"`
	Warnings        []string       `json:"warnings,omitempty"`
	Installed       bool           `json:"installed"`
	LegacyInstalled bool           `json:"legacy_installed"`
	ServiceName     string         `json:"service_name"`
	ServiceState    string         `json:"service_state"`
	DataDirectory   string         `json:"data_directory"`
	ConfigFile      string         `json:"config_file,omitempty"`
	Output          string         `json:"output,omitempty"`
	RemovedData     bool           `json:"removed_data,omitempty"`
	Status          *engine.Status `json:"status,omitempty"`
}

type resultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type commandFailure struct {
	code int
	err  error
}

func (e *commandFailure) Error() string { return e.err.Error() }
func (e *commandFailure) Unwrap() error { return e.err }

func fail(code int, format string, values ...any) error {
	return &commandFailure{code: code, err: fmt.Errorf(format, values...)}
}

// Main runs Wallpaper Identity using the supplied process arguments and I/O.
func Main(args []string, stdout, stderr io.Writer) int { return mainExit(args, stdout, stderr) }

// MainCLI runs the console artifact headlessly by default. The explicit
// --interactive maintenance flag is reserved for user-driven setup and the
// Apps & features uninstall registration.
func MainCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = io.WriteString(stdout, usageText)
		return exitOK
	}
	return mainExitMode(args, stdout, stderr, true)
}

func mainExit(args []string, stdout, stderr io.Writer) int {
	return mainExitMode(args, stdout, stderr, false)
}

func mainExitMode(args []string, stdout, stderr io.Writer, forceHeadless bool) int {
	options, err := parseCommand(args)
	if err != nil {
		result := newResult(commandName(args))
		result.Error = &resultError{Code: "invalid_command_line", Message: err.Error()}
		result.ExitCode = exitUsage
		if path := requestedResultFile(args); path != "" && !rawResultFileConflict(args) {
			if resultErr := writeResultFile(path, result); resultErr != nil {
				result.Warnings = append(result.Warnings, resultErr.Error())
			}
		}
		if jsonRequested(args) {
			writeJSON(stdout, result)
		} else {
			_, _ = fmt.Fprintf(stderr, "Wallpaper Identity: %v\n\n%s", err, usageText)
		}
		return exitUsage
	}
	if options.Name == "help" {
		_, _ = io.WriteString(stdout, usageText)
		return exitOK
	}
	options = applyProcessMode(options, forceHeadless)

	result, err := execute(options)
	code := exitCode(err)
	if err == nil && result.RebootRequired {
		code = exitRebootRequired
	}
	result.Success = err == nil
	result.ExitCode = code
	if err != nil {
		result.Error = &resultError{Code: errorName(code), Message: err.Error()}
	}
	if options.ResultFile != "" {
		if resultErr := writeResultFile(options.ResultFile, result); resultErr != nil {
			err = resultErr
			code = exitOperationError
			result.Success = false
			result.ExitCode = code
			result.Error = &resultError{Code: "result_file_failed", Message: resultErr.Error()}
		}
	}
	if options.JSON {
		writeJSON(stdout, result)
	} else if err != nil {
		_, _ = fmt.Fprintf(stderr, "Wallpaper Identity %s failed: %v\n", options.Name, err)
	} else if !options.Quiet && options.Name != "gui" {
		writeHuman(stdout, result)
	}
	return code
}

func execute(options commandOptions) (commandResult, error) {
	result := newResult(options.Name)
	switch options.Name {
	case "gui":
		return result, ui.Main()
	case "version":
		result.Action = "queried"
		result.Message = fmt.Sprintf("Wallpaper Identity %s (%s)", buildinfo.Version, buildinfo.Commit)
		return result, nil
	case "install", "repair":
		wasInstalled, wasLegacy := setup.IsInstalled(), setup.IsLegacyInstalled()
		if options.ConfigFile != "" {
			absolute, err := filepath.Abs(options.ConfigFile)
			if err != nil {
				return result, fail(exitUsage, "resolve --config: %v", err)
			}
			supplied, err := config.Load(absolute)
			if err != nil {
				return result, fail(exitUsage, "invalid --config %q: %v", absolute, err)
			}
			if err := config.ValidateAssets(supplied); err != nil {
				return result, fail(exitUsage, "invalid --config %q: %v", absolute, err)
			}
			options.ConfigFile = absolute
		}
		if !setup.IsAdministrator() {
			if options.Headless {
				return result, fail(exitAdminRequired, "administrator privileges are required for unattended %s", options.Name)
			}
			if err := setup.RelaunchElevated(elevatedInstallArguments(options)...); err != nil {
				return result, fmt.Errorf("request administrator elevation: %w", err)
			}
			result.Message = "Administrator elevation requested."
			return result, nil
		}
		var setupResult setup.OperationResult
		operation := func(progress setup.ProgressFunc) error {
			var err error
			setupResult, err = setup.InstallWithOptionsResult(progress, setup.InstallOptions{Preset: options.Preset, ConfigFile: options.ConfigFile})
			return err
		}
		var operationErr error
		if options.Headless {
			operationErr = operation(nil)
		} else {
			title := "Installing Wallpaper Identity"
			if options.Name == "repair" {
				title = "Repairing / upgrading Wallpaper Identity"
			}
			operationErr = ui.RunOperation(title, operation)
		}
		result = refreshedResult(result)
		result.RebootRequired = setupResult.RebootRequired
		if operationErr != nil {
			return result, fmt.Errorf("%s: %w", options.Name, operationErr)
		}
		result.ConfigFile = paths.ConfigFile()
		result.Changed = true
		switch {
		case wasLegacy:
			result.Action = "upgraded"
		case wasInstalled:
			result.Action = "repaired"
		default:
			result.Action = "installed"
		}
		result.Message = "Wallpaper Identity is installed and the first refresh completed."
		return result, nil
	case "uninstall":
		if setup.IsLegacyInstalled() {
			return result, fail(exitUpgradeNeeded, "upgrade the previous version before using the v4 uninstaller so policy rollback can be serialized safely")
		}
		wasInstalled := setup.IsInstalled()
		if !setup.IsAdministrator() {
			if options.Headless {
				return result, fail(exitAdminRequired, "administrator privileges are required for unattended uninstall")
			}
			arguments := []string{"uninstall"}
			if options.RemoveData {
				arguments = append(arguments, "--remove-data")
			}
			if options.Interactive {
				arguments = append(arguments, "--interactive")
			}
			if err := setup.RelaunchElevated(arguments...); err != nil {
				return result, fmt.Errorf("request administrator elevation: %w", err)
			}
			result.Message = "Administrator elevation requested."
			return result, nil
		}
		var setupResult setup.OperationResult
		operation := func(progress setup.ProgressFunc) error {
			var err error
			setupResult, err = setup.UninstallResult(progress, options.RemoveData)
			return err
		}
		var operationErr error
		if options.Headless {
			operationErr = operation(nil)
		} else {
			operationErr = ui.RunOperation("Uninstalling Wallpaper Identity", operation)
		}
		result = refreshedResult(result)
		result.RebootRequired = setupResult.RebootRequired
		result.RemovedData = setupResult.RemovedData
		if operationErr != nil {
			return result, fmt.Errorf("uninstall: %w", operationErr)
		}
		result.Message = "Wallpaper Identity was uninstalled."
		result.Action = "uninstalled"
		result.Changed = wasInstalled
		return result, nil
	case "status":
		result.Action = "queried"
		result = refreshedResult(result)
		if result.LegacyInstalled {
			return result, fail(exitUpgradeNeeded, "a previous version is installed; run repair or upgrade to deploy Wallpaper Identity")
		}
		if !result.Installed {
			return result, fail(exitNotInstalled, "Wallpaper Identity is not installed")
		}
		status, err := setup.ReadStatus()
		if err != nil {
			return result, fail(exitUnhealthy, "service status is unavailable: %v", err)
		}
		result.Status = &status
		if result.ServiceState != "Running" {
			return result, fail(exitUnhealthy, "service is not running (state %s)", result.ServiceState)
		}
		if !status.Success {
			return result, fail(exitUnhealthy, "last refresh failed: %s", status.Error)
		}
		cfg, err := config.Load(paths.ConfigFile())
		if err == nil {
			staleAfter := 2*time.Duration(cfg.RefreshMinutes)*time.Minute + time.Minute
			if age := time.Since(status.CompletedAt); status.CompletedAt.IsZero() || age > staleAfter {
				return result, fail(exitUnhealthy, "last refresh is stale (%s old; threshold %s)", age.Round(time.Second), staleAfter)
			}
		}
		if status.EditionSupported {
			result.Message = "Wallpaper Identity is installed and healthy."
		} else {
			result.Message = "Wallpaper Identity is healthy, but this Windows edition does not guarantee machine lock-screen policy support."
			result.Warnings = append(result.Warnings, "Windows edition does not guarantee machine lock-screen policy support")
		}
		return result, nil
	case "refresh":
		if !setup.IsAdministrator() {
			return result, fail(exitAdminRequired, "administrator privileges are required to signal the LocalSystem service")
		}
		status, err := winservice.RequestRefresh(90 * time.Second)
		result = refreshedResult(result)
		if !status.CompletedAt.IsZero() {
			result.Status = &status
		}
		if err != nil {
			return result, fmt.Errorf("refresh: %w", err)
		}
		result.Message = "The pre-login background was refreshed."
		result.Action = "refreshed"
		result.Changed = true
		return result, nil
	case "render":
		absolute, err := filepath.Abs(options.Output)
		if err != nil {
			return result, fail(exitUsage, "resolve --output: %v", err)
		}
		status, err := engine.New(log.New(io.Discard, "", 0)).RenderPreview(absolute)
		result.Status = &status
		result.Output = absolute
		if err != nil {
			return result, fmt.Errorf("render: %w", err)
		}
		result.Message = "Preview rendered."
		result.Action = "rendered"
		result.Changed = true
		return result, nil
	case "service":
		return result, winservice.Run()
	default:
		return result, fail(exitUsage, "unknown command %q", options.Name)
	}
}

func applyProcessMode(options commandOptions, forceHeadless bool) commandOptions {
	if forceHeadless && !options.Interactive {
		options.Headless = true
	}
	return options
}

func parseCommand(args []string) (commandOptions, error) {
	if len(args) == 0 {
		return commandOptions{Name: "gui"}, nil
	}
	name := strings.ToLower(args[0])
	name = strings.TrimLeft(name, "-/")
	switch name {
	case "-h", "h", "help", "?":
		return commandOptions{Name: "help"}, nil
	case "upgrade":
		name = "repair"
	}
	options := commandOptions{Name: name}
	arguments := normalizeWindowsArguments(args[1:])

	newFlags := func() *flag.FlagSet {
		flags := flag.NewFlagSet(name, flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		return flags
	}
	common := func(flags *flag.FlagSet) {
		flags.BoolVar(&options.Headless, "headless", false, "run without windows")
		flags.BoolVar(&options.Headless, "no-ui", false, "run without windows")
		flags.BoolVar(&options.Quiet, "quiet", false, "suppress successful output")
		flags.BoolVar(&options.Quiet, "silent", false, "suppress successful output")
		flags.BoolVar(&options.JSON, "json", false, "write one JSON result")
		flags.StringVar(&options.ResultFile, "result-file", "", "also write the JSON result to a file")
	}

	switch name {
	case "install", "repair":
		flags := newFlags()
		common(flags)
		flags.BoolVar(&options.Interactive, "interactive", false, "allow GUI and UAC elevation")
		flags.StringVar(&options.Preset, "preset", "", "identity, balanced, or operations")
		flags.StringVar(&options.ConfigFile, "config", "", "validated YAML configuration to deploy")
		if err := flags.Parse(arguments); err != nil {
			return options, err
		}
		if flags.NArg() != 0 {
			return options, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		if options.Preset != "" && options.ConfigFile != "" {
			return options, errors.New("--preset and --config are mutually exclusive")
		}
		if options.Preset != "" {
			if _, err := config.ForPreset(options.Preset); err != nil {
				return options, err
			}
			options.Preset = strings.ToLower(options.Preset)
		}
	case "uninstall":
		flags := newFlags()
		common(flags)
		flags.BoolVar(&options.Interactive, "interactive", false, "allow GUI and UAC elevation")
		flags.BoolVar(&options.RemoveData, "remove-data", false, "also remove config, logs, and generated images")
		if err := flags.Parse(arguments); err != nil {
			return options, err
		}
		if flags.NArg() != 0 {
			return options, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
	case "status", "refresh", "version":
		flags := newFlags()
		common(flags)
		if err := flags.Parse(arguments); err != nil {
			return options, err
		}
		if flags.NArg() != 0 {
			return options, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
	case "render":
		flags := newFlags()
		common(flags)
		flags.StringVar(&options.Output, "output", "", "JPEG output path")
		if len(arguments) > 0 && !strings.HasPrefix(arguments[0], "-") {
			options.Output = arguments[0]
			arguments = arguments[1:]
		}
		if err := flags.Parse(arguments); err != nil {
			return options, err
		}
		if flags.NArg() > 1 {
			return options, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
		}
		if options.Output != "" && flags.NArg() != 0 {
			return options, errors.New("use either --output or one positional output path, not both")
		}
		if options.Output == "" && flags.NArg() == 1 {
			options.Output = flags.Arg(0)
		}
		if options.Output == "" {
			options.Output = "WallpaperIdentity-preview.jpg"
		}
	case "service":
		if len(arguments) != 0 {
			return options, errors.New("service mode takes no arguments")
		}
		options.Headless = true
	default:
		return options, fmt.Errorf("unknown command %q", args[0])
	}
	if err := validateResultTarget(options); err != nil {
		return options, err
	}
	if options.Interactive && (options.Headless || options.Quiet || options.JSON || options.ResultFile != "") {
		return options, errors.New("--interactive cannot be combined with headless output options")
	}
	options.Headless = options.Headless || options.Quiet || options.JSON || options.ResultFile != ""
	return options, nil
}

func validateResultTarget(options commandOptions) error {
	if options.ResultFile == "" {
		return nil
	}
	for label, path := range map[string]string{"--config": options.ConfigFile, "render output": options.Output} {
		if path == "" {
			continue
		}
		same, err := sameAbsolutePath(options.ResultFile, path)
		if err != nil {
			return err
		}
		if same {
			return fmt.Errorf("--result-file must not overwrite %s", label)
		}
	}
	return nil
}

func sameAbsolutePath(first, second string) (bool, error) {
	firstAbsolute, err := filepath.Abs(first)
	if err != nil {
		return false, fmt.Errorf("resolve path %q: %w", first, err)
	}
	secondAbsolute, err := filepath.Abs(second)
	if err != nil {
		return false, fmt.Errorf("resolve path %q: %w", second, err)
	}
	return strings.EqualFold(filepath.Clean(firstAbsolute), filepath.Clean(secondAbsolute)), nil
}

func requestedResultFile(args []string) string {
	normalized := normalizeWindowsArguments(args)
	for index, argument := range normalized {
		lower := strings.ToLower(argument)
		if lower == "--result-file" && index+1 < len(normalized) {
			return normalized[index+1]
		}
		if strings.HasPrefix(lower, "--result-file=") {
			return argument[len("--result-file="):]
		}
	}
	return ""
}

func rawResultFileConflict(args []string) bool {
	resultPath := requestedResultFile(args)
	if resultPath == "" {
		return false
	}
	normalized := normalizeWindowsArguments(args)
	for index, argument := range normalized {
		lower := strings.ToLower(argument)
		for _, name := range []string{"config", "output"} {
			var value string
			if lower == "--"+name && index+1 < len(normalized) {
				value = normalized[index+1]
			} else if strings.HasPrefix(lower, "--"+name+"=") {
				value = argument[len("--"+name+"="):]
			}
			if value != "" {
				same, _ := sameAbsolutePath(resultPath, value)
				if same {
					return true
				}
			}
		}
	}
	if commandName(args) == "render" {
		arguments := normalizeWindowsArguments(args[1:])
		for index := 0; index < len(arguments); index++ {
			argument := arguments[index]
			lower := strings.ToLower(argument)
			if strings.HasPrefix(lower, "-") {
				if (lower == "--result-file" || lower == "--output") && index+1 < len(arguments) {
					index++
				}
				continue
			}
			same, _ := sameAbsolutePath(resultPath, argument)
			return same
		}
	}
	return false
}

func normalizeWindowsArguments(arguments []string) []string {
	normalized := make([]string, len(arguments))
	for index, argument := range arguments {
		lower := strings.ToLower(argument)
		switch lower {
		case "/headless", "/no-ui", "/quiet", "/silent", "/json", "/remove-data", "/preset", "/config", "/output", "/result-file", "/interactive":
			normalized[index] = "--" + strings.TrimPrefix(lower, "/")
		default:
			normalized[index] = argument
			for _, name := range []string{"preset", "config", "output", "result-file", "json"} {
				prefix := "/" + name + "="
				if strings.HasPrefix(lower, prefix) {
					normalized[index] = "--" + name + "=" + argument[len(prefix):]
					break
				}
			}
		}
	}
	return normalized
}

func elevatedInstallArguments(options commandOptions) []string {
	arguments := []string{options.Name}
	if options.Preset != "" {
		arguments = append(arguments, "--preset", options.Preset)
	}
	if options.ConfigFile != "" {
		arguments = append(arguments, "--config", options.ConfigFile)
	}
	if options.Interactive {
		arguments = append(arguments, "--interactive")
	}
	return arguments
}

func refreshedResult(result commandResult) commandResult {
	result.Installed = setup.IsInstalled()
	result.LegacyInstalled = setup.IsLegacyInstalled()
	if result.LegacyInstalled {
		result.ServiceName = paths.LegacyServiceName
	} else {
		result.ServiceName = paths.ServiceName
	}
	result.ServiceState, _ = setup.ServiceState()
	if status, err := setup.ReadStatus(); err == nil {
		result.Status = &status
	}
	return result
}

func newResult(operation string) commandResult {
	serviceState, _ := setup.ServiceState()
	installed := setup.IsInstalled()
	legacyInstalled := setup.IsLegacyInstalled()
	serviceName := paths.ServiceName
	if legacyInstalled {
		serviceName = paths.LegacyServiceName
	}
	return commandResult{
		SchemaVersion: 1, Operation: operation, Version: buildinfo.Version, Commit: buildinfo.Commit,
		Installed: installed, LegacyInstalled: legacyInstalled,
		ServiceName: serviceName, ServiceState: serviceState, DataDirectory: paths.DataDir(),
	}
}

func errorName(code int) string {
	switch code {
	case exitUsage:
		return "invalid_command_line"
	case exitNotInstalled:
		return "not_installed"
	case exitUnhealthy:
		return "unhealthy"
	case exitAdminRequired:
		return "elevation_required"
	case exitUpgradeNeeded:
		return "upgrade_required"
	case exitSetupRunning:
		return "setup_already_running"
	default:
		return "operation_failed"
	}
}

func exitCode(err error) int {
	if err == nil {
		return exitOK
	}
	if errors.Is(err, setup.ErrSetupRunning) {
		return exitSetupRunning
	}
	var failure *commandFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return exitOperationError
}

func commandName(args []string) string {
	if len(args) == 0 {
		return "gui"
	}
	return strings.TrimLeft(strings.ToLower(args[0]), "-/")
}

func jsonRequested(args []string) bool {
	for _, argument := range normalizeWindowsArguments(args) {
		switch strings.ToLower(argument) {
		case "--json", "--json=true":
			return true
		}
	}
	return false
}

func writeJSON(output io.Writer, result commandResult) {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(result)
}

func writeResultFile(path string, result commandResult) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve result file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result file: %w", err)
	}
	b = append(b, '\n')
	temporary := absolute + ".tmp"
	if err := os.WriteFile(temporary, b, 0o600); err != nil {
		return fmt.Errorf("write result file: %w", err)
	}
	if err := os.Rename(temporary, absolute); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("commit result file: %w", err)
	}
	return nil
}

func writeHuman(output io.Writer, result commandResult) {
	if result.Message != "" {
		_, _ = fmt.Fprintln(output, result.Message)
	}
	if result.RebootRequired {
		_, _ = fmt.Fprintln(output, "Reboot required: yes")
	}
	if result.Output != "" {
		_, _ = fmt.Fprintf(output, "Output: %s\n", result.Output)
	}
	if result.Operation == "status" && result.Status != nil {
		_, _ = fmt.Fprintf(output, "Service: %s (%s)\nLast refresh: %s\nImage: %s\n", result.ServiceName, result.ServiceState, result.Status.CompletedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status.ImagePath)
	}
}

const usageText = `Wallpaper Identity (W:ID)

Usage:
  WallpaperIdentitySetup.exe                         Open the graphical installer
  WallpaperIdentityCLI.exe install [options]         Install or upgrade
  WallpaperIdentityCLI.exe repair [options]          Repair or upgrade in place
  WallpaperIdentityCLI.exe upgrade [options]         Alias for repair / upgrade
  WallpaperIdentityCLI.exe uninstall [options]       Uninstall and restore policy
  WallpaperIdentityCLI.exe status [output options]   Report service/render health
  WallpaperIdentityCLI.exe refresh [output options]  Ask LocalSystem to refresh now
  WallpaperIdentityCLI.exe render [--output PATH]    Render a preview without policy changes
  WallpaperIdentityCLI.exe version [output options]  Print build identity
  WallpaperIdentityCLI.exe help                      Show this help

Install/repair options:
  --headless              Do not show windows or request UAC; requires Administrator/SYSTEM
  --quiet                 Headless and suppress successful human-readable output
  --json                  Headless and emit one machine-readable JSON result
  --result-file PATH      Also write the JSON result atomically for RMM collection
  --preset NAME           identity, balanced, or operations (new installs only by convention)
  --config PATH           Validate and deploy a power-user config.yml; excludes --preset

Uninstall options:
  --headless              Do not show windows or request UAC; requires Administrator/SYSTEM
  --quiet                 Headless and suppress successful human-readable output
  --json                  Headless and emit one machine-readable JSON result
  --result-file PATH      Also write the JSON result atomically for RMM collection
  --remove-data           Also delete config.yml, logs, and generated images after safe rollback

Stable exit codes:
  0  Success / healthy
  2  Invalid command, option, preset, or configuration
  3  Not installed (status only)
  4  Installed but status is unavailable or the last refresh failed
  5  Administrator or LocalSystem privileges required
  6  A previous version is installed and requires repair / upgrade
 10  Installation, refresh, render, result-file, or uninstall failed
1618  Another install, repair, upgrade, or uninstall is already running
3010  Success; reboot required to complete deferred executable cleanup

RMM examples:
  WallpaperIdentityCLI.exe install --headless --preset balanced
  WallpaperIdentityCLI.exe install --json --config C:\RMM\wid-config.yml
  WallpaperIdentityCLI.exe install --quiet --result-file C:\RMM\wid-result.json
  WallpaperIdentityCLI.exe repair --quiet
  WallpaperIdentityCLI.exe status --json
  WallpaperIdentityCLI.exe refresh --json
  WallpaperIdentityCLI.exe uninstall --headless
  WallpaperIdentityCLI.exe uninstall --headless --remove-data

The setup executable accepts the same commands, but the console-subsystem CLI is recommended when a caller must synchronously capture stdout and the process exit code. Legacy --install, --uninstall, --render, --refresh, --status, and --version command aliases remain accepted.
`
