# BackgroundChanger

A Windows toolkit for customizing your desktop, lock screen, and login screen backgrounds.

## Tools Included

### bgchanger.exe
A command-line tool that sets your desktop wallpaper, lock screen, and login screen background — all at once.

### bgStatusService.exe
A Windows service that displays system information (neofetch-style) on your login screen, so you can identify machines without logging in.

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

- Computer name / Hostname
- Windows version
- CPU model and core count
- RAM amount
- GPU model
- IP address(es)
- Disk space (used / total)
- Serial number

### Features

- **Runs at boot** — Updates the login screen before any user logs in
- **Smart text color** — Automatically chooses white or black text based on background brightness
- **Preserves your wallpaper** — Backs up the original image and applies overlay on top
- **Integrates with bgchanger** — When you change wallpaper with bgchanger, the service uses the new image

### Installation

1. Download both `bgStatusService.exe` and the `install` folder from [Releases](https://github.com/amcchord/BackgroundChanger/releases)

2. Run the installer (will request admin privileges automatically):
```powershell
.\install\install.ps1
```

3. The service will run automatically on next boot, or start it immediately when prompted.

### Uninstallation

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
```

## Project Structure

```
BackgroundChanger/
├── cmd/
│   ├── changer/          # bgchanger source
│   └── statusservice/    # bgStatusService source
├── internal/
│   ├── sysinfo/          # System information gathering
│   ├── overlay/          # Image text rendering
│   └── loginscreen/      # Login screen management
├── install/
│   ├── install.ps1       # Service installer
│   └── uninstall.ps1     # Service uninstaller
└── assets/
    └── fonts/            # Embedded fonts
```

## Credits

- Random wallpapers provided by [slide.recipes](https://www.slide.recipes/bg/)
- Font: [JetBrains Mono](https://www.jetbrains.com/lp/mono/) (OFL License)

## License

MIT
