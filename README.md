<p align="center">
  <img src="assets/branding/wid-logo.png" width="180" alt="Wallpaper Identity W:ID logo">
</p>

<h1 align="center">Wallpaper Identity</h1>

<p align="center"><strong>W:ID</strong> — machine identity and health, visible before sign-in.</p>

Wallpaper Identity renders fastfetch-style machine information and applies it as the managed Windows lock and sign-in background. Its automatic LocalSystem service starts during boot and needs no user session, making it useful in labs, equipment rooms, VM fleets, classrooms, and other places where visually identical computers are difficult to tell apart.

![W:ID showing the current address on the Windows 11 Pro pre-login screen after a powered-off network change](assets/screenshots/prelogin.png)

This is the real Windows 11 Pro pre-login screen from the v4.0.2 validation guest. The adapter changed from `10.77.0.3` to `10.0.2.15` while powered off; W:ID waited for DHCP to settle, generated the shown image, and made Windows rebuild its pre-login bitmap cache before anyone signed in.

## What it shows

- Hostname, Windows version/build, BIOS serial number, and IPv4 addresses
- CPU, GPU, memory, system-disk usage, and uptime
- Running-service count and the state of Defender, DHCP, DNS, Event Log, time sync, and Windows Update
- Pending-reboot state and the exact generation time

Hostname and a timezone-qualified **Generated at** timestamp are permanent parts of every W:ID layout, including custom configurations, so stale output is immediately obvious.

The layout reserves Windows 11's top-center clock area and Windows 10's lower-left clock area. The installer also enables Microsoft's **Show clear logon background** policy so the information remains readable on the credential screen.

## Install

1. Download `WallpaperIdentitySetup.exe` from the [latest release](https://github.com/amcchord/WallpaperIdentity/releases/latest).
2. Open it, choose a starting layout, and select **Install**.
3. Approve the Windows administrator prompt.

The W:ID installer is self-contained and works offline. Click anywhere on a full-size rendered preview for **Identity**, **Balanced**, or **Operations** to select it; **Install** stays in the standard lower-right position. Setup then installs one automatic LocalSystem service and immediately generates the first background. Its blocking progress window finishes at 100% with an explicit **Done** state.

For unattended deployment, the companion `WallpaperIdentityCLI.exe` provides strict, blocking `install`, `repair`, `upgrade`, `status`, `refresh`, `render`, and `uninstall` commands with JSON/result-file output and stable exit codes. See the [headless and RMM guide](docs/CLI.md).

![Wallpaper Identity clean-install window with three clickable layout previews and Install in the lower right](assets/screenshots/installer.png)

![Wallpaper Identity installer with the full Operations preview selected](assets/screenshots/installer-selection.png)

![Wallpaper Identity progress window at its final Done state](assets/screenshots/installer-done.png)

Run the same executable again to repair, upgrade, or uninstall. W:ID is also registered in Windows **Apps & features**.

### Upgrading from version 3

Version 4 detects the previous service automatically. **Repair / Upgrade** preserves and moves its `config.yml`, generated images, logs, and rollback data into the new Wallpaper Identity paths, replaces the service and Apps & features entry, and removes the previous install directory. Custom layouts are retained.

## Supported Windows editions

| Edition | Support |
|---|---|
| Windows 10/11 Enterprise or Education | Supported |
| Windows 10/11 IoT Enterprise | Supported |
| Windows Server | Supported through Group Policy |
| Windows 10/11 Pro | Supported through the default `SetEduPolicies` compatibility switch |
| Windows Home | Not supported |

Microsoft documents that the Personalization CSP works on Pro when SharedPC `SetEduPolicies` is enabled. W:ID enables that one compatibility switch by default on Pro; it does **not** enable Shared PC mode, account cleanup, power policies, kiosk mode, or storage restrictions. The switch disables Windows tips, advertising ID, and Microsoft consumer experiences. Its previous state is backed up and restored during uninstall, and power users can opt out with `enable_pro_compatibility: false` (in which case Pro will report an unhealthy policy instead of a false success). See Microsoft's [Personalization CSP requirements](https://learn.microsoft.com/windows/client-management/mdm/personalization-csp), [`SetEduPolicies` effects](https://learn.microsoft.com/windows/configuration/shared-pc/shared-pc-technical), and [`MDM_SharedPC` class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-sharedpc).

## How refresh works

1. The Service Control Manager starts `WallpaperIdentity` as LocalSystem during boot, before interactive logon.
2. The service gathers current machine state and renders a new, versioned JPEG in `C:\ProgramData\Wallpaper Identity\backgrounds`, then stages the CSP copy in the space-free `C:\ProgramData\WallpaperIdentityCSP` path required by Windows Pro.
3. It applies Microsoft's machine lock-screen Group Policy and LocalSystem-only `MDM_Personalization` WMI bridge. On Pro, it first enables only `SetEduPolicies`.
4. It refreshes after boot settles, every five minutes, and on logon, logoff, lock, and power events.
5. After the boot-settled image is accepted, it uses Windows' documented session API to rotate an empty physical-console login session once per boot. This makes Windows regenerate its protected bitmap cache from the current image.
6. The rotation is skipped if any user is signed in, Explorer is present, the console is not an empty connected login session, or the session changes during a final race check. If a user-owned lock screen is visible later, W:ID restarts only `LockApp.exe`.

Each render uses a new filename to give Windows an unambiguous policy target. A refresh is successful only after W:ID reads back its Group Policy values or Windows reports Personalization CSP status `1` for the exact generated file; Pro requires both verified `SetEduPolicies` and CSP readback. Microsoft documents that [auto-start services run during system boot](https://learn.microsoft.com/windows/win32/services/automatically-starting-services) and that the [`MDM_Personalization` class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-personalization) is available in the LocalSystem partition.

**Windows 11 Pro cache refresh:** Windows can initially paint the previous verified bitmap before DHCP and the Personalization CSP finish. W:ID therefore performs the console rotation only after its 20-second boot-settled collection and exact CSP status `1` readback. The transition can briefly show a black or clock-only frame; the validated Pro guest rebuilt the login screen in about five seconds and displayed the new powered-off-change address. W:ID never terminates or injects into `LogonUI.exe`, never replaces a credential provider, and never disconnects an authenticated session. Power users can opt out with `refresh_login_screen_on_boot: false`; **Generated at** remains visible in every layout.

More detail is in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Configuration and diagnostics

Wallpaper Identity creates:

```text
C:\Program Files\Wallpaper Identity\WallpaperIdentity.exe
C:\ProgramData\Wallpaper Identity\config.yml
C:\ProgramData\Wallpaper Identity\status.json
C:\ProgramData\Wallpaper Identity\pre-login-refresh.json
C:\ProgramData\Wallpaper Identity\WallpaperIdentity.log
C:\ProgramData\Wallpaper Identity\backgrounds\
```

| Preset | Focus | Included details |
|---|---|---|
| **Identity** | Tell similar machines apart quickly | OS/build, IPv4, serial |
| **Balanced** | Everyday machine status | Identity plus CPU/GPU, memory, disk, uptime, service count, restart state, critical services |
| **Operations** | Troubleshooting and fleet health | Resources, service/restart state, and failed automatic services |

`config.yml` exposes every field as a readable Boolean. Set `preset: custom` before changing individual `show` values; changing `preset` to a named value applies that complete preset. You can also tune `refresh_minutes`, opt out of the guarded boot login-screen rotation, control the documented Pro compatibility switch, use a local JPEG/PNG with `base_image`, or force both `width` and `height`. Hostname and **Generated at** cannot be hidden.

Start from [config.example.yml](config.example.yml) and see [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the complete annotated reference. Display changes are read on the next refresh; restart the service after changing `refresh_minutes`.

Synchronous maintenance and automation commands, run from an elevated shell beside the downloaded console artifact:

```powershell
& .\WallpaperIdentityCLI.exe status --json
& .\WallpaperIdentityCLI.exe refresh --json
& .\WallpaperIdentityCLI.exe render --output C:\Temp\preview.jpg
& .\WallpaperIdentityCLI.exe repair --quiet
& .\WallpaperIdentityCLI.exe uninstall --quiet
```

## Build and test

Requirements: Windows and Go 1.24 or newer.

```powershell
go test ./...
powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version v4.5.0
```

Release output is written to `dist\WallpaperIdentitySetup.exe`, `dist\WallpaperIdentityCLI.exe`, and `dist\SHA256SUMS.txt`. Both offline binaries contain the service, renderer, fonts, W:ID icon, and Windows manifest; Setup uses the graphical Windows subsystem while CLI uses the console subsystem for reliable RMM waiting, stdout, and exit codes.

The validation matrix covers Windows 10 Enterprise 22H2, Windows 11 Enterprise 25H2, and Windows 11 Pro 25H2 in parallel 8 GiB VirtualBox guests. See [docs/TESTING.md](docs/TESTING.md) for reproducible evidence.

## Uninstall behavior

Uninstall stops and removes the service, unregisters W:ID, and restores lock-screen/logon policy values plus the Pro compatibility state that existed before installation. Configuration and generated images are kept by default for audit and recovery. Product-key exports and local VM media are ignored by git.

## License

[MIT](LICENSE). JetBrains Mono is distributed under the SIL Open Font License; see [assets/fonts/OFL.txt](assets/fonts/OFL.txt).
