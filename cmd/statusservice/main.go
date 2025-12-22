// Package main implements a Windows service that displays system information
// on the login screen background.
package main

import (
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"

	"github.com/backgroundchanger/internal/loginscreen"
	"github.com/backgroundchanger/internal/overlay"
	"github.com/backgroundchanger/internal/sysinfo"
)

const serviceName = "BgStatusService"

// bgStatusService implements the Windows service interface.
type bgStatusService struct {
	elog debug.Log
}

// Execute is the main entry point for the Windows service.
func (s *bgStatusService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}
	s.elog.Info(1, "Service starting...")

	// Run the main task
	err := runStatusUpdate(s.elog)
	if err != nil {
		s.elog.Error(1, fmt.Sprintf("Failed to update login screen: %v", err))
	} else {
		s.elog.Info(1, "Successfully updated login screen with system info")
	}

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Wait for stop signal
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s.elog.Info(1, "Service stopping...")
				break loop
			default:
				s.elog.Error(1, fmt.Sprintf("Unexpected control request #%d", c))
			}
		}
	}

	changes <- svc.Status{State: svc.StopPending}
	return
}

// runStatusUpdate performs the main task of updating the login screen.
func runStatusUpdate(elog debug.Log) error {
	elog.Info(1, "Starting login screen update...")

	// Step 1: Determine the source image
	var sourceImagePath string
	var sourceImage image.Image
	var err error

	if loginscreen.HasBackup() {
		// Use the backed-up original image
		sourceImagePath, err = loginscreen.GetBackupImage()
		if err != nil {
			return fmt.Errorf("failed to get backup image: %v", err)
		}
		elog.Info(1, fmt.Sprintf("Using backup image: %s", sourceImagePath))
	} else {
		// Try to find the current login screen image
		sourceImagePath, err = loginscreen.GetCurrentLoginScreenImage()
		if err != nil {
			elog.Info(1, "No existing login screen found, creating default background")
			// Create a default dark background (1920x1080)
			sourceImage = loginscreen.CreateDefaultBackground(1920, 1080)
		} else {
			elog.Info(1, fmt.Sprintf("Found current login screen: %s", sourceImagePath))
			// Backup the original image
			err = loginscreen.BackupOriginalImage(sourceImagePath)
			if err != nil {
				elog.Warning(1, fmt.Sprintf("Failed to backup original image: %v", err))
			} else {
				elog.Info(1, "Backed up original login screen image")
			}
		}
	}

	// Load the source image if we haven't created a default one
	if sourceImage == nil {
		sourceImage, err = loginscreen.LoadImage(sourceImagePath)
		if err != nil {
			return fmt.Errorf("failed to load source image: %v", err)
		}
	}

	// Step 2: Gather system information
	elog.Info(1, "Gathering system information...")
	sysInfo, err := sysinfo.Gather()
	if err != nil {
		return fmt.Errorf("failed to gather system info: %v", err)
	}

	infoLines := sysInfo.FormatLines()
	elog.Info(1, fmt.Sprintf("System info: %d lines", len(infoLines)))

	// Step 3: Gather services information
	elog.Info(1, "Gathering services information...")
	servicesInfo, err := sysinfo.GatherServices()
	if err != nil {
		elog.Warning(1, fmt.Sprintf("Failed to gather services info: %v (continuing anyway)", err))
	}

	var serviceLines []string
	if servicesInfo != nil {
		serviceLines = servicesInfo.FormatServiceLines()
		elog.Info(1, fmt.Sprintf("Services info: %d lines, %d running, %d failed",
			len(serviceLines), servicesInfo.RunningCount, len(servicesInfo.FailedServices)))
	}

	// Step 4: Render the dual-panel overlay
	elog.Info(1, "Rendering overlay...")
	resultImage, err := overlay.RenderDualPanelOverlay(sourceImage, serviceLines, infoLines)
	if err != nil {
		return fmt.Errorf("failed to render overlay: %v", err)
	}

	// Step 5: Save the modified image to the permanent data directory
	// Using a unique filename with timestamp to bypass Windows lock screen cache
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	outputPath := filepath.Join(loginscreen.BackupDir, "loginscreen_"+timestamp+".jpg")

	err = loginscreen.SaveImage(resultImage, outputPath)
	if err != nil {
		return fmt.Errorf("failed to save modified image: %v", err)
	}
	elog.Info(1, fmt.Sprintf("Saved modified image to: %s", outputPath))

	// Clean up old loginscreen images (keep only the current one)
	cleanupOldLoginScreenImages(loginscreen.BackupDir, outputPath)

	// Step 6: Set the modified image as the login screen
	elog.Info(1, "Setting login screen...")
	err = loginscreen.SetLoginScreenImage(outputPath)
	if err != nil {
		return fmt.Errorf("failed to set login screen: %v", err)
	}

	// Step 7: Force restart LogonUI to display the new image (only at boot)
	// This is necessary because LogonUI caches the background image at startup
	// We only do this at boot (--boot flag) to avoid disrupting lock screen
	if isBootMode {
		elog.Info(1, "Boot mode: Restarting LogonUI to display new image...")
		restartLogonUICleanly(elog)
	} else {
		elog.Info(1, "Lock/manual mode: Skipping LogonUI restart")
	}

	elog.Info(1, "Login screen updated successfully!")
	return nil
}

// restartLogonUICleanly kills LogonUI and sends Escape to dismiss any password prompt
func restartLogonUICleanly(elog debug.Log) {
	// Check if LogonUI is running (it won't be if a user is logged in without lock screen)
	checkCmd := exec.Command("tasklist", "/fi", "imagename eq LogonUI.exe", "/fo", "csv", "/nh")
	output, _ := checkCmd.Output()
	if !strings.Contains(string(output), "LogonUI.exe") {
		elog.Info(1, "LogonUI not running (user may be logged in) - skipping restart")
		return
	}

	// Kill LogonUI - Windows will automatically restart it
	elog.Info(1, "Killing LogonUI.exe...")
	killCmd := exec.Command("taskkill", "/f", "/im", "LogonUI.exe")
	killCmd.Run()

	// Wait for Windows to restart LogonUI
	elog.Info(1, "Waiting for LogonUI to restart...")
	time.Sleep(2 * time.Second)

	// Send Escape key to dismiss password box and show clean lock screen
	// Using PowerShell with low-level keybd_event API to work on secure desktop
	elog.Info(1, "Sending Escape to dismiss password prompt...")
	psScript := `
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class KeySender {
    [DllImport("user32.dll")]
    public static extern void keybd_event(byte bVk, byte bScan, uint dwFlags, UIntPtr dwExtraInfo);
    public const byte VK_ESCAPE = 0x1B;
    public const uint KEYEVENTF_KEYUP = 0x0002;
    public static void SendEscape() {
        keybd_event(VK_ESCAPE, 0, 0, UIntPtr.Zero);
        System.Threading.Thread.Sleep(100);
        keybd_event(VK_ESCAPE, 0, KEYEVENTF_KEYUP, UIntPtr.Zero);
    }
}
"@
[KeySender]::SendEscape()
Start-Sleep -Milliseconds 500
[KeySender]::SendEscape()
`
	escCmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	if err := escCmd.Run(); err != nil {
		elog.Warning(1, fmt.Sprintf("Failed to send Escape key: %v", err))
	} else {
		elog.Info(1, "Escape key sent successfully")
	}
}

// runInteractive runs the service logic without the Windows service wrapper.
// Used for testing and debugging.
func runInteractive() {
	fmt.Println("BgStatusService - Running in interactive mode")
	fmt.Println("============================================")

	// Create a simple logger that outputs to stdout
	logger := &consoleLog{}

	err := runStatusUpdate(logger)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nDone! Check your login screen (Win+L or restart).")
}

// consoleLog implements debug.Log for console output.
type consoleLog struct{}

func (l *consoleLog) Close() error { return nil }
func (l *consoleLog) Info(eid uint32, msg string) error {
	fmt.Printf("[INFO] %s\n", msg)
	return nil
}
func (l *consoleLog) Warning(eid uint32, msg string) error {
	fmt.Printf("[WARN] %s\n", msg)
	return nil
}
func (l *consoleLog) Error(eid uint32, msg string) error {
	fmt.Printf("[ERROR] %s\n", msg)
	return nil
}

// cleanupOldLoginScreenImages removes old loginscreen_*.jpg files except the current one
func cleanupOldLoginScreenImages(dir, currentFile string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Only delete old loginscreen_*.jpg files
		if strings.HasPrefix(name, "loginscreen_") && strings.HasSuffix(name, ".jpg") {
			fullPath := filepath.Join(dir, name)
			if fullPath != currentFile {
				os.Remove(fullPath)
			}
		}
		// Also delete legacy current_loginscreen.jpg
		if name == "current_loginscreen.jpg" {
			os.Remove(filepath.Join(dir, name))
		}
	}
}

// isBootMode checks if --boot flag was passed (used to trigger LogonUI restart)
var isBootMode bool

func main() {
	// Check for --boot flag
	for _, arg := range os.Args[1:] {
		if arg == "--boot" {
			isBootMode = true
			break
		}
	}

	// Check if we're running as a service
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine if running as service: %v", err)
	}

	if !isService {
		// Running interactively (scheduled task or manual)
		runInteractive()
		return
	}

	// Running as a Windows service
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	err = svc.Run(serviceName, &bgStatusService{elog: elog})
	if err != nil {
		elog.Error(1, fmt.Sprintf("Service failed: %v", err))
		return
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
}

