package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// sqliteFilePath resolves the filesystem part of a SQLite DSN for checks
// made before opening it. URI escaping/options must never become part of
// the filename used for permissions or export collision checks. After open,
// pragma_database_list supplies SQLite's authoritative filename.
func sqliteFilePath(dsn string) string {
	path := dsn
	if strings.HasPrefix(dsn, "file:") {
		u, err := url.Parse(dsn)
		if err != nil || (u.Host != "" && u.Host != "localhost") {
			return ""
		}
		if u.Query().Get("mode") == "memory" {
			return ""
		}
		path = u.Path
		if u.Opaque != "" {
			path, err = url.PathUnescape(u.Opaque)
			if err != nil {
				return ""
			}
		}
		if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		path = filepath.FromSlash(path)
	} else if index := strings.IndexByte(dsn, '?'); index >= 1 {
		// modernc.org/sqlite also accepts driver options on ordinary paths.
		path = dsn[:index]
	}
	if path == ":memory:" {
		return ""
	}
	return path
}

// validateExportPath protects every export entry point, including UI CSV
// and Cabrillo exports, using the actual open database rather than its DSN.
func (s *store) validateExportPath(ctx context.Context, path string) error {
	var filename string
	if err := s.db.QueryRowContext(ctx, `SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&filename); err != nil {
		return fmt.Errorf("resolve database filename for export: %w", err)
	}
	if filename != "" {
		for _, target := range []string{filename, filename + "-wal", filename + "-shm"} {
			if pathsReferToSameFile(path, target) {
				return fmt.Errorf("export path must not be the SQLite database or its sidecars")
			}
		}
	}
	return nil
}
