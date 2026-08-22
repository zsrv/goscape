package req

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// Pins SEC1 M-7: logging a GameLogin at any level must never emit the
// password, ISAAC seed or CRC table. Username/revision/uid stay visible
// because they are the operationally useful fields.
func TestGameLogin_LogValueRedacts(t *testing.T) {
	q := GameLogin{
		Username:         "alice",
		Password:         "s3cretpw",
		ArchiveChecksums: [9]uint32{0xdeadbeef},
		ISAACSeed:        [4]uint32{0x11223344},
		UID:              42,
		Revision:         244,
		LowMemory:        true,
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	log.Debug("unmarshalled", "req", q)
	log.Debug("unmarshalled-ptr", "req", &q)
	out := buf.String()
	for _, forbidden := range []string{"s3cretpw", "deadbeef", "3735928559", "11223344", "287454020"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("log leaked %q:\n%s", forbidden, out)
		}
	}
	for _, want := range []string{"username=alice", "revision=244", "uid=42", "low_memory=true", "password=[redacted]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q:\n%s", want, out)
		}
	}
}
