package winservice

import "testing"

func TestEnqueueRefreshReplacesQueuedEventForCLI(t *testing.T) {
	trigger := make(chan serviceJob, 1)
	enqueueRefresh(trigger, "interval", false)
	enqueueRefresh(trigger, "cli", true)
	if got := <-trigger; got.reason != "cli" {
		t.Fatalf("queued reason = %q, want cli", got.reason)
	}
}

func TestEnqueueRefreshCoalescesOrdinaryEvents(t *testing.T) {
	trigger := make(chan serviceJob, 1)
	enqueueRefresh(trigger, "interval", false)
	enqueueRefresh(trigger, "session-change", false)
	if got := <-trigger; got.reason != "interval" {
		t.Fatalf("queued reason = %q, want first event", got.reason)
	}
}

func TestRestoreJobReplacesQueuedRefresh(t *testing.T) {
	trigger := make(chan serviceJob, 1)
	enqueueRefresh(trigger, "interval", false)
	done := make(chan error, 1)
	enqueueServiceJob(trigger, serviceJob{restore: true, done: done}, true)
	if got := <-trigger; !got.restore || got.done != done {
		t.Fatalf("queued job = %#v, want restore", got)
	}
}
