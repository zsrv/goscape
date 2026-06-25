package log_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/util/log"
)

func TestSourceFormatUnmarshalText(t *testing.T) {
	tests := []struct {
		in   string
		want log.SourceFormat
	}{
		{"relative", log.SourceRelative},
		{"RELATIVE", log.SourceRelative},
		{" relative ", log.SourceRelative},
		{"", log.SourceRelative}, // empty defaults to relative
		{"short", log.SourceShort},
		{"full", log.SourceFull},
	}
	for _, tc := range tests {
		var got log.SourceFormat
		if err := got.UnmarshalText([]byte(tc.in)); err != nil {
			t.Fatalf("UnmarshalText(%q): unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("UnmarshalText(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSourceFormatUnmarshalTextInvalid(t *testing.T) {
	var s log.SourceFormat
	if err := s.UnmarshalText([]byte("bogus")); err == nil {
		t.Fatal("expected error for invalid source format")
	}
}

func TestSourceFormatRoundTrip(t *testing.T) {
	for _, sf := range []log.SourceFormat{log.SourceRelative, log.SourceShort, log.SourceFull} {
		text, err := sf.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", sf, err)
		}
		var back log.SourceFormat
		if err := back.UnmarshalText(text); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", text, err)
		}
		if back != sf {
			t.Errorf("round trip %v -> %q -> %v", sf, text, back)
		}
	}
}

// Default rendering (no option) is module-root-relative, so the path stays
// clickable from the repository root in an IDE.
func TestNewLoggerSourceRelativeByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger, err := log.NewLogger(slog.LevelInfo, "text", &buf)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("hi")
	out := buf.String()
	if !strings.Contains(out, "source=pkg/util/log/source_test.go:") {
		t.Errorf("want module-root-relative source in %q", out)
	}
	if strings.Contains(out, "source=/") {
		t.Errorf("relative source should not be absolute: %q", out)
	}
}

func TestNewLoggerSourceShort(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := log.NewLogger(slog.LevelInfo, "text", &buf, log.WithSourceFormat(log.SourceShort))
	logger.Info("hi")
	out := buf.String()
	if !strings.Contains(out, "source=source_test.go:") {
		t.Errorf("want basename-only source in %q", out)
	}
	if strings.Contains(out, "source=pkg/") {
		t.Errorf("short source should not carry a directory: %q", out)
	}
}

func TestNewLoggerSourceFull(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := log.NewLogger(slog.LevelInfo, "text", &buf, log.WithSourceFormat(log.SourceFull))
	logger.Info("hi")
	out := buf.String()
	// Tests build without -trimpath, so the compiler embeds absolute paths.
	if !strings.Contains(out, "source=/") || !strings.Contains(out, "pkg/util/log/source_test.go") {
		t.Errorf("want full compiler path in %q", out)
	}
}
