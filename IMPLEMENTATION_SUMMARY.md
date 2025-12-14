# Implementation Summary: Windows Login Screen Background Fix

## Changes Made

This document summarizes the changes made to fix the Windows login screen background functionality.

## Problem

The application could change the desktop background but not the login screen background on Windows 10/11. After deep research, several critical issues were identified:

1. Missing 256KB file size limit enforcement for OOBE method
2. Wrong file format handling (not saving as `.jpg` specifically)
3. Missing the most reliable Group Policy registry method
4. OEM Background method only set registry flag without copying files
5. No image compression capability

## Solution Implemented

### 1. Added Image Compression Function

**Function:** `compressImageForOOBE(sourcePath, destPath)`

- Automatically compresses images to under 256KB (targeting 200KB for safety)
- Resizes large images (>1920x1200) while maintaining aspect ratio
- Uses iterative quality reduction to meet size requirements
- Saves as JPEG format required by Windows OOBE method
- Location: Lines 39-119 in `cmd/changer/main.go`

### 2. Implemented Group Policy Registry Method

**Function:** `setLoginScreenBackgroundGroupPolicy(absPath)`

- Sets registry key at `HKLM\SOFTWARE\Policies\Microsoft\Windows\Personalization\LockScreenImage`
- This is the same method used by Windows Group Policy Editor
- Most reliable method according to research
- Also ensures `DisableLogonBackgroundImage` is set to 0 (enabled)
- Location: Lines 517-606 in `cmd/changer/main.go`

### 3. Fixed System32 OOBE Method

**Updates to:** `setLoginScreenBackgroundSystem32(absPath)`

Changes:
- Now uses `compressImageForOOBE()` to ensure file is under 256KB
- Always saves as `backgroundDefault.jpg` (not original extension)
- Enables `OEMBackground` registry flag as required
- Location: Lines 760-818 in `cmd/changer/main.go`

### 4. Fixed OEM Background Method

**Updates to:** `setLoginScreenBackgroundOEM(absPath)`

Changes:
- Now copies compressed image to `C:\Windows\System32\oobe\info\backgrounds\backgroundDefault.jpg`
- Uses proper DWORD values instead of string pointers for registry
- Combines file copying with registry settings
- Location: Lines 608-708 in `cmd/changer/main.go`

### 5. Reordered Methods by Reliability

**Updated:** `setLoginScreenBackground(absPath)`

New order (most to least reliable):
1. Group Policy Registry method (new)
2. System32 OOBE method (fixed)
3. OEM Background method (fixed)
4. LogonUI Registry method (fallback)

Location: Lines 182-222 in `cmd/changer/main.go`

### 6. Enhanced Error Messages and Documentation

- Added clear indication that admin privileges are REQUIRED
- Emphasized that system restart is REQUIRED for login screen changes
- Clarified difference between lock screen (Win+L) and login screen (restart)
- Added note about automatic image compression
- Updated README with comprehensive troubleshooting section

## Technical Details

### Windows Login Screen Methods

#### Method 1: Group Policy Registry (Most Reliable)
```
Registry: HKLM\SOFTWARE\Policies\Microsoft\Windows\Personalization
Key: LockScreenImage (REG_SZ) = full path to image
```

#### Method 2: OEM Background Files (Traditional)
```
File: C:\Windows\System32\oobe\info\backgrounds\backgroundDefault.jpg
Registry: HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Authentication\LogonUI\Background
Key: OEMBackground (DWORD) = 1
Requirement: File must be under 256KB
```

#### Method 3: Disable Default + Set Lock Screen
```
Registry: HKLM\SOFTWARE\Policies\Microsoft\Windows\System
Key: DisableLogonBackgroundImage (DWORD) = 0
Plus: PersonalizationCSP lock screen settings
```

### File Size Limit

Windows has a hard 256KB limit for OOBE background images. The compression function:
- Targets 200KB to leave a safety margin
- Starts at 85% JPEG quality and reduces by 10% increments
- Resizes images if they exceed 1920x1200 pixels
- Uses nearest-neighbor algorithm for speed

### Required Permissions

All login screen methods require:
- Administrator privileges to write to HKLM registry
- Administrator privileges to write to System32 directory
- System restart to see changes

## Files Modified

1. `cmd/changer/main.go` - Main implementation
   - Added imports: `bytes`, `image`, `image/jpeg`
   - Added compression function
   - Added Group Policy method
   - Fixed OOBE and OEM methods
   - Reordered method execution
   - Enhanced error messages

2. `README.md` - Documentation
   - Updated features list
   - Enhanced important notes
   - Comprehensive troubleshooting section

3. `bgchanger.exe` - Rebuilt binary with all changes

## Testing Recommendations

To test the implementation:

1. Run as administrator (required)
2. Test with various image sizes (small, medium, large)
3. Test with different formats (JPG, PNG, BMP)
4. Verify compression output is under 256KB
5. Check that files are created in System32\oobe\info\backgrounds
6. Verify registry keys are set correctly
7. Restart computer and check login screen

## Known Limitations

- Some Windows Home editions may have limited customization
- Antivirus software may block System32 writes
- Very small images may not look good after compression
- Requires system restart (Windows limitation, not app limitation)

## Success Indicators

The fix is successful if:
- Application runs without errors when run as administrator
- Compressed image is created in System32\oobe\info\backgrounds
- Registry keys are set correctly
- Login screen shows new background after restart

## References

Based on research of:
- Windows Group Policy Editor internal methods
- Microsoft PersonalizationCSP documentation
- Windows OOBE background requirements
- Community reports of working methods on Windows 10/11

