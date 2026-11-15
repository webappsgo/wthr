//go:build windows
// +build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// restartProcess starts a fresh copy of the binary; the caller exits afterwards because
// Windows cannot replace a running process image.
func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Start()
}

var platformSignals = []syscall.Signal{}
