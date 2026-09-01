package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
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
	fieldMode
	fieldCount
)

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
)

const (
	detailName = iota
	detailQTH
	detailGrid
	detailState
	detailPOTARef
	detailNotes
	detailFieldCount
)

var detailLabels = [detailFieldCount]string{"Name", "QTH", "Grid", "State", "POTA Ref", "Notes"}

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
	stationNameField = iota
	stationCallsignField
	stationOperatorField
	stationGridField
	stationTimezoneField
	stationClubField
	stationRigField
	stationAntennaField
	stationPowerField
	stationFieldCount
)

var stationFieldLabels = [stationFieldCount]string{
	"Profile", "Callsign", "Operator", "Grid", "Timezone", "Club", "Rig", "Antenna", "Power (W)",
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
	clusterFilterFieldCount
)

var clusterFilterLabels = [clusterFilterFieldCount]string{
	"DX DXCC", "DX ITU", "DX CQ", "DX Continent",
	"DE DXCC", "DE ITU", "DE CQ", "DE Continent",
}

var fieldLabels = [fieldCount]string{
	fieldCall:      "Call",
	fieldRSTSent:   "RST Sent",
	fieldRSTRcvd:   "RST Rcvd",
	fieldBand:      "Band",
	fieldFrequency: "Freq MHz",
	fieldMode:      "Mode",
}

type qso struct {
	call      string
	band      string
	mode      string
	rstSent   string
	rstRcvd   string
	exchange  string
	frequency string
	name      string
	qth       string
	grid      string
	state     string
	country   string
	cqZone    string
	ituZone   string
	comment   string
	potaRef   string
	contestID string
	stx       string
	stxString string
	srx       string
	srxString string
	time      time.Time // QSO start time (UTC)
	timeOff   time.Time // QSO end time (UTC)
	profileID int64

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

	store    *store
	qsoCount int
	table    table.Model

	qrzAPIKey string

	backupInProgress bool

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

	clusterClient       *clusterClient
	clusterSpots        []clusterSpot
	clusterStatus       string
	clusterConnecting   bool
	clusterGeneration   uint64
	clusterFilters      clusterFilters
	clusterFilterFields []textinput.Model
	clusterFilterFocus  int
	clusterBandFocus    int
	adifPathField       textinput.Model
	detailFields        []textinput.Model
	detailFocusIdx      int
	contestFields       []textinput.Model
	contestFocusIdx     int
	events              []eventDefinition
	eventFocus          int
	eventSessionFocus   int
	exchangeChoiceFocus int
}

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00ADD8")).
			Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Width(10)

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

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
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
	fields[fieldMode] = newTextInput("CW", 4)
	fields[fieldMode].SetValue("CW")
	details := []textinput.Model{
		newTextInput("Operator name", 20), newTextInput("City / QTH", 20),
		newTextInput("Grid square", 10), newTextInput("State / province", 12), newTextInput("US-0000", 12), newTextInput("QSO notes", 36),
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
		table.WithHeight(10),
	)

	m := model{
		contestName:    "W4GNS Logger 4 Men — CW Log",
		fields:         fields,
		store:          st,
		table:          t,
		clusterFilters: defaultClusterFilters(),
		detailFields:   details,
		contestFields:  contests,
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
	}
	m.screen = stationSetupScreen
	m.focusStationField(stationNameField)
	m.statusMsg = "Station Setup — Enter saves, Esc cancels"
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

func (m *model) saveStationSetup() {
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
	}
	saved, err := m.store.saveStationProfile(profile)
	if err != nil {
		m.statusMsg = fmt.Sprintf("station setup error: %v", err)
		return
	}
	m.activeStation = saved
	m.screen = qsoEntryScreen
	m.focusField(fieldCall)
	m.statusMsg = fmt.Sprintf("station profile %q saved", saved.Name)
}

func (m *model) openCluster() tea.Cmd {
	m.screen = clusterScreen
	if m.clusterClient != nil || m.clusterConnecting {
		return nil
	}
	if strings.TrimSpace(m.activeStation.Callsign) == "" {
		m.clusterStatus = "set your station callsign in F2 Station Setup to connect to K3LR"
		return nil
	}
	m.clusterGeneration++
	m.clusterConnecting = true
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
	}
	m.screen = clusterFiltersScreen
	m.focusClusterFilterField(0)
	m.statusMsg = "Cluster filters: CW only; use Up/Down and Space to select bands"
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
	m.screen = clusterScreen
	m.clusterStatus = "cluster filters applied — CW only"
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

func (m *model) openADIFImport() {
	m.adifPathField = newStationTextInput("", 60)
	m.adifPathField.Placeholder = "/path/to/log.adi"
	m.adifPathField.Focus()
	m.screen = adifImportScreen
	m.statusMsg = "Enter an ADIF file path"
}

func (m model) importADIFFile(path string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(strings.TrimSpace(path))
		if err != nil {
			return adifImportedMsg{err: fmt.Errorf("open ADIF file: %w", err)}
		}
		defer file.Close()
		result, err := importADIF(context.Background(), file, m.activeStation.ID, m.store)
		return adifImportedMsg{result: result, err: err}
	}
}

func (m *model) disconnectCluster() {
	m.clusterGeneration++ // invalidates a pending dial or reader result.
	err := m.clusterClient.close()
	m.clusterClient = nil
	m.clusterConnecting = false
	if err != nil {
		m.clusterStatus = fmt.Sprintf("cluster disconnect error: %v", err)
		return
	}
	m.clusterStatus = k3lrClusterName + " — disconnected"
}

func (m *model) addClusterSpot(spot clusterSpot) {
	m.clusterSpots = append([]clusterSpot{spot}, m.clusterSpots...)
	if len(m.clusterSpots) > 100 {
		m.clusterSpots = m.clusterSpots[:100]
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, fetchSolarIndicesCmd(), solarTickCmd())
}

func (m *model) refreshTableRows() {
	recent, err := m.store.recentQSOs(50)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
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

	if n, err := m.store.count(); err == nil {
		m.qsoCount = n
	}
}

func (m *model) checkDupe() {
	call := normalizeCall(m.fields[fieldCall].Value())
	m.dupeWarning = false
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
	var contestID, eventID, dupeScope string
	if event, ok := m.eventForContestID(); ok {
		contestID = strings.TrimSpace(m.contestFields[contestName].Value())
		eventID = event.ID
		dupeScope = event.DupeScope
	} else if raw := strings.TrimSpace(m.contestFields[contestName].Value()); raw != "" && raw != m.contestScopeFallbackFor {
		m.contestScopeFallbackFor = raw
		m.statusMsg = fmt.Sprintf("contest %q not found in event catalog — dupe check uses the 15-minute casual window", raw)
	}
	dupe, err := m.store.isDupe(call, m.qsoBand(), contestID, eventID, dupeScope, m.activeStation.ID, time.Now())
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return
	}
	m.dupeWarning = dupe
}

func (m model) qsoBand() string {
	return strings.ToUpper(strings.TrimSpace(m.fields[fieldBand].Value()))
}

func (m model) qsoMode() string {
	return strings.ToUpper(strings.TrimSpace(m.fields[fieldMode].Value()))
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
	contacts, err := m.store.qsosByCall(call)
	if err != nil {
		m.statusMsg = fmt.Sprintf("history error: %v", err)
		return
	}
	rows := make([]table.Row, 0, len(contacts))
	for _, q := range contacts {
		rows = append(rows, table.Row{q.time.Format("2006-01-02 15:04"), q.call, q.band, q.rstSent, q.rstRcvd})
	}
	m.table.SetRows(rows)
	m.workedCall = call
}

func (m *model) focusField(i int) {
	for idx := range m.fields {
		if idx == i {
			m.fields[idx].Focus()
		} else {
			m.fields[idx].Blur()
		}
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
		m.contestFields[contestSerialSent].SetValue("001")
	}
	m.contestFields[contestExchangeSent].Placeholder = event.SentExchangeHint
	m.contestFields[contestExchangeRcvd].Placeholder = event.RcvdExchangeHint
	m.statusMsg = event.Name + " selected"
	m.exchangeChoiceFocus = -1
	m.openQSOContest()
}

func (m model) eventForContestID() (eventDefinition, bool) {
	id := m.contestFields[contestName].Value()
	for _, event := range m.events {
		if strings.HasPrefix(id, event.ID+"-") {
			return event, true
		}
	}
	return eventDefinition{}, false
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
	if m.focusIdx != fieldCall || !m.qsoStartedAt.IsZero() || strings.TrimSpace(m.fields[fieldCall].Value()) == "" {
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

func (m *model) resetQSOClockIfReturningToCall(nextFocus int) {
	if nextFocus != fieldCall || m.qsoStartedAt.IsZero() {
		return
	}
	m.qsoStartedAt = time.Time{}
	m.statusMsg = "QSO timer reset; enter callsign and continue"
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
	if m.dupeWarning {
		m.statusMsg = fmt.Sprintf("DUPE: %s already worked on %s — not logged", call, m.qsoBand())
		return m, nil
	}

	endedAt := time.Now().UTC()
	startedAt := m.qsoStartedAt
	if startedAt.IsZero() {
		// Tab normally starts the clock. This fallback keeps a QSO valid if an
		// operator uses a different terminal navigation sequence.
		startedAt = endedAt
	}
	logged := qso{
		call:      call,
		band:      m.qsoBand(),
		mode:      m.qsoMode(),
		profileID: m.activeStation.ID,
		rstSent:   m.fields[fieldRSTSent].Value(),
		rstRcvd:   m.fields[fieldRSTRcvd].Value(),
		exchange:  "",
		frequency: m.qsoFrequency(),
		name:      strings.TrimSpace(m.detailFields[detailName].Value()),
		qth:       strings.TrimSpace(m.detailFields[detailQTH].Value()),
		grid:      strings.TrimSpace(m.detailFields[detailGrid].Value()),
		state:     strings.TrimSpace(m.detailFields[detailState].Value()),
		potaRef:   strings.ToUpper(strings.TrimSpace(m.detailFields[detailPOTARef].Value())),
		comment:   strings.TrimSpace(m.detailFields[detailNotes].Value()),
		contestID: strings.TrimSpace(m.contestFields[contestName].Value()),
		stx:       strings.TrimSpace(m.contestFields[contestSerialSent].Value()),
		stxString: strings.TrimSpace(m.contestFields[contestExchangeSent].Value()),
		srx:       strings.TrimSpace(m.contestFields[contestSerialRcvd].Value()),
		srxString: strings.TrimSpace(m.contestFields[contestExchangeRcvd].Value()),
		time:      startedAt,
		timeOff:   endedAt,

		myGridSquare:    m.activeStation.MyGridSquare,
		stationCallsign: m.activeStation.Callsign,
		operatorName:    m.activeStation.OperatorName,
		myRig:           m.activeStation.Rig,
		myAntenna:       m.activeStation.Antenna,
		txPower:         m.activeStation.PowerWatts,
	}
	_, err := m.store.insertQSO(logged)
	if err != nil {
		m.statusMsg = fmt.Sprintf("db error: %v", err)
		return m, nil
	}
	m.refreshTableRows()
	m.statusMsg = fmt.Sprintf("logged %s (%s)", call, endedAt.Sub(startedAt).Round(time.Second))
	if event, ok := m.eventForContestID(); ok && len(event.Bands) > 0 && !bandAllowed(event.Bands, logged.band) {
		m.statusMsg += fmt.Sprintf(" — warning: %s is not in %s's allowed bands (%s)", logged.band, event.Name, strings.Join(event.Bands, "/"))
	}

	m.fields[fieldCall].SetValue("")
	m.fields[fieldRSTRcvd].SetValue("")
	for index := range m.detailFields {
		m.detailFields[index].SetValue("")
	}
	for index := range m.contestFields {
		m.contestFields[index].SetValue("")
	}
	m.qsoStartedAt = time.Time{}
	m.workedCall = ""
	m.focusField(fieldCall)
	m.refreshTableRows()
	return m, qrzUploadCmd(m.qrzAPIKey, logged)
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
		if message.reference != "" && strings.TrimSpace(m.detailFields[detailPOTARef].Value()) == "" {
			m.detailFields[detailPOTARef].SetValue(message.reference)
			m.statusMsg = "POTA " + message.reference + " from recent POTA spot"
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
	if message, ok := msg.(qrzUploadMsg); ok {
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("QRZ upload failed for %s: %v", message.call, message.err)
		} else {
			m.statusMsg = fmt.Sprintf("QRZ upload OK for %s (LOGID %s)", message.call, message.logID)
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
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "f8" {
		if m.backupInProgress {
			m.statusMsg = "backup already in progress…"
			return m, nil
		}
		m.backupInProgress = true
		m.statusMsg = "backing up to Google Drive…"
		return m, m.runBackupCmd()
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
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "tab":
			leavingCall := m.focusIdx == fieldCall
			m.startQSOClockIfLeavingCall()
			nextFocus := (m.focusIdx + 1) % len(m.fields)
			m.resetQSOClockIfReturningToCall(nextFocus)
			m.focusField(nextFocus)
			if leavingCall {
				return m, m.autoFillPOTAReference()
			}
			return m, nil
		case "shift+tab":
			nextFocus := (m.focusIdx - 1 + len(m.fields)) % len(m.fields)
			m.resetQSOClockIfReturningToCall(nextFocus)
			m.focusField(nextFocus)
			return m, nil
		case "enter":
			if m.focusIdx == len(m.fields)-1 {
				var cmd tea.Cmd
				m, cmd = m.logCurrentQSO()
				return m, cmd
			}
			leavingCall := m.focusIdx == fieldCall
			m.startQSOClockIfLeavingCall()
			m.focusField(m.focusIdx + 1)
			if leavingCall {
				return m, m.autoFillPOTAReference()
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
		}
		if m.focusIdx == fieldBand {
			// Band is intentionally a closed selector. This prevents an invalid or
			// unsupported band label from being entered into a QSO, while allowing
			// non-key messages (such as textinput cursor-blink ticks) through.
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.fields[m.focusIdx], cmd = m.fields[m.focusIdx].Update(msg)
	if m.focusIdx == fieldCall {
		m.checkDupe()
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
	m.contestFields[m.contestFocusIdx], cmd = m.contestFields[m.contestFocusIdx].Update(msg)
	m.exchangeChoiceFocus = -1
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
			m.clusterStatus = "connecting to " + k3lrClusterAddr + "…"
			return m, connectK3LR(m.activeStation.Callsign, m.clusterGeneration)
		case "f6":
			m.disconnectCluster()
			return m, nil
		case "f4":
			m.openClusterFilters()
			return m, nil
		}
	case clusterConnectedMsg:
		if message.generation != m.clusterGeneration {
			message.client.close()
			return m, nil
		}
		m.clusterConnecting = false
		if message.err != nil {
			m.clusterStatus = message.err.Error()
			return m, nil
		}
		m.clusterClient = message.client
		m.clusterStatus = k3lrClusterName + " — connected"
		return m, m.clusterClient.readNext()
	case clusterLineMsg:
		if message.generation != m.clusterGeneration {
			return m, nil
		}
		if message.err != nil {
			if m.clusterClient != nil {
				m.clusterClient = nil
			}
			m.clusterConnecting = false
			m.clusterStatus = fmt.Sprintf("cluster connection ended: %v", message.err)
			return m, nil
		}
		if spot, ok := parseClusterSpot(message.line, time.Now()); ok && m.clusterFilters.allowsSpot(spot) {
			m.addClusterSpot(spot)
		}
		if m.clusterClient != nil {
			return m, m.clusterClient.readNext()
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
			m.clusterFilters.Bands[band] = !m.clusterFilters.Bands[band]
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
	switch message := msg.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.screen = qsoEntryScreen
			m.focusField(fieldCall)
			return m, nil
		case "enter":
			path := strings.TrimSpace(m.adifPathField.Value())
			if path == "" {
				m.statusMsg = "ADIF file path is required"
				return m, nil
			}
			m.statusMsg = "Importing ADIF…"
			return m, m.importADIFFile(path)
		}
	case adifImportedMsg:
		if message.err != nil {
			m.statusMsg = fmt.Sprintf("ADIF import failed: %v", message.err)
			return m, nil
		}
		m.refreshTableRows()
		m.screen = qsoEntryScreen
		m.focusField(fieldCall)
		m.statusMsg = fmt.Sprintf("ADIF imported: %d CW QSOs; %d skipped", message.result.Imported, message.result.Skipped)
		return m, nil
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
				m.saveStationSetup()
				return m, nil
			}
			m.focusStationField(m.stationFocusIdx + 1)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.stationFields[m.stationFocusIdx], cmd = m.stationFields[m.stationFocusIdx].Update(msg)
	return m, cmd
}

func (m model) renderField(idx int) string {
	box := fieldBoxStyle
	if idx == m.focusIdx {
		box = focusedFieldBoxStyle
	}
	content := labelStyle.Render(fieldLabels[idx]) + m.fields[idx].View()
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
	var b strings.Builder
	b.WriteString(screenHotkeys(qsoEntryScreen))
	b.WriteString("\n")

	now := time.Now()
	header := fmt.Sprintf(
		"%s  |  %s %s  |  UTC %s  |  Local %s (%s)",
		m.contestName,
		m.qsoBand(),
		m.qsoMode(),
		now.UTC().Format("15:04:05Z"),
		now.Format("15:04:05 -07:00"),
		now.Location(),
	)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(m.solarLine()))
	b.WriteString("\n\n")

	fieldViews := make([]string, fieldCount)
	for i := range fieldViews {
		fieldViews[i] = m.renderField(i)
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, fieldViews...))
	b.WriteString("\n")

	if m.dupeWarning {
		b.WriteString(dupeStyle.Render("DUPE"))
		b.WriteString("\n\n")
	} else {
		b.WriteString("\n")
	}

	workedLabel := "Recent QSOs"
	if m.workedCall != "" {
		workedLabel = "Stations Worked: " + m.workedCall + " (prior contacts)"
	}
	b.WriteString(helpStyle.Render(workedLabel))
	b.WriteString("\n")
	b.WriteString(m.table.View())
	b.WriteString("\n\n")

	status := fmt.Sprintf("Qs: %d   %s", m.qsoCount, m.statusMsg)
	b.WriteString(statusBarStyle.Render(status))
	b.WriteString("\n")

	help := "tab/shift+tab: move/edit fields  •  first tab after callsign starts QSO  •  final enter: save next QSO"
	b.WriteString(helpStyle.Render(help))

	return b.String()
}

func (m model) stationSetupView() string {
	var b strings.Builder
	b.WriteString(screenHotkeys(stationSetupScreen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  Station Setup"))
	b.WriteString("\n\n")
	for i := range m.stationFields {
		style := fieldBoxStyle
		if i == m.stationFocusIdx {
			style = focusedFieldBoxStyle
		}
		b.WriteString(style.Render(labelStyle.Render(stationFieldLabels[i]) + m.stationFields[i].View()))
		if i%2 == 1 || i == len(m.stationFields)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(statusBarStyle.Render(m.statusMsg))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("tab/shift+tab: move fields  •  final enter: save station profile"))
	return b.String()
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
	for i := range m.clusterFilterFields {
		style := fieldBoxStyle
		if i == m.clusterFilterFocus {
			style = focusedFieldBoxStyle
		}
		b.WriteString(style.Render(labelStyle.Render(clusterFilterLabels[i]) + m.clusterFilterFields[i].View()))
		if i%2 == 1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\nBands: ")
	for i, band := range cwBands {
		mark := " "
		if m.clusterFilters.Bands[band] {
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

func (m model) qsoPageView(title string, labels []string, fields []textinput.Model, focus int, help string) string {
	var b strings.Builder
	b.WriteString(screenHotkeys(m.screen))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render("W4GNS Logger 4 Men  |  " + title))
	b.WriteString("\n\n")
	for index := range fields {
		style := fieldBoxStyle
		if index == focus {
			style = focusedFieldBoxStyle
		}
		b.WriteString(style.Render(labelStyle.Render(labels[index]) + fields[index].View()))
		if index%2 == 1 || index == len(fields)-1 {
			b.WriteString("\n")
		}
	}
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
	} else if current == qsoDetailsScreen || current == qsoContestScreen || current == eventCatalogScreen {
		escape = "Esc: QSO Entry"
	}
	return helpStyle.Render("F1: QSO Entry  •  F2: Station Setup  •  F3: DX Cluster  •  F4: Filters  •  F5: Import ADIF  •  F6: QSO Details  •  F7: Events  •  F8: Backup  •  " + escape)
}

func main() {
	if exportPath, ok := adifExportPath(os.Args[1:]); ok {
		runADIFExport(exportPath)
		return
	}
	if importPath, ok := adifImportPath(os.Args[1:]); ok {
		runADIFImport(importPath)
		return
	}
	if !hasArg(os.Args[1:], terminalChildArg) && !hasArg(os.Args[1:], inCurrentTerminalArg) {
		if err := launchInOwnTerminal(); err != nil {
			fmt.Fprintf(os.Stderr, "error opening W4GNS Logger 4 Men in its own terminal: %v\n", err)
			os.Exit(1)
		}
		return
	}

	dbPath := "w4gns.db"
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

	// Alt-screen mode gives the logger a clean, dedicated terminal surface and
	// restores the invoking terminal unchanged when the application exits.
	p := tea.NewProgram(m, tea.WithAltScreen())

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
	// through here, so a single backup-on-exit call covers all of them.
	if m, ok := finalModel.(model); ok {
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
		dbPath = "w4gns.db"
	}
	if pathsReferToSameFile(path, dbPath) {
		fmt.Fprintln(os.Stderr, "ADIF export path must not be the SQLite database")
		os.Exit(1)
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
	file, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating ADIF file: %v\n", err)
		os.Exit(1)
	}
	count, err := exportADIF(context.Background(), file, profile.ID, st)
	if err != nil {
		file.Close()
		fmt.Fprintf(os.Stderr, "ADIF export failed: %v\n", err)
		os.Exit(1)
	}
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "error closing ADIF file: %v\n", err)
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
	firstPath, firstErr := filepath.Abs(first)
	secondPath, secondErr := filepath.Abs(second)
	return firstErr == nil && secondErr == nil && firstPath == secondPath
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
		dbPath = "w4gns.db"
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
