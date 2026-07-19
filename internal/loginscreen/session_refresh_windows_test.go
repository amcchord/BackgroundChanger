package loginscreen

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPreLoginRefreshRotatesOnlyOncePerBoot(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pre-login-refresh.json")
	disconnects := 0
	deps := eligibleRefreshDeps()
	deps.disconnect = func(sessionID uint32) error {
		disconnects++
		if sessionID != 7 {
			t.Fatalf("disconnect session = %d, want 7", sessionID)
		}
		return nil
	}
	result, err := refreshPreLoginSession(marker, deps)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Attempted || !result.Refreshed || result.SessionID != 7 || disconnects != 1 {
		t.Fatalf("first result = %#v, disconnects = %d", result, disconnects)
	}
	result, err = refreshPreLoginSession(marker, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempted || result.Refreshed || result.SkippedReason != "already evaluated this boot" || disconnects != 1 {
		t.Fatalf("second result = %#v, disconnects = %d", result, disconnects)
	}
}

func TestWindowsBootIDIsStable(t *testing.T) {
	first, err := windowsBootID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := windowsBootID()
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("boot IDs = %q and %q", first, second)
	}
}

func TestPreLoginRefreshSkipsAuthenticatedSession(t *testing.T) {
	deps := eligibleRefreshDeps()
	deps.sessions = func() ([]wtsSession, error) {
		return []wtsSession{
			{ID: 7, Station: "Console", State: windows.WTSConnected},
			{ID: 8, Station: "RDP-Tcp#1", User: "operator", State: windows.WTSActive},
		}, nil
	}
	result, err := refreshPreLoginSession(filepath.Join(t.TempDir(), "marker.json"), deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempted || result.Refreshed || !strings.Contains(result.SkippedReason, "authenticated user") {
		t.Fatalf("result = %#v", result)
	}
}

func TestPreLoginRefreshRechecksConsoleBeforeDisconnect(t *testing.T) {
	deps := eligibleRefreshDeps()
	calls := 0
	deps.activeConsole = func() uint32 {
		calls++
		if calls == 1 {
			return 7
		}
		return 9
	}
	result, err := refreshPreLoginSession(filepath.Join(t.TempDir(), "marker.json"), deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Attempted || !strings.Contains(result.SkippedReason, "changed during") {
		t.Fatalf("result = %#v", result)
	}
}

func TestPreLoginRefreshReportsDisconnectFailure(t *testing.T) {
	deps := eligibleRefreshDeps()
	deps.disconnect = func(uint32) error { return errors.New("access denied") }
	result, err := refreshPreLoginSession(filepath.Join(t.TempDir(), "marker.json"), deps)
	if err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Fatalf("error = %v", err)
	}
	if !result.Attempted || result.Refreshed {
		t.Fatalf("result = %#v", result)
	}
}

func eligibleRefreshDeps() preLoginRefreshDeps {
	return preLoginRefreshDeps{
		bootID:        func() (string, error) { return "2026-07-19T14:37:13.5Z", nil },
		now:           func() time.Time { return time.Unix(20000, 0).UTC() },
		pause:         func(time.Duration) {},
		activeConsole: func() uint32 { return 7 },
		sessions: func() ([]wtsSession, error) {
			return []wtsSession{{ID: 7, Station: "Console", State: windows.WTSConnected}}, nil
		},
		processes: func(uint32) (sessionProcesses, error) {
			return sessionProcesses{LogonUI: 1, Winlogon: 1}, nil
		},
		disconnect: func(uint32) error { return nil },
	}
}
