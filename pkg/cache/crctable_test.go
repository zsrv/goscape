package cache

import (
	"bytes"
	"log/slog"
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
	if !bytes.Contains([]byte(out), []byte("makeCrc Stat failed")) {
		t.Errorf("expected 'makeCrc Stat failed' in output; got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("/nonexistent/path/that/should/never/exist")) {
		t.Errorf("expected path in output; got: %s", out)
	}
}
