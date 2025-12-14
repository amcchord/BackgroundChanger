// Package sysinfo gathers system information for display on the login screen.
package sysinfo

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yusufpapurcu/wmi"
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
}

// Win32_ComputerSystemProduct is used for WMI query to get serial number.
type Win32_ComputerSystemProduct struct {
	IdentifyingNumber string
}

// Win32_VideoController is used for WMI query to get GPU info.
type Win32_VideoController struct {
	Name string
}

// Win32_Processor is used for WMI query to get detailed CPU info.
type Win32_Processor struct {
	Name          string
	NumberOfCores uint32
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

	return lines
}

func getOSInfo() string {
	hostInfo, err := host.Info()
	if err != nil {
		return "Windows"
	}

	// Format: "Windows 11 Pro 23H2" or similar
	platform := hostInfo.Platform
	version := hostInfo.PlatformVersion

	// Try to get a cleaner Windows version
	osName := "Windows"
	if strings.Contains(platform, "Microsoft Windows") {
		osName = platform
	} else if platform != "" {
		osName = platform
	}

	// Add version if available
	if version != "" {
		// Extract major version info
		if strings.Contains(version, "10.0.22") {
			osName = "Windows 11"
		} else if strings.Contains(version, "10.0") {
			osName = "Windows 10"
		}

		// Try to append build number
		parts := strings.Split(version, ".")
		if len(parts) >= 3 {
			osName = fmt.Sprintf("%s (Build %s)", osName, parts[2])
		}
	}

	return osName
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

