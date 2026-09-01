package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	backupRemote    = "gdrive"
	backupRemoteDir = "W4GNS_Logger_Backups"
	backupKeepCount = 5
	backupTimeout   = 120 * time.Second
)

type backupResult struct {
	dbName   string
	adifName string
}

// backupMu serializes every backup, whether triggered by F8 or by the
// mandatory backup-on-exit in main(). bubbletea does not wait for in-flight
// tea.Cmd goroutines before p.Run() returns, so an F8 backup can still be
// mid-flight (VACUUM INTO / rclone upload) when the exit path starts its own
// backup; running both at once risks a corrupt/partial snapshot and
// second-resolution filename collisions. The exit backup should simply wait
// its turn rather than race the in-flight one.
var backupMu sync.Mutex

// runBackupSerialized is the only entry point callers should use; it wraps
// runBackup with backupMu so at most one backup runs at a time.
func runBackupSerialized(ctx context.Context, st *store, profileID int64) (backupResult, error) {
	backupMu.Lock()
	defer backupMu.Unlock()
	return runBackup(ctx, st, profileID)
}

// runBackup snapshots the database with SQLite's VACUUM INTO (a consistent
// copy that is safe to take even while the app keeps writing), exports the
// current ADIF, uploads both to Google Drive via rclone, and prunes older
// backups so no more than backupKeepCount copies of each file type remain.
func runBackup(ctx context.Context, st *store, profileID int64) (backupResult, error) {
	rclonePath, err := exec.LookPath("rclone")
	if err != nil {
		return backupResult{}, fmt.Errorf("rclone not found in PATH: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "w4gns-backup")
	if err != nil {
		return backupResult{}, fmt.Errorf("create backup staging dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	stamp := time.Now().UTC().Format("20060102-150405")
	dbName := fmt.Sprintf("w4gns-%s.db", stamp)
	adifName := fmt.Sprintf("w4gns-%s.adi", stamp)
	dbStaged := filepath.Join(tempDir, dbName)
	adifStaged := filepath.Join(tempDir, adifName)

	if _, err := st.db.ExecContext(ctx, `VACUUM INTO ?`, dbStaged); err != nil {
		return backupResult{}, fmt.Errorf("snapshot database: %w", err)
	}

	adifFile, err := os.Create(adifStaged)
	if err != nil {
		return backupResult{}, fmt.Errorf("create staged ADIF backup: %w", err)
	}
	if _, err := exportADIF(ctx, adifFile, profileID, st); err != nil {
		adifFile.Close()
		return backupResult{}, fmt.Errorf("export ADIF backup: %w", err)
	}
	if err := adifFile.Close(); err != nil {
		return backupResult{}, fmt.Errorf("close staged ADIF backup: %w", err)
	}

	remoteDir := backupRemote + ":" + backupRemoteDir
	for _, staged := range []string{dbStaged, adifStaged} {
		if err := rcloneCopyTo(ctx, rclonePath, staged, remoteDir+"/"+filepath.Base(staged)); err != nil {
			return backupResult{}, fmt.Errorf("upload %s: %w", filepath.Base(staged), err)
		}
	}

	result := backupResult{dbName: dbName, adifName: adifName}
	if err := pruneRemoteBackups(ctx, rclonePath, remoteDir, "w4gns-*.db"); err != nil {
		return result, fmt.Errorf("prune old database backups: %w", err)
	}
	if err := pruneRemoteBackups(ctx, rclonePath, remoteDir, "w4gns-*.adi"); err != nil {
		return result, fmt.Errorf("prune old ADIF backups: %w", err)
	}
	return result, nil
}

func rcloneCopyTo(ctx context.Context, rclonePath, src, dst string) error {
	cmd := exec.CommandContext(ctx, rclonePath, "copyto", src, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// pruneRemoteBackups keeps the backupKeepCount most recent objects matching
// pattern in remoteDir (filenames are timestamp-sortable) and deletes the rest.
func pruneRemoteBackups(ctx context.Context, rclonePath, remoteDir, pattern string) error {
	cmd := exec.CommandContext(ctx, rclonePath, "lsf", remoteDir, "--include", pattern)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list %s: %w", pattern, err)
	}
	var names []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	sort.Strings(names)
	if len(names) <= backupKeepCount {
		return nil
	}
	for _, name := range names[:len(names)-backupKeepCount] {
		cmd := exec.CommandContext(ctx, rclonePath, "deletefile", remoteDir+"/"+name)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("delete %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}
