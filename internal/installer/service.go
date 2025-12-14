package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	// ServiceName is the Windows service name
	ServiceName = "BgStatusService"

	// ServiceDisplayName is the friendly name shown in services.msc
	ServiceDisplayName = "Background Status Service"

	// ServiceDescription describes the service
	ServiceDescription = "Displays system information on the Windows login screen background."
)

// GetInstallDir returns the installation directory path
func GetInstallDir() string {
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	return filepath.Join(programFiles, "BgStatusService")
}

// GetDataDir returns the data directory path (for backups, etc.)
func GetDataDir() string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, "BgStatusService")
}

// ServiceExists checks if the service is already installed
func ServiceExists() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		// Service doesn't exist
		return false, nil
	}
	s.Close()
	return true, nil
}

// IsServiceRunning checks if the service is currently running
func IsServiceRunning() (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return false, nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false, fmt.Errorf("failed to query service: %w", err)
	}

	return status.State == svc.Running, nil
}

// StopService stops the service if it's running
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		// Service doesn't exist, nothing to stop
		return nil
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service: %w", err)
	}

	if status.State != svc.Running {
		return nil
	}

	// Send stop signal
	_, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	// Wait for service to stop
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		status, err = s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
		if status.State == svc.Stopped {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for service to stop")
}

// DeleteService removes the Windows service
func DeleteService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		// Service doesn't exist
		return nil
	}
	defer s.Close()

	err = s.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Give Windows time to clean up
	time.Sleep(2 * time.Second)
	return nil
}

// InstallService installs the Windows service
func InstallService(exePath string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	// Create installation directory
	installDir := GetInstallDir()
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return fmt.Errorf("failed to create install directory: %w", err)
	}

	// Copy executable to installation directory
	destPath := filepath.Join(installDir, "bgStatusService.exe")
	if err := copyFile(exePath, destPath); err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	// Create the service
	config := mgr.Config{
		DisplayName:  ServiceDisplayName,
		Description:  ServiceDescription,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
	}

	s, err := m.CreateService(ServiceName, destPath, config)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Set recovery options (no restart on failure since it's a one-shot service)
	// This is optional and can be done via sc.exe if needed

	// Create data directory
	dataDir := GetDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Register event log source
	err = eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		// Non-critical, just log it
		// The service will still work without event logging
	}

	return nil
}

// StartService starts the Windows service
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(ServiceName)
	if err != nil {
		return fmt.Errorf("failed to open service: %w", err)
	}
	defer s.Close()

	err = s.Start()
	if err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait for service to start (or complete, since it's a one-shot service)
	timeout := time.Now().Add(30 * time.Second)
	for time.Now().Before(timeout) {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("failed to query service status: %w", err)
		}
		// Service either runs briefly and stops, or stays running
		if status.State == svc.Running || status.State == svc.Stopped {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// RemoveInstallation removes installed files
func RemoveInstallation() error {
	installDir := GetInstallDir()

	// Try to remove the installation directory
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("failed to remove install directory: %w", err)
	}

	return nil
}

// RemoveDataDirectory removes the data directory (backups, etc.)
func RemoveDataDirectory() error {
	dataDir := GetDataDir()

	// Try to remove the data directory
	if err := os.RemoveAll(dataDir); err != nil {
		return fmt.Errorf("failed to remove data directory: %w", err)
	}

	return nil
}

// RemoveEventLogSource removes the event log registration
func RemoveEventLogSource() error {
	return eventlog.Remove(ServiceName)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// GetInstalledExePath returns the path to the installed executable
func GetInstalledExePath() string {
	return filepath.Join(GetInstallDir(), "bgStatusService.exe")
}
