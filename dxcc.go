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

// dxccEntity is the country/zone/continent context resolved from a callsign
// prefix via the bundled cty.dat table. It intentionally omits the ARRL/ADIF
// numeric DXCC entity code: the shipped cty.dat has no reliable mapping to
// that code, and guessing one would be worse than leaving it blank.
type dxccEntity struct {
	Country   string
	CQZone    int
	ITUZone   int
	Continent string
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

var aliasTokenPattern = regexp.MustCompile(`^(=)?([A-Z0-9/]+)(?:\((\d+)\))?(?:\[(\d+)\])?$`)

// loadDXCCTable parses the bundled cty.dat (the standard AD1C-style country
// file: a header line per DXCC entity followed by one or more indented,
// comma-separated alias lines terminated by ';').
func loadDXCCTable() (*dxccTable, error) {
	data, err := ctyDatFS.ReadFile("data/cty.dat")
	if err != nil {
		return nil, fmt.Errorf("read embedded cty.dat: %w", err)
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
			entity, err := parseCtyHeader(line)
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

// parseCtyHeader reads the fixed 8 colon-separated fields of a cty.dat entry
// header: Name, CQ zone, ITU zone, continent, lat, long, UTC offset, primary
// prefix (with a trailing colon).
func parseCtyHeader(line string) (dxccEntity, error) {
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
	return dxccEntity{
		Country:   strings.TrimSpace(fields[0]),
		CQZone:    cqZone,
		ITUZone:   ituZone,
		Continent: strings.ToUpper(strings.TrimSpace(fields[3])),
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
		match := aliasTokenPattern.FindStringSubmatch(token)
		if match == nil {
			return fmt.Errorf("malformed cty.dat alias %q", token)
		}
		entity := base
		if match[3] != "" {
			if zone, err := strconv.Atoi(match[3]); err == nil {
				entity.CQZone = zone
			}
		}
		if match[4] != "" {
			if zone, err := strconv.Atoi(match[4]); err == nil {
				entity.ITUZone = zone
			}
		}
		prefix := match[2]
		if match[1] == "=" {
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
