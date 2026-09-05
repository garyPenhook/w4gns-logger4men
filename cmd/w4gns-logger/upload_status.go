package main

import (
	"crypto/sha256"
	"fmt"
	"time"
)

func uploadBinding(key, logbook string) string {
	if key == "" {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(key+"\x00"+logbook)))
}
func (m model) uploadBindings() map[string]string {
	return map[string]string{uploadDestQRZ: uploadBinding(m.qrzAPIKey, ""), uploadDestWRL: uploadBinding(m.wrlAPIKey, m.wrlLogbookID)}
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
}

// Ctrl+U explicitly reassigns failed/paused work to the currently configured
// destinations. Fresh/in-flight rows without an error keep their leases.
func (m *model) retryFailedUploads() {
	m.qrzAPIKey = loadQRZAPIKey()
	m.wrlAPIKey = loadWRLAPIKey()
	m.wrlLogbookID = loadWRLLogbookID()
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
