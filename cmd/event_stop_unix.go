//go:build !windows

package cmd

import "syscall"

func eventStopSignalName(force bool) string {
	if force {
		return syscall.SIGKILL.String()
	}
	return syscall.SIGTERM.String()
}

func eventStopProcess(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(pid, sig)
}
