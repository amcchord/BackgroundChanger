# Wallpaper Identity architecture

## Design goals

Wallpaper Identity (W:ID) must produce useful output before the first interactive sign-in, refresh without relying on a user's profile, avoid unsupported LogonUI manipulation, and remain easy to deploy to disconnected machines.

## Process model

Two hash-verifiable release artifacts share the same application, setup, service, renderer, and test code but use different Windows subsystems:

| Artifact / mode | Entry point | Purpose |
|---|---|---|
| Graphical Setup | `WallpaperIdentitySetup.exe` with no arguments | Install, migrate, repair/upgrade, and uninstall |
| Console CLI | `WallpaperIdentityCLI.exe COMMAND` | Blocking, headless RMM deployment, health, refresh, render, and removal |
| Windows service | installed runtime with `service` | Boot/session/timer refresh loop under LocalSystem |

Either installer artifact copies itself to `C:\Program Files\Wallpaper Identity\WallpaperIdentity.exe` and registers that copy as the automatic `WallpaperIdentity` service. The graphical build opens Setup by default; the console build prints help and runs headlessly unless the Apps & features registration supplies the explicit interactive maintenance flag. Keeping each artifact self-contained eliminates network downloads, embedded executable drift, and installer/service version mismatches.

## Version 3 migration

The v4 installer treats the previous product identity as an upgrade source, not a parallel installation:

1. Detect and stop the previous service before moving data.
2. Move or merge configuration, generated backgrounds, diagnostic status, logs, and policy/MDM rollback files into `C:\ProgramData\Wallpaper Identity`.
3. Keep the tuned v3 configuration and original rollback files authoritative. If an interrupted attempt already created conflicting W:ID files, preserve those copies beside the migrated files with `.pre-v4` or `.from-v3` suffixes.
4. Rewrite the YAML header with the Wallpaper Identity brand without changing power-user values.
5. Remove the previous service, Apps & features key, and install directory before registering W:ID.
6. Continue using the original policy backup, so a later uninstall restores the state that existed before either product version was installed.

The old identifiers remain only as explicit compatibility constants and ownership checks; no user-facing screen, active service, executable, directory, or registry entry uses them after a successful upgrade.

## Refresh sequence

1. Load or create `C:\ProgramData\Wallpaper Identity\config.yml`, resolving the selected named preset or custom field visibility. An explicit `base_image` wins; otherwise discover replaceable `background.jpg` or `background.png` beside the configuration.
2. Gather machine facts from Windows registry, WMI, network interfaces, and system counters.
3. Render a new resolution-aware JPEG with the W:ID logo, a unique timestamp in its filename, and a non-configurable, timezone-qualified **Generated at** label. Credible detected dimensions are used directly. Otherwise the reported aspect selects a 4:3, 16:10, 16:9, ultrawide, or portrait fallback. Standard/wide displays retain separate side panels; tall displays stack them. Every variable value is measured against its actual pixel width, reduced to a bounded readable font, and finally elided if necessary; vertical row metrics are fitted before drawing.
4. Set `HKLM\Software\Policies\Microsoft\Windows\Personalization\LockScreenImage` and the related no-change/no-overlay values.
5. Enable `HKLM\Software\Policies\Microsoft\Windows\System\DisableAcrylicBackgroundOnLogon`, the registry mapping for Microsoft's **Show clear logon background** policy.
6. On Windows Pro, back up and enable only the `MDM_SharedPC.SetEduPolicies` compatibility switch required by Personalization CSP.
7. Copy the JPEG to the space-free `C:\ProgramData\WallpaperIdentityCSP` delivery cache and apply the documented `MDM_Personalization` WMI bridge. This class is explicitly partitioned for LocalSystem.
8. Read back the exact URL and wait for Personalization CSP status `1`; on Pro, both that proof and verified `SetEduPolicies` are mandatory.
9. On the boot-settled refresh only, query Windows sessions and processes. If every session has an empty user name and the physical console is connected with exactly one `LogonUI`, one `winlogon`, and no Explorer, write a stable `Win32_OperatingSystem.LastBootUpTime` fence, recheck after 500 ms, then call `WTSDisconnectSession` asynchronously for that empty console.
10. Windows recreates the unauthenticated console under a new session ID, regenerating its protected lock-screen bitmap cache from the already-verified current CSP image. A same-boot service restart, repair, or upgrade observes the fence and cannot repeat the transition.
11. If `LockApp.exe` is present for a later user-owned lock screen, terminate only that presentation process so Windows recreates it using the new versioned path.
12. Atomically write `status.json`, retain the newest four backgrounds, and append a diagnostic log entry.

The service reports `Running` promptly, then performs its initial render on one serialized worker. This avoids exhausting the Service Control Manager start window when the CSP provider is slow. A second render follows 20 seconds later so network and service state can settle; timer, session, power, and RMM requests use that same worker. Stop and shutdown allow the worker up to 30 seconds to drain, then release Windows so policy-provider latency cannot indefinitely block shutdown.

## Safety choices

- `LogonUI.exe` is never terminated or injected into.
- `WTSDisconnectSession` is called only for an unauthenticated physical-console session; any non-empty WTS user, Explorer process, missing login process, session change, or explicit configuration opt-out causes a skip.
- The console transition is fenced once per stable Windows boot identity before the API call, so a service failure or repair cannot create a refresh loop.
- No credential provider is installed and no credential data is inspected.
- The service has no listener, remote API, updater, or outbound network dependency.
- Hostname and the generated timestamp cannot be disabled.
- Existing policy values are backed up before the first install and restored only if the active image is still owned by W:ID or the migration source.
- On Pro, the app enables only `SetEduPolicies`; it never enables Shared PC mode, account management/cleanup, power policy, kiosk mode, or storage restrictions. The prior state is serialized and restored during uninstall.
- Product-key exports used for local VM creation are ignored and never read into build artifacts or logs.

## Known platform boundary

Microsoft supports the Group Policy lock-screen setting on Enterprise, Education, IoT Enterprise, and Server. Personalization CSP has separate edition requirements. Microsoft documents `SetEduPolicies` as a Pro compatibility path; W:ID enables that single setting by default and clearly surfaces its three user-policy effects: advertising ID, Windows tips, and Microsoft consumer experiences are disabled. Windows Home remains unsupported.

Windows can retain the previous verified bitmap in `LogonUI`'s protected cache for the first pre-login frame, and the first service render can precede DHCP lease replacement. W:ID does not claim that early frame as current. It waits for the boot-settled render, verifies the exact CSP image, then uses the documented WTS disconnect operation to recreate only the empty console. In the Windows 11 Pro validation guest, the old `10.77.0.3` frame was replaced by the new `10.0.2.15` frame about 55 seconds after power-on, including a roughly five-second Windows-owned transition. If a user is already authenticated or the feature is disabled, W:ID leaves the session untouched and the mandatory generated timestamp exposes the displayed image's age.

Primary references:

- [Configure desktop and lock-screen backgrounds](https://learn.microsoft.com/windows/configuration/background/)
- [ADMX Control Panel Display policy mapping](https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-controlpaneldisplay)
- [Personalization CSP](https://learn.microsoft.com/windows/client-management/mdm/personalization-csp)
- [Shared PC technical reference](https://learn.microsoft.com/windows/configuration/shared-pc/shared-pc-technical)
- [WTSDisconnectSession function](https://learn.microsoft.com/windows/win32/api/wtsapi32/nf-wtsapi32-wtsdisconnectsession)
- [WTS session information classes](https://learn.microsoft.com/windows/win32/api/wtsapi32/ne-wtsapi32-wts_info_class)
- [Win32_OperatingSystem class](https://learn.microsoft.com/windows/win32/cimwin32prov/win32-operatingsystem)
- [`MDM_SharedPC` WMI class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-sharedpc)
- [`MDM_Personalization` WMI class](https://learn.microsoft.com/windows/win32/dmwmibridgeprov/mdm-personalization)
- [Show clear logon background policy](https://learn.microsoft.com/windows/client-management/mdm/policy-csp-admx-logon#disableacrylicbackgroundonlogon)
- [Automatically starting Windows services](https://learn.microsoft.com/windows/win32/services/automatically-starting-services)
