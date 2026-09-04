//go:build !windows

package main

import "syscall"

// setUmask wraps syscall.Umask, which Windows doesn't have — see
// umask_windows.go for the cross-platform stub. Test-only use (confirming
// openStore's permission self-healing actually has something to heal).
func setUmask(new int) (old int) {
	return syscall.Umask(new)
}
