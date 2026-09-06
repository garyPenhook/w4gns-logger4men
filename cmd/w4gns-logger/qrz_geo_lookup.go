package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

// qrzGeoConcurrency bounds how many QRZ XML lookups the map feed runs at
// once, so a burst of many newly seen callsigns (e.g. right after connecting
// to the cluster, or during a high spot-rate contest) queues through a small
// shared budget instead of firing dozens of simultaneous requests at QRZ.
const qrzGeoConcurrency = 2

// qrzGeoSemaphore is package-level rather than per-model: there is only ever
// one running instance of this program, the same convention sharedDXCCTable
// uses for its own process-wide singleton.
var qrzGeoSemaphore = make(chan struct{}, qrzGeoConcurrency)

// qrzMapGeoMsg carries a map-feed QRZ location lookup result, keyed by
// callsign rather than a request ID: unlike the QSO Entry auto-fill flow
// (qrzCallsignLookupMsg), many of these can be in flight concurrently, one
// per newly seen cluster callsign, and the result is looked up by call in
// qrzGeoCache rather than bound to a single active form.
type qrzMapGeoMsg struct {
	call       string
	record     qrzCallsignRecord
	sessionKey string
	err        error
}

// qrzMapGeoLookupCmd looks up call's QRZ profile for the map feed's location
// cache. It reuses lookupQRZCallsignCmdForRequest's login/session-retry
// plumbing (requestID 0, the same sentinel its own compatibility path uses)
// but relabels the result into qrzMapGeoMsg so this flow never shares
// request-ID state with the QSO Entry form's single active-lookup tracking.
func qrzMapGeoLookupCmd(creds qrzXMLCreds, sessionKey, call string) tea.Cmd {
	inner := lookupQRZCallsignCmdForRequest(creds, sessionKey, call, 0)
	if inner == nil {
		return nil
	}
	return func() tea.Msg {
		qrzGeoSemaphore <- struct{}{}
		defer func() { <-qrzGeoSemaphore }()
		msg := inner().(qrzCallsignLookupMsg)
		return qrzMapGeoMsg{call: msg.call, record: msg.record, sessionKey: msg.sessionKey, err: msg.err}
	}
}
