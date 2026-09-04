//go:build windows

package main

import "golang.org/x/sys/windows"

// replaceFileAtomic uses MoveFileEx rather than os.Rename: Windows' rename
// refuses an existing destination, while contest exports intentionally reuse a
// stable filename. MOVEFILE_REPLACE_EXISTING preserves atomic replacement
// semantics without deleting the previous valid export first.
func replaceFileAtomic(tempPath, destination string) error {
	return windows.MoveFileEx(
		windows.StringToUTF16Ptr(tempPath),
		windows.StringToUTF16Ptr(destination),
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}
