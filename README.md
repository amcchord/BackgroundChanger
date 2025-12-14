# BackgroundChanger

A Windows toolkit for customizing your desktop, lock screen, and login screen backgrounds.

## Tools Included

### bgchanger.exe
A command-line tool that sets your desktop wallpaper, lock screen, and login screen background — all at once.

### bgStatusService.exe
A Windows service that displays system information (neofetch-style) on your login screen, so you can identify machines without logging in.

### bgStatusServiceSetup.exe
A GUI installer that downloads and installs the latest bgStatusService from GitHub. No scripts required — just double-click to install or uninstall.

---

## bgchanger

### Features

- **One command, three screens** — Sets desktop, lock screen, and login screen simultaneously
- **Random wallpapers** — Run with no arguments to get a beautiful random wallpaper from [slide.recipes](https://www.slide.recipes/bg/)
- **Local files** — Set any image from your computer
- **Directories** — Pick a random image from a local folder
- **URLs** — Download and set images directly from the web
- **Auto-elevation** — Automatically requests admin privileges when needed
- **Windows 10/11** — Multiple methods for maximum compatibility

### Usage

```
bgchanger [option]
```

| Option | Description |
|--------|-------------|
| *(no args)* | Download a random wallpaper from slide.recipes |
| `<image_path>` | Set a specific image as wallpaper |
| `<directory>` | Pick a random image from a local directory |
| `<url>` | Download and set an image from a URL |
| `help` | Show help message |

### Examples

```powershell
# Get a random wallpaper
bgchanger

# Set a specific image
bgchanger C:\Pictures\wallpaper.jpg

# Random image from a folder
bgchanger C:\Pictures\Wallpapers

# Set from a URL
bgchanger https://example.com/image.png
```

---

## bgStatusService

A Windows service that overlays system information on your login screen background. Perfect for IT environments where you need to identify machines at a glance.

### Information Displayed

**Right Panel (System Info):**
- Computer name / Hostname
- Windows version
- CPU model and core count
- RAM amount
- GPU model
- IP address(es)
- Disk space (used / total)
- Serial number
- System uptime
- Timestamp when the graphic was generated

**Left Panel (Services Status):**
- Running services count
- Critical services status (DHCP, DNS, Windows Update, Defender, etc.)
- Failed services list (auto-start services that aren't running)
- Windows Server support (shows additional server-specific services like AD, IIS, DNS Server, DHCP Server, SQL Server, Hyper-V)

### Features

- **Runs at boot** — Updates the login screen before any user logs in
- **Smart text color** — Automatically chooses white or black text based on background brightness
- **Resolution-aware scaling** — Detects your display resolution and scales text appropriately (readable from 1024x768 to 4K+)
- **Dual panel layout** — Services status on the left, system info on the right
- **Windows Server support** — Automatically detects Server editions and monitors server-specific services
- **Preserves your wallpaper** — Backs up the original image and applies overlay on top
- **Integrates with bgchanger** — When you change wallpaper with bgchanger, the service uses the new image

### Installation (Recommended: GUI Installer)

1. Download `bgStatusServiceSetup.exe` from [Releases](https://github.com/amcchord/BackgroundChanger/releases)

2. Double-click to run — it will request admin privileges automatically

3. Choose "Install" and follow the prompts

4. The service will run automatically on next boot, or start it immediately when prompted

### Installation (PowerShell Scripts)

Alternatively, download both `bgStatusService.exe` and the `install` folder:

```powershell
.\install\install.ps1
```

### Uninstallation

**Using the GUI installer:**
1. Run `bgStatusServiceSetup.exe`
2. Choose "Uninstall"

**Using PowerShell:**
```powershell
.\install\uninstall.ps1
```

The uninstaller will offer to restore your original login screen background.

### Testing Without Installing

Run the executable directly (as Administrator) to test:
```powershell
Start-Process .\bgStatusService.exe -Verb RunAs
```

Then press `Win+L` to see the result.

---

## Supported Image Formats

- JPG / JPEG
- PNG
- BMP

## Notes

- **Admin required** — Both tools require administrator privileges for lock/login screen changes
- **Lock screen** — Press `Win+L` to see changes immediately
- **Login screen** — Sign out or restart to see changes
- **Non-C: drives** — Fully supports Windows installed on any drive

## Building from Source

Requires Go 1.21+

```bash
git clone https://github.com/amcchord/BackgroundChanger.git
cd BackgroundChanger

# Build bgchanger
go build -o bgchanger.exe ./cmd/changer

# Build bgStatusService
go build -o bgStatusService.exe ./cmd/statusservice

# Build bgStatusServiceSetup (GUI installer)
go build -ldflags -H=windowsgui -o bgStatusServiceSetup.exe ./cmd/installer
```

## Project Structure

```
BackgroundChanger/
├── cmd/
│   ├── changer/          # bgchanger source
│   ├── statusservice/    # bgStatusService source
│   └── installer/        # GUI installer source
├── internal/
│   ├── sysinfo/          # System information gathering
│   ├── overlay/          # Image text rendering
│   ├── loginscreen/      # Login screen management
│   └── installer/        # Installer dialogs and service management
├── install/
│   ├── install.ps1       # Service installer (PowerShell)
│   └── uninstall.ps1     # Service uninstaller (PowerShell)
└── assets/
    └── fonts/            # Embedded fonts
```

## Credits

- Random wallpapers provided by [slide.recipes](https://www.slide.recipes/bg/)
- Font: [JetBrains Mono](https://www.jetbrains.com/lp/mono/) (OFL License)

## License

MIT
