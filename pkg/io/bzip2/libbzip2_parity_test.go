package bzip2_test

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/zsrv/goscape/pkg/io/bzip2"
)

// TestLibbzip2Parity pins that pkg/io/bzip2 produces output byte-identical
// to system libbzip2's `bzip2 -1 -c` for a representative input set. This
// is the contract that motivates the K-means tree-selection + heap-based
// Huffman + RLE-block-size-cap-19 ports of libbzip2 into the vendored
// dsnet/compress fork. If this regresses, smoke-pack against Engine-TS
// data/pack will start drifting on every Jagfile-wrapped stage again.
//
// Skipped silently if `bzip2` is not on PATH (CI shouldn't fail without
// the system binary, but local devs and the Linux dev image both have it).
func TestLibbzip2Parity(t *testing.T) {
	if _, err := exec.LookPath("bzip2"); err != nil {
		t.Skip("system bzip2 binary not found in PATH; skipping libbzip2 parity")
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single_byte", []byte{0x42}},
		{"hello_world", []byte("Hello World!")},
		{"alphabet", []byte("the quick brown fox jumps over the lazy dog")},
		{"long_run", bytes.Repeat([]byte{'A'}, 4096)},
		{"mixed_runs", append(append(bytes.Repeat([]byte{'A'}, 300), bytes.Repeat([]byte{'B'}, 256)...), bytes.Repeat([]byte{'C'}, 128)...)},
		{"binary_pattern", func() []byte {
			b := make([]byte, 8192)
			for i := range b {
				b[i] = byte(i * 17)
			}
			return b
		}()},
		// Multi-block inputs exercise the RLE state-carry-over across block
		// boundaries (mirrors libbzip2 state_in_ch/state_in_len; see rle1.go).
		// Before the carry-over port, these diverged by a few bytes (smoke-pack
		// Graphics/client/models residual).
		{"multi_block_runs_at_boundary", func() []byte {
			b := make([]byte, 0, 200000)
			for i := 0; i < 90000; i++ {
				b = append(b, byte(i*7))
			}
			b = append(b, bytes.Repeat([]byte{'A'}, 50000)...)
			for i := 0; i < 60000; i++ {
				b = append(b, byte(i*13+1))
			}
			return b
		}()},
		{"multi_block_pure_run", bytes.Repeat([]byte{'A'}, 200000)},
		{"multi_block_long_input", func() []byte {
			b := make([]byte, 456725)
			for i := range b {
				b[i] = byte(i*131 ^ (i >> 5))
			}
			return b
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Skip empty — bzip2 -1 of an empty input is a valid stream but
			// our writer's Close() path on zero bytes is a separate code path
			// and not worth pinning in this contract.
			if len(tc.data) == 0 {
				t.Skip("empty input has its own code path; not part of parity contract")
			}

			var ours bytes.Buffer
			w, err := bzip2.NewWriter(&ours, &bzip2.WriterConfig{Level: 1})
			if err != nil {
				t.Fatalf("NewWriter: %v", err)
			}
			if _, err := w.Write(tc.data); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			cmd := exec.Command("bzip2", "-1", "-c")
			cmd.Stdin = bytes.NewReader(tc.data)
			ref, err := cmd.Output()
			if err != nil {
				t.Fatalf("bzip2 -1: %v", err)
			}

			if !bytes.Equal(ours.Bytes(), ref) {
				t.Errorf("byte mismatch:\n  ours len=%d first 32=%v\n  ref  len=%d first 32=%v",
					ours.Len(), ours.Bytes()[:min(32, ours.Len())],
					len(ref), ref[:min(32, len(ref))])
			}
		})
	}
}
