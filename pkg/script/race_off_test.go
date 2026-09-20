//go:build !race

package script

// raceEnabled reports whether this test binary was built with -race. Go
// exposes no such predicate outside internal/race, so the idiom is this
// pair of build-tagged files; see race_on_test.go for the other half.
const raceEnabled = false
