package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reserve atomically: concurrent app instances cannot overwrite one another.
func reserveExportPath(path string) (string, error) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 0; ; i++ {
		candidate := path
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		f, err := os.OpenFile(candidate, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("reserve export: %w", err)
		}
		if err := f.Close(); err != nil {
			os.Remove(candidate)
			return "", err
		}
		return candidate, nil
	}
}
