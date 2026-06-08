//go:build !windows
// +build !windows

package cloudflare

import "syscall"

// processGroupAttr returns SysProcAttr settings that put the child into
// its own process group so we can signal the whole tree on cleanup.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the process group rooted at pid.
func killProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}
