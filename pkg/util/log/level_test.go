package log_test

import (
	"log/slog"
	"testing"

	"github.com/zsrv/goscape/pkg/util/log"
)

func TestLevelUnmarshalText(t *testing.T) {
	cases := map[string]slog.Level{
		"trace": log.LevelTrace,
		"TRACE": log.LevelTrace,
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range cases {
		var l log.Level
		if err := l.UnmarshalText([]byte(in)); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", in, err)
		}
		if slog.Level(l) != want {
			t.Errorf("UnmarshalText(%q) = %v, want %v", in, slog.Level(l), want)
		}
	}
}

func TestLevelUnmarshalInvalid(t *testing.T) {
	var l log.Level
	if err := l.UnmarshalText([]byte("loud")); err == nil {
		t.Fatal("expected error for invalid level, got nil")
	}
}

func TestLevelMarshalRoundTrip(t *testing.T) {
	for _, lv := range []log.Level{log.Level(log.LevelTrace), log.Level(slog.LevelInfo), log.Level(slog.LevelError)} {
		b, err := lv.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText(%v): %v", slog.Level(lv), err)
		}
		var got log.Level
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("UnmarshalText(%q): %v", b, err)
		}
		if got != lv {
			t.Errorf("round trip %v -> %q -> %v", slog.Level(lv), b, slog.Level(got))
		}
	}
}

func TestLevelTraceMarshalsAsTRACE(t *testing.T) {
	b, err := log.Level(log.LevelTrace).MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "TRACE" {
		t.Errorf("MarshalText = %q, want TRACE", b)
	}
}
