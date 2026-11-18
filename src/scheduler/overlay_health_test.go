package scheduler

import (
	"errors"
	"testing"
)

type fakeOverlay struct {
	enabled    bool
	status     string
	restartErr error
	restarts   int
}

func (f *fakeOverlay) Enabled() bool  { return f.enabled }
func (f *fakeOverlay) Status() string { return f.status }
func (f *fakeOverlay) Restart() error {
	f.restarts++
	return f.restartErr
}

func TestCheckOverlayHealthNilService(t *testing.T) {
	if err := CheckOverlayHealth("tor", nil, true); err != nil {
		t.Errorf("nil service must be healthy, got %v", err)
	}
}

func TestCheckOverlayHealthHealthyStates(t *testing.T) {
	for _, status := range []string{"healthy", "starting", "disabled"} {
		svc := &fakeOverlay{enabled: true, status: status}
		if err := CheckOverlayHealth("tor", svc, true); err != nil {
			t.Errorf("status %q: %v", status, err)
		}
		if svc.restarts != 0 {
			t.Errorf("status %q must not restart", status)
		}
	}
}

func TestCheckOverlayHealthDisabledServiceSkipped(t *testing.T) {
	svc := &fakeOverlay{enabled: false, status: "unavailable"}
	if err := CheckOverlayHealth("i2p", svc, true); err != nil || svc.restarts != 0 {
		t.Errorf("disabled service must be skipped, err=%v restarts=%d", err, svc.restarts)
	}
}

func TestCheckOverlayHealthUnavailable(t *testing.T) {
	noRestart := &fakeOverlay{enabled: true, status: "unavailable"}
	if err := CheckOverlayHealth("tor", noRestart, false); err == nil {
		t.Error("unavailable service without restart_on_fail must fail the run")
	}
	if noRestart.restarts != 0 {
		t.Error("restart_on_fail=false must not restart")
	}

	restarted := &fakeOverlay{enabled: true, status: "unavailable"}
	if err := CheckOverlayHealth("tor", restarted, true); err != nil || restarted.restarts != 1 {
		t.Errorf("restart expected, err=%v restarts=%d", err, restarted.restarts)
	}

	failing := &fakeOverlay{enabled: true, status: "unavailable", restartErr: errors.New("boom")}
	if err := CheckOverlayHealth("tor", failing, true); err == nil {
		t.Error("a failed restart must fail the run")
	}
}
