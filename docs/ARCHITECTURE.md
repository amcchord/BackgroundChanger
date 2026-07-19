# Architecture

## Design goals

BackgroundChanger must produce useful output before the first interactive sign-in, refresh without relying on a user's profile, avoid unsupported LogonUI manipulation, and remain easy to deploy to disconnected machines.

## Process model

One signed-or-hash-verifiable executable has three entry modes:

| Mode | Entry point | Purpose |
|---|---|---|
| Setup UI | no arguments | Install, repair/upgrade, and uninstall |
| Windows service | `service` | Boot/session/timer refresh loop under LocalSystem |
| Maintenance | `--refresh`, `--render`, `--install`, `--uninstall` | Automation and diagnostics |

The installer copies itself to `C:\Program Files\BackgroundChanger\BackgroundChanger.exe` and registers that copy as an automatic service. Keeping setup and runtime in one binary eliminates network downloads, embedded executable drift, and installer/service version mismatches.

## Refresh sequence

1. Load or create `C:\ProgramData\BackgroundChanger\config.json`.
2. Gather machine facts from Windows registry, WMI, network interfaces, and system counters.
3. Render a new resolution-aware JPEG with a unique timestamp in its filename.
4. Set `HKLM\Software\Policies\Microsoft\Windows\Personalization\LockScreenImage` and the related no-change/no-overlay values.
5. Enable `HKLM\Software\Policies\Microsoft\Windows\System\DisableAcrylicBackgroundOnLogon`, the registry mapping for Microsoft's **Show clear logon background** policy.
6. Attempt the documented `MDM_Personalization` WMI bridge as a second application path. This class is explicitly partitioned for LocalSystem.
7. If `LockApp.exe` is present, terminate only that presentation process so Windows recreates it using the new versioned path.
8. Atomically write `status.json`, retain the newest four backgrounds, and append a diagnostic log entry.

The service performs a synchronous first render before reporting `Running`, a second render 20 seconds later so network/service state has settled, and subsequent renders on the configured timer and relevant service-control events.

## Safety choices

- `LogonUI.exe` is never terminated or injected into.
- No credential provider is installed and no credential data is inspected.
- The service has no listener, remote API, updater, or outbound network dependency.
- Existing policy values are backed up before the first install and restored only if the active image still belongs to BackgroundChanger.
- The app does not change Windows editions or enable Shared PC / education policies to work around Microsoft licensing boundaries.
- The product-key export used for local VM creation is ignored and never read into build artifacts or logs.

## Known platform boundary

Microsoft supports the Group Policy lock-screen setting on Enterprise, Education, IoT Enterprise, and Server. Personalization CSP has its own edition and management requirements. Validation confirmed that an unmanaged Windows 11 Pro guest accepts both application paths but ignores the lock/sign-in background image. Windows Home is not supported.

Microsoft documents two Pro exceptions: SharedPC `SetEduPolicies` and `BootToCloudPCEnhanced`. `SetEduPolicies` marks the machine as an education environment and changes local policies for advertising ID, Windows tips, and Microsoft consumer experiences. That is outside a background utility's implied scope, so BackgroundChanger does not enable it automatically.

Primary references:

- [Configure desktop and lock-screen backgrounds](https://learn.microsoft.com/windows/configuration/background/)
- [ADMX Control Panel Display policy mapping](https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-controlpaneldisplay)
- [Personalization CSP](https://learn.microsoft.com/windows/client-management/mdm/personalization-csp)
- [Shared PC technical reference](https://learn.microsoft.com/windows/configuration/shared-pc/shared-pc-technical)
- [`MDM_Personalization` WMI class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-personalization)
- [Show clear logon background policy](https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-logon#disableacrylicbackgroundonlogon)
- [Automatically starting Windows services](https://learn.microsoft.com/windows/win32/services/automatically-starting-services)
