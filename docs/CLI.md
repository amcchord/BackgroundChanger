# Headless and RMM deployment

Wallpaper Identity ships two builds of the same tested application code: `WallpaperIdentitySetup.exe` is the graphical installer, while `WallpaperIdentityCLI.exe` is a true console-subsystem executable for RMM deployment. The CLI blocks until completion and provides reliable stdout plus process exit codes. The setup binary accepts the same commands, but PowerShell does not reliably wait for GUI-subsystem processes, so automation should use the CLI.

Running the CLI with no arguments prints help and exits successfully; it never opens setup accidentally. CLI install, repair, and uninstall are headless by default. The graphical Apps & features entry uses the explicit `--interactive` maintenance flag so a user-driven uninstall can request elevation and show progress.

Run deployment commands as an elevated administrator or LocalSystem. LocalSystem is preferred for an RMM because it matches the installed service identity and cannot be interrupted by a UAC prompt.

## Common RMM commands

```powershell
# Clean install with the Balanced default
& .\WallpaperIdentityCLI.exe install --headless --preset balanced

# Validate and deploy a centrally managed power-user configuration
& .\WallpaperIdentityCLI.exe install --json --config 'C:\RMM\wid-config.yml'

# Idempotent repair or v3-to-v4 upgrade without replacing config.yml
& .\WallpaperIdentityCLI.exe repair --quiet --result-file 'C:\RMM\wid-result.json'

# Health/detection rule
& .\WallpaperIdentityCLI.exe status --json

# Ask the LocalSystem service to refresh now and wait for its result
& .\WallpaperIdentityCLI.exe refresh --json

# Restore Windows policy and uninstall, keeping config/logs/images
& .\WallpaperIdentityCLI.exe uninstall --headless

# Restore policy, uninstall, and purge retained application data
& .\WallpaperIdentityCLI.exe uninstall --headless --remove-data
```

`install`, `repair`, and `upgrade` share the same safe implementation. With no configuration option, an existing or migrated `config.yml` is retained; a clean install receives Balanced. An explicit `--preset` or `--config` intentionally replaces the active configuration. The supplied YAML is fully validated before the service, registry, policy, or filesystem installation state is changed.

Only one install, repair, upgrade, or uninstall can run at once. A second process exits with `1618` instead of racing service and policy changes.

## Output modes

| Option | Behavior |
|---|---|
| `--headless` / `--no-ui` | No windows or UAC; print a concise successful result |
| `--quiet` / `--silent` | Headless; suppress successful human-readable output |
| `--json` | Headless; emit exactly one versioned JSON result to stdout |
| `--result-file PATH` | Headless; atomically write the same JSON result for RMMs that do not capture GUI-subsystem stdout reliably |

Errors still go to stderr unless `--json` is selected. `--result-file` can be combined with any output mode. Its parent directory is created if necessary.

Example result:

```json
{
  "schema_version": 1,
  "command": "install",
  "action": "upgraded",
  "changed": true,
  "reboot_required": false,
  "success": true,
  "exit_code": 0,
  "version": "v4.5.0",
  "commit": "abc1234",
  "installed": true,
  "legacy_installed": false,
  "service_name": "WallpaperIdentity",
  "service_state": "Running",
  "data_directory": "C:\\ProgramData\\Wallpaper Identity",
  "config_file": "C:\\ProgramData\\Wallpaper Identity\\config.yml"
}
```

On failure, `success` is false and `error` contains a stable code plus an actionable message. The JSON `exit_code` always matches the process exit code.
`reboot_required` is true when a setup executable that is currently running from an installed application directory was scheduled for deletion at the next reboot; an RMM can complete removal by scheduling a reboot in its normal maintenance window.

## Command reference

```text
install [--preset NAME | --config PATH] [output options]
repair  [--preset NAME | --config PATH] [output options]
upgrade [--preset NAME | --config PATH] [output options]
uninstall [--remove-data] [output options]
status [--json] [--result-file PATH]
refresh [--json] [--result-file PATH]
render [--output PATH] [output options]
version [--json] [--result-file PATH]
help
```

`status` verifies that the service is running, the last render succeeded, Windows confirmed an edition-appropriate policy path, and the timestamp is no older than twice `refresh_minutes` plus one minute. A registry write alone is not treated as success on Pro: status requires verified `SetEduPolicies` plus Personalization CSP status `1` for the exact generated file. During boot, the later `boot-settled` status also records the guarded `pre_login_session_refresh` result; `pre-login-refresh.json` preserves the once-per-boot outcome after interval status records replace it. `status` reports the previous service as `legacy_installed` before an upgrade. `refresh` sends a dedicated service control and waits up to 90 seconds for a newer `reason: "cli"` status record produced by LocalSystem. Manual refresh updates policy but does not rotate the console; the guarded rotation is deliberately limited to the boot-settled path. `render` creates a preview only and never changes Windows policy.

The previous `--install`, `--uninstall`, `--render`, `--refresh`, `--status`, and `--version` command aliases remain valid. Windows-style `/install`, `/uninstall`, `/quiet`, `/silent`, `/headless`, `/no-ui`, `/json`, `/result-file`, `/preset`, `/config`, `/output`, and `/remove-data` forms are also accepted. `/?` prints help.

## Exit codes

| Code | Meaning |
|---:|---|
| `0` | Success; `status` is healthy |
| `2` | Invalid command, option, preset, or configuration |
| `3` | Not installed (`status`) |
| `4` | Installed but stopped, stale, unavailable, or last refresh failed |
| `5` | Administrator or LocalSystem privileges required |
| `6` | A previous version is installed and requires repair or upgrade |
| `10` | Install, repair, refresh, render, result-file write, or uninstall failed |
| `1618` | Another setup operation is already running |
| `3010` | Success; reboot required to finish deleting a setup executable that ran from an installed directory |

Uninstall retains configuration, logs, generated images, and any rollback data by default. Even with `--remove-data`, application data is retained if Windows policy restoration fails, so an RMM can inspect the result and retry without losing recovery information.
