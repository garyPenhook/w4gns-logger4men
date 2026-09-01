package main

import "testing"

// TestDXCCLookupResolvesKnownCallsigns is a regression guard for the
// first-byte-bucketed alias index in dxccTable.lookup: it must return the
// same result as the original full linear scan for ordinary calls, exact-
// match exceptions, and portable calls with a slash.
func TestDXCCLookupResolvesKnownCallsigns(t *testing.T) {
	table, err := loadDXCCTable()
	if err != nil {
		t.Fatalf("loadDXCCTable returned error: %v", err)
	}

	entity, ok := table.lookup("W4GNS")
	if !ok {
		t.Fatal("lookup(\"W4GNS\") = not found, want a match")
	}
	if entity.Country != "United States" {
		t.Errorf("lookup(\"W4GNS\").Country = %q, want %q", entity.Country, "United States")
	}
	if entity.DXCCNumber != 291 {
		t.Errorf("lookup(\"W4GNS\").DXCCNumber = %d, want 291 (per the ARRL DXCC List)", entity.DXCCNumber)
	}

	// Portable call: neither side of the slash alone should be treated as
	// authoritative over the other; the longest matching prefix wins.
	if _, ok := table.lookup("PJ4/W4GNS"); !ok {
		t.Error("lookup(\"PJ4/W4GNS\") = not found, want a match on one side of the slash")
	}

	if _, ok := table.lookup(""); ok {
		t.Error("lookup(\"\") = found, want no match for an empty call")
	}
}

// TestDXCCNumberResolvesFromARRLTable spot-checks the ARRL DXCC entity
// number cross-reference (data/arrl_dxcc.dat) against a handful of
// well-known calls, including a case where cty.dat splits one ARRL entity
// across multiple aliases (the UK's constituent nations are each their own
// DXCC entity) and calls resolving to entities ARRL doesn't count as
// separate from a parent (marked with a leading "*" primary prefix in
// cty.dat), which must resolve to DXCCNumber == 0 rather than a guess.
func TestDXCCNumberResolvesFromARRLTable(t *testing.T) {
	table, err := loadDXCCTable()
	if err != nil {
		t.Fatalf("loadDXCCTable returned error: %v", err)
	}
	for _, tc := range []struct {
		call string
		want int
	}{
		{"W4GNS", 291},  // United States
		{"VE3ABC", 1},   // Canada
		{"G0ABC", 223},  // England
		{"GM1ABC", 279}, // Scotland — a separate DXCC entity from England
		{"GW1ABC", 294}, // Wales
		{"GI1ABC", 265}, // Northern Ireland
		{"JA1ABC", 339}, // Japan
		{"3D2ABC", 176}, // Fiji
		{"OK1ABC", 503}, // Czech Republic
	} {
		entity, ok := table.lookup(tc.call)
		if !ok {
			t.Errorf("lookup(%q) = not found", tc.call)
			continue
		}
		if entity.DXCCNumber != tc.want {
			t.Errorf("lookup(%q).DXCCNumber = %d, want %d (%s)", tc.call, entity.DXCCNumber, tc.want, entity.Country)
		}
	}
	for _, call := range []string{"IT9ABC", "IG9ABC"} {
		entity, ok := table.lookup(call)
		if !ok {
			t.Errorf("lookup(%q) = not found", call)
			continue
		}
		if entity.DXCCNumber != 0 {
			t.Errorf("lookup(%q).DXCCNumber = %d, want 0 (%s is not a separate ARRL DXCC entity)", call, entity.DXCCNumber, entity.Country)
		}
	}
}

// TestLoadARRLDXCCNumbersCoversMostCurrentEntities guards against the
// embedded data/arrl_dxcc.dat table silently shrinking (e.g. a bad
// regeneration) — it should cover the vast majority of the roughly 340
// current ARRL DXCC entities.
func TestLoadARRLDXCCNumbersCoversMostCurrentEntities(t *testing.T) {
	numbers, err := loadARRLDXCCNumbers()
	if err != nil {
		t.Fatalf("loadARRLDXCCNumbers returned error: %v", err)
	}
	if len(numbers) < 300 {
		t.Fatalf("loadARRLDXCCNumbers() returned %d entries, want at least 300", len(numbers))
	}
}
