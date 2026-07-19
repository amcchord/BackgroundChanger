# BackgroundChanger

BackgroundChanger puts live, fastfetch-style machine identity and health information on the Windows lock and sign-in background—before anyone logs in.

![BackgroundChanger on the Windows pre-login screen](assets/screenshots/prelogin.png)

It is intended for labs, equipment rooms, VM fleets, classrooms, and other places where visually identical computers are hard to tell apart.

## What it shows

- Hostname, Windows version/build, BIOS serial number, and IPv4 addresses
- CPU, GPU, memory, system-disk usage, and uptime
- Running-service count and the state of Defender, DHCP, DNS, Event Log, time sync, and Windows Update
- Pending-reboot state and the exact generation time

The hostname and a timezone-qualified **Generated at** timestamp are always visible, including in custom configurations. That makes a stale screen obvious instead of merely showing old data without warning.

The layout reserves Windows 11's top-center clock area and Windows 10's lower-left clock area. The installer also enables Microsoft's **Show clear logon background** policy so the information remains readable on the credential screen.

## Install

1. Download `BackgroundChangerSetup.exe` from the [latest release](https://github.com/amcchord/BackgroundChanger/releases/latest).
2. Open it, choose a starting layout, and select **Install**.
3. Approve the Windows administrator prompt.

The installer is self-contained and works offline. Choose one of its three visual presets—**Identity**, **Balanced**, or **Operations**—then it installs one automatic LocalSystem service and immediately generates the first background. The installer shows a real rendered preview of every preset before anything is changed.

![BackgroundChanger graphical installer](assets/screenshots/installer.png)

Run the same executable again to repair, upgrade, or uninstall. It is also registered in Windows **Apps & features**.

## Supported Windows editions

| Edition | Support |
|---|---|
| Windows 10/11 Enterprise or Education | Supported |
| Windows 10/11 IoT Enterprise | Supported |
| Windows Server | Supported through Group Policy |
| Windows Pro | Not supported by default; tested Pro systems accept the writes but ignore the image |
| Windows Home | Not supported |

This boundary comes from Windows policy support, not an installer check. Microsoft documents that the Personalization CSP works on Pro only when the device is already configured with SharedPC `SetEduPolicies` or `BootToCloudPCEnhanced`. BackgroundChanger does not silently enable either broader device-management mode. It reports the detected edition in its status file and UI instead of claiming that the background is active. See Microsoft's [lock-screen configuration matrix](https://learn.microsoft.com/windows/configuration/background/), [Personalization CSP requirements](https://learn.microsoft.com/windows/client-management/mdm/personalization-csp), and [Shared PC policy effects](https://learn.microsoft.com/windows/configuration/shared-pc/shared-pc-technical).

## Why this version refreshes before sign-in

Version 3 replaces the old scheduled-task approach with a normal auto-start Windows service:

1. The Service Control Manager starts `BackgroundChanger` as LocalSystem during boot, before interactive logon.
2. The service gathers current machine state and renders a new, versioned JPEG in `C:\ProgramData\BackgroundChanger\backgrounds`.
3. It applies Microsoft's machine lock-screen Group Policy and, where available, the LocalSystem-only `MDM_Personalization` WMI bridge.
4. It refreshes after boot settles, every five minutes, and on logon, logoff, lock, and power events.
5. If the lock screen is already visible, it restarts only `LockApp.exe`. It never terminates the security-sensitive `LogonUI.exe` process.

Rotating the image filename on every render prevents Windows from reusing a stale cached image. Microsoft documents that [auto-start services run during system boot](https://learn.microsoft.com/windows/win32/services/automatically-starting-services) and that the [`MDM_Personalization` class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-personalization) is available in the LocalSystem partition.

More detail is in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Configuration and diagnostics

BackgroundChanger creates:

```text
C:\Program Files\BackgroundChanger\BackgroundChanger.exe
C:\ProgramData\BackgroundChanger\config.yml
C:\ProgramData\BackgroundChanger\status.json
C:\ProgramData\BackgroundChanger\BackgroundChanger.log
C:\ProgramData\BackgroundChanger\backgrounds\
```

The installer offers three useful starting points:

| Preset | Focus | Included details |
|---|---|---|
| **Identity** | Tell similar machines apart quickly | OS/build, IPv4, serial |
| **Balanced** | Everyday machine status | Identity plus CPU/GPU, memory, disk, uptime, service count, restart state, critical services |
| **Operations** | Troubleshooting and fleet health | Resources, service/restart state, and failed automatic services |

`config.yml` exposes every field as a readable Boolean for power users. Set `preset: custom` before changing individual `show` values; changing `preset` to one of the three named values applies that complete preset. You can also tune `refresh_minutes`, use a local JPEG/PNG with `base_image`, or force both `width` and `height`. Hostname and **Generated at** cannot be hidden.

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for the complete annotated example. Display changes are read on the next refresh; restart the service after changing `refresh_minutes`. The status JSON records the last snapshot, policy results, image path, Windows-edition compatibility, and any warnings. The log is append-only and contains no credentials or product keys.

Maintenance commands, run from an elevated shell:

```powershell
BackgroundChanger.exe --refresh
BackgroundChanger.exe --render C:\Temp\preview.jpg
BackgroundChanger.exe --install --quiet
BackgroundChanger.exe --uninstall --quiet
```

## Build and test

Requirements: Windows and Go 1.24 or newer.

```powershell
go test ./...
powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version v3.0.0
```

Release output is written to `dist\BackgroundChangerSetup.exe` with `dist\SHA256SUMS.txt`. The compiled executable includes the service, installer UI, renderer, font, and Windows manifest.

The release was validated in parallel clean VirtualBox VMs on Windows 10 Enterprise 22H2, Windows 11 Enterprise 25H2, and Windows 11 Pro 25H2. Both Enterprise guests displayed and refreshed the image before sign-in. The Pro guest provided the expected negative result: the service and policy writes succeeded, but Windows retained its stock image. The reproducible matrix and evidence are in [docs/TESTING.md](docs/TESTING.md).

## Uninstall behavior

Uninstall stops and removes the service, unregisters the app, and restores any lock-screen/logon policy values that existed before installation. Configuration and generated images are kept by default for audit and recovery. Product-key exports and local VM media are ignored by git.

## License

[MIT](LICENSE). JetBrains Mono is distributed under the SIL Open Font License; see [assets/fonts/OFL.txt](assets/fonts/OFL.txt).
