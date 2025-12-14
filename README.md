# bgchanger

A fast Windows command-line tool that sets your desktop wallpaper, lock screen, and login screen background — all at once.

## Features

- **One command, three screens** — Sets desktop, lock screen, and login screen simultaneously
- **Random wallpapers** — Run with no arguments to get a beautiful random wallpaper from [slide.recipes](https://www.slide.recipes/bg/)
- **Local files** — Set any image from your computer
- **Directories** — Pick a random image from a local folder
- **URLs** — Download and set images directly from the web
- **Auto-elevation** — Automatically requests admin privileges when needed
- **Windows 10/11** — Multiple methods for maximum compatibility

## Download

Download the latest `bgchanger.exe` from the [Releases](https://github.com/amcchord/BackgroundChanger/releases) page.

## Usage

```
bgchanger [option]
```

### Options

| Option | Description |
|--------|-------------|
| *(no args)* | Download a random wallpaper from slide.recipes |
| `<image_path>` | Set a specific image as wallpaper |
| `<directory>` | Pick a random image from a local directory |
| `<url>` | Download and set an image from a URL |
| `help` | Show help message |

### Examples

**Get a random wallpaper:**
```
bgchanger
```

**Set a specific image:**
```
bgchanger C:\Pictures\wallpaper.jpg
```

**Random image from a folder:**
```
bgchanger C:\Pictures\Wallpapers
```

**Set from a URL:**
```
bgchanger https://example.com/image.png
```

## Supported Formats

- JPG / JPEG
- PNG
- BMP

## How It Works

When you run bgchanger, it:

1. Downloads or locates the image
2. Requests administrator privileges (required for lock/login screen)
3. Sets the desktop wallpaper via Windows API
4. Sets the lock screen using multiple methods for compatibility
5. Sets the login screen via Windows Runtime API and Group Policy

## Notes

- **Admin required** — The app will automatically request elevation via UAC
- **Lock screen** — Press `Win+L` to see changes immediately
- **Login screen** — Sign out or restart to see changes
- JPG format typically works best for compatibility

## Building from Source

Requires Go 1.16+

```bash
git clone https://github.com/amcchord/BackgroundChanger.git
cd BackgroundChanger
go build -o bgchanger.exe ./cmd/changer
```

## Credits

Random wallpapers provided by [slide.recipes](https://www.slide.recipes/bg/)

## License

MIT
