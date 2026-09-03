package main

import (
	"database/sql"
	"fmt"
	"time"
)

// The upload outbox makes external delivery (QRZ Logbook, World Radio League)
// durable. Every logged QSO is enqueued as one row per destination and
// survives process exit, a crash, or a transient upload failure — a periodic
// drain retries pending rows with exponential backoff until each destination
// either accepts the QSO or exhausts its retry budget. This replaces the
// former in-memory 60-second timer, whose single tea.Cmd lost the delivery
// entirely if the app quit (or the upload failed) before it fired.
const (
	uploadDestQRZ = "qrz"
	uploadDestWRL = "wrl"
)

// maxUploadAttempts caps automatic retries per (qso, destination). After this
// many failures the row is parked far in the future rather than deleted, so
// its last_error remains visible and a permanently-rejected QSO stops
// consuming API calls without silently vanishing.
const maxUploadAttempts = 20

// outboxEntry is one pending (qso, destination) delivery claimed for sending.
type outboxEntry struct {
	qsoID       int64
	profileID   int64
	destination string
}

const uploadOutboxSchema = `
CREATE TABLE IF NOT EXISTS upload_outbox (
    qso_id INTEGER NOT NULL,
    profile_id INTEGER NOT NULL,
    destination TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    last_error TEXT,
    created_at TEXT NOT NULL,
    PRIMARY KEY (qso_id, destination)
);
CREATE INDEX IF NOT EXISTS idx_outbox_due ON upload_outbox(next_attempt_at);
`

// enqueueUpload records that qsoID should be delivered to destination no
// earlier than notBefore (used to honor the post-log edit window). INSERT OR
// IGNORE keeps it idempotent: re-enqueuing an already-queued delivery is a
// no-op rather than a duplicate send.
func (s *store) enqueueUpload(qsoID, profileID int64, destination string, notBefore time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO upload_outbox (qso_id, profile_id, destination, attempts, next_attempt_at, created_at)
		 VALUES (?, ?, ?, 0, ?, ?)`,
		qsoID, profileID, destination, notBefore.UTC().Format(time.RFC3339), now,
	)
	if err != nil {
		return fmt.Errorf("enqueue %s upload for qso %d: %w", destination, qsoID, err)
	}
	return nil
}

// claimDueUploads atomically selects deliveries due at now (next_attempt_at <=
// now) and pushes their next_attempt_at forward by lease before returning
// them, so a delivery already in flight isn't re-claimed and double-sent by
// the next drain tick before its result lands. lease must exceed the upload
// timeout. The follow-up markUploadDone / recordUploadFailure sets the real
// next state once the attempt resolves.
func (s *store) claimDueUploads(now time.Time, lease time.Duration, limit int) ([]outboxEntry, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer tx.Rollback()

	nowStr := now.UTC().Format(time.RFC3339)
	rows, err := tx.Query(
		`SELECT qso_id, profile_id, destination FROM upload_outbox
		 WHERE next_attempt_at <= ? ORDER BY next_attempt_at LIMIT ?`,
		nowStr, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query due uploads: %w", err)
	}
	var entries []outboxEntry
	for rows.Next() {
		var e outboxEntry
		if err := rows.Scan(&e.qsoID, &e.profileID, &e.destination); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan due upload: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate due uploads: %w", err)
	}
	rows.Close()

	leaseStr := now.Add(lease).UTC().Format(time.RFC3339)
	for _, e := range entries {
		if _, err := tx.Exec(
			`UPDATE upload_outbox SET next_attempt_at = ? WHERE qso_id = ? AND destination = ?`,
			leaseStr, e.qsoID, e.destination,
		); err != nil {
			return nil, fmt.Errorf("lease due upload: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return entries, nil
}

// markUploadDone removes a delivered (or no-longer-applicable, e.g. its QSO
// was deleted) delivery from the outbox.
func (s *store) markUploadDone(qsoID int64, destination string) error {
	if _, err := s.db.Exec(
		`DELETE FROM upload_outbox WHERE qso_id = ? AND destination = ?`, qsoID, destination,
	); err != nil {
		return fmt.Errorf("mark %s upload done for qso %d: %w", destination, qsoID, err)
	}
	return nil
}

// recordUploadFailure increments the attempt counter, stores the error for
// visibility, and reschedules the delivery with exponential backoff. A row
// that has already been removed (its QSO deleted mid-flight) is left alone.
func (s *store) recordUploadFailure(qsoID int64, destination, errMsg string, now time.Time) error {
	var attempts int
	err := s.db.QueryRow(
		`SELECT attempts FROM upload_outbox WHERE qso_id = ? AND destination = ?`, qsoID, destination,
	).Scan(&attempts)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s upload attempts for qso %d: %w", destination, qsoID, err)
	}
	attempts++
	next := now.Add(uploadBackoff(attempts)).UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`UPDATE upload_outbox SET attempts = ?, next_attempt_at = ?, last_error = ? WHERE qso_id = ? AND destination = ?`,
		attempts, next, errMsg, qsoID, destination,
	); err != nil {
		return fmt.Errorf("reschedule %s upload for qso %d: %w", destination, qsoID, err)
	}
	return nil
}

// uploadBackoff maps an attempt count to a retry delay: 1 minute doubling up
// to a 30-minute ceiling, then effectively parked (a year out) once the retry
// budget is exhausted so a permanently-rejected QSO stops retrying without
// being lost.
func uploadBackoff(attempts int) time.Duration {
	if attempts >= maxUploadAttempts {
		return 365 * 24 * time.Hour
	}
	delay := time.Minute
	for i := 1; i < attempts; i++ {
		delay *= 2
		if delay >= 30*time.Minute {
			return 30 * time.Minute
		}
	}
	return delay
}
