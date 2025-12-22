package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// Scheduled Task constants and functions

const (
	// ScheduledTaskNameLock is the task that runs on lock/logoff
	ScheduledTaskNameLock = "BgStatusServiceLock"
	// ScheduledTaskNameBoot is the task that runs at boot with LogonUI restart
	ScheduledTaskNameBoot = "BgStatusServiceBoot"
)

// ScheduledTaskExists checks if either scheduled task is installed
func ScheduledTaskExists() bool {
	cmd := exec.Command("schtasks", "/query", "/tn", ScheduledTaskNameBoot)
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.Command("schtasks", "/query", "/tn", ScheduledTaskNameLock)
	if err := cmd.Run(); err == nil {
		return true
	}
	return false
}

// InstallScheduledTasks creates the boot and lock scheduled tasks
func InstallScheduledTasks(exePath string) error {
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

	// Create data directory
	dataDir := GetDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Delete existing tasks
	DeleteScheduledTasks()

	// Create boot task XML (runs at boot with --boot flag to restart LogonUI)
	bootTaskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Updates login screen at boot - restarts LogonUI to show fresh system info</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <StartWhenAvailable>true</StartWhenAvailable>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT5M</ExecutionTimeLimit>
    <Priority>1</Priority>
  </Settings>
  <Triggers>
    <BootTrigger>
      <Enabled>true</Enabled>
    </BootTrigger>
  </Triggers>
  <Actions Context="Author">
    <Exec>
      <Command>"%s"</Command>
      <Arguments>--boot</Arguments>
    </Exec>
  </Actions>
</Task>`, ScheduledTaskNameBoot, destPath)

	// Create lock task XML (runs on lock/logoff without restarting LogonUI)
	lockTaskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>Updates login screen on lock/logoff for next viewing</Description>
    <URI>\%s</URI>
  </RegistrationInfo>
  <Principals>
    <Principal id="Author">
      <UserId>S-1-5-18</UserId>
      <RunLevel>HighestAvailable</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <StartWhenAvailable>true</StartWhenAvailable>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <Enabled>true</Enabled>
    <ExecutionTimeLimit>PT10M</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Triggers>
    <SessionStateChangeTrigger>
      <Enabled>true</Enabled>
      <StateChange>SessionLock</StateChange>
    </SessionStateChangeTrigger>
    <SessionStateChangeTrigger>
      <Enabled>true</Enabled>
      <StateChange>ConsoleDisconnect</StateChange>
    </SessionStateChangeTrigger>
  </Triggers>
  <Actions Context="Author">
    <Exec>
      <Command>"%s"</Command>
    </Exec>
  </Actions>
</Task>`, ScheduledTaskNameLock, destPath)

	// Write and import boot task
	tempDir := os.TempDir()
	bootXMLPath := filepath.Join(tempDir, "bgstatus_boot.xml")
	if err := os.WriteFile(bootXMLPath, []byte(bootTaskXML), 0644); err != nil {
		return fmt.Errorf("failed to write boot task XML: %w", err)
	}
	defer os.Remove(bootXMLPath)

	cmd := exec.Command("schtasks", "/create", "/tn", ScheduledTaskNameBoot, "/xml", bootXMLPath, "/f")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create boot task: %w - %s", err, string(output))
	}

	// Write and import lock task
	lockXMLPath := filepath.Join(tempDir, "bgstatus_lock.xml")
	if err := os.WriteFile(lockXMLPath, []byte(lockTaskXML), 0644); err != nil {
		return fmt.Errorf("failed to write lock task XML: %w", err)
	}
	defer os.Remove(lockXMLPath)

	cmd = exec.Command("schtasks", "/create", "/tn", ScheduledTaskNameLock, "/xml", lockXMLPath, "/f")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create lock task: %w - %s", err, string(output))
	}

	// Register event log source
	_ = eventlog.InstallAsEventCreate(ServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)

	return nil
}

// DeleteScheduledTasks removes both scheduled tasks
func DeleteScheduledTasks() {
	exec.Command("schtasks", "/delete", "/tn", ScheduledTaskNameBoot, "/f").Run()
	exec.Command("schtasks", "/delete", "/tn", ScheduledTaskNameLock, "/f").Run()
}

// RunScheduledTask runs the boot task to generate the initial image
func RunScheduledTask() error {
	cmd := exec.Command("schtasks", "/run", "/tn", ScheduledTaskNameBoot)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run task: %w - %s", err, string(output))
	}
	return nil
}

// RunExecutableDirectly runs the service executable directly
func RunExecutableDirectly() error {
	exePath := GetInstalledExePath()
	cmd := exec.Command(exePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if it's just a "not found" type error vs actual failure
		outStr := string(output)
		if strings.Contains(outStr, "Error") {
			return fmt.Errorf("executable failed: %w - %s", err, outStr)
		}
	}
	return nil
}
