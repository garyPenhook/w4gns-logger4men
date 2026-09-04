package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/cty.dat
var ctyDatFS embed.FS

//go:embed data/arrl_dxcc.dat
var arrlDXCCFS embed.FS

// dxccEntity is the country/zone/continent context resolved from a callsign
// prefix via the bundled cty.dat table, plus the numeric ARRL/ADIF DXCC
// entity code cross-referenced from data/arrl_dxcc.dat by primary callsign
// prefix (see loadARRLDXCCNumbers). DXCCNumber is 0 when cty.dat's primary
// prefix has no corresponding ARRL entry — either a cty.dat entity ARRL
// doesn't count as a separate DXCC entity (its primary prefix starts with
// "*"), or lookup() resolved via a non-primary alias whose entity wasn't
// cross-referenced. A zero value is left as-is rather than guessed.
type dxccEntity struct {
	Country    string
	CQZone     int
	ITUZone    int
	Continent  string
	DXCCNumber int
	// Latitude and Longitude are the entity's reference coordinates, degrees
	// north/east (standard signed convention). cty.dat stores longitude
	// west-positive, so it is negated on parse to match. Zero for both means
	// "no coordinate data" (heading/distance callers should skip rather than
	// treat 0,0 as the Gulf of Guinea).
	Latitude  float64
	Longitude float64
}

type dxccAlias struct {
	prefix string
	entity dxccEntity
}

// dxccTable resolves a callsign to a dxccEntity by longest-prefix match
// against the aliases parsed from cty.dat, honoring per-alias CQ/ITU zone
// overrides and "=CALL" exact-match exception entries. Prefix aliases are
// bucketed by their first byte so lookup only scans aliases that could
// possibly match a given call, instead of the full alias list (cty.dat has
// thousands of entries, and lookup runs per cluster spot and per imported
// QSO).
type dxccTable struct {
	aliases       []dxccAlias
	exactAliases  map[string]dxccEntity
	prefixByFirst map[byte][]dxccAlias
}

// aliasOverridePattern matches a single AD1C per-alias override group so they
// can be stripped from a token in any order, leaving the bare prefix. AD1C
// cty.dat defines five override kinds: (CQ zone), [ITU zone], <lat/lon>,
// {continent}, and ~UTC offset~. Earlier versions of this parser accepted only
// (cq) and [itu] via one rigid anchored pattern, so any alias carrying a
// <>/{}/~~ override — routine in upstream cty.dat releases — made the whole
// load fail and silently disabled all DXCC enrichment app-wide. This parser
// instead applies the overrides it uses (CQ/ITU/continent) and ignores the
// rest, so a data refresh degrades gracefully rather than turning enrichment
// off.
var aliasOverridePattern = regexp.MustCompile(`\((\d+)\)|\[(\d+)\]|<([^>]*)>|\{([^}]*)\}|~([^~]*)~`)

// loadDXCCTable parses the bundled cty.dat (the standard AD1C-style country
// file: a header line per DXCC entity followed by one or more indented,
// comma-separated alias lines terminated by ';').
func loadDXCCTable() (*dxccTable, error) {
	data, err := ctyDatFS.ReadFile("data/cty.dat")
	if err != nil {
		return nil, fmt.Errorf("read embedded cty.dat: %w", err)
	}
	dxccNumbers, err := loadARRLDXCCNumbers()
	if err != nil {
		return nil, err
	}
	table := &dxccTable{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var current dxccEntity
	var haveHeader bool
	var pending strings.Builder
	flush := func() error {
		if pending.Len() == 0 {
			return nil
		}
		if err := table.addAliases(current, pending.String()); err != nil {
			return err
		}
		pending.Reset()
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if err := flush(); err != nil {
				return nil, err
			}
			entity, err := parseCtyHeader(line, dxccNumbers)
			if err != nil {
				return nil, err
			}
			current = entity
			haveHeader = true
			continue
		}
		if !haveHeader {
			continue
		}
		pending.WriteString(strings.TrimSpace(line))
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan cty.dat: %w", err)
	}
	return table, nil
}

// loadARRLDXCCNumbers parses the embedded data/arrl_dxcc.dat table (see that
// file's header comment for provenance and matching methodology) into a map
// from cty.dat primary prefix to ARRL/ADIF DXCC entity number.
func loadARRLDXCCNumbers() (map[string]int, error) {
	data, err := arrlDXCCFS.ReadFile("data/arrl_dxcc.dat")
	if err != nil {
		return nil, fmt.Errorf("read embedded arrl_dxcc.dat: %w", err)
	}
	numbers := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			return nil, fmt.Errorf("malformed arrl_dxcc.dat line %q", line)
		}
		number, err := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err != nil {
			return nil, fmt.Errorf("malformed DXCC number in %q: %w", line, err)
		}
		numbers[fields[0]] = number
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan arrl_dxcc.dat: %w", err)
	}
	return numbers, nil
}

// parseCtyHeader reads the fixed 8 colon-separated fields of a cty.dat entry
// header: Name, CQ zone, ITU zone, continent, lat, long, UTC offset, primary
// prefix (with a trailing colon).
func parseCtyHeader(line string, dxccNumbers map[string]int) (dxccEntity, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 8 {
		return dxccEntity{}, fmt.Errorf("malformed cty.dat header %q", line)
	}
	cqZone, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return dxccEntity{}, fmt.Errorf("malformed CQ zone in %q: %w", line, err)
	}
	ituZone, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil {
		return dxccEntity{}, fmt.Errorf("malformed ITU zone in %q: %w", line, err)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
	if err != nil {
		return dxccEntity{}, fmt.Errorf("malformed latitude in %q: %w", line, err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
	if err != nil {
		return dxccEntity{}, fmt.Errorf("malformed longitude in %q: %w", line, err)
	}
	primaryPrefix := strings.TrimSpace(fields[7])
	return dxccEntity{
		Country:    strings.TrimSpace(fields[0]),
		CQZone:     cqZone,
		ITUZone:    ituZone,
		Continent:  strings.ToUpper(strings.TrimSpace(fields[3])),
		DXCCNumber: dxccNumbers[primaryPrefix],
		Latitude:   lat,
		Longitude:  -lon, // cty.dat stores longitude west-positive
	}, nil
}

// addAliases parses one entity's alias list, e.g.
// "1A;" or "9M0,BM9S,3H0(23),B2A[33],=3D2CCC;", applying any per-alias CQ/ITU
// zone overrides and recording exact-match ("=CALL") exceptions.
func (t *dxccTable) addAliases(base dxccEntity, list string) error {
	list = strings.TrimSuffix(strings.TrimSpace(list), ";")
	for _, token := range strings.Split(list, ",") {
		token = strings.ToUpper(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		exact := strings.HasPrefix(token, "=")
		token = strings.TrimPrefix(token, "=")
		entity := base
		// Apply each override group, then strip it, leaving the bare prefix.
		for _, group := range aliasOverridePattern.FindAllStringSubmatch(token, -1) {
			switch {
			case group[1] != "": // (CQ zone)
				if zone, err := strconv.Atoi(group[1]); err == nil {
					entity.CQZone = zone
				}
			case group[2] != "": // [ITU zone]
				if zone, err := strconv.Atoi(group[2]); err == nil {
					entity.ITUZone = zone
				}
			case group[3] != "": // <lat/lon>
				if lat, lon, ok := parseAliasLatLon(group[3]); ok {
					entity.Latitude = lat
					entity.Longitude = lon
				}
			case group[4] != "": // {continent}
				entity.Continent = strings.ToUpper(strings.TrimSpace(group[4]))
			}
			// group[5] (~UTC~) is not used by this app.
		}
		prefix := aliasOverridePattern.ReplaceAllString(token, "")
		if prefix == "" {
			return fmt.Errorf("malformed cty.dat alias %q", token)
		}
		if exact {
			if t.exactAliases == nil {
				t.exactAliases = make(map[string]dxccEntity)
			}
			t.exactAliases[prefix] = entity
			continue
		}
		if t.prefixByFirst == nil {
			t.prefixByFirst = make(map[byte][]dxccAlias)
		}
		t.prefixByFirst[prefix[0]] = append(t.prefixByFirst[prefix[0]], dxccAlias{prefix: prefix, entity: entity})
	}
	return nil
}

// parseAliasLatLon parses a cty.dat per-alias "<lat/lon>" override body
// (already stripped of the angle brackets by aliasOverridePattern), applying
// the same west-positive longitude normalization as the header fields.
func parseAliasLatLon(body string) (lat, lon float64, ok bool) {
	parts := strings.SplitN(body, "/", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	westLon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}
	return lat, -westLon, true
}

// portableCallSuffixes are common modifiers appended to a callsign that never
// carry DXCC-determining prefix information on their own.
var portableCallSuffixes = map[string]bool{
	"P": true, "M": true, "MM": true, "AM": true, "QRP": true,
	"A": true, "B": true, "LGT": true, "BCN": true,
}

// lookup resolves a callsign to its DXCC entity by trying the full call and,
// for portable-style calls like "PJ4/W4GNS" or "W4GNS/PJ4", each side of the
// slash, preferring an exact "=CALL" exception and otherwise the longest
// matching prefix. This is a practical approximation, not a full
// implementation of ARRL/WPX prefix-parsing rules for edge cases such as
// numeral-suffix portable operation.
func (t *dxccTable) lookup(call string) (dxccEntity, bool) {
	call = normalizeCall(call)
	if call == "" || t == nil {
		return dxccEntity{}, false
	}
	candidates := []string{call}
	if idx := strings.Index(call, "/"); idx >= 0 {
		candidates = append(candidates, call[:idx], call[idx+1:])
	}
	var best dxccEntity
	bestLen := -1
	for _, candidate := range candidates {
		if candidate == "" || portableCallSuffixes[candidate] {
			continue
		}
		if entity, ok := t.exactAliases[candidate]; ok {
			return entity, true
		}
		for _, alias := range t.prefixByFirst[candidate[0]] {
			if strings.HasPrefix(candidate, alias.prefix) && len(alias.prefix) > bestLen {
				bestLen = len(alias.prefix)
				best = alias.entity
			}
		}
	}
	return best, bestLen >= 0
}

var (
	sharedDXCCTableOnce sync.Once
	sharedDXCCTableInst *dxccTable
	sharedDXCCTableErr  error
)

// sharedDXCCTable loads the embedded cty.dat once per process. Callers that
// only enrich a record on a best-effort basis (country/CQ/ITU-zone lookup)
// should treat a returned error as "skip enrichment," not a fatal error.
func sharedDXCCTable() (*dxccTable, error) {
	sharedDXCCTableOnce.Do(func() {
		sharedDXCCTableInst, sharedDXCCTableErr = loadDXCCTable()
	})
	return sharedDXCCTableInst, sharedDXCCTableErr
}
