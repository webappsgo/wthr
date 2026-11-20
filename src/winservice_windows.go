//go:build windows

package main

import (
	"log"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/webappsgo/wthr/src/privilege"
)

// windowsServiceStopTimeout is how long the Service Control Manager handler waits for the
// graceful shutdown before it reports the service as stopped.
const windowsServiceStopTimeout = 30 * time.Second

// windowsServiceHandler runs the server under the Windows Service Control Manager
// (AI.md PART 25 "Windows Service").
type windowsServiceHandler struct {
	run func()
}

// Execute starts the server, answers the Service Control Manager and turns a Stop or
// Shutdown request into the graceful shutdown of the server.
func (h *windowsServiceHandler) Execute(args []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	done := make(chan struct{})
	go func() {
		h.run()
		close(done)
	}()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case <-done:
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				close(serviceStopRequested)
				select {
				case <-done:
				case <-time.After(windowsServiceStopTimeout):
				}
				return false, 0
			}
		}
	}
}

// runAsWindowsService runs the server under the Service Control Manager when the process
// was started by it. It returns false when the process runs interactively.
func runAsWindowsService(run func()) bool {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return false
	}

	if err := svc.Run(privilege.InternalName, &windowsServiceHandler{run: run}); err != nil {
		log.Printf("Windows service failed: %v", err)
		os.Exit(1)
	}

	return true
}
