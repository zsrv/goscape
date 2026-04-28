package cache

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestResetCRCStateRestoresInitialState pins NAI-20 Task 3 C1: the
// helper re-initializes CrcBuffer, CrcTable, and CrcBuffer32 to their
// package-init shape. Lockstep with crctable.go init expressions.
func TestResetCRCStateRestoresInitialState(t *testing.T) {
	// Mutate state.
	CrcBuffer = packet.NewPacket(make([]byte, 0, 16))
	CrcBuffer.P4(0xDEADBEEF)
	CrcTable = []uint32{1, 2, 3}
	CrcBuffer32 = 0xCAFEBABE

	ResetCRCState()

	if CrcBuffer.Pos != 0 {
		t.Errorf("CrcBuffer.Pos = %d, want 0", CrcBuffer.Pos)
	}
	if got := cap(CrcBuffer.Bytes()); got < 4*9 {
		t.Errorf("CrcBuffer cap = %d, want at least %d", got, 4*9)
	}
	if CrcTable != nil {
		t.Errorf("CrcTable = %v, want nil", CrcTable)
	}
	if CrcBuffer32 != 0 {
		t.Errorf("CrcBuffer32 = %d, want 0", CrcBuffer32)
	}
	if CrcBytes != nil {
		t.Errorf("CrcBytes = %v, want nil", CrcBytes)
	}
}

// TestMakeCRCsPopulatesCrcBytes pins that MakeCRCs snapshots the CRC
// payload into CrcBytes so HTTP handlers can serve it without a stateful reader.
func TestMakeCRCsPopulatesCrcBytes(t *testing.T) {
	ResetCRCState()
	t.Cleanup(ResetCRCState)

	MakeCRCs() // missing files are silently skipped; at least P4(0) is written

	if CrcBytes == nil {
		t.Fatal("CrcBytes is nil after MakeCRCs")
	}
	if len(CrcBytes) < 4 {
		t.Errorf("CrcBytes len = %d, want >= 4", len(CrcBytes))
	}
	// Must not alias CrcBuffer.Data — mutation of one must not affect the other.
	if len(CrcBytes) > 0 && len(CrcBuffer.Data) > 0 &&
		&CrcBytes[0] == &CrcBuffer.Data[0] {
		t.Error("CrcBytes aliases CrcBuffer.Data; must be an independent copy")
	}
}

// TestMakeCrcWarnsOnMissingFile pins NAI-20 Task 3 C2: makeCrc emits
// a slog.Warn on os.Stat failure. Captures via slog.Default() swap.
func TestMakeCrcWarnsOnMissingFile(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf,
		&slog.HandlerOptions{Level: slog.LevelWarn})))

	makeCrc("/nonexistent/path/that/should/never/exist")

	out := buf.String()
	if out == "" {
		t.Fatalf("expected slog.Warn output, got empty buffer")
	}
	if !strings.Contains(out, "makeCrc Stat failed") {
		t.Errorf("expected 'makeCrc Stat failed' in output; got: %s", out)
	}
	if !strings.Contains(out, "/nonexistent/path/that/should/never/exist") {
		t.Errorf("expected path in output; got: %s", out)
	}
}
