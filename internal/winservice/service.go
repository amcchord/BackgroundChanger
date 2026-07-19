// Package winservice contains the long-running pre-login refresh service.
package winservice

import (
	"log"
	"os"
	"sync"
	"time"

	"github.com/amcchord/BackgroundChanger/internal/config"
	"github.com/amcchord/BackgroundChanger/internal/engine"
	"github.com/amcchord/BackgroundChanger/internal/loginscreen"
	"github.com/amcchord/BackgroundChanger/internal/paths"
	"golang.org/x/sys/windows/svc"
)

const (
	wtsSessionLogon  = 0x5
	wtsSessionLogoff = 0x6
	wtsSessionLock   = 0x7
)

type handler struct{}

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
	trigger := make(chan string, 1)
	var refreshWG sync.WaitGroup
	startRefresh := func(reason string) {
		select {
		case trigger <- reason:
		default:
		}
	}
	refreshWG.Add(1)
	go func() {
		defer refreshWG.Done()
		for reason := range trigger {
			_, _ = worker.Refresh(reason)
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
				changes <- svc.Status{State: svc.StopPending, WaitHint: 15000}
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
				restoreErr := loginscreen.RestoreMDMBridge()
				marker := "ok"
				if restoreErr != nil {
					marker = restoreErr.Error()
					logger.Printf("MDM policy restore failed: %v", restoreErr)
				}
				_ = os.WriteFile(paths.MDMRestoreMarker(), []byte(marker), 0o600)
			}
		}
	}
}

func serviceLogger() (*log.Logger, func()) {
	_ = os.MkdirAll(paths.DataDir(), 0o755)
	f, err := os.OpenFile(paths.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	return log.New(f, "", log.LstdFlags), func() { _ = f.Close() }
}
