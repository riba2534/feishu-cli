//go:build windows

package cmd

import "os"

func eventStopSignalName(force bool) string {
	return "KILL"
}

func eventStopProcess(pid int, force bool) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
