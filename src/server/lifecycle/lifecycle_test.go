package lifecycle

import "testing"

func TestBeginShutdown(t *testing.T) {
	Reset()
	defer Reset()

	if IsShuttingDown() {
		t.Fatal("fresh state must not be shutting down")
	}
	select {
	case <-Done():
		t.Fatal("Done closed before shutdown")
	default:
	}

	BeginShutdown()
	BeginShutdown()

	if !IsShuttingDown() {
		t.Fatal("IsShuttingDown = false after BeginShutdown")
	}
	select {
	case <-Done():
	default:
		t.Fatal("Done not closed after BeginShutdown")
	}
}
