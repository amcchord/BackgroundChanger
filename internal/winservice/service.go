// Package winservice contains the long-running pre-login refresh service.
package winservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/engine"
	"github.com/amcchord/WallpaperIdentity/v4/internal/loginscreen"
	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	wtsSessionLogon  = 0x5
	wtsSessionLogoff = 0x6
	wtsSessionLock   = 0x7
	refreshControl   = svc.Cmd(128)
)

type handler struct{}

type serviceJob struct {
	reason  string
	restore bool
	done    chan error
}

func Run() error { return svc.Run(paths.ServiceName, &handler{}) }

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	accepted := svc.AcceptStop | svc.AcceptShutdown | svc.AcceptSessionChange | svc.AcceptPowerEvent | svc.AcceptParamChange
	changes <- svc.Status{State: svc.StartPending, WaitHint: 60000, CheckPoint: 1}
	logger, closeLog := serviceLogger()
	defer closeLog()
	worker := engine.New(logger)
	_, initialErr := worker.Refresh("service-start")
	if initialErr != nil {
		logger.Printf("initial refresh did not complete: %v", initialErr)
	}
	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	cfg, err := config.LoadOrCreate(paths.ConfigFile())
	if err != nil {
		cfg = config.Default()
	}
	ticker := time.NewTicker(time.Duration(cfg.RefreshMinutes) * time.Minute)
	defer ticker.Stop()
	settle := time.NewTimer(20 * time.Second)
	defer settle.Stop()
	trigger := make(chan serviceJob, 1)
	refreshDisabled := false
	var refreshWG sync.WaitGroup
	startRefresh := func(reason string) {
		if !refreshDisabled {
			enqueueRefresh(trigger, reason, false)
		}
	}
	refreshWG.Add(1)
	go func() {
		defer refreshWG.Done()
		for job := range trigger {
			if job.restore {
				job.done <- loginscreen.RestoreMDMBridge()
				continue
			}
			_, _ = worker.Refresh(job.reason)
		}
	}()

	for {
		select {
		case <-ticker.C:
			startRefresh("interval")
		case <-settle.C:
			startRefresh("boot-settled")
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending, WaitHint: 60000}
				close(trigger)
				refreshWG.Wait()
				return false, 0
			case svc.SessionChange:
				if request.EventType == wtsSessionLock || request.EventType == wtsSessionLogoff || request.EventType == wtsSessionLogon {
					startRefresh("session-change")
				}
			case svc.PowerEvent:
				startRefresh("power-event")
			case svc.ParamChange:
				// Stop accepting refreshes, discard any queued refresh, then run
				// restore behind an in-flight refresh on the same worker. Keeping
				// refreshes disabled after success prevents policy reapplication
				// between the acknowledgement and service removal.
				refreshDisabled = true
				done := make(chan error, 1)
				enqueueServiceJob(trigger, serviceJob{restore: true, done: done}, true)
				restoreErr := <-done
				marker := "ok"
				if restoreErr != nil {
					refreshDisabled = false
					marker = restoreErr.Error()
					logger.Printf("MDM policy restore failed: %v", restoreErr)
				}
				_ = os.WriteFile(paths.MDMRestoreMarker(), []byte(marker), 0o600)
			case refreshControl:
				if !refreshDisabled {
					enqueueRefresh(trigger, "cli", true)
				}
			}
		}
	}
}

func enqueueRefresh(trigger chan serviceJob, reason string, replaceQueued bool) {
	enqueueServiceJob(trigger, serviceJob{reason: reason}, replaceQueued)
}

func enqueueServiceJob(trigger chan serviceJob, job serviceJob, replaceQueued bool) {
	select {
	case trigger <- job:
		return
	default:
	}
	if !replaceQueued {
		return
	}
	select {
	case <-trigger:
	default:
	}
	select {
	case trigger <- job:
	default:
	}
}

// RequestRefresh asks the installed LocalSystem service to refresh and waits
// for the corresponding status record. Running the work in the service keeps
// the LocalSystem-only Personalization CSP behavior identical for GUI and RMM
// callers.
func RequestRefresh(timeout time.Duration) (engine.Status, error) {
	manager, err := mgr.Connect()
	if err != nil {
		return engine.Status{}, fmt.Errorf("connect to Service Control Manager: %w", err)
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(paths.ServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return engine.Status{}, errors.New("Wallpaper Identity is not installed")
		}
		return engine.Status{}, fmt.Errorf("open service: %w", err)
	}
	defer service.Close()
	state, err := service.Query()
	if err != nil {
		return engine.Status{}, fmt.Errorf("query service: %w", err)
	}
	if state.State != svc.Running {
		return engine.Status{}, fmt.Errorf("service is not running (state %d)", state.State)
	}
	previous, _ := readStatus()
	latest := previous
	if _, err := service.Control(refreshControl); err != nil {
		return engine.Status{}, fmt.Errorf("send refresh control: %w", err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := readStatus()
		if err == nil {
			latest = status
			if status.Reason == "cli" && status.CompletedAt.After(previous.CompletedAt) {
				if !status.Success {
					return status, errors.New(status.Error)
				}
				return status, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return latest, fmt.Errorf("timed out after %s waiting for the LocalSystem refresh", timeout.Round(time.Second))
}

func readStatus() (engine.Status, error) {
	b, err := os.ReadFile(paths.StatusFile())
	if err != nil {
		return engine.Status{}, err
	}
	var status engine.Status
	if err := json.Unmarshal(b, &status); err != nil {
		return engine.Status{}, err
	}
	return status, nil
}

func serviceLogger() (*log.Logger, func()) {
	_ = os.MkdirAll(paths.DataDir(), 0o755)
	f, err := os.OpenFile(paths.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	return log.New(f, "", log.LstdFlags), func() { _ = f.Close() }
}
