package wordpack

import (
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// TestPackUnpackRoundTrip exercises the happy path: Pack a
// sentence-cased string, then Unpack the bytes — Unpack applies
// sentence-case so a sentence-cased input round-trips cleanly.
func TestPackUnpackRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   string // already sentence-cased so round-trip equals identity
	}{
		{"simple ASCII", "Hello world"},
		{"mixed punctuation", "Hi! How are you?"},
		{"digits and letters", "Pick 3 swords"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pk := packet.NewPacket(nil)
			Pack(pk, c.in)
			pk2 := packet.NewPacket(pk.Data)
			got := Unpack(pk2, len(pk.Data))
			if got != c.in {
				t.Errorf("round-trip: got %q, want %q (packed: %x)", got, c.in, pk.Data)
			}
		})
	}
}

// TestUnpackAppliesSentenceCase pins the TS WordPack.toSentenceCase
// rules (WordPack.ts:80-94): capitalize after start, '.', or '!'.
func TestUnpackAppliesSentenceCase(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "hello. world! foo")
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	// The trailing 'o' in "foo" leaves carry=4, emitting a pad byte 0x40.
	// Decoding 0x40 produces nibbles (4, 0) → "o" + " " (charLookup[0]=" ").
	// This trailing pad space is TS-faithful codec behavior; the TS
	// implementation does not trim it (WordPack.ts:40 slices to pos which
	// includes the pad-derived chars). Deviation: plan prescribed
	// "Hello. World! Foo" but correct TS-faithful value is "Hello. World! Foo ".
	want := "Hello. World! Foo "
	if got != want {
		t.Errorf("sentence-case: got %q, want %q", got, want)
	}
}

// TestUnpackLengthCap pins TS WordPack.ts:19 (`pos < 80`): Unpack
// stops emitting characters once the decoded output reaches 80 chars,
// even if more input bytes remain.
func TestUnpackLengthCap(t *testing.T) {
	// 90 'a' chars → Pack truncates to 80 → 80 nibbles of index 3 (< 13) →
	// 40 bytes (2 nibbles per byte). Unpack with length=40 → 80 chars,
	// hitting the pos<80 cap exactly.
	src := strings.Repeat("a", 90)
	pk := packet.NewPacket(nil)
	Pack(pk, src) // Pack truncates input to 80 first (TS line 44-46).
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	if len(got) != 80 {
		t.Errorf("Unpack length cap: got %d chars, want 80", len(got))
	}
}

// TestPackLengthCap pins TS WordPack.ts:44-46: Pack truncates input
// to the first 80 characters.
func TestPackLengthCap(t *testing.T) {
	src := strings.Repeat("a", 90)
	pk := packet.NewPacket(nil)
	Pack(pk, src)
	// 80 'a' chars pack at 4 bits per char (index 3 < 13) → 40 bytes.
	if len(pk.Data) != 40 {
		t.Errorf("Pack truncate: got %d bytes, want 40 (80 chars * 4 bits)", len(pk.Data))
	}
}

// TestPackUnpackPoundSign pins multi-byte UTF-8 handling. The TS
// charLookup table includes '£' which is a single code unit in
// UTF-16 but 2 bytes in UTF-8; the Go port uses []string instead
// of []byte specifically to preserve this character.
func TestPackUnpackPoundSign(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "Cost £5") // already sentence-cased
	pk2 := packet.NewPacket(pk.Data)
	got := Unpack(pk2, len(pk.Data))
	if got != "Cost £5" {
		t.Errorf("£ round-trip: got %q, want %q", got, "Cost £5")
	}
}

// TestUnpackEmpty pins zero-length decode behavior.
func TestUnpackEmpty(t *testing.T) {
	pk := packet.NewPacket(nil)
	got := Unpack(pk, 0)
	if got != "" {
		t.Errorf("empty Unpack: got %q, want \"\"", got)
	}
}

// TestPackEmpty pins zero-length encode behavior.
func TestPackEmpty(t *testing.T) {
	pk := packet.NewPacket(nil)
	Pack(pk, "")
	if len(pk.Data) != 0 {
		t.Errorf("empty Pack: got %d bytes, want 0", len(pk.Data))
	}
}
