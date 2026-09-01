package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestImportADIF100kQSOsBenchmark verifies the README's large-log claim
// instead of leaving it unbenchmarked: it actually imports 100,000 CW QSOs
// and confirms every one lands, logging the elapsed time and throughput.
// Skipped under `go test -short` since it takes several seconds.
func TestImportADIF100kQSOsBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large import benchmark in -short mode")
	}
	st, err := openStore(t.TempDir() + "/logger.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	profile, err := st.activeStationProfile()
	if err != nil {
		t.Fatal(err)
	}

	const total = 100000
	var b strings.Builder
	b.Grow(total * 90)
	b.WriteString("<ADIF_VER:5>3.1.7<EOH>")
	for i := 0; i < total; i++ {
		call := fmt.Sprintf("W%dTEST", i)
		fmt.Fprintf(&b, "<CALL:%d>%s<QSO_DATE:8>20260831<TIME_ON:6>120000<BAND:3>20M<MODE:2>CW<EOR>", len(call), call)
	}

	start := time.Now()
	result, err := importADIF(context.Background(), strings.NewReader(b.String()), profile.ID, st)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("importADIF: %v", err)
	}
	if result.Imported != total {
		t.Fatalf("Imported = %d, want %d", result.Imported, total)
	}
	count, err := st.count()
	if err != nil {
		t.Fatal(err)
	}
	if count != total {
		t.Fatalf("stored count = %d, want %d", count, total)
	}
	t.Logf("imported %d QSOs in %s (%.0f QSOs/sec)", total, elapsed, float64(total)/elapsed.Seconds())
	if raceEnabled {
		// The race detector's per-access overhead makes any fixed time
		// budget meaningless (and flaky) here; correctness was already
		// checked above.
		return
	}
	// Generous ceiling, not a target: catches a catastrophic regression (e.g.
	// an accidental O(n^2) path) without being flaky on slower CI hardware.
	const budget = 30 * time.Second
	if elapsed > budget {
		t.Fatalf("import took %s, want under %s", elapsed, budget)
	}
}
