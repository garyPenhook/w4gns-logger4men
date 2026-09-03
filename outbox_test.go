package main

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *store {
	t.Helper()
	st, err := openStore(filepath.Join(t.TempDir(), "logger.db"))
	if err != nil {
		t.Fatalf("openStore returned error: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestOutboxEnqueueIsIdempotent confirms re-enqueuing the same (qso,
// destination) doesn't create a duplicate delivery.
func TestOutboxEnqueueIsIdempotent(t *testing.T) {
	st := openTestStore(t)
	now := time.Now()
	if err := st.enqueueUpload(1, 1, uploadDestQRZ, now); err != nil {
		t.Fatalf("enqueueUpload: %v", err)
	}
	if err := st.enqueueUpload(1, 1, uploadDestQRZ, now.Add(time.Hour)); err != nil {
		t.Fatalf("enqueueUpload (repeat): %v", err)
	}
	entries, err := st.claimDueUploads(now.Add(2*time.Hour), time.Minute, 10)
	if err != nil {
		t.Fatalf("claimDueUploads: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("claimed %d entries, want 1 (enqueue must be idempotent)", len(entries))
	}
}

// TestOutboxClaimLeasesAndDoesNotDoubleClaim confirms a claimed delivery is
// pushed past `now` by the lease, so a second immediate drain won't re-send
// the same in-flight upload.
func TestOutboxClaimLeasesAndDoesNotDoubleClaim(t *testing.T) {
	st := openTestStore(t)
	now := time.Now()
	if err := st.enqueueUpload(7, 1, uploadDestWRL, now.Add(-time.Second)); err != nil {
		t.Fatalf("enqueueUpload: %v", err)
	}
	first, err := st.claimDueUploads(now, 90*time.Second, 10)
	if err != nil || len(first) != 1 {
		t.Fatalf("first claim = %d entries, err = %v, want 1", len(first), err)
	}
	second, err := st.claimDueUploads(now, 90*time.Second, 10)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("second immediate claim = %d entries, want 0 (leased in-flight)", len(second))
	}
	// After the lease elapses the same delivery becomes claimable again.
	third, err := st.claimDueUploads(now.Add(2*time.Minute), 90*time.Second, 10)
	if err != nil || len(third) != 1 {
		t.Fatalf("post-lease claim = %d entries, err = %v, want 1", len(third), err)
	}
}

// TestOutboxMarkDoneRemovesEntry confirms a delivered upload is removed and
// never claimed again.
func TestOutboxMarkDoneRemovesEntry(t *testing.T) {
	st := openTestStore(t)
	now := time.Now()
	if err := st.enqueueUpload(3, 1, uploadDestQRZ, now.Add(-time.Second)); err != nil {
		t.Fatalf("enqueueUpload: %v", err)
	}
	if err := st.markUploadDone(3, uploadDestQRZ); err != nil {
		t.Fatalf("markUploadDone: %v", err)
	}
	entries, err := st.claimDueUploads(now.Add(time.Hour), time.Minute, 10)
	if err != nil {
		t.Fatalf("claimDueUploads: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("claimed %d entries after markUploadDone, want 0", len(entries))
	}
}

// TestOutboxRecordFailureReschedulesWithBackoff confirms a failed delivery
// isn't retried immediately and stops auto-retrying once its budget is spent.
func TestOutboxRecordFailureReschedulesWithBackoff(t *testing.T) {
	st := openTestStore(t)
	now := time.Now()
	if err := st.enqueueUpload(5, 1, uploadDestQRZ, now.Add(-time.Second)); err != nil {
		t.Fatalf("enqueueUpload: %v", err)
	}
	if _, err := st.claimDueUploads(now, 90*time.Second, 10); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := st.recordUploadFailure(5, uploadDestQRZ, "boom", now); err != nil {
		t.Fatalf("recordUploadFailure: %v", err)
	}
	// One minute out (first backoff) it is not yet due.
	if entries, err := st.claimDueUploads(now.Add(30*time.Second), 90*time.Second, 10); err != nil || len(entries) != 0 {
		t.Fatalf("claim 30s after failure = %d entries, err = %v, want 0", len(entries), err)
	}
	// Past the first backoff it becomes due again.
	if entries, err := st.claimDueUploads(now.Add(2*time.Minute), 90*time.Second, 10); err != nil || len(entries) != 1 {
		t.Fatalf("claim 2m after failure = %d entries, err = %v, want 1", len(entries), err)
	}
}

// TestUploadBackoffCapsAndParks verifies the backoff schedule: it grows, caps
// at 30 minutes, and parks far in the future once attempts are exhausted.
func TestUploadBackoffCapsAndParks(t *testing.T) {
	if got := uploadBackoff(1); got != time.Minute {
		t.Errorf("uploadBackoff(1) = %v, want 1m", got)
	}
	if got := uploadBackoff(10); got != 30*time.Minute {
		t.Errorf("uploadBackoff(10) = %v, want the 30m cap", got)
	}
	if got := uploadBackoff(maxUploadAttempts); got < 24*time.Hour {
		t.Errorf("uploadBackoff(%d) = %v, want it parked far out", maxUploadAttempts, got)
	}
}
