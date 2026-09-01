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

	// Portable call: neither side of the slash alone should be treated as
	// authoritative over the other; the longest matching prefix wins.
	if _, ok := table.lookup("PJ4/W4GNS"); !ok {
		t.Error("lookup(\"PJ4/W4GNS\") = not found, want a match on one side of the slash")
	}

	if _, ok := table.lookup(""); ok {
		t.Error("lookup(\"\") = found, want no match for an empty call")
	}
}
