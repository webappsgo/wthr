package scheduler

import (
	"fmt"
	"log"
)

// OverlayHealth is the view of an overlay network service (Tor hidden service,
// I2P eepsite) that the tor_health and i2p_health tasks need. Status returns one
// of the AI.md PART 13 words: disabled, starting, healthy or unavailable.
type OverlayHealth interface {
	Enabled() bool
	Status() string
	Restart() error
}

// CheckOverlayHealth is the body of the tor_health and i2p_health tasks (AI.md PART 19).
// A disabled service and a service that is still starting are healthy for the
// purposes of the task; an unavailable service is restarted when restartOnFail is
// set. Overlay failures never stop the server: the error only marks the task run
// failed so the scheduler records it and applies its retry policy.
func CheckOverlayHealth(name string, service OverlayHealth, restartOnFail bool) error {
	if service == nil || !service.Enabled() {
		return nil
	}

	switch service.Status() {
	case "healthy", "starting", "disabled":
		return nil
	}

	log.Printf("WARNING: %s health check: service is unavailable", name)
	if !restartOnFail {
		return fmt.Errorf("%s is unavailable", name)
	}

	log.Printf("INFO: %s health check: restarting the service", name)
	if err := service.Restart(); err != nil {
		return fmt.Errorf("%s restart failed: %w", name, err)
	}
	return nil
}
