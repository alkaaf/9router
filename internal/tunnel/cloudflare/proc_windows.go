//go:build windows
// +build windows

package cloudflare

import "syscall"

// processGroupAttr returns SysProcAttr settings suitable for Windows.
// Windows doesn't have process groups in the Unix sense; we just inherit
// the default behaviour.
func processGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

// killProcessGroup is a no-op on Windows. Process tree cleanup on Windows
// is handled by Job Objects, which would be set up at process spawn time
// if we needed it. For now, the direct Kill() on the child is sufficient.
func killProcessGroup(pid int) error {
	return nil
}
