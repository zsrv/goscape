package log_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/util/log"
)

func TestNewLoggerRendersTrace(t *testing.T) {
	var buf bytes.Buffer
	logger, err := log.NewLogger(log.LevelTrace, "text", &buf)
	if err != nil {
		t.Fatal(err)
	}
	log.Trace(logger, "firehose")
	out := buf.String()
	if !strings.Contains(out, "level=TRACE") {
		t.Errorf("want level=TRACE in %q", out)
	}
	if !strings.Contains(out, "msg=firehose") {
		t.Errorf("want msg=firehose in %q", out)
	}
}

func TestTraceHiddenAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := log.NewLogger(slog.LevelDebug, "text", &buf)
	log.Trace(logger, "should not appear")
	if buf.Len() != 0 {
		t.Errorf("trace record emitted at debug level: %q", buf.String())
	}
}

// Source-attribute rendering (default, short, full) is covered in
// source_test.go.

func TestInvalidFormatErrors(t *testing.T) {
	if _, err := log.NewLogger(slog.LevelInfo, "xml", &bytes.Buffer{}); err == nil {
		t.Fatal("expected error for invalid format")
	}
}
