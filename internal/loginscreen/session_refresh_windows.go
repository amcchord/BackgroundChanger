package loginscreen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"
)

const invalidSessionID = ^uint32(0)

var (
	wtsAPI                  = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySessionInfo = wtsAPI.NewProc("WTSQuerySessionInformationW")
	procWTSDisconnect       = wtsAPI.NewProc("WTSDisconnectSession")
)

// PreLoginRefreshResult records whether Windows was asked to rebuild its
// unauthenticated physical-console session after accepting the current image.
// The operation is intentionally separate from Apply: only the boot-settled
// refresh is eligible to perform it.
type PreLoginRefreshResult struct {
	Attempted     bool   `json:"attempted"`
	Refreshed     bool   `json:"refreshed"`
	SessionID     uint32 `json:"session_id,omitempty"`
	SkippedReason string `json:"skipped_reason,omitempty"`
}

type preLoginRefreshMarker struct {
	BootID      string    `json:"boot_id"`
	SessionID   uint32    `json:"session_id,omitempty"`
	EvaluatedAt time.Time `json:"evaluated_at"`
	Outcome     string    `json:"outcome"`
}

type wtsSession struct {
	ID      uint32
	Station string
	User    string
	State   uint32
}

type sessionProcesses struct {
	LogonUI  int
	Winlogon int
	Explorer int
}

type preLoginRefreshDeps struct {
	bootID        func() (string, error)
	now           func() time.Time
	pause         func(time.Duration)
	activeConsole func() uint32
	sessions      func() ([]wtsSession, error)
	processes     func(uint32) (sessionProcesses, error)
	disconnect    func(uint32) error
}

// RefreshPreLoginSession asks Windows to disconnect and recreate only an
// empty physical-console login session. Windows then regenerates its protected
// LogonUI bitmap cache from the current, already-verified policy image. It
// never logs off an authenticated user and never terminates LogonUI.
func RefreshPreLoginSession(markerPath string) (PreLoginRefreshResult, error) {
	return refreshPreLoginSession(markerPath, preLoginRefreshDeps{
		bootID:        windowsBootID,
		now:           time.Now,
		pause:         time.Sleep,
		activeConsole: windows.WTSGetActiveConsoleSessionId,
		sessions:      enumerateWTSSessions,
		processes:     processesInSession,
		disconnect:    disconnectWTSSession,
	})
}

func refreshPreLoginSession(markerPath string, deps preLoginRefreshDeps) (PreLoginRefreshResult, error) {
	result := PreLoginRefreshResult{}
	bootID, err := deps.bootID()
	if err != nil || bootID == "" {
		if err == nil {
			err = errors.New("Windows boot identity is unavailable")
		}
		return result, fmt.Errorf("read boot identity: %w", err)
	}
	if marker, err := readPreLoginRefreshMarker(markerPath); err == nil && marker.BootID == bootID {
		result.SessionID = marker.SessionID
		result.SkippedReason = "already evaluated this boot"
		return result, nil
	}

	sessionID := deps.activeConsole()
	result.SessionID = sessionID
	marker := preLoginRefreshMarker{
		BootID: bootID, SessionID: sessionID, EvaluatedAt: deps.now(), Outcome: "evaluating",
	}
	// This fence is written before any session operation. A service crash or
	// restart can therefore never rotate the console twice in the same boot.
	if err := writePreLoginRefreshMarker(markerPath, marker); err != nil {
		return result, fmt.Errorf("write once-per-boot fence: %w", err)
	}

	skip := func(reason string) (PreLoginRefreshResult, error) {
		result.SkippedReason = reason
		marker.Outcome = "skipped: " + reason
		return result, writeMarkerOutcome(markerPath, marker)
	}
	if sessionID == invalidSessionID || sessionID == 0 {
		return skip("no physical-console session is available")
	}
	if ok, reason, err := eligiblePreLoginConsole(sessionID, deps); err != nil {
		marker.Outcome = "error: " + err.Error()
		_ = writePreLoginRefreshMarker(markerPath, marker)
		return result, err
	} else if !ok {
		return skip(reason)
	}

	// Recheck after a short quiet period. This does not expose pre-auth input,
	// but it closes the common race where a user session completes while the
	// service is preparing the refresh.
	deps.pause(500 * time.Millisecond)
	if deps.activeConsole() != sessionID {
		return skip("the physical-console session changed during the safety check")
	}
	if ok, reason, err := eligiblePreLoginConsole(sessionID, deps); err != nil {
		marker.Outcome = "error: " + err.Error()
		_ = writePreLoginRefreshMarker(markerPath, marker)
		return result, err
	} else if !ok {
		return skip(reason)
	}

	result.Attempted = true
	marker.Outcome = "disconnect requested"
	if err := writePreLoginRefreshMarker(markerPath, marker); err != nil {
		return result, fmt.Errorf("record disconnect request: %w", err)
	}
	if err := deps.disconnect(sessionID); err != nil {
		marker.Outcome = "disconnect failed: " + err.Error()
		_ = writePreLoginRefreshMarker(markerPath, marker)
		return result, fmt.Errorf("disconnect empty console session %d: %w", sessionID, err)
	}
	result.Refreshed = true
	marker.Outcome = "refresh requested"
	if err := writePreLoginRefreshMarker(markerPath, marker); err != nil {
		return result, fmt.Errorf("record successful request: %w", err)
	}
	return result, nil
}

type win32OperatingSystemBoot struct {
	LastBootUpTime time.Time
}

func windowsBootID() (string, error) {
	var operatingSystems []win32OperatingSystemBoot
	if err := wmi.Query("SELECT LastBootUpTime FROM Win32_OperatingSystem", &operatingSystems); err != nil {
		return "", err
	}
	if len(operatingSystems) != 1 || operatingSystems[0].LastBootUpTime.IsZero() {
		return "", errors.New("Win32_OperatingSystem returned no boot time")
	}
	return operatingSystems[0].LastBootUpTime.UTC().Format(time.RFC3339Nano), nil
}

func eligiblePreLoginConsole(sessionID uint32, deps preLoginRefreshDeps) (bool, string, error) {
	sessions, err := deps.sessions()
	if err != nil {
		return false, "", fmt.Errorf("enumerate Windows sessions: %w", err)
	}
	var console *wtsSession
	for index := range sessions {
		session := &sessions[index]
		if strings.TrimSpace(session.User) != "" {
			return false, "an authenticated user session is present", nil
		}
		if session.ID == sessionID {
			console = session
		}
	}
	if console == nil || console.State != windows.WTSConnected || !strings.EqualFold(console.Station, "Console") || console.User != "" {
		return false, "the physical console is not an empty connected login session", nil
	}
	processes, err := deps.processes(sessionID)
	if err != nil {
		return false, "", fmt.Errorf("inspect console processes: %w", err)
	}
	if processes.Explorer != 0 {
		return false, "Explorer is present in the physical-console session", nil
	}
	if processes.LogonUI != 1 || processes.Winlogon != 1 {
		return false, "the Windows login surface is not ready", nil
	}
	return true, "", nil
}

func enumerateWTSSessions() ([]wtsSession, error) {
	var raw *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &raw, &count); err != nil {
		return nil, err
	}
	if raw == nil || count == 0 {
		return nil, nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(raw)))
	entries := unsafe.Slice(raw, int(count))
	result := make([]wtsSession, 0, len(entries))
	for _, entry := range entries {
		user, err := queryWTSSessionString(entry.SessionID, 5) // WTSUserName
		if err != nil {
			return nil, err
		}
		station := ""
		if entry.WindowStationName != nil {
			station = windows.UTF16PtrToString(entry.WindowStationName)
		}
		result = append(result, wtsSession{ID: entry.SessionID, Station: station, User: user, State: entry.State})
	}
	return result, nil
}

func queryWTSSessionString(sessionID uint32, informationClass uint32) (string, error) {
	var buffer *uint16
	var bytes uint32
	r1, _, callErr := procWTSQuerySessionInfo.Call(
		0,
		uintptr(sessionID),
		uintptr(informationClass),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 {
		if callErr == windows.ERROR_SUCCESS {
			callErr = windows.ERROR_GEN_FAILURE
		}
		return "", callErr
	}
	if buffer == nil {
		return "", nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	return windows.UTF16PtrToString(buffer), nil
}

func processesInSession(sessionID uint32) (sessionProcesses, error) {
	result := sessionProcesses{}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return result, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return result, err
	}
	for {
		var processSession uint32
		if windows.ProcessIdToSessionId(entry.ProcessID, &processSession) == nil && processSession == sessionID {
			switch strings.ToLower(windows.UTF16ToString(entry.ExeFile[:])) {
			case "logonui.exe":
				result.LogonUI++
			case "winlogon.exe":
				result.Winlogon++
			case "explorer.exe":
				result.Explorer++
			}
		}
		err := windows.Process32Next(snapshot, &entry)
		if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
			break
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func disconnectWTSSession(sessionID uint32) error {
	r1, _, callErr := procWTSDisconnect.Call(0, uintptr(sessionID), 0)
	if r1 != 0 {
		return nil
	}
	if callErr == windows.ERROR_SUCCESS {
		return windows.ERROR_GEN_FAILURE
	}
	return callErr
}

func readPreLoginRefreshMarker(path string) (preLoginRefreshMarker, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return preLoginRefreshMarker{}, err
	}
	var marker preLoginRefreshMarker
	if err := json.Unmarshal(b, &marker); err != nil {
		return preLoginRefreshMarker{}, err
	}
	return marker, nil
}

func writeMarkerOutcome(path string, marker preLoginRefreshMarker) error {
	if err := writePreLoginRefreshMarker(path, marker); err != nil {
		return fmt.Errorf("record pre-login refresh outcome: %w", err)
	}
	return nil
}

func writePreLoginRefreshMarker(path string, marker preLoginRefreshMarker) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
