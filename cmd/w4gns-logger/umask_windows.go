//go:build windows

package main

// setUmask is a no-op on Windows, which has no umask concept; file
// permission bits are simulated differently there. Test-only use.
func setUmask(new int) (old int) {
	return 0
}
