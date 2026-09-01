//go:build !race

package main

// raceEnabled reports whether the binary was built with -race. The race
// detector adds substantial per-access overhead, so timing-sensitive tests
// (e.g. the large-import benchmark) use this to skip their time budget
// rather than become flaky under `go test -race`.
const raceEnabled = false
