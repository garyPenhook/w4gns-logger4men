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
// per newly seen cluster callsign, and the location result is looked up by
// call in qrzGeoCache rather than bound to a single active form. credGeneration
// is a separate staleness guard for the one piece of state this message
// carries that *isn't* safe to accept from just any in-flight request: the
// session key. See qrzMapGeoLookupCmd's doc comment.
type qrzMapGeoMsg struct {
	call           string
	record         qrzCallsignRecord
	sessionKey     string
	credGeneration uint64
	err            error
}

// qrzMapGeoLookupCmd looks up call's QRZ profile for the map feed's location
// cache. It reuses lookupQRZCallsignCmdForRequest's login/session-retry
// plumbing (requestID 0, the same sentinel its own compatibility path uses)
// but relabels the result into qrzMapGeoMsg so this flow never shares
// request-ID state with the QSO Entry form's single active-lookup tracking.
//
// credGeneration is captured at request time and echoed back unchanged so
// Update can tell a session key returned under the credentials in effect
// when this lookup started apart from one returned under credentials since
// replaced. Domain review finding: qrzCallsignLookupMsg's handler already
// gained this protection (a prior fix, R22 in docs/ROADMAP.md) after a
// stale QRZ Entry-screen lookup was found able to resurrect an old account's
// session after Station Setup cleared it — but this second QRZ response
// path (map feed geo-lookups, not QSO Entry autofill) had the identical gap:
// its handler restored any inbound sessionKey unconditionally. The location
// cache itself (qrzGeoCache, keyed by call) is intentionally left
// uncorrelated to any generation — that's just location data, not
// credential-scoped state, so accepting it from any in-flight request is
// fine and is not part of this fix.
func qrzMapGeoLookupCmd(creds qrzXMLCreds, sessionKey, call string, credGeneration uint64) tea.Cmd {
	inner := lookupQRZCallsignCmdForRequest(creds, sessionKey, call, 0)
	if inner == nil {
		return nil
	}
	return func() tea.Msg {
		qrzGeoSemaphore <- struct{}{}
		defer func() { <-qrzGeoSemaphore }()
		msg := inner().(qrzCallsignLookupMsg)
		return qrzMapGeoMsg{call: msg.call, record: msg.record, sessionKey: msg.sessionKey, credGeneration: credGeneration, err: msg.err}
	}
}
