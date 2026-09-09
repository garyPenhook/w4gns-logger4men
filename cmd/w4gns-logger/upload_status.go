package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

func uploadBinding(key, logbook string) string {
	if key == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key+"\x00"+logbook)))
}
func (m model) uploadBindings() map[string]string {
	return map[string]string{uploadDestQRZ: uploadBinding(m.qrzAPIKey, ""), uploadDestWRL: uploadBinding(m.wrlAPIKey, m.wrlLogbookID), uploadDestLoTW: m.lotwBinding()}
}

// lotwBinding ties an in-flight LoTW outbox row to the station location and
// resolved tqsl binary used to sign it. Changing either (or losing tqsl from
// PATH) invalidates in-flight rows the same way a changed QRZ key does — the
// drain pauses them with a "missing or changed credentials" last_error rather
// than silently signing under a different location, or failing to find tqsl
// at all every drain tick.
func (m model) lotwBinding() string {
	station := strings.TrimSpace(m.lotwStation)
	if station == "" {
		return ""
	}
	tqslPath, err := findTQSL()
	if err != nil {
		return ""
	}
	return uploadBinding(station, tqslPath)
}
func (m *model) refreshUploadStatus() {
	var count, failed, exhausted int
	err := m.store.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(last_error IS NOT NULL),0),COALESCE(SUM(attempts>=20),0) FROM upload_outbox WHERE profile_id=?`, m.activeStation.ID).Scan(&count, &failed, &exhausted)
	if err != nil {
		m.uploadQueueStatus = "Upload queue unavailable: " + err.Error()
		return
	}
	m.uploadQueueStatus = fmt.Sprintf("Uploads: %d pending, %d need attention, %d exhausted — Ctrl+U retries failed/paused with current credentials", count, failed, exhausted)
	if failed > 0 {
		var last string
		if err := m.store.db.QueryRow(`SELECT last_error FROM upload_outbox WHERE profile_id=? AND last_error IS NOT NULL ORDER BY next_attempt_at DESC LIMIT 1`, m.activeStation.ID).Scan(&last); err == nil {
			m.uploadQueueStatus += "\nLast upload error: " + sanitizeClusterText(last)
		}
	}
	var destination, call, status, refID, occurredAt string
	if err := m.store.db.QueryRow(`SELECT destination, call, status, COALESCE(ref_id,''), occurred_at FROM upload_log ORDER BY id DESC LIMIT 1`).Scan(&destination, &call, &status, &refID, &occurredAt); err == nil {
		summary := fmt.Sprintf("Last delivery: %s to %s %s at %s", strings.ToUpper(destination), call, status, occurredAt)
		if status == uploadLogSent && refID != "" {
			summary += " (ref " + refID + ")"
		}
		m.uploadQueueStatus += "\n" + summary
	}
}

// Ctrl+U explicitly reassigns failed/paused work to the currently configured
// destinations. Fresh/in-flight rows without an error keep their leases.
func (m *model) retryFailedUploads() {
	m.qrzAPIKey = loadQRZAPIKey()
	m.wrlAPIKey = loadWRLAPIKey()
	m.wrlLogbookID = loadWRLLogbookID()
	m.lotwStation = loadLoTWStation()
	m.lotwPass = loadLoTWPass()
	for dest, binding := range m.uploadBindings() {
		if binding == "" {
			continue
		}
		if _, err := m.store.db.Exec(`UPDATE upload_outbox SET attempts=0,next_attempt_at=?,last_error=NULL,binding=? WHERE profile_id=? AND destination=? AND last_error IS NOT NULL`, time.Now().UTC().Format(time.RFC3339), binding, m.activeStation.ID, dest); err != nil {
			m.statusMsg = err.Error()
			return
		}
	}
	m.refreshUploadStatus()
	m.statusMsg = "failed/paused uploads queued with current configured destinations"
}
