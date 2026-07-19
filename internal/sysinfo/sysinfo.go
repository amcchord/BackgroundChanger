// Package sysinfo gathers the small set of machine facts shown before sign-in.
package sysinfo

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"
)

type ServiceState struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

type Snapshot struct {
	Hostname           string         `json:"hostname"`
	OS                 string         `json:"os"`
	Edition            string         `json:"edition"`
	Version            string         `json:"version"`
	Build              string         `json:"build"`
	CPU                string         `json:"cpu"`
	Memory             string         `json:"memory"`
	GPU                string         `json:"gpu"`
	Disk               string         `json:"disk"`
	IPs                []string       `json:"ip_addresses"`
	Serial             string         `json:"serial"`
	Uptime             string         `json:"uptime"`
	PendingReboot      bool           `json:"pending_reboot"`
	ServicesRunning    int            `json:"services_running"`
	ServicesTotal      int            `json:"services_total"`
	CriticalServices   []ServiceState `json:"critical_services"`
	FailedAutoServices []string       `json:"failed_auto_services,omitempty"`
	DisplayWidth       int            `json:"display_width"`
	DisplayHeight      int            `json:"display_height"`
	RefreshedAt        time.Time      `json:"refreshed_at"`
}

type win32Processor struct {
	Name                      string
	NumberOfLogicalProcessors uint32
}

type win32VideoController struct {
	Name                        string
	CurrentHorizontalResolution uint32
	CurrentVerticalResolution   uint32
}

type win32BIOS struct{ SerialNumber string }

type win32Service struct {
	Name      string
	State     string
	StartMode string
}

func Gather() Snapshot {
	hostname, _ := os.Hostname()
	s := Snapshot{Hostname: hostname, RefreshedAt: time.Now()}
	s.OS, s.Edition, s.Version, s.Build = windowsVersion()
	s.CPU = cpuInfo()
	s.Memory = memoryInfo()
	s.GPU, s.DisplayWidth, s.DisplayHeight = displayInfo()
	s.Disk = diskInfo()
	s.IPs = ipAddresses()
	s.Serial = serialNumber()
	s.Uptime = uptime()
	s.PendingReboot = pendingReboot()
	s.ServicesRunning, s.ServicesTotal, s.CriticalServices, s.FailedAutoServices = serviceInfo()
	if s.DisplayWidth < 800 || s.DisplayHeight < 600 {
		s.DisplayWidth, s.DisplayHeight = 1920, 1080
	}
	return s
}

func windowsVersion() (product, edition, version, build string) {
	product = "Windows"
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return product, "Unknown", "", ""
	}
	defer key.Close()
	if value, _, err := key.GetStringValue("ProductName"); err == nil && value != "" {
		product = value
	}
	edition, _, _ = key.GetStringValue("EditionID")
	version, _, _ = key.GetStringValue("DisplayVersion")
	build, _, _ = key.GetStringValue("CurrentBuildNumber")
	if ubr, _, err := key.GetIntegerValue("UBR"); err == nil && build != "" {
		build += "." + strconv.FormatUint(ubr, 10)
	}
	baseBuild, _ := strconv.Atoi(strings.Split(build, ".")[0])
	if baseBuild >= 22000 && strings.Contains(product, "Windows 10") {
		product = strings.Replace(product, "Windows 10", "Windows 11", 1)
	}
	return product, edition, version, build
}

func cpuInfo() string {
	var processors []win32Processor
	if err := wmi.Query("SELECT Name, NumberOfLogicalProcessors FROM Win32_Processor", &processors); err == nil && len(processors) > 0 {
		name := compact(strings.TrimSpace(processors[0].Name), 48)
		logical := uint32(0)
		for _, p := range processors {
			logical += p.NumberOfLogicalProcessors
		}
		return fmt.Sprintf("%s · %d logical", name, logical)
	}
	return fmt.Sprintf("%d logical processors", runtime.NumCPU())
}

func memoryInfo() string {
	m, err := mem.VirtualMemory()
	if err != nil {
		return "Unavailable"
	}
	return fmt.Sprintf("%.1f / %.1f GiB · %.0f%%", bytesToGiB(m.Used), bytesToGiB(m.Total), m.UsedPercent)
}

func displayInfo() (string, int, int) {
	var adapters []win32VideoController
	if err := wmi.Query("SELECT Name, CurrentHorizontalResolution, CurrentVerticalResolution FROM Win32_VideoController", &adapters); err != nil || len(adapters) == 0 {
		return "Unavailable", 1920, 1080
	}
	name := compact(strings.TrimSpace(adapters[0].Name), 48)
	width, height := 0, 0
	for _, adapter := range adapters {
		if int(adapter.CurrentHorizontalResolution*adapter.CurrentVerticalResolution) > width*height {
			width = int(adapter.CurrentHorizontalResolution)
			height = int(adapter.CurrentVerticalResolution)
		}
	}
	return name, width, height
}

func diskInfo() string {
	root := os.Getenv("SystemDrive")
	if root == "" {
		root = "C:"
	}
	u, err := disk.Usage(root + `\`)
	if err != nil {
		return "Unavailable"
	}
	return fmt.Sprintf("%.1f / %.1f GiB · %.0f%%", bytesToGiB(u.Used), bytesToGiB(u.Total), u.UsedPercent)
}

func ipAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var result []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.To4() == nil {
				continue
			}
			result = append(result, ip.String())
		}
	}
	sort.Strings(result)
	return unique(result)
}

func serialNumber() string {
	var bios []win32BIOS
	if err := wmi.Query("SELECT SerialNumber FROM Win32_BIOS", &bios); err == nil && len(bios) > 0 {
		serial := strings.TrimSpace(bios[0].SerialNumber)
		if serial != "" {
			return compact(serial, 40)
		}
	}
	return "Unavailable"
}

func uptime() string {
	seconds, err := host.Uptime()
	if err != nil {
		return "Unavailable"
	}
	d := time.Duration(seconds) * time.Second
	days := int(d / (24 * time.Hour))
	hours := int((d % (24 * time.Hour)) / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

func pendingReboot() bool {
	keys := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired`,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`,
	}
	for _, path := range keys {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
		if err == nil {
			key.Close()
			return true
		}
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager`, registry.QUERY_VALUE)
	if err == nil {
		defer key.Close()
		if values, _, err := key.GetStringsValue("PendingFileRenameOperations"); err == nil && len(values) > 0 {
			return true
		}
	}
	return false
}

func serviceInfo() (running, total int, critical []ServiceState, failed []string) {
	var services []win32Service
	if err := wmi.Query("SELECT Name, State, StartMode FROM Win32_Service", &services); err != nil {
		return 0, 0, nil, nil
	}
	total = len(services)
	byName := make(map[string]win32Service, len(services))
	for _, service := range services {
		byName[strings.ToLower(service.Name)] = service
		if strings.EqualFold(service.State, "Running") {
			running++
		}
		if strings.EqualFold(service.StartMode, "Auto") && !strings.EqualFold(service.State, "Running") && !strings.EqualFold(service.State, "Start Pending") {
			failed = append(failed, service.Name)
		}
	}
	criticalNames := []string{"WinDefend", "Dhcp", "Dnscache", "EventLog", "W32Time", "wuauserv"}
	for _, name := range criticalNames {
		if service, ok := byName[strings.ToLower(name)]; ok {
			critical = append(critical, ServiceState{Name: friendlyServiceName(name), Running: strings.EqualFold(service.State, "Running")})
		}
	}
	sort.Strings(failed)
	if len(failed) > 6 {
		failed = failed[:6]
	}
	return running, total, critical, failed
}

func friendlyServiceName(name string) string {
	names := map[string]string{
		"WinDefend": "Defender", "Dhcp": "DHCP", "Dnscache": "DNS Client",
		"EventLog": "Event Log", "W32Time": "Time Sync", "wuauserv": "Windows Update",
	}
	if value, ok := names[name]; ok {
		return value
	}
	return name
}

func SupportsMachineLockScreenPolicy(edition string) bool {
	e := strings.ToLower(edition)
	return strings.Contains(e, "enterprise") || strings.Contains(e, "education") || strings.Contains(e, "server") || strings.Contains(e, "iot")
}

func CurrentEdition() string {
	_, edition, _, _ := windowsVersion()
	return edition
}

func IsProfessionalEdition(edition string) bool {
	e := strings.ToLower(edition)
	return strings.Contains(e, "professional") && !strings.Contains(e, "education")
}

func bytesToGiB(value uint64) float64 { return float64(value) / (1024 * 1024 * 1024) }

func compact(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	if max < 2 {
		return value[:max]
	}
	return value[:max-1] + "…"
}

func unique(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:0]
	for i, value := range values {
		if i == 0 || value != values[i-1] {
			result = append(result, value)
		}
	}
	return result
}
