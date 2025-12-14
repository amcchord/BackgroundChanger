# Windows Background Changer

A Go command-line application to set Windows wallpaper and lock screen backgrounds.

## Features

- Set any image as your desktop wallpaper, lock screen background, and login screen
- Choose a random image from a directory
- Supports common image formats: JPG, JPEG, PNG, BMP
- Automatic image compression to meet Windows 256KB size requirement for login screens
- Multiple methods for maximum compatibility across Windows 10 and 11

## Installation

1. Make sure you have Go installed (version 1.16+)
2. Clone this repository
3. Build the application:

```
go build -o bgchanger.exe ./cmd/changer
```

## Usage

```
bgchanger.exe <path-to-image-or-directory>
```

### Examples:

Set a specific image:
```
bgchanger.exe C:\Users\Pictures\wallpaper.jpg
```

Set a random image from a directory:
```
bgchanger.exe C:\Users\Pictures\Wallpapers
```

### Important Notes:

- **You MUST run the application as administrator** for login screen and lock screen changes to work
- After setting a new lock screen background, lock your screen (Win+L) to see the changes
- **For login screen changes, you MUST restart your computer** to see the new background
- Images are automatically compressed to meet Windows 256KB size limit for login screens
- Lock screen (Win+L) and login screen (restart/sign out) are different - this tool changes both

## Requirements

- Windows 10/11
- Go 1.16 or newer

## Troubleshooting

### Lock Screen Not Changing:

1. Run the application as administrator (required)
2. Lock your screen (Win+L) to see changes
3. Some systems may require signing out and back in

### Login Screen Not Changing:

1. **Run the application as administrator** (absolutely required)
2. **Restart your computer** (required, not optional)
3. The login screen appears before you log in, not when you lock (Win+L)
4. If still not working after restart:
   - Check Windows Event Viewer for permission errors
   - Temporarily disable antivirus software
   - Ensure you're using a supported Windows edition (some Home editions have limitations)
   - Try a different image file

### General Issues:

- Make sure your Windows settings allow personalization changes
- Ensure the image file is not corrupted
- Images are automatically compressed, but very small images may not look good
- JPG format typically works best for compatibility 