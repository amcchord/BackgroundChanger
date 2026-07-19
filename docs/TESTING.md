# Test plan and release evidence

## Automated checks

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go)
go test ./...
go test -race ./internal/...
go vet ./...
git diff --check
powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version v3.0.0
```

Tests cover YAML configuration validation/migration/round-tripping, the three distinct presets, the permanent generated timestamp, edition classification, path ownership, PowerShell command encoding, snapshot formatting helpers, and renderer dimensions.

## VirtualBox release matrix

Validation completed July 18, 2026 with VirtualBox 7.2.4. Each clean guest used EFI, Secure Boot where supported, four vCPUs, 8 GiB RAM, a dynamically allocated 64 GiB system disk, and Guest Additions 7.2.4.

| Guest | OS build | Result |
|---|---|---|
| Windows 11 Enterprise Evaluation | 25H2, 26200.6584 | Pass: install, LocalSystem service, Group Policy, MDM bridge, boot-time refresh before any user session, lock screen, clear credential screen, repair/upgrade |
| Windows 10 Enterprise Evaluation | 22H2, 19045.2006 | Pass: install, LocalSystem service, Group Policy, MDM bridge, lock screen, Windows 10 clock-safe layout |
| Windows 11 Pro | 25H2, 26200.6584 | Expected unsupported result: service/render and both policy writes succeed, but Windows retains the stock lock-screen image |

The Windows 11 Enterprise guest had unattended automatic logon disabled before the final reboot. Without signing in, `status.json` reported `reason: "boot-settled"`, a fresh timestamp, 8 GiB of detected memory, and successful policy application. The screenshot in `assets/screenshots/prelogin.png` was captured at that point.

The Pro guest also validated the compact setup UI at 1024×768, all three live preview examples, and an Identity-preset installation. The resulting `config.yml` and rendered JPEG contained only the selected optional fields while still showing hostname and **Generated at**.

Uninstall was exercised on the Pro guest. The service, installed executable, LocalSystem MDM backup, machine lock-screen values, and clear-logon value were removed or restored; configuration and generated images remained as documented.

## Media provenance

No product key was required for these tests, and no key export was read. The Enterprise guests used Microsoft evaluation media; the Pro guest used Microsoft's public multi-edition ISO.

| Media | SHA-256 |
|---|---|
| Windows 11 Enterprise Evaluation 25H2 | `a61adeab895ef5a4db436e0a7011c92a2ff17bb0357f58b13bbc4062e535e7b9` |
| Windows 10 Enterprise Evaluation 22H2 | `ef7312733a9f5d7d51cfa04ac497671995674ca5e1058d5164d6028f0938d668` |
| Windows 11 multi-edition 25H2 | `d141f6030fed50f75e2b03e1eb2e53646c4b21e5386047cb860af5223f102a32` |

## Manual acceptance checklist

1. Start from a clean supported Enterprise, Education, IoT Enterprise, or Server guest.
2. Copy `BackgroundChangerSetup.exe` into the guest and open it.
3. Confirm the uninstalled state, inspect all three screenshot previews, choose a preset, select **Install**, and approve elevation.
4. Confirm service `BackgroundChanger` is `Running`, `Automatic`, and uses `LocalSystem`.
5. Confirm `status.json` reports `success: true`, a supported edition, a versioned JPEG, and successful Group Policy/MDM application.
6. Disable any test-only automatic logon, reboot, and do not sign in.
7. Confirm a second `boot-settled` image appears with current network, memory, service, and refresh data, plus a current timezone-qualified **Generated at** value.
8. Advance to credentials and confirm the background remains visible without acrylic blur.
9. Sign in, run repair/upgrade, and confirm the installed service version changes.
10. Run uninstall and confirm the service and Apps & features entry are gone and prior policy values are restored.

Do not place activation keys, passwords, unattended-answer files, local ISO paths, or machine-specific logs in screenshots or commits.
