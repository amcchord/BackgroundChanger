// Package sysinfo gathers system information for display on the login screen.
package sysinfo

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"
)

// SystemInfo contains all gathered system information.
type SystemInfo struct {
	Hostname     string
	OS           string
	CPU          string
	RAM          string
	GPU          string
	IPAddresses  []string
	DiskInfo     []string
	SerialNumber string
	Uptime       string
	GeneratedAt  string
}

// Win32_ComputerSystemProduct is used for WMI query to get serial number.
type Win32_ComputerSystemProduct struct {
	IdentifyingNumber string
}

// Win32_VideoController is used for WMI query to get GPU info.
type Win32_VideoController struct {
	Name string
}

// Win32_VideoControllerResolution is used for WMI query to get display resolution.
type Win32_VideoControllerResolution struct {
	CurrentHorizontalResolution uint32
	CurrentVerticalResolution   uint32
}

// DisplayResolution contains the current display resolution.
type DisplayResolution struct {
	Width  int
	Height int
}

// Win32_Processor is used for WMI query to get detailed CPU info.
type Win32_Processor struct {
	Name          string
	NumberOfCores uint32
}

// Win32_Service is used for WMI query to get service information.
type Win32_Service struct {
	Name      string
	State     string
	StartMode string
}

// Win32_OperatingSystem is used for WMI query to detect Windows Server.
type Win32_OperatingSystem struct {
	Caption string
}

// ServiceStatus represents the status of a single service.
type ServiceStatus struct {
	Name    string
	State   string
	IsOK    bool
}

// ServicesSummary contains information about Windows services.
type ServicesSummary struct {
	RunningCount     int
	StoppedCount     int
	TotalCount       int
	FailedServices   []ServiceStatus // Auto-start services that aren't running
	CriticalServices []ServiceStatus // Status of critical services
	IsServer         bool
}

// Gather collects all system information and returns a SystemInfo struct.
func Gather() (*SystemInfo, error) {
	info := &SystemInfo{}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		info.Hostname = "Unknown"
	} else {
		info.Hostname = hostname
	}

	// Get OS information
	info.OS = getOSInfo()

	// Get CPU information
	info.CPU = getCPUInfo()

	// Get RAM information
	info.RAM = getRAMInfo()

	// Get GPU information
	info.GPU = getGPUInfo()

	// Get IP addresses
	info.IPAddresses = getIPAddresses()

	// Get disk information
	info.DiskInfo = getDiskInfo()

	// Get serial number
	info.SerialNumber = getSerialNumber()

	// Get uptime
	info.Uptime = getUptime()

	// Get generation timestamp
	info.GeneratedAt = time.Now().Format("Generated: Jan 2, 2006 3:04 PM")

	return info, nil
}

// FormatLines returns the system info as a slice of strings for display.
func (s *SystemInfo) FormatLines() []string {
	lines := []string{}

	lines = append(lines, s.Hostname)
	lines = append(lines, s.OS)
	lines = append(lines, s.CPU)
	lines = append(lines, s.RAM)

	if s.GPU != "" && s.GPU != "Unknown" {
		lines = append(lines, s.GPU)
	}

	// Add first IP address (or first two if multiple)
	for i, ip := range s.IPAddresses {
		if i >= 2 {
			break
		}
		lines = append(lines, ip)
	}

	// Add disk info
	for _, diskLine := range s.DiskInfo {
		lines = append(lines, diskLine)
	}

	if s.SerialNumber != "" && s.SerialNumber != "Unknown" {
		lines = append(lines, fmt.Sprintf("SN: %s", s.SerialNumber))
	}

	// Add uptime
	if s.Uptime != "" {
		lines = append(lines, fmt.Sprintf("Uptime: %s", s.Uptime))
	}

	// Add generation timestamp
	if s.GeneratedAt != "" {
		lines = append(lines, s.GeneratedAt)
	}

	return lines
}

func getOSInfo() string {
	// Use WMI to get the accurate OS caption (e.g., "Microsoft Windows 11 Pro")
	var osInfo []Win32_OperatingSystem
	err := wmi.Query("SELECT Caption FROM Win32_OperatingSystem", &osInfo)
	if err == nil && len(osInfo) > 0 {
		caption := osInfo[0].Caption
		// Clean up the caption - remove "Microsoft " prefix for brevity
		caption = strings.TrimPrefix(caption, "Microsoft ")

		// Try to get the display version (e.g., "24H2") from registry
		displayVersion := getWindowsDisplayVersion()
		if displayVersion != "" {
			return fmt.Sprintf("%s %s", caption, displayVersion)
		}
		return caption
	}

	// Fallback to gopsutil if WMI fails
	hostInfo, err := host.Info()
	if err != nil {
		return "Windows"
	}

	version := hostInfo.PlatformVersion
	osName := "Windows"

	// Determine Windows 10 vs 11 based on build number
	// Windows 11 starts at build 22000
	if version != "" {
		parts := strings.Split(version, ".")
		if len(parts) >= 3 {
			buildNum := parts[2]
			// Convert to int for comparison
			var build int
			fmt.Sscanf(buildNum, "%d", &build)

			if build >= 22000 {
				osName = "Windows 11"
			} else {
				osName = "Windows 10"
			}
			osName = fmt.Sprintf("%s (Build %s)", osName, buildNum)
		}
	}

	return osName
}

// getWindowsDisplayVersion gets the display version (e.g., "24H2") from registry
func getWindowsDisplayVersion() string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	displayVersion, _, err := key.GetStringValue("DisplayVersion")
	if err != nil {
		return ""
	}

	return displayVersion
}

func getCPUInfo() string {
	// Try WMI first for more detailed info
	var processors []Win32_Processor
	err := wmi.Query("SELECT Name, NumberOfCores FROM Win32_Processor", &processors)
	if err == nil && len(processors) > 0 {
		proc := processors[0]
		// Clean up CPU name (remove extra spaces)
		name := strings.Join(strings.Fields(proc.Name), " ")
		return fmt.Sprintf("%s (%d cores)", name, proc.NumberOfCores)
	}

	// Fallback to gopsutil
	cpuInfo, err := cpu.Info()
	if err != nil || len(cpuInfo) == 0 {
		// Ultimate fallback
		return fmt.Sprintf("CPU (%d cores)", runtime.NumCPU())
	}

	return fmt.Sprintf("%s (%d cores)", cpuInfo[0].ModelName, runtime.NumCPU())
}

func getRAMInfo() string {
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		return "RAM: Unknown"
	}

	totalGB := float64(memInfo.Total) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.0f GB RAM", totalGB)
}

func getGPUInfo() string {
	var controllers []Win32_VideoController
	err := wmi.Query("SELECT Name FROM Win32_VideoController", &controllers)
	if err != nil || len(controllers) == 0 {
		return "Unknown"
	}

	// Return primary GPU (first one)
	return controllers[0].Name
}

func getIPAddresses() []string {
	var ips []string

	interfaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Only include IPv4 addresses, skip loopback
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			ips = append(ips, ip.String())
		}
	}

	return ips
}

func getDiskInfo() []string {
	var diskLines []string

	partitions, err := disk.Partitions(false)
	if err != nil {
		return diskLines
	}

	for _, partition := range partitions {
		// Only include physical drives (skip network, CD-ROM, etc.)
		if partition.Fstype == "" {
			continue
		}

		usage, err := disk.Usage(partition.Mountpoint)
		if err != nil {
			continue
		}

		// Format: "C: 256GB / 1TB"
		usedGB := float64(usage.Used) / (1024 * 1024 * 1024)
		totalGB := float64(usage.Total) / (1024 * 1024 * 1024)

		var usedStr, totalStr string

		if usedGB >= 1024 {
			usedStr = fmt.Sprintf("%.1fTB", usedGB/1024)
		} else {
			usedStr = fmt.Sprintf("%.0fGB", usedGB)
		}

		if totalGB >= 1024 {
			totalStr = fmt.Sprintf("%.1fTB", totalGB/1024)
		} else {
			totalStr = fmt.Sprintf("%.0fGB", totalGB)
		}

		// Extract drive letter (e.g., "C:" from "C:\")
		drive := strings.TrimSuffix(partition.Mountpoint, "\\")
		diskLines = append(diskLines, fmt.Sprintf("%s %s / %s", drive, usedStr, totalStr))
	}

	return diskLines
}

func getSerialNumber() string {
	var products []Win32_ComputerSystemProduct
	err := wmi.Query("SELECT IdentifyingNumber FROM Win32_ComputerSystemProduct", &products)
	if err != nil || len(products) == 0 {
		return "Unknown"
	}

	serial := products[0].IdentifyingNumber
	// Some machines return placeholder values
	if serial == "" || serial == "To be filled by O.E.M." || serial == "Default string" {
		return "Unknown"
	}

	return serial
}

func getUptime() string {
	uptime, err := host.Uptime()
	if err != nil {
		return "Unknown"
	}

	// Convert seconds to days, hours, minutes
	days := uptime / 86400
	hours := (uptime % 86400) / 3600
	minutes := (uptime % 3600) / 60

	// Format based on duration
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// GetDisplayResolution queries the current display resolution from the system.
// Returns the primary monitor's resolution, or a default of 1920x1080 if unable to detect.
func GetDisplayResolution() DisplayResolution {
	// Default resolution as fallback
	defaultRes := DisplayResolution{Width: 1920, Height: 1080}

	// Query Win32_VideoController for current resolution
	var controllers []struct {
		CurrentHorizontalResolution uint32
		CurrentVerticalResolution   uint32
	}

	err := wmi.Query("SELECT CurrentHorizontalResolution, CurrentVerticalResolution FROM Win32_VideoController WHERE CurrentHorizontalResolution IS NOT NULL", &controllers)
	if err != nil || len(controllers) == 0 {
		return defaultRes
	}

	// Use the first controller with valid resolution
	for _, ctrl := range controllers {
		if ctrl.CurrentHorizontalResolution > 0 && ctrl.CurrentVerticalResolution > 0 {
			return DisplayResolution{
				Width:  int(ctrl.CurrentHorizontalResolution),
				Height: int(ctrl.CurrentVerticalResolution),
			}
		}
	}

	return defaultRes
}

// isWindowsServer checks if the current OS is Windows Server.
func isWindowsServer() bool {
	var osInfo []Win32_OperatingSystem
	err := wmi.Query("SELECT Caption FROM Win32_OperatingSystem", &osInfo)
	if err != nil || len(osInfo) == 0 {
		return false
	}

	caption := strings.ToLower(osInfo[0].Caption)
	return strings.Contains(caption, "server")
}

// getCriticalServiceNames returns a list of critical service names based on OS type.
func getCriticalServiceNames(isServer bool) []string {
	// Desktop critical services
	services := []string{
		"Dhcp",           // DHCP Client
		"Dnscache",       // DNS Client
		"wuauserv",       // Windows Update
		"WinDefend",      // Windows Defender
		"Spooler",        // Print Spooler
		"EventLog",       // Windows Event Log
		"Schedule",       // Task Scheduler
		"W32Time",        // Windows Time
	}

	// Add server-specific services
	if isServer {
		serverServices := []string{
			"NTDS",         // Active Directory Domain Services
			"DNS",          // DNS Server
			"DHCPServer",   // DHCP Server
			"W3SVC",        // IIS World Wide Web Publishing Service
			"MSSQLSERVER",  // SQL Server
			"vmms",         // Hyper-V Virtual Machine Management
			"CertSvc",      // Active Directory Certificate Services
			"Netlogon",     // Netlogon (domain controller)
			"DFSR",         // DFS Replication
			"LanmanServer", // Server (file sharing)
		}
		services = append(services, serverServices...)
	}

	return services
}

// GatherServices collects information about Windows services.
func GatherServices() (*ServicesSummary, error) {
	summary := &ServicesSummary{}
	summary.IsServer = isWindowsServer()

	// Query all services
	var services []Win32_Service
	err := wmi.Query("SELECT Name, State, StartMode FROM Win32_Service", &services)
	if err != nil {
		return summary, fmt.Errorf("failed to query services: %v", err)
	}

	summary.TotalCount = len(services)

	// Build a map for quick lookup
	serviceMap := make(map[string]Win32_Service)
	for _, svc := range services {
		serviceMap[svc.Name] = svc

		if svc.State == "Running" {
			summary.RunningCount++
		} else {
			summary.StoppedCount++
		}

		// Check for failed services (auto-start but not running)
		if svc.StartMode == "Auto" && svc.State != "Running" {
			summary.FailedServices = append(summary.FailedServices, ServiceStatus{
				Name:  svc.Name,
				State: svc.State,
				IsOK:  false,
			})
		}
	}

	// Check critical services
	criticalNames := getCriticalServiceNames(summary.IsServer)
	for _, name := range criticalNames {
		svc, exists := serviceMap[name]
		if !exists {
			// Service not installed, skip it (common for server services on desktop)
			continue
		}

		isOK := svc.State == "Running"
		summary.CriticalServices = append(summary.CriticalServices, ServiceStatus{
			Name:  name,
			State: svc.State,
			IsOK:  isOK,
		})
	}

	return summary, nil
}

// FormatServiceLines returns the services summary as a slice of strings for display.
func (s *ServicesSummary) FormatServiceLines() []string {
	lines := []string{}

	// Header
	lines = append(lines, "Services Status")
	lines = append(lines, "")

	// Summary line
	lines = append(lines, fmt.Sprintf("Running: %d / %d", s.RunningCount, s.TotalCount))

	// Critical services status
	if len(s.CriticalServices) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Critical Services:")

		for _, svc := range s.CriticalServices {
			status := "OK"
			if !svc.IsOK {
				status = svc.State
			}
			// Use friendly names for common services
			displayName := getServiceDisplayName(svc.Name)
			lines = append(lines, fmt.Sprintf("  %s: %s", displayName, status))
		}
	}

	// Failed services (auto-start but not running)
	if len(s.FailedServices) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Failed Services:")

		// Limit to first 10 to avoid overflow
		count := len(s.FailedServices)
		if count > 10 {
			count = 10
		}

		for i := 0; i < count; i++ {
			svc := s.FailedServices[i]
			displayName := getServiceDisplayName(svc.Name)
			lines = append(lines, fmt.Sprintf("  %s: %s", displayName, svc.State))
		}

		if len(s.FailedServices) > 10 {
			lines = append(lines, fmt.Sprintf("  ... and %d more", len(s.FailedServices)-10))
		}
	} else {
		lines = append(lines, "")
		lines = append(lines, "No failed services")
	}

	return lines
}

// getServiceDisplayName returns a friendly display name for common services.
func getServiceDisplayName(serviceName string) string {
	displayNames := map[string]string{
		"Dhcp":         "DHCP Client",
		"Dnscache":     "DNS Client",
		"wuauserv":     "Windows Update",
		"WinDefend":    "Windows Defender",
		"Spooler":      "Print Spooler",
		"EventLog":     "Event Log",
		"Schedule":     "Task Scheduler",
		"W32Time":      "Windows Time",
		"NTDS":         "AD Domain Services",
		"DNS":          "DNS Server",
		"DHCPServer":   "DHCP Server",
		"W3SVC":        "IIS Web Server",
		"MSSQLSERVER":  "SQL Server",
		"vmms":         "Hyper-V Manager",
		"CertSvc":      "Certificate Services",
		"Netlogon":     "Netlogon",
		"DFSR":         "DFS Replication",
		"LanmanServer": "File Server",
	}

	if displayName, exists := displayNames[serviceName]; exists {
		return displayName
	}
	return serviceName
}

