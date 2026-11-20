package main

// serviceStopRequested is closed when the Windows Service Control Manager asks the service
// to stop or shut down; the main loop then runs the same graceful shutdown as SIGTERM.
var serviceStopRequested = make(chan struct{})
