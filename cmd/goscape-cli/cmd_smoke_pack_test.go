package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSmokePack_HelpFlagReturns0 pins -h/--help → exit 0 with flag listing on stderr.
func TestRunSmokePack_HelpFlagReturns0(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		t.Run(arg, func(t *testing.T) {
			var stderr bytes.Buffer
			code := runSmokePack([]string{arg}, io.Discard, &stderr)
			if code != 0 {
				t.Fatalf("runSmokePack(%q) returned %d, want 0", arg, code)
			}
			if !strings.Contains(stderr.String(), "content-dir") {
				t.Errorf("stderr %q missing flag listing", stderr.String())
			}
		})
	}
}

// TestRunSmokePack_UnknownFlagReturns2 pins flag-parse error → exit 2.
func TestRunSmokePack_UnknownFlagReturns2(t *testing.T) {
	var stderr bytes.Buffer
	code := runSmokePack([]string{"--no-such-flag"}, io.Discard, &stderr)
	if code != 2 {
		t.Fatalf("runSmokePack returned %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "no-such-flag") {
		t.Errorf("stderr %q does not mention unknown flag", stderr.String())
	}
}

// TestRunSmokePack_MissingContentDirReturns3 pins required-flag → exit 3.
func TestRunSmokePack_MissingContentDirReturns3(t *testing.T) {
	var stderr bytes.Buffer
	code := runSmokePack(nil, io.Discard, &stderr)
	if code != 3 {
		t.Fatalf("runSmokePack returned %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "content-dir") {
		t.Errorf("stderr %q missing content-dir mention", stderr.String())
	}
}

// TestRunSmokePack_NonExistentContentDirReturns3 pins setup error → exit 3.
func TestRunSmokePack_NonExistentContentDirReturns3(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	var stderr bytes.Buffer
	code := runSmokePack([]string{"--content-dir", missing}, io.Discard, &stderr)
	if code != 3 {
		t.Fatalf("runSmokePack returned %d, want 3", code)
	}
	if !strings.Contains(stderr.String(), "content-dir") && !strings.Contains(stderr.String(), missing) {
		t.Errorf("stderr %q missing path or content-dir mention", stderr.String())
	}
}
