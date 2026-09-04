//go:build !windows

package main

import "os"

// replaceFileAtomic publishes a fully-written temporary file. POSIX rename
// atomically replaces an existing destination, preserving either the old
// complete export or the new complete export across interruption.
func replaceFileAtomic(tempPath, destination string) error {
	return os.Rename(tempPath, destination)
}
