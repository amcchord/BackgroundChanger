# Power-user configuration

Wallpaper Identity (W:ID) creates `C:\ProgramData\Wallpaper Identity\config.yml` during installation. The LocalSystem service reads display settings on every render, so most edits take effect at the next refresh. Restart the `WallpaperIdentity` service after changing `refresh_minutes` because that value controls the service timer itself.

The repository includes a deployment-ready [config.example.yml](../config.example.yml). Copy it to your RMM content, rename it to `config.yml`, and adjust the custom field switches as needed.

## Presets

Set `preset` to `identity`, `balanced`, or `operations` to apply the same complete layouts shown in the installer. Named presets are authoritative: their built-in field set is used even if old `show` values remain in the file.

Set `preset: custom` to tune individual fields. Hostname and the timezone-qualified **Generated at** value are always rendered and therefore have no switches.

## Complete example

```yaml
# Wallpaper Identity (W:ID) power-user configuration.
# Hostname and the Generated at timestamp are always shown.
# Windows Pro compatibility uses Microsoft's SetEduPolicies switch.
# Set preset to custom after changing individual show values.
preset: custom
refresh_minutes: 5
enable_pro_compatibility: true
refresh_login_screen_on_boot: true
base_image: "C:\\Windows\\Web\\Wallpaper\\Windows\\img0.jpg"
background_color: blue
background_mode: dark
width: 0
height: 0
show:
  os: true
  build: true
  cpu: true
  gpu: true
  memory: true
  disk: true
  ip: true
  serial: true
  uptime: true
  services: true
  restart: true
  critical_services: true
  failed_auto_services: false
```

## Settings

| Key | Meaning |
|---|---|
| `preset` | `identity`, `balanced`, `operations`, or `custom` |
| `refresh_minutes` | Timer interval from 1 to 1440 minutes; restart the service after changing it |
| `enable_pro_compatibility` | On Pro, enable Microsoft's `SetEduPolicies` requirement; defaults to `true` and disables tips, advertising ID, and consumer experiences. It does not enable Shared PC mode, account cleanup, kiosk, power, or storage settings. |
| `refresh_login_screen_on_boot` | Defaults to `true`. After the boot-settled image is accepted, rotate only an empty physical-console session once per Windows boot so `LogonUI` rebuilds its bitmap cache. It is skipped if any user is signed in or a console safety check fails. Set `false` to keep Windows' original cache behavior. |
| `base_image` | Optional absolute path to a centrally managed local JPEG or PNG; takes precedence over the standard files below |
| `background_color` | `blue`, `teal`, `green`, `purple`, `slate`, or `copper`; used when no image is active |
| `background_mode` | `dark` or `light`; controls the gradient plus matching panel and text contrast, including over custom images |
| `width`, `height` | Output dimensions; leave both `0` for the current display size and aspect-aware fallback, or set both from 600 to 7680 pixels on each axis |
| `show.os` | Windows product and release |
| `show.build` | Windows build number |
| `show.cpu`, `show.gpu` | Processor and graphics adapter |
| `show.memory`, `show.disk` | Used/total memory and system-disk capacity |
| `show.ip`, `show.serial` | IPv4 addresses and firmware serial number |
| `show.uptime` | Time since boot |
| `show.services` | Running/total service count |
| `show.restart` | Pending-reboot state |
| `show.critical_services` | Health dots for the built-in critical-service set |
| `show.failed_auto_services` | Names of automatic services that are not running |

To apply display edits immediately from an elevated PowerShell window:

```powershell
& 'C:\Program Files\Wallpaper Identity\WallpaperIdentity.exe' --refresh
```

Invalid YAML or out-of-range values do not replace the last successful background. The error is recorded in `C:\ProgramData\Wallpaper Identity\status.json` and `WallpaperIdentity.log` for diagnosis.

## Replace-in-place background

When `base_image` is empty, W:ID checks these stable paths on every render, in order:

1. `C:\ProgramData\Wallpaper Identity\background.jpg`
2. `C:\ProgramData\Wallpaper Identity\background.png`

On a truly fresh install, setup snapshots the current Windows login background to the matching standard path. The graphical installer also copies a browsed or dropped replacement there and removes the other extension. Replace that file later and the service will use the new content on its next scheduled, boot, session, or manual refresh; setup does not need to run again. Selecting a color in setup removes the standard image. A non-empty `base_image` remains authoritative for RMM-managed files elsewhere on disk.

The boot login-screen rotation can briefly show a black or clock-only frame while Windows recreates the empty console. W:ID records its once-per-boot fence in `pre-login-refresh.json`; restarting or repairing the service cannot repeat the transition in the same boot. The service never terminates `LogonUI.exe` and never rotates an authenticated session.

For RMM deployment, validate and import a prepared file atomically:

```powershell
& .\WallpaperIdentityCLI.exe install --json --config 'C:\RMM\wid-config.yml'
```

An explicit `--config` replaces the active configuration. `--preset` replaces only the information mix and preserves independent background, timing, compatibility, and sizing settings. Omitting both during `repair` or `upgrade` preserves the existing or migrated file. See [CLI.md](CLI.md) for the complete headless contract.
