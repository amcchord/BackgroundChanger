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
base_image: "C:\\Windows\\Web\\Wallpaper\\Windows\\img0.jpg"
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
| `base_image` | Optional absolute path to a local JPEG or PNG |
| `width`, `height` | Output dimensions; leave both `0` for current display size, or set both between 800×600 and 7680×4320 |
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

For RMM deployment, validate and import a prepared file atomically:

```powershell
& .\WallpaperIdentityCLI.exe install --json --config 'C:\RMM\wid-config.yml'
```

An explicit `--config` or `--preset` replaces the active configuration. Omitting both during `repair` or `upgrade` preserves the existing or migrated file. See [CLI.md](CLI.md) for the complete headless contract.
