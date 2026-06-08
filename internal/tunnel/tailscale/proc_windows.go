//go:build windows
// +build windows

package tailscale

import "syscall"

func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func killProcessGroup(pid int) error {
	return nil
}
