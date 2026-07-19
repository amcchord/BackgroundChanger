# Wallpaper Identity v4.0.2

Windows 11 Pro can keep showing a previous verified lock-screen bitmap even after boot generates a current image and Personalization CSP reports status `1`. v4.0.2 closes that gap without editing Windows' protected cache or terminating `LogonUI.exe`.

## Highlights

- Waits for the existing boot-settled render so DHCP has time to replace an address retained from the previous network.
- After Windows accepts that exact image, calls the documented `WTSDisconnectSession` operation only for an empty physical-console login session.
- Requires every WTS session to have no authenticated user, the console to be connected, exactly one `LogonUI` and `winlogon`, no Explorer, and an unchanged session after a 500 ms race check.
- Writes a stable `Win32_OperatingSystem.LastBootUpTime` fence before the operation, preventing service restarts, repairs, upgrades, or crashes from rotating the console twice in one boot.
- Adds `refresh_login_screen_on_boot: true` to `config.yml`; power users can opt out and retain Windows' original cache behavior.
- Records attempted, refreshed, session, and skip details in `status.json`, with the durable once-per-boot outcome in `pre-login-refresh.json`.

## Windows 11 Pro validation

The final v4.0.2 binary was installed in the activated Windows 11 Pro 25H2 VirtualBox guest. Its adapter changed from `10.77.0.3` to `10.0.2.15` while powered off. The old address was still visible 30 seconds after power-on. W:ID's boot-settled render captured `10.0.2.15`, verified CSP status `1`, requested the empty-console refresh, and Windows recreated session 1 as session 2. By 55 seconds the actual pre-login screen showed `10.0.2.15` and the matching generation time.

The screenshot attached to this release and embedded in the README is that real post-transition frame. A same-boot headless repair was also verified to skip the rotation, while a CSP status `2` test failed closed without touching the session.

![Windows 11 Pro pre-login screen after the powered-off IP change](https://raw.githubusercontent.com/amcchord/WallpaperIdentity/v4.0.2/assets/screenshots/prelogin.png)

![Wallpaper Identity graphical installer](https://raw.githubusercontent.com/amcchord/WallpaperIdentity/v4.0.2/assets/screenshots/installer.png)

## Upgrade

Run the graphical setup again, or use the blocking RMM path:

```powershell
& .\WallpaperIdentityCLI.exe upgrade --json
```

Existing `config.yml`, generated images, logs, and rollback records are preserved. Existing configurations that do not contain the new key receive the guarded `true` default; set `refresh_login_screen_on_boot: false` to opt out.
