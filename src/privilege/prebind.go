package privilege

import (
	"fmt"
	"net"
	"strconv"
	"sync"
)

var (
	preBoundMutex     sync.Mutex
	preBoundListeners = map[int]net.Listener{}
)

// BindPrivilegedPorts binds every port below 1024 while the process is still root and
// keeps the sockets for the HTTP server to adopt after the privilege drop (AI.md PART 8
// step 8f). Any failure closes the sockets already bound and is returned, so startup
// exits instead of running half bound. Ports at or above 1024 are bound later.
func BindPrivilegedPorts(host string, ports []int) error {
	preBoundMutex.Lock()
	defer preBoundMutex.Unlock()

	bound := make([]int, 0, len(ports))
	for _, port := range ports {
		if port <= 0 || port >= PrivilegedPortLimit {
			continue
		}
		if _, already := preBoundListeners[port]; already {
			continue
		}
		listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			for _, done := range bound {
				_ = preBoundListeners[done].Close()
				delete(preBoundListeners, done)
			}
			return fmt.Errorf("binding privileged port %d: %w", port, err)
		}
		preBoundListeners[port] = listener
		bound = append(bound, port)
	}
	return nil
}

// TakeListener hands over the pre-bound socket of a port exactly once. It returns nil
// when the port was not pre-bound, and the caller then binds it itself.
func TakeListener(port int) net.Listener {
	preBoundMutex.Lock()
	defer preBoundMutex.Unlock()

	listener := preBoundListeners[port]
	delete(preBoundListeners, port)
	return listener
}
