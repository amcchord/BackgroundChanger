# Test plan and release evidence

## Automated checks

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go)
go test ./...
go test -race ./internal/...
go vet ./...
git diff --check
powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version v4.7.0
```

Tests cover YAML validation, migration, and round-tripping; all twelve background variants; standard-image discovery and JPEG/PNG replacement; measured truncation; aspect-aware fallbacks; bounded panel and row metrics across 800×600, 1024×768, 1280×1024, 1366×768, 1920×1080, 2560×1080, 3440×1440, and 1080×1920; the guarded boot-refresh default and opt-out; stable Windows boot identity; authenticated-session, console-change, process, API-failure, and once-per-boot paths; v3-to-W:ID data migration; the three presets; installer preset/background-selection and final Done-state copy; the permanent generated timestamp; edition classification; policy ownership; space-free CSP image staging; PowerShell command encoding; service queue/drain behavior; snapshot formatting; and renderer dimensions.

## Responsive backgrounds and installer validation for v4.7.0

- Rendered deliberately long hostname, OS, CPU, GPU, address, serial, failure, and service values at standard, 4:3, 5:4, ultrawide, and portrait dimensions; measured fitting kept each value and row inside its assigned bounds.
- Rendered and decoded all six color families in both Dark and Light modes.
- Opened the built native installer at 150% Windows scaling, navigated both pages, selected Dark/Light and multiple color families, and confirmed the live preview, swatches, selection copy, Back/Close/Repair ordering, and lower-right advancing action.
- Opened the native JPEG/PNG browser from the drop well and confirmed the same control is registered as a file-drop target.
- Verified standard-path discovery, PNG-to-JPEG replacement, obsolete-extension cleanup, and explicit-path precedence in automated tests.

## Installer UX validation for v4.5.0

The final installer was inspected on the Windows host at 150% display scaling in clean-install and installed-maintenance modes. DPI-aware preview bitmaps filled their native selection wells in both modes. The following states were captured from the built Windows executable and committed as release evidence:

- Clean install with Balanced selected and the default **Install** action in the lower right
- Full-card click selection of Operations with visible selection text and check mark
- Installed maintenance with the current layout retained until an explicit replacement is chosen
- Active blocking progress with controls disabled
- Successful completion at 100%, headed exactly **Done**, with **Close** focused in the lower right

The image scaling helper is also unit tested at 96, 120, 144, and 192 DPI.

## VirtualBox release matrix

Validation was updated July 19, 2026 with VirtualBox 7.2.4. Guests use EFI, four vCPUs, 8 GiB RAM, a dynamically allocated 64 GiB system disk, and Guest Additions 7.2.4.

| Guest | OS build | Evidence |
|---|---|---|
| Activated Windows 11 Pro | 25H2, 26200.6584 | v4.0.2 pass: v4.0.1 lifecycle plus a powered-off `10.77.0.3` to `10.0.2.15` change, boot-settled CSP status `1`, guarded empty-console recreation, current visible frame, and same-boot repair suppression |
| Windows 11 Enterprise Evaluation | 25H2, 26200.6584 | v4 baseline pass: in-place v3 migration, automatic LocalSystem service, configuration migration, Group Policy, and MDM bridge; later Windows servicing prevented a final v4.0.1 guest cycle |
| Windows 10 Enterprise Evaluation | 22H2, 19045.2006 | v3 baseline pass: LocalSystem service, Group Policy, MDM bridge, lock screen, and Windows 10 clock-safe layout; Windows servicing prevented the final v4 upgrade cycle |

The release-candidate Pro lifecycle used the actual console artifact. A clean unattended `install --headless --preset balanced`, `repair`, `status --json`, and `uninstall --headless --remove-data` all blocked until completion and returned matching process/JSON success. The service ran as automatic LocalSystem. The uninstall audit confirmed that the service, Apps & features entry, product data, and space-free CSP cache were absent; W:ID's Group Policy values were absent; `LockScreenImageUrl` was empty; and `SetEduPolicies` had returned to its previous `false` value.

An older migration backup exposed an additional rollback edge case: the Personalization singleton existed before W:ID but its lock-screen URL was empty. Restoring an empty string and deleting the singleton are both rejected by the Windows 11 provider. The final implementation uses the provider's accepted null leaf update, which clears only `LockScreenImageUrl` and preserves a possible unrelated `DesktopImageUrl`; the same failing state then passed uninstall and the LocalSystem audit.

## Boot cache investigation and v4.0.2 freshness result

The v4.0.1 Pro experiment established the failure boundary. Windows materialized the exact current CSP image, but `C:\ProgramData\Microsoft\Windows\SystemData\S-1-5-18\ReadOnly\LockScreen_P` and the visible pre-login surface retained the previous verified image. Reapplying the CSP, using the documented WinRT lock-screen setter as LocalSystem, changing display size, and restarting DWM in the same session did not reload it. No test terminated `LogonUI.exe`.

Calling Microsoft's `WTSDisconnectSession` for the connected console while WTS reported no user, `LogonUI` and `winlogon` were present, and Explorer was absent succeeded. Windows moved the console from session 1 to session 2, recreated `LogonUI` and DWM, regenerated every `LockScreen_P` derivative at the same timestamp, and displayed the current CSP image in about five seconds. This uses a documented session operation and does not edit the protected cache.

The finished v4.0.2 binary was then tested end to end:

- The guest was shut down normally on a VirtualBox NAT Network with `10.77.0.3`, changed while powered off to standard NAT, and started without an interactive sign-in.
- At 30 seconds the visible surface still showed `10.77.0.3`, proving the old-cache condition remained reproducible.
- The first service render began before DHCP had fully replaced its lease. W:ID deliberately waited for its queued boot-settled render, which recorded `10.0.2.15`, generation time 10:50:12 EDT, Group Policy readback, verified `SetEduPolicies`, and Personalization CSP status `1`.
- The guarded WTS result recorded `attempted: true`, `refreshed: true`, and session 1. Windows recreated the empty console as session 2; by 55 seconds the visible screen showed `10.0.2.15` and the same 10:50:12 generation time.
- A same-boot headless repair retained the configuration and returned success. The stable `LastBootUpTime` fence produced `attempted: false` with `skipped_reason: "already evaluated this boot"`, and the console session did not rotate again.
- A deliberately isolated network left the Pro CSP at status `2`; W:ID failed closed, reported the exact error, and did not rotate the console.

The committed README screenshot is the real post-transition Windows 11 Pro frame from this final powered-off IP-change test.

## Media and secret handling

Enterprise guests used Microsoft evaluation media and the Pro guest used Microsoft's multi-edition ISO. Activation material was supplied locally from an ignored developer export. No key, password, unattended-answer file, VM log, or machine-specific secret is part of the repository or release assets.

| Media | SHA-256 |
|---|---|
| Windows 11 Enterprise Evaluation 25H2 | `a61adeab895ef5a4db436e0a7011c92a2ff17bb0357f58b13bbc4062e535e7b9` |
| Windows 10 Enterprise Evaluation 22H2 | `ef7312733a9f5d7d51cfa04ac497671995674ca5e1058d5164d6028f0938d668` |
| Windows 11 multi-edition 25H2 | `d141f6030fed50f75e2b03e1eb2e53646c4b21e5386047cb860af5223f102a32` |

## Manual acceptance checklist

1. Start from a clean supported guest and, for Pro, ensure Windows is activated.
2. Inspect all three installer previews, choose a preset, install, and approve elevation.
3. Confirm `WallpaperIdentity` is automatic, running as LocalSystem, and `status.json` reports a successful edition-appropriate policy proof.
4. Confirm the rendered JPEG includes the hostname and a timezone-qualified **Generated at** value.
5. Reboot without an interactive sign-in and confirm a new `service-start` or `boot-settled` record contains current machine/network state.
6. On an empty console with `refresh_login_screen_on_boot: true`, allow the brief Windows-owned transition and confirm the visible address and generated timestamp match the boot-settled status. Confirm `pre-login-refresh.json` records the boot identity and outcome.
7. Advance to credentials and confirm the configured background remains readable without acrylic blur.
8. Restart or repair the service in the same boot and confirm the console does not rotate again.
9. Run elevated headless repair, status, refresh, retained-data uninstall, reinstall, and purge uninstall paths.
10. Confirm uninstall restores only W:ID-owned policy state and preserves unrelated policy values.

Do not place activation keys, passwords, unattended-answer files, local ISO paths, or machine-specific logs in screenshots or commits.
