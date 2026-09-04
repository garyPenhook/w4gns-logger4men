package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	fieldCall = iota
	fieldRSTSent
	fieldRSTRcvd
	fieldBand
	fieldFrequency
	fieldCount
)

// recentQSOsVisibleRows is the Recent QSOs table's height, shared with the
// DX Spots panel that renders alongside it on QSO Entry so the two stay the
// same height.
const recentQSOsVisibleRows = 10

// cwMode is the only mode this logger supports — see "CW only logger, life
// is too short for QRM." in README.md. There is no Mode field in the
// QSO-entry UI; validateQSO still enforces it on every insert.
const cwMode = "CW"

// appVersion is shown in the UI so a stale, not-yet-rebuilt binary is
// obvious at a glance instead of silently missing recent features. Keep in
// sync with the latest entry in CHANGELOG.md.
const appVersion = "1.18.0"

type screen int

const (
	qsoEntryScreen screen = iota
	stationSetupScreen
	clusterScreen
	clusterFiltersScreen
	adifImportScreen
	eventCatalogScreen
	qsoDetailsScreen
	qsoContestScreen
	continentScreen
	helpScreen
)

const (
	detailName = iota
	detailQTH
	detailGrid
	detailState
	detailCounty
	detailEmail
	detailPOTARef
	detailParkName
	detailNotes
	detailFieldCount
)

var detailLabels = [detailFieldCount]string{"Name", "QTH", "Grid", "State", "County", "Email", "POTA Ref", "Park Name", "Notes"}

const (
	contestName = iota
	contestSerialSent
	contestExchangeSent
	contestSerialRcvd
	contestExchangeRcvd
	contestFieldCount
)

var contestLabels = [contestFieldCount]string{"Contest", "Serial Sent", "Exchange Sent", "Serial Rcvd", "Exchange Rcvd"}

const (
	postTimestamp = iota
	postFieldCount
)

// postTimestampLayout is the operator-typed format for POST (after-contest)
// entry mode's Date/Time field — no zone offset, so time.Parse reads it as
// UTC, matching how every other timestamp in this app is stored.
const postTimestampLayout = "2006-01-02 15:04"

const (
	stationNameField = iota
	stationCallsignField
	stationOperatorField
	stationGridField
	stationTimezoneField
	stationClubField
	stationRigField
	stationAntennaField
	stationPowerField
	stationCategoryOperatorField
	stationCategoryAssistedField
	stationCategoryPowerField
	stationAddressField
	stationQRZXMLUserField
	stationQRZXMLPassField
	stationFieldCount
)

var stationFieldLabels = [stationFieldCount]string{
	"Profile", "Callsign", "Operator", "Grid", "Timezone", "Club", "Rig", "Antenna", "Power (W)",
	"Cat-Operator", "Cat-Assisted", "Cat-Power", "Address",
	"QRZ XML User", "QRZ XML Pass",
}

const (
	clusterDXCCField = iota
	clusterDXITUField
	clusterDXCQField
	clusterDXContinentField
	clusterDECCField
	clusterDEITUField
	clusterDECQField
	clusterDEContinentField
	clusterDECallAreaField
	clusterFilterFieldCount
)

// "Country", not "DXCC": these filters match substrings of the resolved
// entity's country name (e.g. "United States"), not the numeric ADIF/ARRL
// DXCC entity code — the bundled cty.dat has no reliable mapping to that
// code (see dxccEntity's doc comment), so this app never resolves or filters
// on it.
var clusterFilterLabels = [clusterFilterFieldCount]string{
	"DX Country", "DX ITU", "DX CQ", "DX Continent",
	"DE Country", "DE ITU", "DE CQ", "DE Continent", "DE Call Area",
}

var fieldLabels = [fieldCount]string{
	fieldCall:      "Call",
	fieldRSTSent:   "RST Sent",
	fieldRSTRcvd:   "RST Rcvd",
	fieldBand:      "Band",
	fieldFrequency: "Freq MHz",
}

type qso struct {
	id         int64 // 0 for a QSO not yet persisted
	call       string
	band       string
	mode       string
	rstSent    string
	rstRcvd    string
	exchange   string
	frequency  string
	name       string
	qth        string
	grid       string
	state      string
	county     string
	country    string
	cqZone     string
	ituZone    string
	dxccNumber string
	email      string
	comment    string
	potaRef    string
	parkName   string
	contestID  string
	stx        string
	stxString  string
	srx        string
	srxString  string
	time       time.Time // QSO start time (UTC)
	timeOff    time.Time // QSO end time (UTC)
	profileID  int64
	// unscored is the /X (logged-but-unscored) flag: the QSO stays in the log
	// and Cabrillo output (as an X-QSO: line) but is excluded from
	// contestState's scoring tallies. Toggled via store.setQSOUnscored, not
	// updateQSO — it isn't one of the operator-editable form fields.
	unscored bool

	// Station-identity snapshot taken from the active profile at log time, so
	// later edits to the station profile never rewrite the operating context
	// of a past QSO.
	myGridSquare    string
	stationCallsign string
	operatorName    string
	myRig           string
	myAntenna       string
	txPower         string
}

type model struct {
	contestName string
	// fields holds the entry-row inputs in the QSO-entry tab order.
	// Kept as a value slice (not pointers to named struct fields) so mutations
	// survive Update's value-receiver copy: the slice header copies, but the
	// backing array is shared, so in-place index writes stay visible.
	fields   []textinput.Model
	focusIdx int

	store        *store
	qsoCount     int
	table        table.Model
	recentQSOs   []qso // in the same order as table's rows; index maps a selected row back to a qso.id
	tableFocused bool
	deleteArmed  bool

	// termWidth/termHeight track the most recent tea.WindowSizeMsg so the
	// terminal size in effect at exit can be remembered for next launch
	// (see saveWindowSize in windowsize.go). They're 0 until the first
	// WindowSizeMsg arrives.
	termWidth  int
	termHeight int

	// editingQSOID is 0 while entering a new QSO, or the id of an existing
	// QSO currently loaded into the entry/detail/contest fields for editing.
	// editingOriginal holds that QSO's non-editable fields (start/end time,
	// station-identity snapshot, DXCC context) so saving an edit only
	// overwrites what the operator actually changed on screen.
	editingQSOID    int64
	editingOriginal qso
	// preEditContestName/SerialSent/ExchangeSent snapshot the contest-selection
	// fields beginEditQSO is about to overwrite with the edited QSO's own
	// values (which may belong to a different contest, or none). Restored by
	// restorePreEditContestSelection when the edit finishes (save or cancel)
	// so the operator's active contest session survives editing a QSO from a
	// different one — otherwise every QSO logged after the edit would be
	// silently mis-tagged with the edited row's contest instead of the active
	// one.
	preEditContestName         string
	preEditContestSerialSent   string
	preEditContestExchangeSent string

	// dupeBaselineAfter is the SETDUPE command's effect: zero means no
	// baseline (the normal dupe-scope rules apply unbounded in time), a
	// non-zero value means only QSOs logged at or after this instant count
	// as a prior work for dupe purposes. Reset to zero on contest switch
	// (selectEvent) since a stale baseline from a different contest/session
	// would be meaningless there.
	dupeBaselineAfter time.Time

	// QRZ lookups start before the QSO has a database id. A monotonically
	// increasing request id binds each asynchronous result to exactly one form
	// and, if saved first, exactly one QSO. Callsign-keyed FIFO queues are not
	// sufficient: a result that arrived before save leaves a stale same-call id
	// which a later lookup can otherwise patch instead of the new QSO.
	qrzLookupSequence uint64
	qrzActiveLookup   uint64
	qrzLookups        map[uint64]qrzLookupPending

	qrzAPIKey    string
	wrlAPIKey    string
	wrlLogbookID string

	qrzXMLCreds      qrzXMLCreds
	qrzXMLSessionKey string

	backupInProgress         bool
	cabrilloExportInProgress bool
	adifExportInProgress     bool
	csvExportInProgress      bool
	// importInProgress guards against launching a second ADIF import while
	// one is still running (repeated Enter on the Import screen), which would
	// otherwise start concurrent jobs racing on the same database.
	importInProgress bool

	// bgCtx/bgCancel/bgTasks coordinate background database work (currently
	// the async ADIF import) with shutdown: main() cancels bgCtx and waits on
	// bgTasks before closing the database, so an import can't outlive the UI
	// and write into a closing/closed database. bgTasks is a pointer so it
	// survives Update's value-receiver copies.
	bgCtx    context.Context
	bgCancel context.CancelFunc
	bgTasks  *sync.WaitGroup

	dupeWarning  bool
	statusMsg    string
	qsoStartedAt time.Time
	workedCall   string

	// contestScopeFallbackFor is the last free-typed contest name (not found
	// in the event catalog) that checkDupe already warned about, so the
	// warning shows once per distinct value instead of on every keystroke.
	contestScopeFallbackFor string

	solar    solarIndices
	solarErr string

	screen          screen
	activeStation   stationProfile
	stationFields   []textinput.Model
	stationFocusIdx int

	clusterClient      *clusterClient
	clusterSpots       []clusterSpot
	clusterSpotsScroll int
	clusterStatus      string
	clusterConnecting  bool
	clusterGeneration  uint64
	// clusterReconnect is true while the operator wants the feed up, so a
	// dropped connection auto-reconnects; a manual disconnect clears it.
	// clusterReconnectDelay is the current exponential backoff.
	clusterReconnect      bool
	clusterReconnectDelay time.Duration
	clusterFilters        clusterFilters
	clusterFilterFields   []textinput.Model
	clusterFilterFocus    int
	clusterBandFocus      int
	// editClusterBands is the working copy of the band selection while the
	// Cluster Filters edit screen is open. Space toggles this clone, not the
	// live clusterFilters.Bands, so Esc discards band changes and only Enter
	// (saveClusterFilters) commits them — matching how the DX/DE text fields
	// are only read back into clusterFilters on Enter.
	editClusterBands map[string]bool
	adifPathField    textinput.Model
	detailFields     []textinput.Model
	detailFocusIdx   int
	contestFields    []textinput.Model
	contestFocusIdx  int
	// nextSerial is the serial number the operator will send on the next QSO in
	// a serial-exchange contest (e.g. CW Open). It is 0 when no serial contest
	// is active. Selecting a serial event sets it to 1; each logged QSO advances
	// it past the serial actually sent, so a manual correction to the Sent
	// Serial field carries forward. clearQSOForm re-displays it in the Sent
	// Serial field so it survives the between-QSO reset.
	nextSerial          int
	events              []eventDefinition
	eventFocus          int
	eventSessionFocus   int
	exchangeChoiceFocus int
	// contestIndex is the in-memory analysis index (contest_state.go) for
	// whichever contest contestIndexID names — the shared backend the
	// Analysis panel and (eventually) scoring read so they can't disagree.
	// nil when no contest is active. Kept live by rebuildContestIndex and its
	// call sites; see that function's doc comment for the sync rules.
	contestIndex   *contestState
	contestIndexID string
	// continentBandFocus pages the Worked/Needed by Continent screen (Ctrl+W)
	// one band at a time through the active event's allowed bands (SD's
	// "F1/F2 across bands" for the live worked/needed grids, roadmap
	// Appendix D — bound here to Left/Right instead, since F1 is already the
	// app-wide "QSO Entry" hotkey on every screen).
	continentBandFocus int
	// helpReturnScreen is the screen Ctrl+G was pressed from, so the Help
	// screen's Esc/F1/Ctrl+G returns the operator exactly where they were
	// instead of always bouncing to QSO Entry (Help is reachable globally,
	// unlike the other single-purpose panels).
	helpReturnScreen screen
	// contestExchangeRcvdEdited is true once the operator has typed into (or
	// picked a suggested value for) contestExchangeRcvd this QSO, so
	// autofillReceivedExchange stops overwriting it — the same
	// autofill-until-overridden shape as nextSerial's carried-forward manual
	// correction. Reset in clearQSOForm and selectEvent (both already blank
	// the field for a fresh QSO/contest).
	contestExchangeRcvdEdited bool

	// postMode is SD's "POST" (after-contest) entry mode: when true, the
	// operator is re-logging QSOs from a paper log after the contest, so
	// logCurrentQSO uses the operator-typed postFields[postTimestamp] value
	// instead of time.Now() for the QSO's time. Toggled by Ctrl+P; an extra
	// entrySlot for the Date/Time field is appended only while this is true.
	// Independent of the active contest — left untouched by selectEvent.
	postMode   bool
	postFields []textinput.Model
}

// formatSerial renders a running serial number the way contest exchanges are
// sent: zero-padded to at least three digits (001, 002, … 999, 1000).
func formatSerial(n int) string {
	return fmt.Sprintf("%03d", n)
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ADD8")).
			Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Width(13)

	fieldBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1).
			Margin(0, 1, 1, 0)

	focusedFieldBoxStyle = fieldBoxStyle.Copy().
				BorderForeground(lipgloss.Color("#00ADD8"))

	dupeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("#D8003A")).
			Padding(0, 2)

	// newMultStyle flags a callsign that would advance a multiplier if
	// logged now, in the Analysis panel (analysis_panel.go).
	newMultStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("10"))

	editingStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("11")).
			Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	// hotkeyStyle is used only for screenHotkeys' version/keybinding rows —
	// helpStyle's dim gray (240) was hard to read there, unlike the other,
	// less load-bearing help lines it still styles elsewhere.
	hotkeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	solarStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("11")).
			Padding(0, 1)
)

// tableStylesFocused/tableStylesUnfocused control whether the Recent QSOs
// table's cursor row is visually highlighted. table.DefaultStyles() bolds
// and recolors the cursor row (row 0 by default) regardless of
// WithFocused(false), which — since recentQSOs returns newest-first —
// always singled out the most recent QSO with no meaning until the table
// became a real interactive selector (F9). Now the highlight only appears
// while it's actually meaningful: while browsing/selecting a row (F9).
var (
	tableStylesFocused   = table.DefaultStyles()
	tableStylesUnfocused = func() table.Styles {
		s := table.DefaultStyles()
		s.Selected = s.Cell
		return s
	}()
)

// maxEventSelectionLength bounds the Contest Name field so it can hold the
// longest "event.ID-session.ID" value the catalog generates (see
// TestEventSelectionIDsFitContestField), plus headroom for manually typed
// contest names.
const maxEventSelectionLength = 64

func newTextInput(placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 20
	ti.Width = width
	return ti
}

func initialModel(st *store) model {
	events, err := loadEventCatalog()
	if err != nil {
		panic(err)
	}
	fields := make([]textinput.Model, fieldCount)
	fields[fieldCall] = newTextInput("W1AW", 14)
	fields[fieldRSTSent] = newTextInput("599", 6)
	fields[fieldRSTSent].SetValue("599")
	fields[fieldRSTRcvd] = newTextInput("599", 6)
	fields[fieldBand] = newTextInput("20M", 6)
	fields[fieldBand].SetValue("20M")
	fields[fieldFrequency] = newTextInput("14.025", 9)
	fields[fieldFrequency].SetValue("14.025")
	details := []textinput.Model{
		newTextInput("Operator name", 20), newTextInput("City / QTH", 20),
		newTextInput("Grid square", 10), newTextInput("State / province", 12),
		newTextInput("County", 16), newTextInput("Email", 24),
		newTextInput("US-0000", 12), newTextInput("Park name", 30), newTextInput("QSO notes", 36),
	}
	contests := []textinput.Model{
		newTextInput("Contest name", 20), newTextInput("001", 8), newTextInput("Sent exchange", 16),
		newTextInput("001", 8), newTextInput("Received exchange", 16),
	}
	// Selecting a catalog event writes "event.ID-session.ID" into this field
	// (see setSelectedEvent); the longest generated value in the catalog is 56
	// characters, so the default 20-character CharLimit would silently
	// truncate most of them.
	contests[contestName].CharLimit = maxEventSelectionLength
	contests[contestName].Width = 30
	postFields := []textinput.Model{newTextInput(postTimestampLayout, 18)}

	cols := []table.Column{
		{Title: "UTC", Width: 8},
		{Title: "Call", Width: 12},
		{Title: "Band", Width: 6},
		{Title: "Sent", Width: 6},
		{Title: "Rcvd", Width: 6},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(false),
		table.WithHeight(recentQSOsVisibleRows),
		table.WithStyles(tableStylesUnfocused),
	)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	m := model{
		contestName:    "W4GNS Logger 4 Men — CW Log",
		fields:         fields,
		store:          st,
		table:          t,
		clusterFilters: defaultClusterFilters(),
		bgCtx:          bgCtx,
		bgCancel:       bgCancel,
		bgTasks:        &sync.WaitGroup{},
		detailFields:   details,
		contestFields:  contests,
		postFields:     postFields,
		events:         events,
	}
	profile, err := st.activeStationProfile()
	if err != nil {
		m.statusMsg = fmt.Sprintf("station profile error: %v", err)
	} else {
		m.activeStation = profile
	}
	m.focusField(fieldCall)
	m.refreshTableRows()
	return m
}

func newStationTextInput(value string, width int) textinput.Model {
	ti := newTextInput("", width)
	ti.CharLimit = 80
	ti.SetValue(value)
	return ti
}

// newCabrilloCategoryInput shows placeholder as the value the Cabrillo
// export falls back to when this field is left blank (see
// cabrilloOrDefault in cabrillo_export.go), so the operator sees what
// they're getting without it being force-filled into their saved profile.
func newCabrilloCategoryInput(value, placeholder string) textinput.Model {
	ti := newStationTextInput(value, 16)
	ti.Placeholder = placeholder
	return ti
}

func (m *model) openStationSetup() {
	profile := m.activeStation
	m.stationFields = []textinput.Model{
		newStationTextInput(profile.Name, 20),
		newStationTextInput(profile.Callsign, 14),
		newStationTextInput(profile.OperatorName, 24),
		newStationTextInput(profile.MyGridSquare, 12),
		newStationTextInput(profile.Timezone, 24),
		newStationTextInput(profile.Club, 20),
		newStationTextInput(profile.Rig, 24),
		newStationTextInput(profile.Antenna, 24),
		newStationTextInput(profile.PowerWatts, 10),
		newCabrilloCategoryInput(profile.CategoryOperator, "SINGLE-OP"),
		newCabrilloCategoryInput(profile.CategoryAssisted, "NON-ASSISTED"),
		newCabrilloCategoryInput(profile.CategoryPower, "LOW"),
		newStationTextInput(profile.Address, 40),
		newStationTextInput(m.qrzXMLCreds.username, 24),
		newQRZXMLPasswordInput(m.qrzXMLCreds.password),
	}
	m.screen = stationSetupScreen
	m.focusStationField(stationNameField)
	m.statusMsg = "Station Setup — Enter saves, Esc cancels"
}

// newQRZXMLPasswordInput masks the QRZ XML password on screen the same way
// the field's own EchoMode always has, so it doesn't rely on the terminal or
// screenshots/screen-sharing not exposing it.
func newQRZXMLPasswordInput(value string) textinput.Model {
	ti := newStationTextInput(value, 24)
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	return ti
}

func (m *model) focusStationField(index int) {
	for i := range m.stationFields {
		if i == index {
			m.stationFields[i].Focus()
		} else {
			m.stationFields[i].Blur()
		}
	}
	m.stationFocusIdx = index
}

// saveStationSetup returns a tea.Cmd (possibly nil) rather than nothing, so
// a callsign entered for the first time (or after a previous save left it
// blank) can retry the DX cluster connection immediately. Without this, an
// operator who fills in Station Setup after the app has already started —
// startup is the only other place a connection attempt fires — would need
// to visit the DX Cluster (F3) screen by hand to ever connect at all.
func (m *model) saveStationSetup() tea.Cmd {
	previous := m.activeStation
	profile := stationProfile{
		ID:           m.activeStation.ID,
		Name:         m.stationFields[stationNameField].Value(),
		Callsign:     m.stationFields[stationCallsignField].Value(),
		OperatorName: m.stationFields[stationOperatorField].Value(),
		MyGridSquare: m.stationFields[stationGridField].Value(),
		Timezone:     m.stationFields[stationTimezoneField].Value(),
		Club:         m.stationFields[stationClubField].Value(),
		Rig:          m.stationFields[stationRigField].Value(),
		Antenna:      m.stationFields[stationAntennaField].Value(),
		PowerWatts:   m.stationFields[stationPowerField].Value(),

		CategoryOperator: m.stationFields[stationCategoryOperatorField].Value(),
		CategoryAssisted: m.stationFields[stationCategoryAssistedField].Value(),
		CategoryPower:    m.stationFields[stationCategoryPowerField].Value(),
		Address:          m.stationFields[stationAddressField].Value(),
	}
	saved, err := m.store.saveStationProfile(profile)
	if err != nil {
		m.statusMsg = fmt.Sprintf("station setup error: %v", err)
		return nil
	}
	m.activeStation = saved
	identityChanged := previous.Callsign != saved.Callsign || previous.MyGridSquare != saved.MyGridSquare
	if identityChanged {
		// pointsRule classification and bearing origin both depend on the
		// station identity. The contest id itself did not change, so the usual
		// lazy contest-id sync would otherwise leave a stale index alive.
		m.rebuildContestIndex()
	}
	if previous.Callsign != saved.Callsign && (m.clusterClient != nil || m.clusterConnecting) {
		// A DX-cluster login identifies the station. Keep no live/pending socket
		// authenticated as the old call after Station Setup changes it.
		m.disconnectCluster()
	}

	creds := qrzXMLCreds{
		username: m.stationFields[stationQRZXMLUserField].Value(),
		password: m.stationFields[stationQRZXMLPassField].Value(),
	}
	if err := saveQRZXMLCredentials(creds); err != nil {
		m.screen = qsoEntryScreen
		m.focusField(fieldCall)
		m.statusMsg = fmt.Sprintf("station profile %q saved, but QRZ XML credentials failed to save: %v", saved.Name, err)
		return m.connectClusterIfNeeded()
	}
	// Credentials may have changed (or been cleared), so the cached session
	// key — tied to whichever account last logged in — is no longer valid
	// for the next lookup.
	m.qrzXMLCreds = creds
	m.qrzXMLSessionKey = ""

	m.screen = qsoEntryScreen
	m.focusField(fieldCall)
	m.statusMsg = fmt.Sprintf("station profile %q saved", saved.Name)
	return m.connectClusterIfNeeded()
}

func (m *model) openCluster() tea.Cmd {
	m.screen = clusterScreen
	return m.connectClusterIfNeeded()
}

// connectClusterIfNeeded starts a K3LR connection unless one is already up
// or in flight, or no station callsign is configured yet. Split out of
// openCluster so main() can also call it at startup, populating the DX
// Spots panel on QSO Entry without requiring a visit to the DX Cluster (F3)
// screen first.
func (m *model) connectClusterIfNeeded() tea.Cmd {
	if m.clusterClient != nil || m.clusterConnecting {
		return nil
	}
	if strings.TrimSpace(m.activeStation.Callsign) == "" {
		m.clusterStatus = "set your station callsign in F2 Station Setup to connect to K3LR"
		return nil
	}
	m.clusterGeneration++
	m.clusterConnecting = true
	m.clusterReconnect = true
	m.clusterReconnectDelay = 0
	m.clusterStatus = "connecting to " + k3lrClusterAddr + "…"
	return connectK3LR(m.activeStation.Callsign, m.clusterGeneration)
}

func (m *model) openClusterFilters() {
	f := m.clusterFilters
	m.clusterFilterFields = []textinput.Model{
		newStationTextInput(f.DXCC, 14), newStationTextInput(f.DXITUZone, 14),
		newStationTextInput(f.DXCQZone, 14), newStationTextInput(f.DXContinent, 14),
		newStationTextInput(f.DECC, 14), newStationTextInput(f.DEITUZone, 14),
		newStationTextInput(f.DECQZone, 14), newStationTextInput(f.DEContinent, 14),
		newStationTextInput(f.DECallArea, 14),
	}
	m.editClusterBands = make(map[string]bool, len(m.clusterFilters.Bands))
	for band, enabled := range m.clusterFilters.Bands {
		m.editClusterBands[band] = enabled
	}
	m.screen = clusterFiltersScreen
	m.focusClusterFilterField(0)
	m.statusMsg = "Cluster filters: CW only; use Up/Down and Space to select bands. DE Call Area e.g. \"2,3,4\""
}

func (m *model) focusClusterFilterField(index int) {
	for i := range m.clusterFilterFields {
		if i == index {
			m.clusterFilterFields[i].Focus()
		} else {
			m.clusterFilterFields[i].Blur()
		}
	}
	m.clusterFilterFocus = index
}

func (m *model) saveClusterFilters() {
	m.clusterFilters.DXCC = strings.TrimSpace(m.clusterFilterFields[clusterDXCCField].Value())
	m.clusterFilters.DXITUZone = strings.TrimSpace(m.clusterFilterFields[clusterDXITUField].Value())
	m.clusterFilters.DXCQZone = strings.TrimSpace(m.clusterFilterFields[clusterDXCQField].Value())
	m.clusterFilters.DXContinent = strings.TrimSpace(m.clusterFilterFields[clusterDXContinentField].Value())
	m.clusterFilters.DECC = strings.TrimSpace(m.clusterFilterFields[clusterDECCField].Value())
	m.clusterFilters.DEITUZone = strings.TrimSpace(m.clusterFilterFields[clusterDEITUField].Value())
	m.clusterFilters.DECQZone = strings.TrimSpace(m.clusterFilterFields[clusterDECQField].Value())
	m.clusterFilters.DEContinent = strings.TrimSpace(m.clusterFilterFields[clusterDEContinentField].Value())
	m.clusterFilters.DECallArea = strings.TrimSpace(m.clusterFilterFields[clusterDECallAreaField].Value())
	if m.editClusterBands != nil {
		m.clusterFilters.Bands = m.editClusterBands
		m.editClusterBands = nil
	}
	// Re-apply the just-committed filters to spots already buffered under the
	// old filters, so a spot that's no longer allowed (a now-deselected band,
	// a newly narrowed DX/DE filter) disappears immediately instead of
	// lingering until it scrolls off the capped list.
	m.pruneClusterSpots()
	m.screen = clusterScreen
	m.clusterStatus = "cluster filters applied — CW only"
}

// pruneClusterSpots drops every buffered spot the current filters no longer
// allow and clamps the scroll offset to the shortened list.
func (m *model) pruneClusterSpots() {
	kept := m.clusterSpots[:0]
	for _, spot := range m.clusterSpots {
		if m.clusterFilters.allowsSpot(spot) {
			kept = append(kept, spot)
		}
	}
	m.clusterSpots = kept
	// Clamp the scroll offset to the shortened list without the status-bar
	// side effect scrollClusterSpots carries.
	maxScroll := len(m.clusterSpots) - recentQSOsVisibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.clusterSpotsScroll > maxScroll {
		m.clusterSpotsScroll = maxScroll
	}
}

type adifImportedMsg struct {
	result adifImportResult
	err    error
}

type backupCompletedMsg struct {
	result backupResult
	err    error
}

func (m model) runBackupCmd() tea.Cmd {
	st := m.store
	profileID := m.activeStation.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
		defer cancel()
		result, err := runBackupSerialized(ctx, st, profileID)
		return backupCompletedMsg{result: result, err: err}
	}
}

type cabrilloExportedMsg struct {
	path  string
	count int
	score contestScore
	err   error
}

// cabrilloExportCmd writes a Cabrillo submission for the currently active
// contest (whatever's entered in the Contest Entry (F7) screen's Contest
// field) to the operator's Downloads folder. contestID is captured by value
// from the model before the command runs, matching runBackupCmd/
// importADIFFile's pattern of snapshotting what the async closure needs
// rather than reading the model again after Update's value-receiver copy is
// gone.
func (m model) cabrilloExportCmd(contestID string) tea.Cmd {
	st := m.store
	profile := m.activeStation
	event, ok := m.eventForContestID()
	wg := m.bgTasks
	bgCtx := m.bgCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	// Register synchronously (this runs inside Update, before the returned
	// closure's goroutine starts) so shutdown's bgTasks.Wait() drains this
	// export before the store is closed and can't miss it.
	if wg != nil {
		wg.Add(1)
	}
	return func() tea.Msg {
		if wg != nil {
			defer wg.Done()
		}
		if !ok {
			return cabrilloExportedMsg{err: fmt.Errorf("no matching event/contest found for %q — select one on the Events (F7) screen first", contestID)}
		}
		if !event.cabrilloReady() {
			return cabrilloExportedMsg{err: fmt.Errorf("%s has no verified Cabrillo layout yet; CSV export remains available", event.Name)}
		}
		downloads, err := defaultDownloadsDir()
		if err != nil {
			return cabrilloExportedMsg{err: err}
		}
		if err := os.MkdirAll(downloads, 0o700); err != nil {
			return cabrilloExportedMsg{err: fmt.Errorf("create Downloads folder: %w", err)}
		}
		callsign := profile.Callsign
		if callsign == "" {
			callsign = "LOG"
		}
		filename := fmt.Sprintf("%s_%s.cbr", sanitizeFilenameComponent(callsign), sanitizeFilenameComponent(contestID))
		path := filepath.Join(downloads, filename)
		ctx, cancel := context.WithTimeout(bgCtx, backupTimeout)
		defer cancel()
		count, score, err := writeCabrilloAtomic(ctx, downloads, path, profile, event, contestID, st)
		if err != nil {
			return cabrilloExportedMsg{err: err}
		}
		return cabrilloExportedMsg{path: path, count: count, score: score}
	}
}

type csvExportedMsg struct {
	path  string
	count int
	err   error
}

// csvExportCmd writes a CSV listing for the currently active contest
// (whatever's entered in the Contest Entry (F7) screen's Contest field) to
// the operator's Downloads folder, mirroring cabrilloExportCmd's shape and
// snapshotting-by-value the same way.
func (m model) csvExportCmd(contestID string) tea.Cmd {
	st := m.store
	profile := m.activeStation
	_, ok := m.eventForContestID()
	wg := m.bgTasks
	bgCtx := m.bgCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	if wg != nil {
		wg.Add(1)
	}
	return func() tea.Msg {
		if wg != nil {
			defer wg.Done()
		}
		if !ok {
			return csvExportedMsg{err: fmt.Errorf("no matching event/contest found for %q — select one on the Events (F7) screen first", contestID)}
		}
		downloads, err := defaultDownloadsDir()
		if err != nil {
			return csvExportedMsg{err: err}
		}
		if err := os.MkdirAll(downloads, 0o700); err != nil {
			return csvExportedMsg{err: fmt.Errorf("create Downloads folder: %w", err)}
		}
		callsign := profile.Callsign
		if callsign == "" {
			callsign = "LOG"
		}
		filename := fmt.Sprintf("%s_%s.csv", sanitizeFilenameComponent(callsign), sanitizeFilenameComponent(contestID))
		path := filepath.Join(downloads, filename)
		ctx, cancel := context.WithTimeout(bgCtx, backupTimeout)
		defer cancel()
		count, err := writeCSVAtomic(ctx, downloads, path, profile, contestID, st)
		if err != nil {
			return csvExportedMsg{err: err}
		}
		return csvExportedMsg{path: path, count: count}
	}
}

type adifExportedMsg struct {
	path  string
	count int
	err   error
}

// adifExportCmd writes the active station profile's full log to the
// operator's Downloads folder, timestamped so repeated exports don't
// silently overwrite an earlier one (unlike the per-contest Cabrillo export,
// which is meant to be re-run for the same contest).
func (m model) adifExportCmd() tea.Cmd {
	st := m.store
	profile := m.activeStation
	wg := m.bgTasks
	bgCtx := m.bgCtx
	if bgCtx == nil {
		bgCtx = context.Background()
	}
	// Register synchronously (this runs inside Update, before the returned
	// closure's goroutine starts) so shutdown's bgTasks.Wait() drains this
	// export before the store is closed and can't miss it.
	if wg != nil {
		wg.Add(1)
	}
	return func() tea.Msg {
		if wg != nil {
			defer wg.Done()
		}
		downloads, err := defaultDownloadsDir()
		if err != nil {
			return adifExportedMsg{err: err}
		}
		if err := os.MkdirAll(downloads, 0o700); err != nil {
			return adifExportedMsg{err: fmt.Errorf("create Downloads folder: %w", err)}
		}
		callsign := profile.Callsign
		if callsign == "" {
			callsign = "LOG"
		}
		filename := fmt.Sprintf("%s_%s.adi", sanitizeFilenameComponent(callsign), time.Now().UTC().Format("20060102-150405"))
		path := nonCollidingPath(filepath.Join(downloads, filename))

		ctx, cancel := context.WithTimeout(bgCtx, backupTimeout)
		defer cancel()
		count, err := writeADIFAtomic(ctx, downloads, path, profile.ID, st)
		if err != nil {
			return adifExportedMsg{err: err}
		}
		return adifExportedMsg{path: path, count: count}
	}
}

// nonCollidingPath returns path unchanged if nothing exists there, otherwise
// inserts "-1", "-2", … before the extension until it finds a free name. The
// ADIF export filename is timestamped only to the second, so two exports in
// the same second would otherwise land on the same path and the second would
// silently overwrite the first.
func nonCollidingPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

func (m *model) openADIFImport() {
	m.adifPathField = newStationTextInput("", 60)
	m.adifPathField.Placeholder = "/path/to/log.adi"
	m.adifPathField.Focus()
	m.screen = adifImportScreen
	m.statusMsg = "Enter an ADIF file path"
}

func (m model) importADIFFile(path string) tea.Cmd {
	ctx := m.bgCtx
	if ctx == nil {
		ctx = context.Background()
	}
	wg := m.bgTasks
	st := m.store
	profileID := m.activeStation.ID
	return func() tea.Msg {
		// The matching bgTasks.Add(1) runs on the update loop before this
		// goroutine starts (see updateADIFImport), so shutdown's Wait() sees
		// this job; release it here whichever way the import ends.
		if wg != nil {
			defer wg.Done()
		}
		file, err := os.Open(strings.TrimSpace(path))
		if err != nil {
			return adifImportedMsg{err: fmt.Errorf("open ADIF file: %w", err)}
		}
		defer file.Close()
		result, err := importADIF(ctx, file, profileID, st)
		return adifImportedMsg{result: result, err: err}
	}
}

func (m *model) disconnectCluster() {
	m.clusterGeneration++ // invalidates a pending dial or reader result.
	m.clusterReconnect = false
	m.clusterReconnectDelay = 0
	err := m.clusterClient.close()
	m.clusterClient = nil
	m.clusterConnecting = false
	if err != nil {
		m.clusterStatus = fmt.Sprintf("cluster disconnect error: %v", err)
		return
	}
	m.clusterStatus = k3lrClusterName + " — disconnected"
}

// scheduleClusterReconnect starts a backed-off reconnect attempt after the
// feed drops (dial failure or read error), unless the operator has disconnected
// (clusterReconnect cleared). It bumps the generation so the in-flight failed
// attempt's late results are ignored, then dials again after an exponentially
// growing delay capped at clusterReconnectMax. Returns nil when no reconnect
// should be attempted, so callers fall back to their normal error status.
func (m *model) scheduleClusterReconnect(cause error) tea.Cmd {
	if !m.clusterReconnect || strings.TrimSpace(m.activeStation.Callsign) == "" {
		return nil
	}
	if m.clusterReconnectDelay <= 0 {
		m.clusterReconnectDelay = clusterReconnectBase
	} else {
		m.clusterReconnectDelay *= 2
		if m.clusterReconnectDelay > clusterReconnectMax {
			m.clusterReconnectDelay = clusterReconnectMax
		}
	}
	m.clusterGeneration++
	m.clusterConnecting = true
	m.clusterStatus = fmt.Sprintf("%s — reconnecting in %s (%v)", k3lrClusterName, m.clusterReconnectDelay.Round(time.Second), cause)
	return connectK3LRAfter(m.clusterReconnectDelay, m.activeStation.Callsign, m.clusterGeneration)
}

// clusterDupeWindow is how long a station stays suppressed on the same band
// after being shown once, so the same spot relayed by several cluster nodes
// doesn't flood the list.
const clusterDupeWindow = 3 * time.Minute

func (m *model) addClusterSpot(spot clusterSpot) {
	if m.isDuplicateClusterSpot(spot) {
		return
	}
	m.clusterSpots = append([]clusterSpot{spot}, m.clusterSpots...)
	if len(m.clusterSpots) > 100 {
		m.clusterSpots = m.clusterSpots[:100]
	}
}

// scrollClusterSpots moves the DX Spots panel's scroll offset by delta rows,
// clamped to the valid range so PgUp/PgDn can't scroll past the first or
// last spot in m.clusterSpots.
func (m *model) scrollClusterSpots(delta int) {
	maxScroll := len(m.clusterSpots) - recentQSOsVisibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	m.clusterSpotsScroll += delta
	if m.clusterSpotsScroll < 0 {
		m.clusterSpotsScroll = 0
	}
	if m.clusterSpotsScroll > maxScroll {
		m.clusterSpotsScroll = maxScroll
	}
	if len(m.clusterSpots) == 0 {
		return
	}
	first := m.clusterSpotsScroll + 1
	last := m.clusterSpotsScroll + recentQSOsVisibleRows
	if last > len(m.clusterSpots) {
		last = len(m.clusterSpots)
	}
	m.statusMsg = fmt.Sprintf("DX Spots %d-%d of %d", first, last, len(m.clusterSpots))
}

// isDuplicateClusterSpot reports whether spot's callsign was already shown
// on the same band within the last clusterDupeWindow. clusterSpots is kept
// newest-first, so the scan can stop at the first entry older than the
// window instead of walking the whole (capped at 100) list every time.
func (m *model) isDuplicateClusterSpot(spot clusterSpot) bool {
	band, _, ok := bandForFrequency(spot.Frequency)
	if !ok {
		return false
	}
	cutoff := spot.Received.Add(-clusterDupeWindow)
	for _, existing := range m.clusterSpots {
		if existing.Received.Before(cutoff) {
			break
		}
		if !strings.EqualFold(existing.Callsign, spot.Callsign) {
			continue
		}
		if existingBand, _, ok := bandForFrequency(existing.Frequency); ok && existingBand == band {
			return true
		}
	}
	return false
}

func (m model) Init() tea.Cmd {
	// uploadDrainTickCmd starts the outbox drain loop, which also resumes any
	// deliveries left pending in the database from a previous run (a crash,
	// quit, or transient failure), so no logged QSO's upload is lost.
	cmds := []tea.Cmd{textinput.Blink, fetchSolarIndicesCmd(), solarTickCmd(), uploadDrainTickCmd()}
	// main() pre-sets clusterConnecting (and the generation/status that go
	// with it) before constructing the program, since Init has a value
	// receiver and can't mutate the model itself — only kick off the
	// connect this model was already flagged for.
	if m.clusterConnecting && m.clusterClient == nil {
		cmds = append(cmds, connectK3LR(m.activeStation.Callsign, m.clusterGeneration))
	}
	return tea.Batch(cmds...)
}

// setTableFocused toggles whether the Recent QSOs table is the active
// interactive selector: it takes/releases keyboard focus and swaps in the
// highlight style that makes the current selection visible only while it's
// actually a selection.
func (m *model) setTableFocused(focused bool) {
	m.tableFocused = focused
	m.deleteArmed = false
	if focused {
		m.table.Focus()
		m.table.SetStyles(tableStylesFocused)
	} else {
		m.table.Blur()
		m.table.SetStyles(tableStylesUnfocused)
	}
}

func (m *model) refreshTableRows() {
	recent, err := m.store.recentQSOs(m.activeStation.ID, 50)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	// This repopulates the table with the default Recent QSOs list, so the
	// "Stations Worked" view is no longer showing; clear workedCall or the
	// header keeps labeling the default list as a call's prior contacts (e.g.
	// after deleting a row while the F9 worked-call view was up).
	m.workedCall = ""
	rows := make([]table.Row, 0, len(recent))
	for _, q := range recent {
		rows = append(rows, table.Row{
			q.time.Format("15:04:05"),
			q.call,
			q.band,
			q.rstSent,
			q.rstRcvd,
		})
	}
	m.table.SetRows(rows)
	m.recentQSOs = recent
	// table.SetRows leaves the cursor at -1 the first time it's called with
	// zero rows (its own clamp is cursor = len(rows)-1), and never recovers
	// it once real rows arrive — reset it whenever it's out of [0, len(rows)).
	if len(rows) > 0 {
		if cur := m.table.Cursor(); cur < 0 || cur >= len(rows) {
			m.table.SetCursor(0)
		}
	}

	if n, err := m.store.count(m.activeStation.ID); err == nil {
		m.qsoCount = n
	}
}

// selectedRecentQSO returns the full QSO backing the table's currently
// highlighted row, if any (empty when there are no recent QSOs to select).
func (m model) selectedRecentQSO() (qso, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.recentQSOs) {
		return qso{}, false
	}
	return m.recentQSOs[i], true
}

// beginEditQSO loads an existing QSO into the entry/detail/contest fields
// for editing. The next final Enter (see logCurrentQSO) saves changes back
// to this same row instead of inserting a new one.
func (m *model) beginEditQSO(q qso) {
	full, err := m.store.qsoByID(m.activeStation.ID, q.id)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	if m.editingQSOID == 0 {
		// Only capture on the first beginEditQSO of an edit session: switching
		// to edit a second row before saving/cancelling the first must not
		// clobber this snapshot with the first row's (already-substituted)
		// contest values — that would make the active contest unrecoverable.
		m.preEditContestName = m.contestFields[contestName].Value()
		m.preEditContestSerialSent = m.contestFields[contestSerialSent].Value()
		m.preEditContestExchangeSent = m.contestFields[contestExchangeSent].Value()
	}
	m.editingQSOID = full.id
	m.editingOriginal = full

	m.fields[fieldCall].SetValue(full.call)
	m.fields[fieldRSTSent].SetValue(full.rstSent)
	m.fields[fieldRSTRcvd].SetValue(full.rstRcvd)
	m.fields[fieldBand].SetValue(full.band)
	m.fields[fieldFrequency].SetValue(full.frequency)
	m.detailFields[detailName].SetValue(full.name)
	m.detailFields[detailQTH].SetValue(full.qth)
	m.detailFields[detailGrid].SetValue(full.grid)
	m.detailFields[detailState].SetValue(full.state)
	m.detailFields[detailCounty].SetValue(full.county)
	m.detailFields[detailEmail].SetValue(full.email)
	m.detailFields[detailPOTARef].SetValue(full.potaRef)
	m.detailFields[detailParkName].SetValue(full.parkName)
	m.detailFields[detailNotes].SetValue(full.comment)
	m.contestFields[contestName].SetValue(full.contestID)
	m.contestFields[contestSerialSent].SetValue(full.stx)
	m.contestFields[contestExchangeSent].SetValue(full.stxString)
	m.contestFields[contestSerialRcvd].SetValue(full.srx)
	m.contestFields[contestExchangeRcvd].SetValue(full.srxString)
	// This QSO's received exchange is a real, previously-logged value, not a
	// fresh guess to autofill over — treat it like an operator edit so
	// autofillReceivedExchange leaves it alone if the operator retypes the
	// call while correcting something else.
	m.contestExchangeRcvdEdited = true

	m.setTableFocused(false)
	m.qsoStartedAt = time.Time{} // editing doesn't run the new-QSO timer
	m.workedCall = ""
	m.statusMsg = "editing " + full.call + " — Esc cancels, final Enter saves"
	m.focusField(fieldCall)
	// The contestFields[contestName] overwrite above doesn't go through the
	// per-keystroke Update path that normally notices a contest switch, so
	// contestIndex wouldn't otherwise resync to the edited QSO's own
	// contest. rebuildContestIndex directly, not checkDupe: checkDupe would
	// also re-run showWorkedCall(full.call) and undo the m.workedCall reset
	// two lines up, flipping the Recent QSOs table to that call's history
	// instead of leaving it as it was when the edit began.
	m.rebuildContestIndex()
}

// cancelEditQSO discards an in-progress edit and returns to a blank
// new-QSO form without touching the database.
func (m *model) cancelEditQSO() {
	call := m.editingOriginal.call
	m.editingQSOID = 0
	m.editingOriginal = qso{}
	m.clearQSOForm()
	m.restorePreEditContestSelection()
	// Swapping the contest selection back needs the same contestIndex
	// resync beginEditQSO triggers on the way in; rebuildContestIndex
	// directly for the same reason given there (avoid checkDupe's
	// showWorkedCall side effect).
	m.rebuildContestIndex()
	m.statusMsg = "cancelled editing " + call
}

// restorePreEditContestSelection puts the contest-selection fields back to
// what they were before beginEditQSO overwrote them with the edited QSO's
// own values. Called after an edit finishes (save or cancel) so the
// operator's active contest keeps logging correctly for the next QSO.
func (m *model) restorePreEditContestSelection() {
	m.contestFields[contestName].SetValue(m.preEditContestName)
	m.contestFields[contestExchangeSent].SetValue(m.preEditContestExchangeSent)
	if m.nextSerial == 0 {
		m.contestFields[contestSerialSent].SetValue(m.preEditContestSerialSent)
	}
	m.preEditContestName, m.preEditContestSerialSent, m.preEditContestExchangeSent = "", "", ""
}

// handleCallFieldCommand recognizes SD-style commands typed into the Call
// field and confirmed with Enter, rather than a callsign: ZAP and SETDUPE
// while logging a new QSO, /Z and /X while an existing QSO is loaded for
// editing (beginEditQSO leaves focus on Call, so these are reachable the
// moment a recalled QSO is on screen). Returns true when msg was consumed as
// a command, so the caller must not fall through to the normal Enter
// handling (advance-field / logCurrentQSO).
func (m *model) handleCallFieldCommand() bool {
	if m.focusIdx != fieldCall {
		return false
	}
	command := strings.ToUpper(strings.TrimSpace(m.fields[fieldCall].Value()))
	if m.editingQSOID != 0 {
		switch command {
		case "/Z":
			m.deleteEditingQSO()
			return true
		case "/X":
			m.toggleEditingQSOUnscored()
			return true
		}
		return false
	}
	switch command {
	case "ZAP":
		m.zapLastQSO()
		return true
	case "SETDUPE":
		m.setDupeBaseline()
		return true
	}
	return false
}

// deleteEditingQSO is the /Z command: permanently removes the QSO currently
// loaded for editing (SD's "mark old QSO for delete") instead of saving
// changes to it, then leaves edit mode the same way cancelEditQSO does.
func (m *model) deleteEditingQSO() {
	id, call := m.editingQSOID, m.editingOriginal.call
	if err := m.store.deleteQSO(m.activeStation.ID, id); err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	m.editingQSOID = 0
	m.editingOriginal = qso{}
	m.clearQSOForm()
	m.restorePreEditContestSelection()
	m.rebuildContestIndex()
	m.refreshTableRows()
	m.statusMsg = fmt.Sprintf("/Z: deleted QSO #%d (%s)", id, call)
}

// toggleEditingQSOUnscored is the /X command: flips the logged-but-unscored
// flag on the QSO currently loaded for editing (SD's X-QSO) and leaves edit
// mode without touching any other field.
func (m *model) toggleEditingQSOUnscored() {
	id, call := m.editingQSOID, m.editingOriginal.call
	unscored := !m.editingOriginal.unscored
	if err := m.store.setQSOUnscored(m.activeStation.ID, id, unscored); err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	m.editingQSOID = 0
	m.editingOriginal = qso{}
	m.clearQSOForm()
	m.restorePreEditContestSelection()
	m.rebuildContestIndex()
	m.refreshTableRows()
	if unscored {
		m.statusMsg = fmt.Sprintf("/X: QSO #%d (%s) marked unscored", id, call)
	} else {
		m.statusMsg = fmt.Sprintf("/X: QSO #%d (%s) restored to scored", id, call)
	}
}

// zapLastQSO is the ZAP command: permanently deletes the single most
// recently logged QSO for the active station profile (SD's quick "oops,
// undo that" — no confirmation, matching SD's own ZAP behavior).
func (m *model) zapLastQSO() {
	recent, err := m.store.recentQSOs(m.activeStation.ID, 1)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	if len(recent) == 0 {
		m.statusMsg = "ZAP: no QSO to delete"
		m.clearQSOForm()
		return
	}
	last := recent[0]
	if err := m.store.deleteQSO(m.activeStation.ID, last.id); err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	m.rebuildContestIndex()
	m.refreshTableRows()
	m.clearQSOForm()
	m.statusMsg = fmt.Sprintf("ZAP: deleted QSO #%d (%s)", last.id, last.call)
}

// setDupeBaseline is the SETDUPE command: resets the dupe-check baseline to
// now (see model.dupeBaselineAfter), so a station worked earlier in the
// contest no longer blocks working it again — for multi-period sprints the
// event catalog doesn't model as distinct sessions.
func (m *model) setDupeBaseline() {
	m.dupeBaselineAfter = time.Now().UTC()
	m.clearQSOForm()
	m.checkDupe()
	m.statusMsg = "SETDUPE: dupe check now ignores QSOs logged before now"
}

// dupeCheckScope resolves the contest_id/event/dupe_scope to check against
// for the currently selected contest (blank fields mean "no known contest",
// which isDupe treats as the casual 15-minute window). Shared by the live
// dupeWarning indicator (checkDupe) and the authoritative check performed
// immediately before insert (logCurrentQSO), so both always agree.
func (m model) dupeCheckScope() (contestID, eventID, dupeScope string) {
	if event, ok := m.eventForContestID(); ok {
		return strings.TrimSpace(m.contestFields[contestName].Value()), event.ID, event.DupeScope
	}
	return "", "", ""
}

// checkDupe refreshes the dupeWarning indicator shown while a QSO is being
// entered. It must be called whenever anything the dupe check depends on
// changes — not just the callsign or band, but also which contest is
// selected — or the indicator can go stale and disagree with the
// authoritative check logCurrentQSO performs right before insert.
func (m *model) checkDupe() {
	contestID, eventID, dupeScope := m.dupeCheckScope()
	// checkDupe already runs on every keystroke that could change which
	// contest is active (Call field, band change, catalog selection, F7
	// contest-name typing), so this is the one place that needs to notice a
	// contest switch and keep contestIndex in sync — see
	// rebuildContestIndex's doc comment for the other sync points (edit
	// save, delete) that a same-contest data change needs instead.
	if contestID != m.contestIndexID {
		m.rebuildContestIndex()
	}
	call := normalizeCall(m.fields[fieldCall].Value())
	m.dupeWarning = false
	// Runs even for a blank call (e.g. the operator backspaced it out) so a
	// stale autofilled zone from the previous partial call doesn't linger.
	m.autofillReceivedExchange(call)
	if call == "" {
		if m.workedCall != "" {
			m.workedCall = ""
			m.refreshTableRows()
		}
		return
	}
	if m.workedCall != call {
		m.showWorkedCall(call)
	}
	if _, ok := m.eventForContestID(); !ok {
		if raw := strings.TrimSpace(m.contestFields[contestName].Value()); raw != "" && raw != m.contestScopeFallbackFor {
			m.contestScopeFallbackFor = raw
			m.statusMsg = fmt.Sprintf("contest %q not found in event catalog — dupe check uses the 15-minute casual window", raw)
		}
	}
	dupe, err := m.store.isDupe(call, m.qsoBand(), contestID, eventID, dupeScope, m.activeStation.ID, m.editingQSOID, time.Now(), m.dupeBaselineAfter)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	m.dupeWarning = dupe
}

// autofillReceivedExchange prefills the worked station's received-exchange
// field with a zone derived from its callsign (roadmap Appendix B.8), for
// contests whose exchange the catalog identifies as a CQ or ITU zone
// (eventDefinition.receivedExchangeZoneKind). It runs on every Call-field
// keystroke alongside checkDupe, so the guess sharpens as the operator types
// more of the call, and never touches the field once the operator has edited
// it themselves this QSO (contestExchangeRcvdEdited) — the operator's typed
// value always wins, matching nextSerial's carried-forward-manual-correction
// pattern.
func (m *model) autofillReceivedExchange(call string) {
	if m.contestExchangeRcvdEdited {
		return
	}
	event, ok := m.eventForContestID()
	if !ok {
		return
	}
	kind := event.receivedExchangeZoneKind()
	if kind == "" {
		return
	}
	zone := ""
	if call != "" {
		if table, err := sharedDXCCTable(); err == nil {
			if entity, found := table.lookup(call); found {
				switch kind {
				case "cq_zone":
					if entity.CQZone > 0 {
						zone = strconv.Itoa(entity.CQZone)
					}
				case "itu_zone":
					if entity.ITUZone > 0 {
						zone = strconv.Itoa(entity.ITUZone)
					}
				}
			}
		}
	}
	m.contestFields[contestExchangeRcvd].SetValue(zone)
}

func (m model) qsoBand() string {
	return strings.ToUpper(strings.TrimSpace(m.fields[fieldBand].Value()))
}

func (m model) qsoFrequency() string {
	return strings.TrimSpace(m.fields[fieldFrequency].Value())
}

func (m *model) selectBand(direction int) {
	index := bandIndex(m.qsoBand())
	if index < 0 {
		index = 0
	}
	index = (index + direction + len(amateurBands)) % len(amateurBands)
	band := amateurBands[index]
	m.fields[fieldBand].SetValue(band.Name)
	m.fields[fieldFrequency].SetValue(band.DefaultMHz)
	m.checkDupe()
	m.statusMsg = fmt.Sprintf("%s selected — frequency %s MHz", band.Name, band.DefaultMHz)
}

func (m *model) showWorkedCall(call string) {
	contacts, err := m.store.qsosByCall(m.activeStation.ID, call)
	if err != nil {
		m.statusMsg = fmt.Sprintf("history error: %v", err)
		return
	}
	rows := make([]table.Row, 0, len(contacts))
	for _, q := range contacts {
		rows = append(rows, table.Row{q.time.Format("2006-01-02 15:04"), q.call, q.band, q.rstSent, q.rstRcvd})
	}
	m.table.SetRows(rows)
	// recentQSOs backs whatever the table currently displays, not just the
	// default Recent QSOs list — F9 resolves a selected row through this
	// slice, and it must match what's on screen or edit/delete would act on
	// the wrong QSO.
	m.recentQSOs = contacts
	if len(rows) > 0 {
		if cur := m.table.Cursor(); cur < 0 || cur >= len(rows) {
			m.table.SetCursor(0)
		}
	}
	m.workedCall = call
}

// entrySlot identifies one focusable input in the QSO Entry field row: a base
// field (m.fields[idx]) or, while a contest is active, a received-exchange
// field captured inline (m.contestFields[idx]) so the worked station's
// exchange is logged without leaving the screen for Contest Entry (F7). Base
// slots always occupy positions 0..fieldCount-1 in the returned order, so the
// existing focusIdx==fieldCall/fieldBand checks keep working unchanged.
type entrySlot struct {
	contest bool
	post    bool
	idx     int
	label   string
}

// entrySlots returns the ordered focusable inputs for the QSO Entry screen: the
// base fields, followed (when a contest is active) by the worked station's
// received exchange, followed (in POST mode) by the operator-typed Date/Time.
// A serial-exchange contest (e.g. CW Open) receives a serial plus an exchange;
// other contests receive just the exchange (zone, state, …). The POST slot is
// always last so it never disturbs the fixed 0..fieldCount-1 base-slot
// positions or the fieldCount jump target Enter's fast-path relies on.
func (m model) entrySlots() []entrySlot {
	slots := make([]entrySlot, 0, fieldCount+3)
	for i := 0; i < fieldCount; i++ {
		slots = append(slots, entrySlot{idx: i, label: fieldLabels[i]})
	}
	if event, ok := m.eventForContestID(); ok {
		if event.SentSerial {
			slots = append(slots, entrySlot{contest: true, idx: contestSerialRcvd, label: "Rcv #"})
		}
		slots = append(slots, entrySlot{contest: true, idx: contestExchangeRcvd, label: "Rcv Exch"})
	}
	if m.postMode && m.editingQSOID == 0 {
		// logCurrentQSO's edit branch never reads postFields — editing an
		// existing QSO doesn't rewrite its timestamp — so the slot is hidden
		// during an edit rather than shown but silently inert.
		slots = append(slots, entrySlot{post: true, idx: postTimestamp, label: "Date/Time UTC"})
	}
	return slots
}

// focusedInput returns a pointer to the textinput backing the focused slot, so
// the Update loop can route keystrokes and edits to the right input whether it
// is a base field or an inline received-exchange field.
func (m *model) focusedInput() *textinput.Model {
	slots := m.entrySlots()
	if m.focusIdx < 0 || m.focusIdx >= len(slots) {
		return nil
	}
	switch s := slots[m.focusIdx]; {
	case s.contest:
		return &m.contestFields[s.idx]
	case s.post:
		return &m.postFields[s.idx]
	default:
		return &m.fields[s.idx]
	}
}

func (m *model) focusField(i int) {
	for idx := range m.fields {
		m.fields[idx].Blur()
	}
	for idx := range m.contestFields {
		m.contestFields[idx].Blur()
	}
	for idx := range m.postFields {
		m.postFields[idx].Blur()
	}
	slots := m.entrySlots()
	if i < 0 || i >= len(slots) {
		i = 0
	}
	switch s := slots[i]; {
	case s.contest:
		m.contestFields[s.idx].Focus()
	case s.post:
		m.postFields[s.idx].Focus()
	default:
		m.fields[s.idx].Focus()
	}
	m.focusIdx = i
}

func focusTextFields(fields []textinput.Model, focused int) {
	for index := range fields {
		if index == focused {
			fields[index].Focus()
		} else {
			fields[index].Blur()
		}
	}
}

func (m *model) openQSODetails() {
	m.screen = qsoDetailsScreen
	m.detailFocusIdx = 0
	focusTextFields(m.detailFields, m.detailFocusIdx)
}

func (m *model) openQSOContest() {
	m.screen = qsoContestScreen
	m.contestFocusIdx = 0
	focusTextFields(m.contestFields, m.contestFocusIdx)
}

// openContinentPanel opens the Worked/Needed by Continent screen (Ctrl+W)
// for the active contest. continentBandFocus starts on the currently
// selected QSO Entry band so the operator lands on the band they're running.
func (m *model) openContinentPanel() {
	m.screen = continentScreen
	bands := m.continentPanelBands()
	m.continentBandFocus = 0
	current := strings.ToUpper(strings.TrimSpace(m.qsoBand()))
	for index, band := range bands {
		if band == current {
			m.continentBandFocus = index
			break
		}
	}
}

// openHelpPanel opens the in-app command reference (Ctrl+G), reachable from
// any screen (roadmap §3 Phase 3: "in-app HELP for the new commands").
func (m *model) openHelpPanel() {
	m.helpReturnScreen = m.screen
	m.screen = helpScreen
}

// continentPanelBands returns the bands the Worked/Needed by Continent
// screen pages through: the active event's allowed bands when one is
// selected (narrower and more relevant), else every supported amateur band.
func (m model) continentPanelBands() []string {
	if event, ok := m.eventForContestID(); ok && len(event.Bands) > 0 {
		return event.Bands
	}
	bands := make([]string, len(amateurBands))
	for i, b := range amateurBands {
		bands[i] = b.Name
	}
	return bands
}

func (m *model) openEventCatalog() {
	m.screen = eventCatalogScreen
	m.eventFocus = 0
	m.eventSessionFocus = 0
}

func (m *model) selectEvent(event eventDefinition, session eventSession) {
	for index := range m.contestFields {
		m.contestFields[index].SetValue("")
	}
	m.contestFields[contestName].SetValue(event.ID + "-" + session.ID)
	if event.SentSerial {
		m.nextSerial = 1
		m.contestFields[contestSerialSent].SetValue(formatSerial(m.nextSerial))
	} else {
		m.nextSerial = 0
	}
	m.contestFields[contestExchangeSent].Placeholder = event.SentExchangeHint
	m.contestFields[contestExchangeRcvd].Placeholder = event.RcvdExchangeHint
	m.contestExchangeRcvdEdited = false
	m.dupeBaselineAfter = time.Time{}
	m.statusMsg = event.Name + " selected"
	m.exchangeChoiceFocus = -1
	m.openQSOContest()
	// Selecting a contest changes its dupe_scope, so the dupeWarning
	// indicator (set for whatever contest — or none — was active before)
	// must be recomputed against the new scope.
	m.checkDupe()
}

// eventForContestID resolves the free-typed/catalog-selected contest name
// back to its catalog event, preferring the longest matching event.ID. The
// catalog has real cases where one event's ID is itself a prefix of
// another's (e.g. "UBA-SPRING-CONTEST" and "UBA-SPRING-CONTEST-2"): taking
// the first prefix match in catalog order could resolve a
// "UBA-SPRING-CONTEST-2-<session>" contest name to the wrong, shorter
// event, using its bands/dupe_scope instead of the correct one's.
func (m model) eventForContestID() (eventDefinition, bool) {
	id := m.contestFields[contestName].Value()
	var best eventDefinition
	found := false
	for _, event := range m.events {
		if strings.HasPrefix(id, event.ID+"-") && (!found || len(event.ID) > len(best.ID)) {
			best = event
			found = true
		}
	}
	return best, found
}

func (m model) exchangeChoices() []exchangeOption {
	if m.contestFocusIdx != contestExchangeRcvd {
		return nil
	}
	event, ok := m.eventForContestID()
	if !ok {
		return nil
	}
	prefix := strings.ToUpper(strings.TrimSpace(m.contestFields[contestExchangeRcvd].Value()))
	var matches []exchangeOption
	for _, option := range event.ReceivedExchangeOptions {
		if prefix == "" || strings.HasPrefix(strings.ToUpper(option.Code), prefix) || strings.HasPrefix(strings.ToUpper(option.Name), prefix) {
			matches = append(matches, option)
		}
	}
	return matches
}

func (m *model) startQSOClockIfLeavingCall() {
	if m.editingQSOID != 0 || m.focusIdx != fieldCall || !m.qsoStartedAt.IsZero() || strings.TrimSpace(m.fields[fieldCall].Value()) == "" {
		return
	}
	m.qsoStartedAt = time.Now().UTC()
	m.statusMsg = "QSO timer started"
}

func (m *model) autoFillPOTAReference() tea.Cmd {
	call := normalizeCall(m.fields[fieldCall].Value())
	if call == "" {
		return nil
	}
	if reference, ok := recentClusterPOTAReference(m.clusterSpots, call, time.Now()); ok && strings.TrimSpace(m.detailFields[detailPOTARef].Value()) == "" {
		m.detailFields[detailPOTARef].SetValue(reference)
		m.statusMsg = "POTA " + reference + " from recent cluster spot"
	}
	return lookupPOTASpot(call, time.Now())
}

func (m *model) autoFillFromQRZ() tea.Cmd {
	call := normalizeCall(m.fields[fieldCall].Value())
	if call == "" || m.qrzXMLCreds.empty() {
		return nil
	}
	// There is one active entry form. Starting a lookup for a new call means
	// the old unsaved form was abandoned, so its result must be discarded.
	if m.qrzActiveLookup != 0 {
		delete(m.qrzLookups, m.qrzActiveLookup)
	}
	m.qrzLookupSequence++
	requestID := m.qrzLookupSequence
	if m.qrzLookups == nil {
		m.qrzLookups = make(map[uint64]qrzLookupPending)
	}
	m.qrzLookups[requestID] = qrzLookupPending{call: call}
	m.qrzActiveLookup = requestID
	return lookupQRZCallsignCmdForRequest(m.qrzXMLCreds, m.qrzXMLSessionKey, call, requestID)
}

// applyQRZRecordToLoggedQSO patches a QRZ callsign-lookup result into a QSO
// that was already saved to the database, for the case where the lookup
// resolves after the operator logged the QSO and cleared the form (see the
// qrzCallsignLookupMsg handler in Update). It only fills fields that are
// still blank on the saved record, mirroring the live-form autofill's
// don't-overwrite-what-the-operator-typed behavior.
func (m *model) applyQRZRecordToLoggedQSO(id int64, call string, record qrzCallsignRecord) error {
	q, err := m.store.qsoByID(m.activeStation.ID, id)
	if err != nil {
		return err
	}
	if normalizeCall(q.call) != call {
		// The id has since been edited to a different callsign; the lookup
		// no longer applies to whatever this id now holds.
		return nil
	}
	filled := false
	if record.name != "" && strings.TrimSpace(q.name) == "" {
		q.name = record.name
		filled = true
	}
	if record.qth != "" && strings.TrimSpace(q.qth) == "" {
		q.qth = record.qth
		filled = true
	}
	if record.grid != "" && strings.TrimSpace(q.grid) == "" {
		q.grid = record.grid
		filled = true
	}
	if record.state != "" && strings.TrimSpace(q.state) == "" {
		q.state = record.state
		filled = true
	}
	if record.county != "" && strings.TrimSpace(q.county) == "" {
		q.county = record.county
		filled = true
	}
	if record.email != "" && strings.TrimSpace(q.email) == "" {
		q.email = record.email
		filled = true
	}
	if !filled {
		return nil
	}
	if err := m.store.updateQSO(id, q); err != nil {
		return err
	}
	m.statusMsg = "QRZ: filled details for " + call + " (already logged)"
	m.refreshTableRows()
	return nil
}

type qrzLookupPending struct {
	call  string
	qsoID int64 // zero until the operator saves the form that started it
}

// bindQRZLookupToQSO attaches the one lookup started for the current form to
// the just-inserted QSO. A result that already arrived has been removed from
// qrzLookups, so it cannot leave a stale id for a later same-call lookup.
func (m *model) bindQRZLookupToQSO(call string, id int64) {
	requestID := m.qrzActiveLookup
	m.qrzActiveLookup = 0
	pending, ok := m.qrzLookups[requestID]
	if !ok || pending.call != call {
		return
	}
	pending.qsoID = id
	m.qrzLookups[requestID] = pending
}

func (m *model) resetQSOClockIfReturningToCall(nextFocus int) {
	if nextFocus != fieldCall || m.qsoStartedAt.IsZero() {
		return
	}
	m.qsoStartedAt = time.Time{}
	m.statusMsg = "QSO timer reset; enter callsign and continue"
}

// updateRecentQSOsTable handles input while the Recent QSOs table has
// focus (toggled by F9): Up/Down (and the table's other built-in bindings)
// move the selection, Enter opens the selected QSO for editing, "d" deletes
// it after a second "d" confirms, and Esc/F9 returns focus to the entry
// fields.
func (m model) updateRecentQSOsTable(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.setTableFocused(false)
		m.statusMsg = ""
		return m, nil
	case "enter":
		if q, ok := m.selectedRecentQSO(); ok {
			m.beginEditQSO(q)
		}
		return m, nil
	case "d":
		q, ok := m.selectedRecentQSO()
		if !ok {
			return m, nil
		}
		if !m.deleteArmed {
			m.deleteArmed = true
			m.statusMsg = fmt.Sprintf("press d again to permanently delete %s, any other key cancels", q.call)
			return m, nil
		}
		m.deleteArmed = false
		if err := m.store.deleteQSO(m.activeStation.ID, q.id); err != nil {
			m.statusMsg = fmt.Sprintf("db error: %v", err)
			return m, nil
		}
		m.statusMsg = "deleted " + q.call
		m.refreshTableRows()
		// Unconditional full recompute: the deleted QSO may have belonged to
		// the active contest (cheap no-op via rebuildContestIndex otherwise).
		m.rebuildContestIndex()
		return m, nil
	default:
		if m.deleteArmed {
			m.deleteArmed = false
			m.statusMsg = "delete cancelled"
			return m, nil
		}
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
}

func (m model) logCurrentQSO() (model, tea.Cmd) {
	call := normalizeCall(m.fields[fieldCall].Value())
	if call == "" {
		m.statusMsg = "callsign required"
		return m, nil
	}
	if err := validateBandFrequency(m.qsoBand(), m.qsoFrequency()); err != nil {
		m.statusMsg = err.Error()
		return m, nil
	}
	if event, ok := m.eventForContestID(); ok && len(event.Bands) > 0 && !bandAllowed(event.Bands, m.qsoBand()) {
		m.statusMsg = fmt.Sprintf("%s is not in %s's allowed bands (%s) — not logged", m.qsoBand(), event.Name, strings.Join(event.Bands, "/"))
		return m, nil
	}
	var postTime time.Time
	if m.postMode {
		typed := strings.TrimSpace(m.postFields[postTimestamp].Value())
		parsed, err := time.Parse(postTimestampLayout, typed)
		if err != nil {
			m.statusMsg = fmt.Sprintf("POST mode: Date/Time must be %q (UTC) — not logged", postTimestampLayout)
			return m, nil
		}
		postTime = parsed
	}
	// Re-check against the database rather than trusting the cached
	// dupeWarning indicator: the operator can change the contest selection
	// (which changes dupe_scope) or the band without every intermediate
	// state necessarily having gone through checkDupe, and this is the last
	// chance to catch a real contest duplicate before it's committed.
	// editingQSOID excludes the record itself from the check when editing.
	contestID, eventID, dupeScope := m.dupeCheckScope()
	dupeAt := time.Now()
	if !postTime.IsZero() {
		dupeAt = postTime
	}
	dupe, err := m.store.isDupe(call, m.qsoBand(), contestID, eventID, dupeScope, m.activeStation.ID, m.editingQSOID, dupeAt, m.dupeBaselineAfter)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return m, nil
	}
	if dupe {
		m.dupeWarning = true
		m.statusMsg = fmt.Sprintf("DUPE: %s already worked on %s — not logged", call, m.qsoBand())
		return m, nil
	}

	// The fields the operator can actually edit, whether logging a new QSO
	// or correcting an existing one.
	edited := qso{
		call:      call,
		band:      m.qsoBand(),
		rstSent:   m.fields[fieldRSTSent].Value(),
		rstRcvd:   m.fields[fieldRSTRcvd].Value(),
		frequency: m.qsoFrequency(),
		name:      strings.TrimSpace(m.detailFields[detailName].Value()),
		qth:       strings.TrimSpace(m.detailFields[detailQTH].Value()),
		grid:      strings.TrimSpace(m.detailFields[detailGrid].Value()),
		state:     strings.TrimSpace(m.detailFields[detailState].Value()),
		county:    strings.TrimSpace(m.detailFields[detailCounty].Value()),
		email:     strings.TrimSpace(m.detailFields[detailEmail].Value()),
		potaRef:   strings.ToUpper(strings.TrimSpace(m.detailFields[detailPOTARef].Value())),
		parkName:  strings.TrimSpace(m.detailFields[detailParkName].Value()),
		comment:   strings.TrimSpace(m.detailFields[detailNotes].Value()),
		contestID: strings.TrimSpace(m.contestFields[contestName].Value()),
		stx:       strings.TrimSpace(m.contestFields[contestSerialSent].Value()),
		stxString: strings.TrimSpace(m.contestFields[contestExchangeSent].Value()),
		srx:       strings.TrimSpace(m.contestFields[contestSerialRcvd].Value()),
		srxString: strings.TrimSpace(m.contestFields[contestExchangeRcvd].Value()),
	}

	if m.editingQSOID != 0 {
		// Start from the original record so its start/end time and
		// station-identity snapshot (never rewritten by a later profile
		// change — see "Log data" in README.md) survive untouched; only the
		// fields actually shown for editing are overwritten.
		logged := m.editingOriginal
		logged.call, logged.band, logged.rstSent, logged.rstRcvd, logged.frequency = edited.call, edited.band, edited.rstSent, edited.rstRcvd, edited.frequency
		logged.name, logged.qth, logged.grid, logged.state, logged.county, logged.email, logged.potaRef, logged.parkName, logged.comment = edited.name, edited.qth, edited.grid, edited.state, edited.county, edited.email, edited.potaRef, edited.parkName, edited.comment
		logged.contestID, logged.stx, logged.stxString, logged.srx, logged.srxString = edited.contestID, edited.stx, edited.stxString, edited.srx, edited.srxString
		if logged.call != m.editingOriginal.call {
			// A changed callsign can resolve to a different DXCC entity;
			// don't carry forward context resolved for the old one.
			logged.country, logged.cqZone, logged.ituZone, logged.dxccNumber = "", "", "", ""
		}
		if err := m.store.updateQSO(m.editingQSOID, logged); err != nil {
			m.statusMsg = fmt.Sprintf("db error: %v", err)
			return m, nil
		}
		m.editingQSOID = 0
		m.editingOriginal = qso{}
		m.clearQSOForm()
		m.restorePreEditContestSelection()
		// Unconditional, not checkDupe's lazy diff-check: editing a QSO's
		// call/band within the contest that's still active afterward doesn't
		// change contestIndexID, but the underlying data did change.
		m.rebuildContestIndex()
		m.statusMsg = "updated " + call
		m.refreshTableRows()
		return m, nil
	}

	var startedAt, endedAt time.Time
	if m.postMode {
		// POST mode (SD's after-contest re-entry): postTime was parsed before
		// duplicate checking so both persistence and the casual 15-minute
		// duplicate window use the actual paper-log instant.
		startedAt, endedAt = postTime, postTime
	} else {
		endedAt = time.Now().UTC()
		startedAt = m.qsoStartedAt
		if startedAt.IsZero() {
			// Tab normally starts the clock. This fallback keeps a QSO valid if an
			// operator uses a different terminal navigation sequence.
			startedAt = endedAt
		}
	}
	logged := edited
	logged.mode = cwMode
	logged.profileID = m.activeStation.ID
	logged.time = startedAt
	logged.timeOff = endedAt
	logged.myGridSquare = m.activeStation.MyGridSquare
	logged.stationCallsign = m.activeStation.Callsign
	logged.operatorName = m.activeStation.OperatorName
	logged.myRig = m.activeStation.Rig
	logged.myAntenna = m.activeStation.Antenna
	logged.txPower = m.activeStation.PowerWatts
	destinations := m.uploadDestinations()
	id, err := m.store.insertQSOWithUploads(logged, destinations, time.Now().Add(uploadBufferDelay))
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return m, nil
	}
	m.statusMsg = fmt.Sprintf("logged %s (%s)", call, endedAt.Sub(startedAt).Round(time.Second))
	// Incremental update instead of a full rebuild — cheap, and correct as
	// long as the QSO just logged belongs to the contest the index was built
	// for (checkDupe kept contestIndexID in sync with contestID while the
	// operator was typing, so this should always hold).
	if m.contestIndex != nil && strings.TrimSpace(logged.contestID) == m.contestIndexID {
		m.contestIndex.record(logged)
	}
	// Advance the running serial past the one just sent so the next QSO shows
	// the next number. Deriving it from the value actually logged (rather than
	// a blind ++) carries forward any manual correction the operator made to
	// the Sent Serial field.
	if m.nextSerial > 0 {
		if sent, err := strconv.Atoi(logged.stx); err == nil {
			m.nextSerial = sent + 1
		} else {
			m.nextSerial++
		}
	}
	m.qsoStartedAt = time.Time{}
	m.workedCall = ""
	m.bindQRZLookupToQSO(call, id)
	m.clearQSOForm()
	m.refreshTableRows()
	return m, nil
}

// uploadBufferDelay is how long a freshly logged QSO waits before its first
// delivery attempt. It gives the operator a window to fix a mistyped call or
// other field (via Edit) before the QSO goes out to external services, which
// don't offer a clean way to retract or correct an upload. The QSO is read
// fresh from the database at send time, so an edit made during this window is
// picked up automatically.
const uploadBufferDelay = 60 * time.Second

// uploadDrainInterval is how often the outbox is polled for due deliveries.
// uploadInFlightLease exceeds the per-upload timeout so a delivery already in
// flight is never re-claimed and double-sent by the next tick.
const (
	uploadDrainInterval = 20 * time.Second
	uploadInFlightLease = 90 * time.Second
	uploadDrainBatch    = 20
)

// uploadDrainMsg wakes the outbox drain on a fixed interval.
type uploadDrainMsg struct{}

func uploadDrainTickCmd() tea.Cmd {
	return tea.Tick(uploadDrainInterval, func(time.Time) tea.Msg { return uploadDrainMsg{} })
}

// uploadDestinations reports the configured external destinations to insert
// with a new QSO. logCurrentQSO passes this list to insertQSOWithUploads so the
// QSO and its initial durable delivery rows commit together. A destination with
// no credentials is omitted so its rows cannot pile up unsendable.
func (m model) uploadDestinations() []string {
	var destinations []string
	if strings.TrimSpace(m.qrzAPIKey) != "" {
		destinations = append(destinations, uploadDestQRZ)
	}
	if strings.TrimSpace(m.wrlAPIKey) != "" {
		destinations = append(destinations, uploadDestWRL)
	}
	return destinations
}

// drainOutbox claims every delivery now due and returns the upload commands to
// run for them. Each claimed QSO is read fresh (picking up edits); a delivery
// whose QSO was deleted, or whose destination is no longer configured, is
// dropped from the outbox instead of retried forever.
func (m *model) drainOutbox() []tea.Cmd {
	entries, err := m.store.claimDueUploads(time.Now(), uploadInFlightLease, uploadDrainBatch)
	if err != nil {
		m.statusMsg = fmt.Sprintf("upload queue error: %v", err)
		return nil
	}
	var cmds []tea.Cmd
	for _, e := range entries {
		q, err := m.store.qsoByID(e.profileID, e.qsoID)
		if err != nil {
			_ = m.store.markUploadDone(e.qsoID, e.destination)
			continue
		}
		var cmd tea.Cmd
		switch e.destination {
		case uploadDestQRZ:
			cmd = m.qrzOutboxUploadCmd(q)
		case uploadDestWRL:
			cmd = m.wrlOutboxUploadCmd(q)
		}
		if cmd == nil {
			// Unknown or unconfigured destination: don't leave it stuck.
			_ = m.store.markUploadDone(e.qsoID, e.destination)
			continue
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

// qrzOutboxUploadCmd persists the outcome before returning its Tea message.
// Bubble Tea intentionally does not wait for commands when the UI exits; if a
// service accepted a request but only Update removed the outbox row, quitting
// in that gap caused a duplicate remote delivery on next launch. bgTasks makes
// shutdown wait for this closure while the store is still open.
func (m model) qrzOutboxUploadCmd(q qso) tea.Cmd {
	if strings.TrimSpace(m.qrzAPIKey) == "" {
		return nil
	}
	parent := m.bgCtx
	if parent == nil {
		parent = context.Background()
	}
	wg := m.bgTasks
	if wg != nil {
		wg.Add(1)
	}
	st, apiKey := m.store, m.qrzAPIKey
	return func() tea.Msg {
		if wg != nil {
			defer wg.Done()
		}
		ctx, cancel := context.WithTimeout(parent, qrzUploadTimeout)
		defer cancel()
		logID, err := uploadQSOToQRZ(ctx, apiKey, q)
		if err != nil {
			queueErr := st.recordUploadFailure(q.id, uploadDestQRZ, err.Error(), time.Now())
			return qrzUploadMsg{qsoID: q.id, call: q.call, err: err, deliveryPersisted: queueErr == nil, queueErr: queueErr}
		}
		queueErr := st.markUploadDone(q.id, uploadDestQRZ)
		return qrzUploadMsg{qsoID: q.id, call: q.call, logID: logID, deliveryPersisted: queueErr == nil, queueErr: queueErr}
	}
}

func (m model) wrlOutboxUploadCmd(q qso) tea.Cmd {
	if strings.TrimSpace(m.wrlAPIKey) == "" {
		return nil
	}
	parent := m.bgCtx
	if parent == nil {
		parent = context.Background()
	}
	wg := m.bgTasks
	if wg != nil {
		wg.Add(1)
	}
	st, apiKey, logbookID := m.store, m.wrlAPIKey, m.wrlLogbookID
	return func() tea.Msg {
		if wg != nil {
			defer wg.Done()
		}
		ctx, cancel := context.WithTimeout(parent, wrlUploadTimeout)
		defer cancel()
		err := uploadQSOToWRL(ctx, apiKey, logbookID, q)
		if err != nil {
			queueErr := st.recordUploadFailure(q.id, uploadDestWRL, err.Error(), time.Now())
			return wrlUploadMsg{qsoID: q.id, call: q.call, err: err, deliveryPersisted: queueErr == nil, queueErr: queueErr}
		}
		queueErr := st.markUploadDone(q.id, uploadDestWRL)
		return wrlUploadMsg{qsoID: q.id, call: q.call, deliveryPersisted: queueErr == nil, queueErr: queueErr}
	}
}

// clearQSOForm resets the fields that should go blank between QSOs. Band,
// Frequency, and RST Sent are intentionally left as-is: an operator
// typically stays on the same band/frequency for consecutive contacts, and
// 599 is the default sent report. The contest name/session and the operator's
// own sent exchange are likewise kept so an operator stays "in the event"
// across contacts; only the other station's received exchange is cleared. The
// Sent Serial field is re-displayed with the next running serial (see
// nextSerial) so the operator always sees the number they will send next.
func (m *model) clearQSOForm() {
	m.fields[fieldCall].SetValue("")
	m.fields[fieldRSTRcvd].SetValue("")
	for index := range m.detailFields {
		m.detailFields[index].SetValue("")
	}
	m.contestFields[contestSerialRcvd].SetValue("")
	m.contestFields[contestExchangeRcvd].SetValue("")
	m.contestExchangeRcvdEdited = false
	if m.nextSerial > 0 {
		m.contestFields[contestSerialSent].SetValue(formatSerial(m.nextSerial))
	}
	m.focusField(fieldCall)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if message, ok := msg.(potaLookupMsg); ok {
		call := normalizeCall(m.fields[fieldCall].Value())
		if message.call != call {
			return m, nil
		}
		if message.err != nil {
			m.statusMsg = "POTA lookup unavailable: " + message.err.Error()
			return m, nil
		}
		filled := false
		if message.reference != "" && strings.TrimSpace(m.detailFields[detailPOTARef].Value()) == "" {
			m.detailFields[detailPOTARef].SetValue(message.reference)
			filled = true
		}
		if message.parkName != "" && strings.TrimSpace(m.detailFields[detailParkName].Value()) == "" {
			m.detailFields[detailParkName].SetValue(message.parkName)
			filled = true
		}
		if filled {
			label := message.reference
			if label == "" {
				label = message.parkName
			}
			m.statusMsg = "POTA " + label + " from recent POTA spot"
		}
		return m, nil
	}
	if message, ok := msg.(qrzCallsignLookupMsg); ok {
		if message.sessionKey != "" {
			m.qrzXMLSessionKey = message.sessionKey
		}
		call := normalizeCall(m.fields[fieldCall].Value())
		if message.requestID != 0 {
			pending, known := m.qrzLookups[message.requestID]
			if !known {
				// A newer form superseded this request, or its result was already
				// handled. Never apply an orphaned result by callsign alone.
				return m, nil
			}
			delete(m.qrzLookups, message.requestID)
			if m.qrzActiveLookup == message.requestID {
				m.qrzActiveLookup = 0
			}
			if message.err != nil {
				if pending.qsoID == 0 && message.call == call {
					m.statusMsg = "QRZ lookup unavailable: " + message.err.Error()
				}
				return m, nil
			}
			if pending.qsoID != 0 {
				if m.editingQSOID != pending.qsoID {
					if err := m.applyQRZRecordToLoggedQSO(pending.qsoID, pending.call, message.record); err != nil {
						m.statusMsg = fmt.Sprintf("db error: %v", err)
					}
				}
				return m, nil
			}
			if message.call != call {
				return m, nil
			}
		}
		if message.call != call {
			// Legacy/test messages without a request id cannot safely be bound
			// after the operator moved on, so discard them.
			return m, nil
		}
		if message.err != nil {
			m.statusMsg = "QRZ lookup unavailable: " + message.err.Error()
			return m, nil
		}
		filled := false
		if message.record.name != "" && strings.TrimSpace(m.detailFields[detailName].Value()) == "" {
			m.detailFields[detailName].SetValue(message.record.name)
			filled = true
		}
		if message.record.qth != "" && strings.TrimSpace(m.detailFields[detailQTH].Value()) == "" {
			m.detailFields[detailQTH].SetValue(message.record.qth)
			filled = true
		}
		if message.record.grid != "" && strings.TrimSpace(m.detailFields[detailGrid].Value()) == "" {
			m.detailFields[detailGrid].SetValue(message.record.grid)
			filled = true
		}
		if message.record.state != "" && strings.TrimSpace(m.detailFields[detailState].Value()) == "" {
			m.detailFields[detailState].SetValue(message.record.state)
			filled = true
		}
		if message.record.county != "" && strings.TrimSpace(m.detailFields[detailCounty].Value()) == "" {
			m.detailFields[detailCounty].SetValue(message.record.county)
			filled = true
		}
		if message.record.email != "" && strings.TrimSpace(m.detailFields[detailEmail].Value()) == "" {
			m.detailFields[detailEmail].SetValue(message.record.email)
			filled = true
		}
		if filled {
			m.statusMsg = "QRZ: filled details for " + message.call
		}
		return m, nil
	}
	if _, ok := msg.(solarTickMsg); ok {
		return m, tea.Batch(fetchSolarIndicesCmd(), solarTickCmd())
	}
	if message, ok := msg.(solarIndicesMsg); ok {
		if message.err != nil {
			m.solarErr = message.err.Error()
		} else {
			m.solarErr = ""
			m.solar = message.indices
		}
		return m, nil
	}
	if message, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth, m.termHeight = message.Width, message.Height
		return m, nil
	}
	if _, ok := msg.(uploadDrainMsg); ok {
		cmds := m.drainOutbox()
		cmds = append(cmds, uploadDrainTickCmd())
		return m, tea.Batch(cmds...)
	}
	if message, ok := msg.(qrzUploadMsg); ok {
		if message.queueErr != nil {
			m.statusMsg = fmt.Sprintf("upload queue error: %v", message.queueErr)
		} else if message.err != nil {
			if message.deliveryPersisted {
				m.statusMsg = fmt.Sprintf("QRZ upload failed for %s (will retry): %v", message.call, message.err)
				return m, nil
			}
			if err := m.store.recordUploadFailure(message.qsoID, uploadDestQRZ, message.err.Error(), time.Now()); err != nil {
				m.statusMsg = fmt.Sprintf("upload queue error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("QRZ upload failed for %s (will retry): %v", message.call, message.err)
			}
		} else {
			if message.deliveryPersisted {
				m.statusMsg = fmt.Sprintf("QRZ upload OK for %s (LOGID %s)", message.call, message.logID)
				return m, nil
			}
			if err := m.store.markUploadDone(message.qsoID, uploadDestQRZ); err != nil {
				m.statusMsg = fmt.Sprintf("upload queue error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("QRZ upload OK for %s (LOGID %s)", message.call, message.logID)
			}
		}
		return m, nil
	}
	if message, ok := msg.(wrlUploadMsg); ok {
		if message.queueErr != nil {
			m.statusMsg = fmt.Sprintf("upload queue error: %v", message.queueErr)
		} else if message.err != nil {
			if message.deliveryPersisted {
				m.statusMsg = fmt.Sprintf("WRL upload failed for %s (will retry): %v", message.call, message.err)
				return m, nil
			}
			if err := m.store.recordUploadFailure(message.qsoID, uploadDestWRL, message.err.Error(), time.Now()); err != nil {
				m.statusMsg = fmt.Sprintf("upload queue error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("WRL upload failed for %s (will retry): %v", message.call, message.err)
			}
		} else {
			if message.deliveryPersisted {
				m.statusMsg = fmt.Sprintf("WRL upload OK for %s", message.call)
				return m, nil
			}
			if err := m.store.markUploadDone(message.qsoID, uploadDestWRL); err != nil {
				m.statusMsg = fmt.Sprintf("upload queue error: %v", err)
			} else {
				m.statusMsg = fmt.Sprintf("WRL upload OK for %s", message.call)
			}
		}
		return m, nil
	}
	if message, ok := msg.(cabrilloExportedMsg); ok {
		m.cabrilloExportInProgress = false
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("Cabrillo export failed: %v", message.err)
		} else if message.score.total() > 0 {
			m.statusMsg = fmt.Sprintf("Cabrillo exported: %d QSOs, claimed score %d (%d pts x %d mults) -> %s",
				message.count, message.score.total(), message.score.qsoPoints, message.score.multipliers, message.path)
		} else {
			m.statusMsg = fmt.Sprintf("Cabrillo exported: %d QSOs -> %s", message.count, message.path)
		}
		return m, nil
	}
	if message, ok := msg.(adifExportedMsg); ok {
		m.adifExportInProgress = false
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("ADIF export failed: %v", message.err)
		} else {
			m.statusMsg = fmt.Sprintf("ADIF exported: %d QSOs -> %s", message.count, message.path)
		}
		return m, nil
	}
	if message, ok := msg.(csvExportedMsg); ok {
		m.csvExportInProgress = false
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("CSV export failed: %v", message.err)
		} else {
			m.statusMsg = fmt.Sprintf("CSV exported: %d QSOs -> %s", message.count, message.path)
		}
		return m, nil
	}
	if message, ok := msg.(backupCompletedMsg); ok {
		m.backupInProgress = false
		if message.err != nil {
			m.statusMsg = "backup failed: " + message.err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("backed up to Google Drive: %s, %s", message.result.dbName, message.result.adifName)
		}
		return m, nil
	}
	// Handled globally, not only within updateCluster: the connection is now
	// started at app startup (see connectClusterIfNeeded), while the
	// operator is on QSO Entry, not the DX Cluster screen — these results
	// arriving while m.screen != clusterScreen must not be silently dropped,
	// or the connection (and the DX Spots panel depending on it) gets stuck
	// showing "connecting…" forever despite succeeding in the background.
	if message, ok := msg.(clusterConnectedMsg); ok {
		if message.generation != m.clusterGeneration {
			message.client.close()
			return m, nil
		}
		m.clusterConnecting = false
		if message.err != nil {
			// A dial failure is retried on the same backoff schedule as a
			// dropped read, so the feed recovers from a node that's briefly
			// down without operator intervention.
			if cmd := m.scheduleClusterReconnect(message.err); cmd != nil {
				return m, cmd
			}
			m.clusterStatus = message.err.Error()
			return m, nil
		}
		m.clusterClient = message.client
		m.clusterReconnectDelay = 0 // reset backoff on a healthy connection
		m.clusterStatus = k3lrClusterName + " — connected"
		return m, m.clusterClient.readNext()
	}
	if message, ok := msg.(clusterLineMsg); ok {
		if message.generation != m.clusterGeneration {
			return m, nil
		}
		if message.err != nil {
			// Explicitly close the socket rather than just dropping the
			// reference: the read failed, but the underlying TCP connection
			// (and its file descriptor) may still be open, so leaking it would
			// accumulate half-open sockets over repeated reconnects.
			if m.clusterClient != nil {
				m.clusterClient.close()
			}
			m.clusterClient = nil
			m.clusterConnecting = false
			if cmd := m.scheduleClusterReconnect(message.err); cmd != nil {
				return m, cmd
			}
			m.clusterStatus = fmt.Sprintf("cluster connection ended: %v", message.err)
			return m, nil
		}
		if spot, ok := parseClusterSpot(message.line, time.Now()); ok && m.clusterFilters.allowsSpot(spot) {
			m.addClusterSpot(spot)
		}
		if m.clusterClient != nil {
			return m, m.clusterClient.readNext()
		}
		return m, nil
	}
	// Handled globally, not only within updateADIFImport: the import runs
	// as an async tea.Cmd, so pressing Esc to leave the Import ADIF screen
	// before it finishes must not cause this result to be silently dropped
	// when it later arrives on a different screen.
	if message, ok := msg.(adifImportedMsg); ok {
		m.importInProgress = false
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("ADIF import failed: %v", message.err)
			return m, nil
		}
		m.refreshTableRows()
		if m.screen == adifImportScreen {
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
		}
		m.statusMsg = fmt.Sprintf("ADIF imported: %d CW QSOs; %d skipped", message.result.Imported, message.result.Skipped)
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "f8" {
		if m.backupInProgress {
			m.statusMsg = "backup already in progress…"
			return m, nil
		}
		m.backupInProgress = true
		m.statusMsg = "backing up to Google Drive…"
		return m, m.runBackupCmd()
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+w" && m.screen != continentScreen {
		m.openContinentPanel()
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+g" && m.screen != helpScreen {
		m.openHelpPanel()
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+x" {
		if m.cabrilloExportInProgress {
			m.statusMsg = "Cabrillo export already in progress…"
			return m, nil
		}
		contestID := strings.TrimSpace(m.contestFields[contestName].Value())
		if contestID == "" {
			m.statusMsg = "no contest loaded — set Contest on the Events (F7) screen first"
			return m, nil
		}
		m.cabrilloExportInProgress = true
		m.statusMsg = "exporting Cabrillo…"
		return m, m.cabrilloExportCmd(contestID)
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+o" {
		if m.adifExportInProgress {
			m.statusMsg = "ADIF export already in progress…"
			return m, nil
		}
		m.adifExportInProgress = true
		m.statusMsg = "exporting ADIF…"
		return m, m.adifExportCmd()
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+r" {
		if m.csvExportInProgress {
			m.statusMsg = "CSV export already in progress…"
			return m, nil
		}
		contestID := strings.TrimSpace(m.contestFields[contestName].Value())
		if contestID == "" {
			m.statusMsg = "no contest loaded — set Contest on the Events (F7) screen first"
			return m, nil
		}
		m.csvExportInProgress = true
		m.statusMsg = "exporting CSV…"
		return m, m.csvExportCmd(contestID)
	}
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+p" && m.screen == qsoEntryScreen {
		if m.editingQSOID != 0 {
			m.statusMsg = "POST mode: finish or cancel the current edit first"
			return m, nil
		}
		m.postMode = !m.postMode
		if m.postMode {
			m.postFields[postTimestamp].SetValue(time.Now().UTC().Format(postTimestampLayout))
			m.statusMsg = "POST mode ON — type each QSO's actual Date/Time (" + postTimestampLayout + " UTC) instead of using the live clock"
		} else {
			m.postFields[postTimestamp].SetValue("")
			m.statusMsg = "POST mode OFF — logging uses the live clock again"
		}
		m.focusField(m.focusIdx)
		return m, nil
	}
	if m.screen == stationSetupScreen {
		return m.updateStationSetup(msg)
	}
	if m.screen == clusterScreen {
		return m.updateCluster(msg)
	}
	if m.screen == clusterFiltersScreen {
		return m.updateClusterFilters(msg)
	}
	if m.screen == adifImportScreen {
		return m.updateADIFImport(msg)
	}
	if m.screen == eventCatalogScreen {
		return m.updateEventCatalog(msg)
	}
	if m.screen == qsoDetailsScreen {
		return m.updateQSODetails(msg)
	}
	if m.screen == qsoContestScreen {
		return m.updateQSOContest(msg)
	}
	if m.screen == continentScreen {
		return m.updateContinentPanel(msg)
	}
	if m.screen == helpScreen {
		return m.updateHelpPanel(msg)
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "f9" {
			m.setTableFocused(!m.tableFocused)
			if m.tableFocused {
				m.statusMsg = "Recent QSOs: ↑/↓ select, Enter view/edit, d delete, Esc/F9 done"
			} else {
				m.statusMsg = ""
			}
			return m, nil
		}
		if m.tableFocused {
			return m.updateRecentQSOsTable(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.editingQSOID != 0 {
				m.cancelEditQSO()
				return m, nil
			}
			return m, tea.Quit
		case "tab":
			slotCount := len(m.entrySlots())
			leavingCall := m.focusIdx == fieldCall
			m.startQSOClockIfLeavingCall()
			nextFocus := (m.focusIdx + 1) % slotCount
			m.resetQSOClockIfReturningToCall(nextFocus)
			m.focusField(nextFocus)
			if leavingCall {
				return m, tea.Batch(m.autoFillPOTAReference(), m.autoFillFromQRZ())
			}
			return m, nil
		case "shift+tab":
			slotCount := len(m.entrySlots())
			nextFocus := (m.focusIdx - 1 + slotCount) % slotCount
			m.resetQSOClockIfReturningToCall(nextFocus)
			m.focusField(nextFocus)
			return m, nil
		case "enter":
			if m.handleCallFieldCommand() {
				return m, nil
			}
			if m.focusIdx == len(m.entrySlots())-1 {
				var cmd tea.Cmd
				m, cmd = m.logCurrentQSO()
				return m, cmd
			}
			leavingCall := m.focusIdx == fieldCall
			m.startQSOClockIfLeavingCall()
			nextFocus := m.focusIdx + 1
			if _, contestActive := m.eventForContestID(); leavingCall && contestActive {
				// A contest is active: RST/Band/Freq are auto-filled (599 default,
				// event/cluster-selected band, typed freq) and rarely need touching
				// mid-QSO, so Enter fast-paths straight to the worked station's
				// received exchange. Tab still visits every field for the rare
				// correction, per the roadmap's "keep Tab for full field-by-field".
				// Gated on the contest itself, not just "slots grew past
				// fieldCount" — POST mode alone (no contest) also grows the
				// slot list via its trailing Date/Time slot, and RST/Band/Freq
				// still need a look when re-entering a paper log.
				nextFocus = fieldCount
			}
			m.focusField(nextFocus)
			if leavingCall {
				return m, tea.Batch(m.autoFillPOTAReference(), m.autoFillFromQRZ())
			}
			return m, nil
		case "f2":
			m.openStationSetup()
			return m, nil
		case "f3":
			return m, m.openCluster()
		case "f4":
			m.openClusterFilters()
			return m, nil
		case "f5":
			m.openADIFImport()
			return m, nil
		case "f6":
			m.openQSODetails()
			return m, nil
		case "f7":
			m.openEventCatalog()
			return m, nil
		case "left", "down":
			if m.focusIdx == fieldBand {
				m.selectBand(-1)
				return m, nil
			}
		case "right", "up":
			if m.focusIdx == fieldBand {
				m.selectBand(1)
				return m, nil
			}
		case "pgup":
			m.scrollClusterSpots(-recentQSOsVisibleRows)
			return m, nil
		case "pgdown":
			m.scrollClusterSpots(recentQSOsVisibleRows)
			return m, nil
		}
		if m.focusIdx == fieldBand {
			// Band is intentionally a closed selector. This prevents an invalid or
			// unsupported band label from being entered into a QSO, while allowing
			// non-key messages (such as textinput cursor-blink ticks) through.
			return m, nil
		}
	case tea.MouseMsg:
		// Mouse wheel scrolls the DX Spots panel regardless of where the
		// cursor is over the screen — it's the only scrollable content on
		// QSO Entry, and PageUp/PageDown are unreliable across terminal
		// emulators/multiplexers that bind them to their own scrollback.
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollClusterSpots(-3)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollClusterSpots(3)
			return m, nil
		}
		return m, nil
	}

	var cmd tea.Cmd
	// Captured before the update so a pure cursor-movement key (left/right/
	// home/end — textinput handles these but leaves Value() unchanged) isn't
	// mistaken for the operator overriding the autofilled zone; only an
	// actual content change should disable autofillReceivedExchange.
	before := ""
	if input := m.focusedInput(); input != nil {
		before = input.Value()
		*input, cmd = input.Update(msg)
	}
	if m.focusIdx == fieldCall {
		m.checkDupe()
	}
	if slots := m.entrySlots(); m.focusIdx >= 0 && m.focusIdx < len(slots) {
		if s := slots[m.focusIdx]; s.contest && s.idx == contestExchangeRcvd {
			if m.contestFields[contestExchangeRcvd].Value() != before {
				m.contestExchangeRcvdEdited = true
			}
		}
	}
	return m, cmd
}

func (m model) updateQSODetails(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "f7":
			m.openEventCatalog()
			return m, nil
		case "tab", "enter":
			m.detailFocusIdx = (m.detailFocusIdx + 1) % len(m.detailFields)
			focusTextFields(m.detailFields, m.detailFocusIdx)
			return m, nil
		case "shift+tab":
			m.detailFocusIdx = (m.detailFocusIdx - 1 + len(m.detailFields)) % len(m.detailFields)
			focusTextFields(m.detailFields, m.detailFocusIdx)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.detailFields[m.detailFocusIdx], cmd = m.detailFields[m.detailFocusIdx].Update(msg)
	return m, cmd
}

func (m model) updateQSOContest(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		choices := m.exchangeChoices()
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "f6":
			m.openQSODetails()
			return m, nil
		case "f7":
			m.openEventCatalog()
			return m, nil
		case "up":
			if len(choices) > 0 {
				m.exchangeChoiceFocus = (m.exchangeChoiceFocus - 1 + len(choices)) % len(choices)
				return m, nil
			}
		case "down":
			if len(choices) > 0 {
				m.exchangeChoiceFocus = (m.exchangeChoiceFocus + 1) % len(choices)
				return m, nil
			}
		case "tab", "enter":
			if key.String() == "enter" && m.exchangeChoiceFocus >= 0 && m.exchangeChoiceFocus < len(choices) {
				m.contestFields[contestExchangeRcvd].SetValue(choices[m.exchangeChoiceFocus].Code)
				m.contestExchangeRcvdEdited = true
				m.exchangeChoiceFocus = -1
				return m, nil
			}
			m.contestFocusIdx = (m.contestFocusIdx + 1) % len(m.contestFields)
			focusTextFields(m.contestFields, m.contestFocusIdx)
			return m, nil
		case "shift+tab":
			m.contestFocusIdx = (m.contestFocusIdx - 1 + len(m.contestFields)) % len(m.contestFields)
			focusTextFields(m.contestFields, m.contestFocusIdx)
			return m, nil
		}
	}
	var cmd tea.Cmd
	beforeExchangeRcvd := m.contestFields[contestExchangeRcvd].Value()
	m.contestFields[m.contestFocusIdx], cmd = m.contestFields[m.contestFocusIdx].Update(msg)
	m.exchangeChoiceFocus = -1
	if m.contestFocusIdx == contestName {
		// The contest name determines dupe_scope; keep the dupeWarning
		// indicator in sync as the operator types it.
		m.checkDupe()
	}
	if m.contestFocusIdx == contestExchangeRcvd && m.contestFields[contestExchangeRcvd].Value() != beforeExchangeRcvd {
		m.contestExchangeRcvdEdited = true
	}
	return m, cmd
}

func (m model) updateEventCatalog(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "f6":
			m.openQSODetails()
			return m, nil
		case "up", "k":
			m.eventFocus = (m.eventFocus - 1 + len(m.events)) % len(m.events)
			m.eventSessionFocus = 0
			return m, nil
		case "down", "j":
			m.eventFocus = (m.eventFocus + 1) % len(m.events)
			m.eventSessionFocus = 0
			return m, nil
		case "left", "h":
			sessions := m.events[m.eventFocus].Sessions
			m.eventSessionFocus = (m.eventSessionFocus - 1 + len(sessions)) % len(sessions)
			return m, nil
		case "right", "l":
			sessions := m.events[m.eventFocus].Sessions
			m.eventSessionFocus = (m.eventSessionFocus + 1) % len(sessions)
			return m, nil
		case "enter":
			event := m.events[m.eventFocus]
			m.selectEvent(event, event.Sessions[m.eventSessionFocus])
			return m, nil
		}
	}
	return m, nil
}

// updateContinentPanel drives the Worked/Needed by Continent screen
// (roadmap §3 Phase 2, Appendix B.9): Left/Right page one band at a time
// through continentPanelBands. F1 is deliberately left bound to its
// app-wide "QSO Entry" meaning (screenHotkeys' line1 advertises it on every
// screen) rather than reused for band paging as in SD, to avoid a hotkey
// that silently means different things depending which screen is up.
func (m model) updateContinentPanel(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		bands := m.continentPanelBands()
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "f1", "ctrl+w":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "left", "h":
			if len(bands) > 0 {
				m.continentBandFocus = (m.continentBandFocus - 1 + len(bands)) % len(bands)
			}
			return m, nil
		case "right", "l":
			if len(bands) > 0 {
				m.continentBandFocus = (m.continentBandFocus + 1) % len(bands)
			}
			return m, nil
		}
	}
	return m, nil
}

// continentPanelView renders the Worked/Needed by Continent grid for the
// currently paged-to band: each continent marked worked (with a QSO count)
// or needed. Falls back to a "no active contest" notice when there's
// nothing to index, matching the other contest-scoped screens.
func (m model) continentPanelView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(continentScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Worked/Needed by Continent"))
	b.WriteString("\n\n")

	event, ok := m.eventForContestID()
	if !ok || m.contestIndex == nil {
		b.WriteString(helpStyle.Render("no active contest — set one on the Events (F7) screen first"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Esc/Ctrl+W: QSO Entry"))
		return b.String()
	}
	b.WriteString(statusBarStyle.Render(event.Name))
	b.WriteString("\n\n")

	bands := m.continentPanelBands()
	if len(bands) == 0 {
		b.WriteString(helpStyle.Render("event defines no bands"))
		return b.String()
	}
	focus := m.continentBandFocus
	if focus >= len(bands) {
		focus = 0
	}
	band := bands[focus]
	b.WriteString(focusedFieldBoxStyle.Render(band))
	b.WriteString("\n\n")

	for _, continent := range continents {
		worked, count := m.contestIndex.continentSummary(continent, band)
		var line string
		if worked {
			line = newMultStyle.Render(fmt.Sprintf("%s  worked (%d)", continent, count))
		} else {
			line = helpStyle.Render(fmt.Sprintf("%s  needed", continent))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Left/Right: band  •  Esc/Ctrl+W: QSO Entry"))
	return b.String()
}

// updateHelpPanel drives the in-app command reference (roadmap §3 Phase 3:
// "in-app HELP for the new commands"). Esc/Ctrl+G return to whichever screen
// Ctrl+G was pressed from, since Help is reachable globally. F1 is kept
// consistent with its app-wide meaning on every other screen — always QSO
// Entry, never "back" — so the hotkey bar's "F1: QSO Entry" stays true here.
func (m model) updateHelpPanel(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "esc", "ctrl+g":
			m.screen = m.helpReturnScreen
			if m.screen == qsoEntryScreen {
				m.focusField(fieldCall)
			}
			return m, nil
		}
	}
	return m, nil
}

// helpPanelView renders the static in-app command reference: every screen
// hotkey, the QSO Entry field/editing keys, and the as-you-type contest
// tools, so the operator doesn't need docs/ROADMAP.md open to find a command.
func (m model) helpPanelView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(helpScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("Help — Commands & Keys"))
	b.WriteString("\n\n")

	section := func(title string, lines ...string) {
		b.WriteString(statusBarStyle.Render(title))
		b.WriteString("\n")
		for _, line := range lines {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}

	section("Screens",
		"F1  QSO Entry",
		"F2  Station Setup",
		"F3  DX Cluster",
		"F4  DX Cluster Filters",
		"F5  Import ADIF",
		"F6  QSO Details",
		"F7  Events (contest catalog)",
		"F8  Backup to Google Drive",
		"F9  Browse/Edit Recent QSOs (↑/↓ select, Enter view/edit, d delete, Esc/F9 done)",
		"Ctrl+W  Worked/Needed by Continent",
		"Ctrl+P  Toggle POST (after-contest) entry mode",
		"Ctrl+G  This help screen",
		"Esc  Context-dependent: quit QSO Entry, cancel an edit, or back up one screen",
	)
	section("Export",
		"Ctrl+O  Export ADIF",
		"Ctrl+X  Export Cabrillo (per-session filename when a session-specific contest ID is selected)",
		"Ctrl+R  Export CSV",
	)
	section("QSO Entry",
		"Tab / Shift+Tab  Move between fields without logging",
		"Enter  Accept field and advance; on Call with a contest active, fast-paths past auto-filled RST/Band/Freq to the received exchange",
		"Left/Right (on Band)  Cycle bands",
		"PgUp/PgDn or mouse wheel  Scroll DX Spots",
	)
	section("POST mode (re-logging QSOs from a paper log after the contest)",
		"Ctrl+P  Toggle on/off (blocked while editing a QSO)",
		"Adds a Date/Time UTC field (format 2006-01-02 15:04) as the last field in the entry row — Enter there logs the QSO with that time instead of the live clock",
		"The field keeps its last value between QSOs so consecutive entries only need the time edited",
	)
	section("Contest tools (shown when a contest is active, update as you type the call)",
		"Analysis panel  Dupe flag, country/CQ/ITU/continent, beam heading+distance, new-multiplier flag, band-worked matrix",
		"Check Partial  Prior logged calls containing the in-progress fragment — bold: new on this band, dim: dupe here",
		"Rate meter  Q/hr (last 10 / last 100 / overall) and Q/Mult, shown under Recent QSOs/DX Spots once something's logged",
		"Zone auto-fill  CQ/ITU zone exchange fields prefill from the resolved DXCC entity; typing into the field stops autofill for that QSO",
	)
	section("Typed commands (type into Call, then Enter)",
		"ZAP  Delete the most recently logged QSO (while entering a new QSO)",
		"SETDUPE  Reset the dupe-check baseline to now — QSOs logged before this no longer count as dupes",
		"/Z  Delete the QSO currently loaded for editing (F9 to recall one first)",
		"/X  Toggle the recalled QSO's logged-but-unscored (X-QSO) flag — still logged/exported, excluded from CLAIMED-SCORE",
	)

	b.WriteString(helpStyle.Render("Esc/Ctrl+G: back to " + screenName(m.helpReturnScreen) + "  •  F1: QSO Entry"))
	return b.String()
}

// screenName gives a short human label for a screen, used by helpPanelView's
// return hint so the operator knows where Esc/Ctrl+G will land them.
func screenName(s screen) string {
	switch s {
	case qsoEntryScreen:
		return "QSO Entry"
	case stationSetupScreen:
		return "Station Setup"
	case clusterScreen:
		return "DX Cluster"
	case clusterFiltersScreen:
		return "DX Cluster Filters"
	case adifImportScreen:
		return "Import ADIF"
	case eventCatalogScreen:
		return "Events"
	case qsoDetailsScreen:
		return "QSO Details"
	case qsoContestScreen:
		return "QSO Contest"
	case continentScreen:
		return "Worked/Needed by Continent"
	default:
		return "QSO Entry"
	}
}

func (m model) updateCluster(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			m.disconnectCluster()
			return m, tea.Quit
		case "esc", "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "f2":
			m.openStationSetup()
			return m, nil
		case "f5":
			if m.clusterClient != nil || m.clusterConnecting {
				m.clusterStatus = k3lrClusterName + " — connection already active"
				return m, nil
			}
			if strings.TrimSpace(m.activeStation.Callsign) == "" {
				m.clusterStatus = "set your station callsign in F2 Station Setup before connecting"
				return m, nil
			}
			m.clusterGeneration++
			m.clusterConnecting = true
			m.clusterReconnect = true
			m.clusterReconnectDelay = 0
			m.clusterStatus = "connecting to " + k3lrClusterAddr + "…"
			return m, connectK3LR(m.activeStation.Callsign, m.clusterGeneration)
		case "f6":
			m.disconnectCluster()
			return m, nil
		case "f4":
			m.openClusterFilters()
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateClusterFilters(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc", "f3":
			m.screen = clusterScreen
			return m, nil
		case "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "f2":
			m.openStationSetup()
			return m, nil
		case "tab":
			m.focusClusterFilterField((m.clusterFilterFocus + 1) % len(m.clusterFilterFields))
			return m, nil
		case "shift+tab":
			m.focusClusterFilterField((m.clusterFilterFocus - 1 + len(m.clusterFilterFields)) % len(m.clusterFilterFields))
			return m, nil
		case "up":
			m.clusterBandFocus = (m.clusterBandFocus - 1 + len(cwBands)) % len(cwBands)
			return m, nil
		case "down":
			m.clusterBandFocus = (m.clusterBandFocus + 1) % len(cwBands)
			return m, nil
		case " ":
			band := cwBands[m.clusterBandFocus]
			m.editClusterBands[band] = !m.editClusterBands[band]
			return m, nil
		case "enter":
			m.saveClusterFilters()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.clusterFilterFields[m.clusterFilterFocus], cmd = m.clusterFilterFields[m.clusterFilterFocus].Update(msg)
	return m, cmd
}

func (m model) updateADIFImport(msg tea.Msg) (tea.Model, tea.Cmd) {
	// adifImportedMsg (the async import's result) is handled globally in
	// Update, not here, so it isn't lost if Esc leaves this screen before
	// the import finishes.
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "enter":
			if m.importInProgress {
				m.statusMsg = "An ADIF import is already running…"
				return m, nil
			}
			path := strings.TrimSpace(m.adifPathField.Value())
			if path == "" {
				m.statusMsg = "ADIF file path is required"
				return m, nil
			}
			m.importInProgress = true
			m.statusMsg = "Importing ADIF…"
			// Register the job synchronously on the update loop (before the
			// async command's goroutine starts) so main()'s bgTasks.Wait() on
			// shutdown can never miss it.
			m.bgTasks.Add(1)
			return m, m.importADIFFile(path)
		}
	}
	var cmd tea.Cmd
	m.adifPathField, cmd = m.adifPathField.Update(msg)
	return m, cmd
}

func (m model) updateStationSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			m.statusMsg = "Station Setup cancelled"
			return m, nil
		case "f1":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			m.statusMsg = "QSO Entry"
			return m, nil
		case "f3":
			return m, m.openCluster()
		case "f4":
			m.openClusterFilters()
			return m, nil
		case "tab":
			m.focusStationField((m.stationFocusIdx + 1) % len(m.stationFields))
			return m, nil
		case "shift+tab":
			m.focusStationField((m.stationFocusIdx - 1 + len(m.stationFields)) % len(m.stationFields))
			return m, nil
		case "enter":
			if m.stationFocusIdx == len(m.stationFields)-1 {
				return m, m.saveStationSetup()
			}
			m.focusStationField(m.stationFocusIdx + 1)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.stationFields[m.stationFocusIdx], cmd = m.stationFields[m.stationFocusIdx].Update(msg)
	return m, cmd
}

// renderSlot renders one QSO Entry field box for the slot at position pos in
// the entrySlots order, highlighting it when focused. Both base and inline
// received-exchange fields render the same way.
func (m model) renderSlot(pos int, s entrySlot) string {
	box := fieldBoxStyle
	if pos == m.focusIdx {
		box = focusedFieldBoxStyle
	}
	input := m.fields[s.idx]
	switch {
	case s.contest:
		input = m.contestFields[s.idx]
	case s.post:
		input = m.postFields[s.idx]
	}
	content := labelStyle.Render(s.label) + input.View()
	return box.Render(content)
}

func (m model) View() string {
	if m.screen == stationSetupScreen {
		return m.stationSetupView()
	}
	if m.screen == clusterScreen {
		return m.clusterView()
	}
	if m.screen == clusterFiltersScreen {
		return m.clusterFiltersView()
	}
	if m.screen == adifImportScreen {
		return m.adifImportView()
	}
	if m.screen == qsoDetailsScreen {
		return m.qsoDetailsView()
	}
	if m.screen == qsoContestScreen {
		return m.qsoContestView()
	}
	if m.screen == eventCatalogScreen {
		return m.eventCatalogView()
	}
	if m.screen == continentScreen {
		return m.continentPanelView()
	}
	if m.screen == helpScreen {
		return m.helpPanelView()
	}
	var b strings.Builder
	b.WriteString(screenHotkeys(qsoEntryScreen))
	b.WriteString("\n")

	now := time.Now()
	// "Local" reflects the configured station profile's timezone, not
	// necessarily the host machine's — an operator running on a remote or
	// differently-configured host still wants their station's own local
	// time. Fall back to the host's local time if the stored zone somehow
	// fails to load.
	localNow := now
	if loc, err := time.LoadLocation(m.activeStation.Timezone); err == nil {
		localNow = now.In(loc)
	}
	header := fmt.Sprintf(
		"%s  |  %s  |  UTC %s  |  Local %s (%s)",
		m.contestName,
		m.qsoBand(),
		now.UTC().Format("15:04:05Z"),
		localNow.Format("15:04:05 -07:00"),
		localNow.Location(),
	)
	// In a serial-exchange contest, mirror the serial the operator will send
	// next right on QSO Entry so they don't have to switch to Contest Entry
	// (F7) to read it. The field value is authoritative (it reflects a manual
	// correction typed there); nextSerial is the fallback when it's blank.
	if m.nextSerial > 0 {
		serial := strings.TrimSpace(m.contestFields[contestSerialSent].Value())
		if serial == "" {
			serial = formatSerial(m.nextSerial)
		}
		header += "  |  Sending # " + serial
	}
	b.WriteString(headerStyle.Render(header))
	if m.postMode {
		b.WriteString("\n")
		b.WriteString(dupeStyle.Render("POST MODE — logging with typed Date/Time, not the live clock"))
	}
	b.WriteString("\n")
	b.WriteString(solarStyle.Render(m.solarLine()))
	b.WriteString("\n\n")

	slots := m.entrySlots()
	fieldViews := make([]string, len(slots))
	for i, s := range slots {
		fieldViews[i] = m.renderSlot(i, s)
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, fieldViews...))
	b.WriteString("\n")

	switch {
	case m.dupeWarning:
		b.WriteString(dupeStyle.Render("DUPE"))
		b.WriteString("\n\n")
	case m.editingQSOID != 0:
		hint := "EDITING #%d — Esc cancels, final Enter saves, /Z deletes, /X marks unscored"
		if m.editingOriginal.unscored {
			hint = "EDITING #%d [X-QSO unscored] — Esc cancels, final Enter saves, /Z deletes, /X restores"
		}
		b.WriteString(editingStyle.Render(fmt.Sprintf(hint, m.editingQSOID)))
		b.WriteString("\n\n")
	default:
		b.WriteString("\n")
	}

	workedLabel := "Recent QSOs (F9: browse/edit)"
	if m.workedCall != "" {
		workedLabel = "Stations Worked: " + m.workedCall + " (prior contacts)"
	}
	recentBlock := helpStyle.Render(workedLabel) + "\n" + m.table.View()
	// Fills the empty space to the right of Recent QSOs on wide enough
	// terminals; on narrow ones dxSpotsPanel returns "" and this just
	// renders recentBlock alone, unchanged from before this panel existed.
	const spotsPanelGap = 4
	spotsPanel := m.dxSpotsPanel(m.termWidth - lipgloss.Width(recentBlock) - spotsPanelGap)
	if spotsPanel == "" {
		b.WriteString(recentBlock)
	} else {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, recentBlock, strings.Repeat(" ", spotsPanelGap), spotsPanel))
	}
	b.WriteString("\n\n")

	if rate := m.rateMeterLine(); rate != "" {
		b.WriteString(helpStyle.Render(rate))
		b.WriteString("\n\n")
	}

	status := fmt.Sprintf("Qs: %d   %s", m.qsoCount, m.statusMsg)
	b.WriteString(statusBarStyle.Render(status))
	b.WriteString("\n")

	help := "tab/shift+tab: move/edit fields  •  first tab after callsign starts QSO  •  final enter: save next QSO"
	b.WriteString(helpStyle.Render(help))

	left := b.String()
	// Analysis panel spans the whole right-hand column alongside QSO Entry
	// (fields + Recent QSOs/DX Spots), not just one row of it — same
	// width-gating idiom as dxSpotsPanel, applied one level up.
	const analysisPanelGap = 4
	panel := m.analysisPanel(m.termWidth - lipgloss.Width(left) - analysisPanelGap)
	if panel == "" {
		return left
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", analysisPanelGap), panel)
}

func (m model) stationSetupView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(stationSetupScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  Station Setup"))
	b.WriteString("\n\n")
	b.WriteString(renderFieldGrid(stationFieldLabels[:], m.stationFields, m.stationFocusIdx))
	b.WriteString("\n")
	b.WriteString(statusBarStyle.Render(m.statusMsg))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("tab/shift+tab: move fields  •  final enter: save station profile"))
	return b.String()
}

// dxSpotsPanelMinWidth is the floor below which there's no point rendering
// the panel at all — not even enough room for "HH:MM freq call".
const dxSpotsPanelMinWidth = 24

// dxSpotsPanelCommentMinWidth is the width at which a spot's comment starts
// getting appended after time/freq/call, rather than being dropped to keep
// the line from wrapping.
const dxSpotsPanelCommentMinWidth = 45

// dxSpotsPanel renders the DX Spots panel shown beside Recent QSOs on QSO
// Entry: every CW spot currently in m.clusterSpots, across all bands,
// independent of whatever band the operator has selected for the next QSO.
// It reuses the same feed (and thus the same Cluster Filters (F4)
// configuration) as the full DX Cluster (F3) screen rather than
// maintaining a second one, so the two never disagree about what counts as
// a CW spot. width is the space left over after Recent QSOs; a width below
// dxSpotsPanelMinWidth returns "" so the caller falls back to rendering
// Recent QSOs alone, matching the layout from before this panel existed.
func (m model) dxSpotsPanel(width int) string {
	if width < dxSpotsPanelMinWidth {
		return ""
	}
	var b strings.Builder
	title := "DX Spots (CW, all bands)"
	if len(m.clusterSpots) > recentQSOsVisibleRows {
		title += "  (PgUp/PgDn)"
	}
	b.WriteString(helpStyle.Render(truncateToWidth(title, width)))
	b.WriteString("\n")
	if len(m.clusterSpots) == 0 {
		status := m.clusterStatus
		if status == "" {
			status = "no spots yet"
		}
		b.WriteString(helpStyle.Render(truncateToWidth(status, width)))
		return b.String()
	}
	showComment := width >= dxSpotsPanelCommentMinWidth
	maxScroll := len(m.clusterSpots) - recentQSOsVisibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.clusterSpotsScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + recentQSOsVisibleRows
	if end > len(m.clusterSpots) {
		end = len(m.clusterSpots)
	}
	spots := m.clusterSpots[scroll:end]
	lines := make([]string, len(spots))
	for i, spot := range spots {
		line := fmt.Sprintf("%s %-8s %-10s", spot.Received.Format("15:04"), spot.Frequency, spot.Callsign)
		if showComment {
			if room := width - lipgloss.Width(line) - 1; room > 0 {
				line += " " + truncateToWidth(spot.Comment, room)
			}
		}
		lines[i] = truncateToWidth(line, width)
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}

// truncateToWidth shortens s to at most width runes, so a long comment or
// callsign can't push the DX Spots panel wider than the space actually left
// over next to Recent QSOs.
func truncateToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width])
}

func (m model) clusterView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(clusterScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  DX Cluster  |  " + k3lrClusterName))
	b.WriteString("\n\n")
	b.WriteString(statusBarStyle.Render(m.clusterStatus))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(" UTC      Spotter       Freq       Call         Comment"))
	b.WriteString("\n")
	for _, spot := range m.clusterSpots {
		comment := spot.Comment
		if len(comment) > 48 {
			comment = comment[:45] + "..."
		}
		b.WriteString(fmt.Sprintf(" %-8s %-13s %-10s %-12s %s\n", spot.Received.Format("15:04:05"), spot.Spotter, spot.Frequency, spot.Callsign, comment))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("F4: Filters  •  F5: Connect K3LR  •  F6: Disconnect  •  Esc/F1: QSO Entry"))
	return b.String()
}

func (m model) clusterFiltersView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(clusterFiltersScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  Cluster Filters  |  CW only"))
	b.WriteString("\n\n")
	b.WriteString(renderFieldGrid(clusterFilterLabels[:], m.clusterFilterFields, m.clusterFilterFocus))
	b.WriteString("\nBands: ")
	for i, band := range cwBands {
		mark := " "
		if m.editClusterBands[band] {
			mark = "x"
		}
		item := fmt.Sprintf("[%s] %s", mark, band)
		if i == m.clusterBandFocus {
			item = focusedFieldBoxStyle.Render(item)
		}
		b.WriteString(item + "  ")
		if i == 5 {
			b.WriteString("\n       ")
		}
	}
	b.WriteString("\n\n")
	b.WriteString(statusBarStyle.Render(m.statusMsg))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Tab: DX/DE fields  •  Up/Down: band  •  Space: toggle band  •  Enter: apply  •  Esc: cluster"))
	return b.String()
}

func (m model) adifImportView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(adifImportScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  Import ADIF"))
	b.WriteString("\n\n")
	b.WriteString(focusedFieldBoxStyle.Render(labelStyle.Render("ADIF file") + m.adifPathField.View()))
	b.WriteString("\n\n")
	b.WriteString(statusBarStyle.Render(m.statusMsg))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Enter: import CW QSOs  •  Esc: cancel"))
	return b.String()
}

func (m model) qsoDetailsView() string {
	return m.qsoPageView("QSO Details", detailLabels[:], m.detailFields, m.detailFocusIdx, "F1/Esc: QSO Entry  •  F7: Contest Entry  •  Tab: next field")
}

func (m model) qsoContestView() string {
	view := m.qsoPageView("Contest Entry", contestLabels[:], m.contestFields, m.contestFocusIdx, "F1/Esc: QSO Entry  •  F6: QSO Details  •  F7: Events  •  Tab: next field")
	choices := m.exchangeChoices()
	if len(choices) == 0 {
		return view
	}
	var b strings.Builder
	b.WriteString(view)
	b.WriteString("\nCounty matches: ")
	for index, choice := range choices {
		item := choice.Code + " " + choice.Name
		if index == m.exchangeChoiceFocus {
			item = focusedFieldBoxStyle.Render(item)
		}
		b.WriteString(item + "  ")
		if index == 5 {
			break
		}
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Up/Down: choose county  •  Enter: insert county code"))
	return b.String()
}

func (m model) eventCatalogView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(eventCatalogScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(fmt.Sprintf("W4GNS Logger 4 Men  |  Events & Contests (%d)", len(m.events))))
	b.WriteString("\n\n")
	const visibleEvents = 10
	start := m.eventFocus - visibleEvents/2
	if start < 0 {
		start = 0
	}
	if end := start + visibleEvents; end > len(m.events) {
		start = len(m.events) - visibleEvents
		if start < 0 {
			start = 0
		}
	}
	end := start + visibleEvents
	if end > len(m.events) {
		end = len(m.events)
	}
	for index := start; index < end; index++ {
		event := m.events[index]
		line := fmt.Sprintf("%s — %s", event.Name, event.Schedule)
		if index == m.eventFocus {
			line = focusedFieldBoxStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	if len(m.events) > 0 {
		event := m.events[m.eventFocus]
		session := event.Sessions[m.eventSessionFocus]
		b.WriteString("\n")
		b.WriteString(statusBarStyle.Render(session.Label + " — " + session.Schedule + "  •  " + event.Kind + "  •  exchange: " + event.RcvdExchangeHint))
		b.WriteString("\n")
		b.WriteString(statusBarStyle.Render(eventDetailLine(event)))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Up/Down: event  •  Left/Right: session  •  Enter: use session  •  F1/Esc: QSO Entry"))
	return b.String()
}

// eventDetailLine surfaces the catalog fields that were previously loaded
// but never shown anywhere in the UI: organizer, allowed bands, rules URL,
// and score-submission URL.
func eventDetailLine(event eventDefinition) string {
	parts := []string{}
	if event.Organizer != "" {
		parts = append(parts, "by "+event.Organizer)
	}
	if len(event.Bands) > 0 {
		parts = append(parts, "bands: "+strings.Join(event.Bands, "/"))
	}
	if event.RulesURL != "" {
		parts = append(parts, "rules: "+event.RulesURL)
	}
	if event.ScoreSubmissionURL != "" {
		parts = append(parts, "scores: "+event.ScoreSubmissionURL)
	}
	return strings.Join(parts, "  •  ")
}

// renderFieldGrid lays fields out two per row using lipgloss.JoinHorizontal,
// the same way QSO Entry's own field row already does. Two multi-line
// bordered boxes written to a strings.Builder back to back (the previous
// approach in every caller here) just stack vertically instead of sitting
// side by side — plain string concatenation has no notion of "these two
// belong on the same row" — so this was quietly wasting roughly double the
// intended vertical space on every field-grid screen (Station Setup, QSO
// Details, Contest Entry, Cluster Filters) since long before Station Setup
// grew enough fields to overflow a typical terminal height because of it.
func renderFieldGrid(labels []string, fields []textinput.Model, focus int) string {
	var b strings.Builder
	boxStyleFor := func(index int) lipgloss.Style {
		if index == focus {
			return focusedFieldBoxStyle
		}
		return fieldBoxStyle
	}
	renderOne := func(index int) string {
		return boxStyleFor(index).Render(labelStyle.Render(labels[index]) + fields[index].View())
	}
	for index := 0; index < len(fields); index += 2 {
		if index+1 >= len(fields) {
			b.WriteString(renderOne(index))
		} else {
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, renderOne(index), renderOne(index+1)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) qsoPageView(title string, labels []string, fields []textinput.Model, focus int, help string) string {
	var b strings.Builder
	b.WriteString(screenHotkeys(m.screen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  " + title))
	b.WriteString("\n\n")
	b.WriteString(renderFieldGrid(labels, fields, focus))
	b.WriteString("\n")
	b.WriteString(statusBarStyle.Render(m.statusMsg))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(help))
	return b.String()
}

func screenHotkeys(current screen) string {
	escape := "Esc: Quit"
	if current == stationSetupScreen {
		escape = "Esc: Cancel Setup"
	} else if current == clusterScreen {
		escape = "Esc: QSO Entry"
	} else if current == clusterFiltersScreen {
		escape = "Esc: Cluster"
	} else if current == adifImportScreen {
		escape = "Esc: QSO Entry"
	} else if current == qsoDetailsScreen || current == qsoContestScreen || current == eventCatalogScreen || current == continentScreen {
		escape = "Esc: QSO Entry"
	} else if current == helpScreen {
		escape = "Esc: Back"
	}
	line1 := "W4GNS-Logger v" + appVersion + "  •  F1: QSO Entry  •  F2: Station Setup  •  F3: DX Cluster  •  F4: Filters  •  F5: Import ADIF"
	line2 := "F6: QSO Details  •  F7: Events  •  F8: Backup  •  F9: Browse/Edit  •  Ctrl+O: Export ADIF  •  Ctrl+X: Export Cabrillo  •  Ctrl+R: Export CSV  •  Ctrl+W: Worked/Needed  •  Ctrl+P: POST mode  •  Ctrl+G: Help  •  " + escape
	return hotkeyStyle.Render(line1) + "\n" + hotkeyStyle.Render(line2)
}

func main() {
	if err := validateArgs(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\nusage: %s [--export-adif PATH | --import-adif PATH | --version]\n", err, filepath.Base(os.Args[0]))
		os.Exit(2)
	}
	if exportPath, ok := adifExportPath(os.Args[1:]); ok {
		runADIFExport(exportPath)
		return
	}
	if importPath, ok := adifImportPath(os.Args[1:]); ok {
		runADIFImport(importPath)
		return
	}
	if hasArg(os.Args[1:], "--version") {
		fmt.Println(appVersion)
		return
	}
	if !hasArg(os.Args[1:], terminalChildArg) && !hasArg(os.Args[1:], inCurrentTerminalArg) {
		if err := launchInOwnTerminal(); err == nil {
			return
		} else {
			// Fall back to running in the current terminal instead of exiting.
			// macOS and Windows — both published release targets — have none
			// of the Linux terminal emulators launchInOwnTerminal searches
			// for, and a minimal Linux host may not either. Exiting here left
			// those users unable to start the app at all unless they happened
			// to discover the --in-current-terminal flag. bubbletea's TUI runs
			// fine in the invoking terminal (Terminal.app, Windows Terminal,
			// cmd, or any Linux shell), so this is a safe default.
			fmt.Fprintf(os.Stderr, "no separate terminal window available (%v); running in the current terminal\n", err)
		}
	}

	dbPath := defaultDBPath()
	if v := os.Getenv("W4GNS_DB"); v != "" {
		dbPath = v
	}

	st, err := openStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	m := initialModel(st)
	m.qrzAPIKey = loadQRZAPIKey()
	m.wrlAPIKey = loadWRLAPIKey()
	m.wrlLogbookID = loadWRLLogbookID()
	m.qrzXMLCreds = loadQRZXMLCredentials()
	// Connect to the DX cluster at startup, not only when the operator
	// visits the DX Cluster (F3) screen, so the DX Spots panel on QSO Entry
	// has something to show right away. connectClusterIfNeeded no-ops (and
	// leaves clusterConnecting false) if no station callsign is configured.
	m.connectClusterIfNeeded()

	// Alt-screen mode gives the logger a clean, dedicated terminal surface and
	// restores the invoking terminal unchanged when the application exits.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// bubbletea only listens for SIGINT/SIGTERM. SIGHUP (sent when the
	// controlling terminal window is closed) would otherwise kill the process
	// immediately and skip the shutdown backup below, so route it through the
	// same graceful QuitMsg path as Esc/Ctrl+C.
	hangup := make(chan os.Signal, 1)
	signal.Notify(hangup, syscall.SIGHUP)
	go func() {
		if _, ok := <-hangup; ok {
			p.Send(tea.Quit())
		}
	}()

	finalModel, err := p.Run()
	signal.Stop(hangup)

	// A plain kill -INT (as opposed to an in-terminal Ctrl+C, which arrives as
	// a KeyMsg handled in Update) makes bubbletea return ErrInterrupted. That
	// is still an orderly shutdown request, not a crash, so it must still run
	// the backup below rather than exiting immediately.
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Every shutdown path (Esc, Ctrl+C, SIGTERM, SIGHUP, or kill -INT) routes
	// through here, so a single backup-on-exit call (and remembering the
	// terminal size for next launch) covers all of them.
	if m, ok := finalModel.(model); ok {
		// Cancel and drain any in-flight background database work (an ADIF
		// import) before the backup reads the database and before defer
		// st.Close() runs, so it can't write into a closing/closed database
		// or leave the shutdown snapshot half-imported. bubbletea does not
		// wait for outstanding tea.Cmd goroutines before p.Run() returns.
		if m.bgCancel != nil {
			m.bgCancel()
		}
		if m.bgTasks != nil {
			m.bgTasks.Wait()
		}
		saveWindowSize(m.termWidth, m.termHeight)
		fmt.Println("backing up to Google Drive…")
		ctx, cancel := context.WithTimeout(context.Background(), backupTimeout)
		// runBackupSerialized (not runBackup) so this waits for, rather than
		// races, an F8 backup that was still in flight when the program quit:
		// bubbletea does not wait for outstanding tea.Cmd goroutines before
		// p.Run() returns.
		result, backupErr := runBackupSerialized(ctx, m.store, m.activeStation.ID)
		cancel()
		if backupErr != nil {
			fmt.Fprintf(os.Stderr, "backup failed: %v\n", backupErr)
		} else {
			fmt.Printf("backup saved: %s, %s\n", result.dbName, result.adifName)
		}
	}

	if err != nil {
		os.Exit(130)
	}
}

// recognizedArgs lists every command-line flag main understands.
var recognizedArgs = map[string]bool{
	"--export-adif":      true,
	"--import-adif":      true,
	"--version":          true,
	terminalChildArg:     true,
	inCurrentTerminalArg: true,
}

// validateArgs rejects an unrecognized flag or one of --export-adif/
// --import-adif missing its required path, instead of silently falling
// through to launching the TUI — which would otherwise hide a typo'd flag
// (e.g. --export-adiff) or an accidentally dropped path argument.
func validateArgs(args []string) error {
	sawExport, sawImport := false, false
	for i, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			continue
		}
		if !recognizedArgs[arg] {
			return fmt.Errorf("unrecognized flag %q", arg)
		}
		if arg == "--export-adif" {
			sawExport = true
		}
		if arg == "--import-adif" {
			sawImport = true
		}
		if arg == "--export-adif" || arg == "--import-adif" {
			// The path operand is required and must be an actual path, not the
			// next flag: "--export-adif --version" (or "--export-adif
			// --import-adif x") would otherwise treat "--version" as the export
			// path and silently do the wrong thing.
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path argument", arg)
			}
			if recognizedArgs[args[i+1]] {
				return fmt.Errorf("%s requires a file path argument, not the flag %q", arg, args[i+1])
			}
		}
	}
	if sawExport && sawImport {
		return fmt.Errorf("--export-adif and --import-adif cannot be combined")
	}
	return nil
}

func adifImportPath(args []string) (string, bool) {
	for index, arg := range args {
		if arg == "--import-adif" && index+1 < len(args) {
			return args[index+1], true
		}
	}
	return "", false
}

func adifExportPath(args []string) (string, bool) {
	for index, arg := range args {
		if arg == "--export-adif" && index+1 < len(args) {
			return args[index+1], true
		}
	}
	return "", false
}

func runADIFExport(path string) {
	dbPath := os.Getenv("W4GNS_DB")
	if dbPath == "" {
		dbPath = defaultDBPath()
	}
	if exportTargetCollidesWithDB(path, dbPath) {
		fmt.Fprintln(os.Stderr, "ADIF export path must not be the SQLite database")
		os.Exit(1)
	}
	st, err := openStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	// Re-check now that openStore has created the database (and its WAL/SHM
	// sidecars) on disk. A path that looked distinct before — a dangling
	// symlink pointing at the not-yet-created database, say — can resolve onto
	// it only once the target exists, and the export's final rename would then
	// clobber the live database.
	if exportTargetCollidesWithDB(path, dbPath) {
		fmt.Fprintln(os.Stderr, "ADIF export path must not be the SQLite database")
		os.Exit(1)
	}
	profile, err := st.activeStationProfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading station profile: %v\n", err)
		os.Exit(1)
	}
	count, err := writeADIFAtomic(context.Background(), filepath.Dir(path), path, profile.ID, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ADIF export failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ADIF export complete: %d QSOs written to %s\n", count, path)
}

func pathsReferToSameFile(first, second string) bool {
	firstInfo, firstErr := os.Stat(first)
	secondInfo, secondErr := os.Stat(second)
	if firstErr == nil && secondErr == nil && os.SameFile(firstInfo, secondInfo) {
		return true
	}
	return resolvePath(first) == resolvePath(second)
}

// resolvePath returns path's symlink-resolved absolute form when possible,
// falling back to its lexically-cleaned absolute form. filepath.EvalSymlinks
// fails on a path that doesn't exist yet or is a dangling symlink, so the
// fallback keeps two plain distinct paths distinguishable while still
// collapsing a resolvable symlink onto its real target.
func resolvePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		if abs, err := filepath.Abs(resolved); err == nil {
			return abs
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// exportTargetCollidesWithDB reports whether exportPath resolves onto the
// SQLite database file or one of its WAL/SHM sidecars. Overwriting any of the
// three would corrupt the live log, so the CLI export refuses all of them.
func exportTargetCollidesWithDB(exportPath, dbPath string) bool {
	for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if pathsReferToSameFile(exportPath, p) {
			return true
		}
	}
	return false
}

func runADIFImport(path string) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening ADIF file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()
	dbPath := os.Getenv("W4GNS_DB")
	if dbPath == "" {
		dbPath = defaultDBPath()
	}
	st, err := openStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading station profile: %v\n", err)
		os.Exit(1)
	}
	result, err := importADIF(context.Background(), file, profile.ID, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ADIF import failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("ADIF import complete: %d CW QSOs imported, %d records skipped\n", result.Imported, result.Skipped)
}
