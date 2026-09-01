package main

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestBackupMuSerializesConcurrentBackups is a white-box test of the
// serialization primitive runBackupSerialized relies on: at most one backup
// (F8-triggered or the mandatory exit backup) may run its critical section at
// a time, so second-resolution backup filenames never collide and the exit
// backup never races an in-flight F8 backup.
func TestBackupMuSerializesConcurrentBackups(t *testing.T) {
	var active, maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			backupMu.Lock()
			defer backupMu.Unlock()
			n := atomic.AddInt32(&active, 1)
			for {
				observed := atomic.LoadInt32(&maxActive)
				if n <= observed || atomic.CompareAndSwapInt32(&maxActive, observed, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()
	if maxActive > 1 {
		t.Fatalf("backupMu allowed %d concurrent critical sections, want at most 1", maxActive)
	}
}

// TestRunBackupSerializedReportsMissingRclone covers the ordinary failure
// path (no rclone in PATH, as in this test environment) without touching the
// network: it must return a descriptive error rather than panicking or
// hanging, and must always release backupMu.
func TestRunBackupSerializedReportsMissingRclone(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // guarantees rclone is not found
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runBackupSerialized(context.Background(), st, profile.ID); err == nil {
		t.Fatal("expected an error when rclone is not in PATH")
	}

	// backupMu must have been released; a second call should not hang.
	done := make(chan struct{})
	go func() {
		runBackupSerialized(context.Background(), st, profile.ID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBackupSerialized did not release backupMu after returning")
	}
}
