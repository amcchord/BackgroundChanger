# Wallpaper Identity v4.0.1

Version 4.0.1 is a lock-screen reliability release. It adds the documented Windows Pro policy path, replaces false-positive health with exact Windows readback, hardens service shutdown and unattended deployment, and clearly distinguishes a current boot-generated file from the bitmap Windows may already have cached for `LogonUI`.

## Windows Pro compatibility

- Enables Microsoft's documented `SetEduPolicies` compatibility switch on Windows 10/11 Pro by default
- Does not enable Shared PC mode, account cleanup, power policies, kiosk mode, or storage restrictions
- Clearly documents the three effects of `SetEduPolicies`: Windows tips, advertising ID, and Microsoft consumer experiences are disabled
- Adds `enable_pro_compatibility` to `config.yml` for explicit power-user opt-out
- Backs up the previous `SetEduPolicies` state and positively verifies its restoration during uninstall

## No more false-positive health

- Reads back every Group Policy value after writing it
- Waits for `MDM_Personalization` to report status `1` for the exact generated image URL
- Stages CSP delivery images in a space-free path because the Pro provider did not complete file URLs containing spaces
- Requires both verified `SetEduPolicies` and verified Personalization CSP state on Pro
- Reports a failed refresh and unhealthy CLI status when Windows has not accepted an effective policy
- Normalizes stale rollback records owned by version 3 or an earlier W:ID installation instead of restoring a deleted image path

## Service and deployment robustness

- Reports the Windows service as running before starting potentially slow CSP work, avoiding Service Control Manager startup timeouts
- Keeps the initial render serialized ahead of boot-settled, session, timer, and RMM refreshes
- Gives an in-flight worker a bounded 30-second drain on stop/shutdown so CSP latency cannot indefinitely block Windows shutdown
- Extends bounded setup and uninstall waits for slow policy providers
- Verifies LocalSystem device-policy rollback before uninstall removes recovery files
- Correctly restores an absent or empty prior CSP lock-screen value without deleting the Personalization singleton or an unrelated desktop-image setting

## Windows-managed display cache

- The automatic service does generate during boot and captures an IP address that changed while the machine was powered off
- Every layout permanently includes a timezone-qualified **Generated at** value
- Windows 11 Pro can still display its previous verified bitmap on the first pre-login frame even after the new file and CSP status are current
- W:ID does not add an ineffective boot delay, replace a credential provider, or terminate security-sensitive `LogonUI` to bypass that Windows behavior

The release remains self-contained and offline. Use `WallpaperIdentitySetup.exe` for the graphical installer or `WallpaperIdentityCLI.exe` for blocking, headless RMM deployment.
