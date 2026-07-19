# Test plan and release evidence

## Automated checks

```powershell
gofmt -w (Get-ChildItem -Recurse -Filter *.go)
go test ./...
go test -race ./internal/...
go vet ./...
git diff --check
powershell -ExecutionPolicy Bypass -File .\build.ps1 -Version v4.0.1
```

Tests cover YAML validation, migration, and round-tripping; v3-to-W:ID data migration; the three presets; the permanent generated timestamp; edition classification; policy ownership; space-free CSP image staging; PowerShell command encoding; service queue/drain behavior; snapshot formatting; and renderer dimensions.

## VirtualBox release matrix

Validation was updated July 19, 2026 with VirtualBox 7.2.4. Guests use EFI, four vCPUs, 8 GiB RAM, a dynamically allocated 64 GiB system disk, and Guest Additions 7.2.4.

| Guest | OS build | Evidence |
|---|---|---|
| Activated Windows 11 Pro | 25H2, 26200.6584 | v4.0.1 candidate pass: graphical previews, headless install/repair/status, exact Group Policy readback, `SetEduPolicies`, Personalization CSP status `1`, boot generation after a powered-off IP change, bounded shutdown, headless uninstall, and audited rollback |
| Windows 11 Enterprise Evaluation | 25H2, 26200.6584 | v4 baseline pass: in-place v3 migration, automatic LocalSystem service, configuration migration, Group Policy, and MDM bridge; later Windows servicing prevented a final v4.0.1 guest cycle |
| Windows 10 Enterprise Evaluation | 22H2, 19045.2006 | v3 baseline pass: LocalSystem service, Group Policy, MDM bridge, lock screen, and Windows 10 clock-safe layout; Windows servicing prevented the final v4 upgrade cycle |

The release-candidate Pro lifecycle used the actual console artifact. A clean unattended `install --headless --preset balanced`, `repair`, `status --json`, and `uninstall --headless --remove-data` all blocked until completion and returned matching process/JSON success. The service ran as automatic LocalSystem. The uninstall audit confirmed that the service, Apps & features entry, product data, and space-free CSP cache were absent; W:ID's Group Policy values were absent; `LockScreenImageUrl` was empty; and `SetEduPolicies` had returned to its previous `false` value.

An older migration backup exposed an additional rollback edge case: the Personalization singleton existed before W:ID but its lock-screen URL was empty. Restoring an empty string and deleting the singleton are both rejected by the Windows 11 provider. The final implementation uses the provider's accepted null leaf update, which clears only `LockScreenImageUrl` and preserves a possible unrelated `DesktopImageUrl`; the same failing state then passed uninstall and the LocalSystem audit.

## Boot freshness experiment

The Pro VM was first booted on VirtualBox NAT, where W:ID recorded `10.0.2.15`. It was shut down normally, its adapter was changed while powered off to an isolated DHCP network, and it was started without an interactive sign-in.

- Windows shutdown completed in 7.0 seconds.
- The next `reason: "service-start"` status recorded `192.168.77.3`, a new generated timestamp, successful Group Policy readback, verified `SetEduPolicies`, and Personalization CSP status `1` for the new file.
- A screenshot of the simultaneously visible pre-login surface still contained `10.0.2.15` and the prior **Generated at** value.
- A synchronous machine-startup script and foreground-policy experiment also produced the current file before releasing startup, but did not change the first visible bitmap. That experiment was removed from the product because it only delayed boot.

This proves the service captures an address that changed while the machine was off, while also establishing the Windows boundary: a healthy W:ID status cannot guarantee that an already-created `LogonUI` surface has discarded its previous bitmap cache. W:ID never terminated `LogonUI` during testing.

The repository screenshots show the graphical installer and the real W:ID renderer. They are product examples, not a claim that every first LogonUI frame is cache-free.

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
6. Compare the visible generated timestamp with `status.json`; record a previous visible bitmap as a Windows cache observation, not as successful current-frame proof.
7. Advance to credentials and confirm the configured background remains readable without acrylic blur.
8. Run elevated headless repair, status, refresh, retained-data uninstall, reinstall, and purge uninstall paths.
9. Confirm uninstall restores only W:ID-owned policy state and preserves unrelated policy values.

Do not place activation keys, passwords, unattended-answer files, local ISO paths, or machine-specific logs in screenshots or commits.
