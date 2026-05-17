package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestDispatch_NoArgs returns 2 and prints usage to stderr.
func TestDispatch_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch(nil, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("stderr %q missing usage", stderr.String())
	}
}

// TestDispatch_UnknownVerb returns 2 and names the verb in stderr.
func TestDispatch_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"frobnicate"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "frobnicate") {
		t.Errorf("stderr %q does not mention unknown verb", stderr.String())
	}
}

// TestDispatch_HelpFlag returns 0 and prints usage to stdout.
func TestDispatch_HelpFlag(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := dispatch([]string{arg}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("dispatch(%q) returned %d, want 0", arg, code)
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Errorf("stdout %q missing usage", stdout.String())
			}
		})
	}
}

// TestDispatch_PackRouting verifies the `pack` verb is dispatched to
// runPack (not happy-path — just that bad flags reach the pack flag
// set, surfacing exit code 2).
func TestDispatch_PackRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"pack", "--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestDispatch_CompileRouting verifies the `compile` verb is
// dispatched to runCompile (bad flags reach the compile flag set,
// surfacing exit code 2).
func TestDispatch_CompileRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"compile", "--no-such-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestDispatch_JagRouting verifies the `jag` verb is dispatched to
// runJag (bare `jag` returns 2 with missing-sub-verb diagnostic).
func TestDispatch_JagRouting(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := dispatch([]string{"jag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("dispatch returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "missing sub-verb") {
		t.Errorf("stderr %q missing sub-verb diagnostic", stderr.String())
	}
}
