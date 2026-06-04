# NAI-16 — MIDI Encoders + PRELOADED Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port TS `PreloadedPacks.ts` (3-dir client-asset registry), register `OpMidiSong` / `OpMidiJingle` wire opcodes, implement byte-level `encodeMidiSong` / `encodeMidiJingle` helpers, and wire the write path into `(*Player).PlaySong` / `PlayJingle` to retire S7h-D1.

**Architecture:** Three-package port. `pkg/cache/preloaded.go` adds two exported module-level map vars (`Preloaded`, `PreloadedCRC`) and an eager `PreloadClient(baseDir string) error`. World module's `startingFn` calls it before the TCP listener goes live (fail-fast). `pkg/io/protocol/game/server/prot.go` gains two Op constants. `modules/world/midi_encoders.go` adds two byte-level encoder helpers (PJStrLF+P4+P4 / P2+PData) following goscape's existing co-located encoder convention. `(*Player).PlaySong` / `PlayJingle` replace their S7h-D1 deferred-comment bodies with PRELOADED lookup + encoder + `p.writeOut`. Five existing absence-pin tests are renamed/rewritten as positive-pins, and three new miss-path tests pin the silent-no-op branches.

**Tech Stack:** Go 1.26+, `pkg/io/packet` primitives (PJStrLF, P2, P4, PData, GetCRC, NewPacket), `internal/dskit/services` BasicService lifecycle, `os` + `path/filepath` for dir walking, hash/crc32/IEEE (transitive via `packet.GetCRC`).

**Spec:** `docs/superpowers/specs/2026-04-24-nai-16-midi-encoders-design.md` (commits `5761ca1`, `ca0f0b4`).

**Predecessor:** S7i closed at HEAD=`2531588`. Spec on `main` adds two commits ahead of that.

---

## Task 1: PRELOADED registry — `pkg/cache/preloaded.go`

**Files:**
- Create: `pkg/cache/preloaded.go`
- Create: `pkg/cache/preloaded_test.go`

**TS reference:** `LostCityRS/Engine-TS/src/cache/PreloadedPacks.ts:1-41`

**Goal:** Two exported module-level map vars + one eager-loading function, mirroring TS `PRELOADED` / `PRELOADED_CRC` / `preloadClient()`. Hybrid test strategy — synthetic fixtures always run; one skip-if-missing integration test against `data/pack/client/songs/adventure.mid`.

- [ ] **Step 1: Write the test file with all 9 tests + reset helper**

Create `pkg/cache/preloaded_test.go`:

```go
package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// resetPreloadedForTest clears Preloaded + PreloadedCRC and registers a
// t.Cleanup to clear them again after the test. Preloaded and PreloadedCRC
// are package-global maps that bleed between tests if not isolated.
func resetPreloadedForTest(t *testing.T) {
	t.Helper()
	for k := range Preloaded {
		delete(Preloaded, k)
	}
	for k := range PreloadedCRC {
		delete(PreloadedCRC, k)
	}
	t.Cleanup(func() {
		for k := range Preloaded {
			delete(Preloaded, k)
		}
		for k := range PreloadedCRC {
			delete(PreloadedCRC, k)
		}
	})
}

// mkdirAll writes file with bytes inside <root>/<sub>/<name>, creating
// intermediate dirs. Test helper.
func writeFixture(t *testing.T, root, sub, name string, data []byte) {
	t.Helper()
	dir := filepath.Join(root, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write %s/%s: %v", dir, name, err)
	}
}

func TestPreloadClient3DirWalkPopulatesBothMaps(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	mapBytes := []byte("land")
	songBytes := []byte{0xFF, 0x00}
	jingleBytes := []byte{0x01}
	writeFixture(t, root, "maps", "m0_0", mapBytes)
	writeFixture(t, root, "songs", "test.mid", songBytes)
	writeFixture(t, root, "jingles", "fanfare.mid", jingleBytes)

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}

	cases := []struct {
		key   string
		bytes []byte
	}{
		{"m0_0", mapBytes},
		{"test.mid", songBytes},
		{"fanfare.mid", jingleBytes},
	}
	for _, c := range cases {
		got, ok := Preloaded[c.key]
		if !ok {
			t.Errorf("Preloaded[%q] missing", c.key)
			continue
		}
		if !bytes.Equal(got, c.bytes) {
			t.Errorf("Preloaded[%q] = %v, want %v", c.key, got, c.bytes)
		}
		gotCRC, ok := PreloadedCRC[c.key]
		if !ok {
			t.Errorf("PreloadedCRC[%q] missing", c.key)
			continue
		}
		wantCRC := packet.GetCRC(c.bytes, 0, len(c.bytes))
		if gotCRC != wantCRC {
			t.Errorf("PreloadedCRC[%q] = 0x%08x, want 0x%08x", c.key, gotCRC, wantCRC)
		}
	}
}

func TestPreloadClientEmptyDirsOK(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	if len(Preloaded) != 0 {
		t.Errorf("Preloaded has %d entries; want 0", len(Preloaded))
	}
	if len(PreloadedCRC) != 0 {
		t.Errorf("PreloadedCRC has %d entries; want 0", len(PreloadedCRC))
	}
}

func TestPreloadClientZeroByteFile(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	writeFixture(t, root, "maps", "placeholder", []byte{})
	writeFixture(t, root, "songs", "empty.mid", []byte{})
	writeFixture(t, root, "jingles", "blank.mid", []byte{})

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	for _, key := range []string{"placeholder", "empty.mid", "blank.mid"} {
		got, ok := Preloaded[key]
		if !ok || len(got) != 0 {
			t.Errorf("Preloaded[%q] = %v, want []byte{}", key, got)
		}
		if PreloadedCRC[key] != 0 {
			t.Errorf("PreloadedCRC[%q] = 0x%08x, want 0 (CRC32 of empty)", key, PreloadedCRC[key])
		}
	}
}

func TestPreloadClientSkipsSubdirs(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	// Create a stray subdir under maps/.
	if err := os.MkdirAll(filepath.Join(root, "maps", "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	if _, ok := Preloaded["sub"]; ok {
		t.Errorf("Preloaded[\"sub\"] should not be present (subdir skipped)")
	}
}

func TestPreloadClientMissingMapsDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"songs", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload maps") {
		t.Errorf("error %q does not contain \"preload maps\"", err.Error())
	}
}

func TestPreloadClientMissingSongsDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "jingles"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload songs") {
		t.Errorf("error %q does not contain \"preload songs\"", err.Error())
	}
}

func TestPreloadClientMissingJinglesDirReturnsError(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	for _, sub := range []string{"maps", "songs"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	err := PreloadClient(root)
	if err == nil {
		t.Fatal("PreloadClient: want error, got nil")
	}
	if !strings.Contains(err.Error(), "preload jingles") {
		t.Errorf("error %q does not contain \"preload jingles\"", err.Error())
	}
}

func TestPreloadClientKeyCollisionLastWins(t *testing.T) {
	resetPreloadedForTest(t)
	root := t.TempDir()
	mapBytes := []byte{0xAA}
	songBytes := []byte{0xBB}
	jingleBytes := []byte{0xCC}
	writeFixture(t, root, "maps", "shared.mid", mapBytes)
	writeFixture(t, root, "songs", "shared.mid", songBytes)
	writeFixture(t, root, "jingles", "shared.mid", jingleBytes)

	if err := PreloadClient(root); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	// Iteration order is maps → songs → jingles; last writer wins.
	if got := Preloaded["shared.mid"]; !bytes.Equal(got, jingleBytes) {
		t.Errorf("Preloaded[\"shared.mid\"] = %v, want %v (jingles wins, dir-order semantics)", got, jingleBytes)
	}
}

func TestPreloadClientAgainstStagedDataLoadsAdventure(t *testing.T) {
	resetPreloadedForTest(t)
	const knownPath = "../../data/pack/client/songs/adventure.mid"
	if _, err := os.Stat(knownPath); err != nil {
		t.Skipf("staged data not present at %s; skipping integration test", knownPath)
	}
	if err := PreloadClient("../../data/pack/client"); err != nil {
		t.Fatalf("PreloadClient: %v", err)
	}
	got, ok := Preloaded["adventure.mid"]
	if !ok {
		t.Fatal("Preloaded[\"adventure.mid\"] missing after load against staged data")
	}
	if len(got) == 0 {
		t.Errorf("Preloaded[\"adventure.mid\"] is empty; want non-empty")
	}
	want, err := os.ReadFile(knownPath)
	if err != nil {
		t.Fatalf("read direct: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Preloaded[\"adventure.mid\"] does not match direct os.ReadFile of %s", knownPath)
	}
	if PreloadedCRC["adventure.mid"] != packet.GetCRC(want, 0, len(want)) {
		t.Errorf("PreloadedCRC[\"adventure.mid\"] does not match direct GetCRC")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (no `Preloaded` symbol yet)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/cache/ -v`

Expected: build failure. Errors will be `undefined: Preloaded`, `undefined: PreloadedCRC`, `undefined: PreloadClient` — all in the test file. **No package-level test runs at this point; we want the build to fail.**

- [ ] **Step 3: Write `pkg/cache/preloaded.go`**

Create `pkg/cache/preloaded.go`:

```go
package cache

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// Preloaded maps bare filenames (e.g. "m30_72", "adventure.mid",
// "advance agility.mid") to their raw bytes. Mirrors TS
// PreloadedPacks.ts's `PRELOADED` Map<string, Uint8Array>.
//
// Write-once at world-module startup via PreloadClient; read-many at
// runtime by (*Player).PlaySong / PlayJingle (and future Rebuild*
// consumers — see TS RebuildNormalEncoder.ts:18-19,
// RebuildGetMapsHandler.ts:44,54).
//
// Distinct from CrcTable / CrcBuffer in crctable.go — those are the
// 9-slot JAG archive-CRC table served by the /crc HTTP endpoint; this
// is per-individual-file state for MIDI playback + map/loc streaming.
var Preloaded = map[string][]byte{}

// PreloadedCRC pairs with Preloaded: bare-filename → CRC32/IEEE of the
// raw bytes. Mirrors TS PRELOADED_CRC. Same write/read posture as
// Preloaded above.
var PreloadedCRC = map[string]uint32{}

// PreloadClient walks baseDir/{maps,songs,jingles} and populates
// Preloaded + PreloadedCRC. Mirrors TS preloadClient() at
// PreloadedPacks.ts:8-41.
//
// Error-returning (vs TS's throw-on-failure) so the caller can fail
// the world startingFn cleanly. Eager: all three dirs read
// synchronously before return.
//
// Partial-success leak: if maps/ loads but songs/ fails, Preloaded
// already contains map entries when the error returns. Not retried
// (the services.BasicService lifecycle treats startingFn as one-shot;
// failure halts the service). Documented; acceptable.
func PreloadClient(baseDir string) error {
	for _, sub := range []string{"maps", "songs", "jingles"} {
		dir := filepath.Join(baseDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("preload %s: %w", sub, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("preload read %s: %w", path, err)
			}
			Preloaded[name] = data
			PreloadedCRC[name] = packet.GetCRC(data, 0, len(data))
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/cache/ -v`

Expected: 9 tests pass (8 synthetic + 1 integration that either runs against staged data OR `t.Skip`s if data is missing). Build clean. No vet warnings.

If `TestPreloadClientAgainstStagedDataLoadsAdventure` runs (rather than skips) on your machine, it confirms the relative path `../../data/pack/client` resolves correctly from the package directory (Go test working directory is the package dir).

- [ ] **Step 5: Run full test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green. The new package state (empty `Preloaded` / `PreloadedCRC` maps when no caller has loaded them) does not affect any existing code path because no existing code references these symbols.

- [ ] **Step 6: Commit**

```bash
git add pkg/cache/preloaded.go pkg/cache/preloaded_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(cache): NAI-16 Task 1 — PRELOADED registry port

Adds pkg/cache/preloaded.go with two exported module-level map vars
(Preloaded, PreloadedCRC) and PreloadClient(baseDir string) error.
Walks baseDir/{maps,songs,jingles}, populates both maps keyed by
bare filename with CRC32/IEEE. Mirrors TS PreloadedPacks.ts.

Error-returning (vs TS's throw) so the world startingFn caller can
fail cleanly. Eager: all three dirs read before return. Independent
of the existing CrcTable / CrcBuffer state in crctable.go (those are
the JAG archive-CRC table; these are per-file blob+CRC for MIDI
playback and future Rebuild* consumers).

9 tests in preloaded_test.go: 3-dir walk pin, empty-dir tolerance,
zero-byte file, subdir skip, three missing-dir error pins, key-
collision dir-order pin, and one skip-if-missing integration test
against data/pack/client/songs/adventure.mid.
EOF
)"
```

---

## Task 2: World startup wire-in

**Files:**
- Modify: `modules/world/world.go` (in `NewWorldService`'s `startingFn`)

**Goal:** Call `cache.PreloadClient("data/pack/client")` as the first statement in the world service's startingFn. Failure halts startup before TCP listener goes live.

- [ ] **Step 1: Read the current startingFn body**

Run: `grep -n "startingFn" modules/world/world.go`

Note the line number. Read 30 lines around it to understand the current body shape (which uses `*slog.Logger` and what other init steps exist).

- [ ] **Step 2: Add the preload call as the first statement in startingFn**

Find this region in `modules/world/world.go` (approx. line 81):

```go
	startingFn := func(ctx context.Context) error {
		// ... existing body ...
	}
```

Replace with:

```go
	startingFn := func(ctx context.Context) error {
		if err := cache.PreloadClient("data/pack/client"); err != nil {
			return fmt.Errorf("world: preload client assets: %w", err)
		}
		// ... existing body unchanged ...
	}
```

Add the import at the top of `world.go` if not already present:

```go
	"github.com/zsrv/goscape/pkg/cache"
```

The `fmt` import should already be present; verify with `grep '"fmt"' modules/world/world.go` and add if missing.

- [ ] **Step 3: Run build to verify imports are clean**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean build. No new compile errors.

- [ ] **Step 4: Run vet to verify no warnings**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/`

Expected: clean.

- [ ] **Step 5: Run full test suite**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green. No existing test exercises `world.NewWorldService`'s startingFn end-to-end (per spec § Test strategy 5d, that lifecycle test is flagged as "decide during plan-write" and is skipped here as not worth the lifecycle-plumbing cost).

The pre-existing `TestPreloadClientAgainstStagedDataLoadsAdventure` from Task 1 already covers "preload works against real staged data" — this Task 2 adds no new test, only the production wire-in.

- [ ] **Step 6: Verify the preload runs on a real boot**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build -trimpath -o /tmp/goscape-task2 ./cmd/goscape`

Then a short boot smoke (the user must run this themselves per `smoke_test_server_handoff` memory — Claude's sandboxed `goscape` won't see the host's `data/pack/client/`). Document this in the commit message.

**Implementer:** if you cannot reach `data/pack/client` from the sandbox, simply confirm `go build` succeeds and skip the boot smoke. Note the deferred-smoke status in the commit body.

- [ ] **Step 7: Commit**

```bash
git add modules/world/world.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-16 Task 2 — wire cache.PreloadClient into startingFn

Adds cache.PreloadClient("data/pack/client") as the first statement
in world.NewWorldService's startingFn. Failure propagates out of
startingFn and halts the world service via the standard dskit
lifecycle (services.BasicService → sm.StartAsync → App.serviceFailed).

Hardcoded baseDir "data/pack/client" matches existing cache.MakeCRCs
literal pattern in crctable.go and asset/handler.go. Future config-
driven path is a cross-cutting concern (out of NAI-16 scope).

Boot smoke deferred to user (sandbox cannot reach host data dir per
smoke_test_server_handoff memory).
EOF
)"
```

---

## Task 3: Op constants in `prot.go`

**Files:**
- Modify: `pkg/io/protocol/game/server/prot.go` (append near the end of the existing `var (...)` block)

**Goal:** Register `OpMidiSong = Op{54, -1}` and `OpMidiJingle = Op{212, -2}`. Verified TS source: `ServerGameProt.ts:81-82`.

- [ ] **Step 1: Read current end-of-file structure**

Run: `tail -10 pkg/io/protocol/game/server/prot.go`

Confirm the closing `)` of the `var (...)` block and the existing trailing comment about `OpMessageGame`.

- [ ] **Step 2: Append the two new constants**

Find this block at the end of the existing `var (...)` block:

```go
	// RuneScript S2 — chat output emitted by the MES opcode.
	OpMessageGame = Op{Opcode: 4, PayloadSize: -1}
)
```

Replace with (append immediately above the closing `)`):

```go
	// RuneScript S2 — chat output emitted by the MES opcode.
	OpMessageGame = Op{Opcode: 4, PayloadSize: -1}

	// MIDI client-audio packets (verified against TS ServerGameProt.ts:81-82).
	// MIDI_SONG streams a song reference (name + crc + length so the client
	// can fetch the .mid blob from the asset server); MIDI_JINGLE streams
	// an inline jingle payload. Wired from the MIDI_SONG (2064) / MIDI_JINGLE
	// (2063) script opcodes via (*Player).PlaySong / PlayJingle.
	OpMidiSong   = Op{Opcode: 54, PayloadSize: -1}
	OpMidiJingle = Op{Opcode: 212, PayloadSize: -2}
)
```

- [ ] **Step 3: Run build to verify**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean. The new Op constants have no consumers yet so nothing else changes.

- [ ] **Step 4: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 5: Verify the constants resolve**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go doc github.com/zsrv/goscape/pkg/io/protocol/game/server.OpMidiSong`

Expected: shows `var OpMidiSong = Op{Opcode: 54, PayloadSize: -1}` (or similar formatted output).

- [ ] **Step 6: Commit**

```bash
git add pkg/io/protocol/game/server/prot.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(proto): NAI-16 Task 3 — register OpMidiSong + OpMidiJingle

Adds OpMidiSong (opcode 54, PayloadSize -1) and OpMidiJingle (opcode
212, PayloadSize -2) to pkg/io/protocol/game/server/prot.go. Verified
against TS ServerGameProt.ts:81-82.

Op{54,-1} pairs with TS MidiSongEncoder.ts (pjstr+p4+p4 payload).
Op{212,-2} pairs with TS MidiJingleEncoder.ts (p2+pdata payload).
Both wire into (*Player).PlaySong / PlayJingle in Task 5.
EOF
)"
```

---

## Task 4: MIDI encoders + tests

**Files:**
- Create: `modules/world/midi_encoders.go`
- Create: `modules/world/midi_encoders_test.go`

**TS reference:** `MidiSongEncoder.ts:6-18`, `MidiJingleEncoder.ts:6-17`

**Goal:** Two unexported byte-level encoder helpers, exhaustively tested per `rsbuf_roundtrip_tests` pattern (field-decode + byte-exact pin per encoder). Total: 6 tests across 2 encoders.

- [ ] **Step 1: Write the encoder test file with all 6 tests**

Create `modules/world/midi_encoders_test.go`:

```go
package world

import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestEncodeMidiSongFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 64))
	encodeMidiSong(buf, "adventure", 0xDEADBEEF, 2048)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.GJStrLF(); got != "adventure" {
		t.Errorf("GJStrLF = %q, want \"adventure\"", got)
	}
	if got := r.G4(); got != 0xDEADBEEF {
		t.Errorf("G4 (crc) = 0x%08x, want 0xDEADBEEF", got)
	}
	if got := r.G4(); got != 2048 {
		t.Errorf("G4 (length) = %d, want 2048", got)
	}
	if r.Pos != len(buf.Bytes()) {
		t.Errorf("not all bytes consumed: pos=%d, len=%d", r.Pos, len(buf.Bytes()))
	}
}

func TestEncodeMidiSongBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	encodeMidiSong(buf, "a", 0x01020304, 0x05060708)
	want := []byte{
		0x61,                   // 'a'
		0x0A,                   // PJStrLF terminator
		0x01, 0x02, 0x03, 0x04, // P4(crc)
		0x05, 0x06, 0x07, 0x08, // P4(length)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiSongEmptyNameValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	encodeMidiSong(buf, "", 0, 0)
	want := []byte{0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiJingleFieldsDecodeInClientOrder(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 32))
	data := []byte{0x01, 0x02, 0x03}
	encodeMidiJingle(buf, 500, data)

	r := packet.NewPacket(buf.Bytes())
	r.Pos = 0
	if got := r.G2(); got != 500 {
		t.Errorf("G2 (delay) = %d, want 500", got)
	}
	rest := buf.Bytes()[r.Pos:]
	if !bytes.Equal(rest, data) {
		t.Errorf("data tail mismatch: got %v, want %v", rest, data)
	}
}

func TestEncodeMidiJingleBytesExact(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 16))
	encodeMidiJingle(buf, 0x0102, []byte{0xFF})
	want := []byte{0x01, 0x02, 0xFF}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}

func TestEncodeMidiJingleEmptyDataValid(t *testing.T) {
	buf := packet.NewPacket(make([]byte, 0, 8))
	encodeMidiJingle(buf, 0, []byte{})
	want := []byte{0x00, 0x00}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("bytes mismatch:\n got: % 02x\nwant: % 02x", buf.Bytes(), want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail (no encoder symbols yet)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEncodeMidi -v`

Expected: build failure. `undefined: encodeMidiSong`, `undefined: encodeMidiJingle`.

- [ ] **Step 3: Write `modules/world/midi_encoders.go`**

Create `modules/world/midi_encoders.go`:

```go
package world

import "github.com/zsrv/goscape/pkg/io/packet"

// encodeMidiSong writes a MidiSong payload per TS MidiSongEncoder.ts:
//
//	buf.pjstr(message.name);
//	buf.p4(message.crc);
//	buf.p4(message.length);
//
// Byte-aligned. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
//
// The string terminator is 0x0A (LF) per TS Packet.pjstr at
// io/Packet.ts:330-337 (universal goscape PJStrLF precedent).
func encodeMidiSong(buf *packet.Packet, name string, crc uint32, length uint32) {
	buf.PJStrLF(name)
	buf.P4(crc)
	buf.P4(length)
}

// encodeMidiJingle writes a MidiJingle payload per TS MidiJingleEncoder.ts:
//
//	buf.p2(message.delay);
//	buf.pdata(message.data, 0, message.data.length);
//
// Byte-aligned. Caller wraps in:
//
//	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
//
// goscape's PData(src) takes no offset/length and writes the whole
// slice; TS's pdata(src, 0, src.length) reduces to the same output.
func encodeMidiJingle(buf *packet.Packet, delay uint16, data []byte) {
	buf.P2(delay)
	buf.PData(data)
}
```

- [ ] **Step 4: Run encoder tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestEncodeMidi -v`

Expected: 6 tests pass.

- [ ] **Step 5: Run full module test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/`

Expected: all green. The two new symbols are unexported and have no callers yet, so no existing test path changes.

- [ ] **Step 6: Run vet**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/`

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add modules/world/midi_encoders.go modules/world/midi_encoders_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-16 Task 4 — encodeMidiSong + encodeMidiJingle helpers

Adds modules/world/midi_encoders.go with two unexported byte-level
encoder helpers mirroring TS MidiSongEncoder.ts and MidiJingleEncoder.ts:
  encodeMidiSong(buf, name, crc, length): PJStrLF + P4 + P4
  encodeMidiJingle(buf, delay, data):     P2 + PData

Encoder placement follows goscape convention (modules/world/ owns
byte-packing for per-player wire ops: see player_interface.go,
data_map.go, stat_update.go, message_game.go, inv_stop_transmit.go).

6 tests in midi_encoders_test.go per rsbuf_roundtrip_tests pattern:
field-level decode-in-client-order + byte-exact regression pin per
encoder (3 tests × 2 encoders).
EOF
)"
```

---

## Task 5: Wire PRELOADED + encoders into Player; flip absence-pins

**Files:**
- Modify: `modules/world/player_script.go` (lines ~562-607: replace `PlaySong` and `PlayJingle` bodies + doc-comments)
- Modify: `modules/world/player_script_test.go` (rename 2 tests + rewrite bodies; add 3 new tests; add `seedCachedMidi` helper)

**TS reference:** `Player.ts:1902-1914` (playSong), `:1916-1926` (playJingle).

**Goal:** Replace the two `// deferred (S7h-D1): ...` bodies with PRELOADED lookup + encoder + `p.writeOut`. Flip the two absence-pin tests to positive-pins. Add three miss-path tests covering the silent-no-op guards.

- [ ] **Step 1: Read the existing PlaySong / PlayJingle method bodies**

Run: `sed -n '545,610p' modules/world/player_script.go`

Confirm the current S7h-D1 deferred bodies match the spec's expected pre-state (normalize → empty-check → comment).

- [ ] **Step 2: Read the existing absence-pin tests**

Run: `grep -n 'TestPlay\(Song\|Jingle\)NoWriteOut' modules/world/player_script_test.go`

Confirm the line ranges of `TestPlaySongNoWriteOut` and `TestPlayJingleNoWriteOut`.

- [ ] **Step 3: Write the new test file mutations**

In `modules/world/player_script_test.go`, the imports section likely already imports `testing` and the world package's own internal types. Add an import for `cache` if not present:

```go
import (
	// ... existing imports ...
	"github.com/zsrv/goscape/pkg/cache"
)
```

Locate the `TestPlaySongNoWriteOut` function (use line numbers from Step 2). Replace with the renamed `TestPlaySongWritesOut`:

```go
// seedCachedMidi seeds both cache.Preloaded and cache.PreloadedCRC under
// `name` and registers a t.Cleanup to remove both entries after the test.
// Mirrors the production PreloadClient write shape without touching the
// filesystem. Usable for both song and jingle test paths (PlayJingle
// ignores the CRC entry; the wasted write is harmless).
func seedCachedMidi(t *testing.T, name string, data []byte, crc uint32) {
	t.Helper()
	cache.Preloaded[name] = data
	cache.PreloadedCRC[name] = crc
	t.Cleanup(func() {
		delete(cache.Preloaded, name)
		delete(cache.PreloadedCRC, name)
	})
}

// TestPlaySongWritesOut pins NAI-16's retirement of S7h-D1:
// (*Player).PlaySong now issues a writeOut after the PRELOADED lookup.
// Failure signal = "write-path broken or PRELOADED seeding broken."
// Replaces the prior absence-pin (which was the S7h-D1 escalation
// signal — now satisfied by NAI-16).
func TestPlaySongWritesOut(t *testing.T) {
	seedCachedMidi(t, "adventure.mid", []byte{0x01, 0x02, 0x03}, 0xDEADBEEF)
	p, _ := newTestPlayer(t)
	p.PlaySong("adventure")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlaySong wrote 0 bytes to c.bufw; want >0 (NAI-16 positive pin)")
	}
}

// TestPlaySongMissingFromPreloadedReturnsSilently pins TS's
// `if (song && crc)` guard at Player.ts:1910. PlaySong with a name that
// is not in PRELOADED must be a silent no-op.
func TestPlaySongMissingFromPreloadedReturnsSilently(t *testing.T) {
	// Do NOT seed the cache for "missing.mid".
	p, _ := newTestPlayer(t)
	p.PlaySong("missing")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong with missing PRELOADED key wrote %d bytes; want 0 (silent no-op)", n)
	}
}

// TestPlaySongSongSeededButCRCMissingReturnsSilently pins the `||`
// conjunction in the (*Player).PlaySong guard: both Preloaded AND
// PreloadedCRC must be populated for the write to fire. Defensive
// guard against future test seeding that populates only one map.
func TestPlaySongSongSeededButCRCMissingReturnsSilently(t *testing.T) {
	// Seed Preloaded but not PreloadedCRC.
	cache.Preloaded["orphan.mid"] = []byte{0xAA}
	t.Cleanup(func() {
		delete(cache.Preloaded, "orphan.mid")
	})
	p, _ := newTestPlayer(t)
	p.PlaySong("orphan")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlaySong with PRELOADED-only seed wrote %d bytes; want 0", n)
	}
}
```

Locate `TestPlayJingleNoWriteOut` and replace with:

```go
// TestPlayJingleWritesOut pins NAI-16's retirement of S7h-D1 (jingle side):
// (*Player).PlayJingle now issues a writeOut after the PRELOADED lookup.
func TestPlayJingleWritesOut(t *testing.T) {
	seedCachedMidi(t, "fanfare.mid", []byte{0xAB, 0xCD}, 0)
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "fanfare")
	if n := p.client.bufw.Buffered(); n == 0 {
		t.Errorf("PlayJingle wrote 0 bytes to c.bufw; want >0 (NAI-16 positive pin)")
	}
}

// TestPlayJingleMissingFromPreloadedReturnsSilently pins TS's
// `if (jingle)` guard at Player.ts:1923.
func TestPlayJingleMissingFromPreloadedReturnsSilently(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.PlayJingle(3, "missing")
	if n := p.client.bufw.Buffered(); n != 0 {
		t.Errorf("PlayJingle with missing PRELOADED key wrote %d bytes; want 0 (silent no-op)", n)
	}
}
```

**Note:** the existing `TestPlaySongEmptyNameReturnsSilently`, `TestPlayJingleEmptyNameReturnsSilently`, `TestNormalizeSongName*`, `TestNormalizeJingleName*` tests are **preserved unchanged**. Do not delete or modify them.

- [ ] **Step 4: Run the test mutations to verify they fail (PlaySong/PlayJingle still in S7h-D1 state)**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlaySongWritesOut|TestPlayJingleWritesOut' -v`

Expected: both tests FAIL. The renamed tests exist but `(*Player).PlaySong` / `PlayJingle` still have the S7h-D1 deferred-comment bodies (no writeOut), so `Buffered() == 0` and the new positive-pin assertions trip.

The miss-path tests (`TestPlaySongMissingFromPreloadedReturnsSilently`, etc.) will pass at this point because PlaySong currently writes nothing regardless. After Step 5 they remain passing — by intent (the silent-no-op guard added in Step 5 must preserve this).

- [ ] **Step 5: Update `modules/world/player_script.go` — rewrite PlaySong and PlayJingle bodies + doc-comments**

In `modules/world/player_script.go`, locate `PlaySong` (around line 562) and replace the function (and its docstring) with:

```go
// PlaySong normalizes the song name per TS Player.playSong
// (Engine-TS/src/engine/entity/Player.ts:1902-1914), looks up the
// preloaded blob + CRC, and writes MidiSong to the client. Silent
// no-op on empty name or missing PRELOADED entry (mirrors TS's
// `if (song && crc)` guard at Player.ts:1910).
//
// NAI-16 retires S7h-D1: the PRELOADED lookup and MidiSong write are
// now wired. TestPlaySongWritesOut is the positive-pin; the miss-path
// pins (TestPlaySong*ReturnsSilently) verify the silent-no-op guards.
func (p *Player) PlaySong(name string) {
	name = normalizeSongName(name)
	if name == "" {
		return
	}
	key := name + ".mid"
	song, okSong := cache.Preloaded[key]
	crc, okCRC := cache.PreloadedCRC[key]
	if !okSong || !okCRC {
		return
	}
	buf := packet.NewPacket(make([]byte, 0, 16+len(song)))
	encodeMidiSong(buf, name, crc, uint32(len(song)))
	p.writeOut(gameserver.OpMidiSong, buf.Bytes())
}
```

Locate `PlayJingle` (around line 591) and replace with:

```go
// PlayJingle normalizes the jingle name per TS Player.playJingle
// (Engine-TS/src/engine/entity/Player.ts:1916-1926), looks up the
// preloaded blob, and writes MidiJingle to the client. Silent no-op
// on empty name or missing PRELOADED entry (mirrors TS's `if (jingle)`
// guard at Player.ts:1923).
//
// NAI-16 retires S7h-D1 (jingle side). TestPlayJingleWritesOut pins
// the positive path; TestPlayJingleMissingFromPreloadedReturnsSilently
// pins the silent-no-op guard.
func (p *Player) PlayJingle(delay int, name string) {
	name = normalizeJingleName(name)
	if name == "" {
		return
	}
	jingle, ok := cache.Preloaded[name+".mid"]
	if !ok {
		return
	}
	buf := packet.NewPacket(make([]byte, 0, 2+len(jingle)))
	encodeMidiJingle(buf, uint16(delay), jingle)
	p.writeOut(gameserver.OpMidiJingle, buf.Bytes())
}
```

Note the `_ = delay` stub from the S7h-D1 body is removed (delay is now consumed).

Add the imports at the top of `player_script.go` if not already present:

```go
	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/io/packet"
```

The existing `gameserver` import alias and `strings` import carry over. Run `goimports` or verify manually that the import block is sorted/grouped per Go convention.

- [ ] **Step 6: Run mutated tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'TestPlaySong|TestPlayJingle|TestNormalize' -v`

Expected: all pass:
- `TestPlaySongWritesOut` — passes (writeOut now fires)
- `TestPlayJingleWritesOut` — passes
- `TestPlaySongMissingFromPreloadedReturnsSilently` — passes (silent-no-op guard preserved)
- `TestPlayJingleMissingFromPreloadedReturnsSilently` — passes
- `TestPlaySongSongSeededButCRCMissingReturnsSilently` — passes (`||` guard fires)
- `TestPlaySongEmptyNameReturnsSilently` — passes (preserved)
- `TestPlayJingleEmptyNameReturnsSilently` — passes (preserved)
- `TestNormalizeSongName*` / `TestNormalizeJingleName*` — pass (preserved)

- [ ] **Step 7: Run full whole-module test suite to confirm no regressions**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green. The new player-side wiring uses `cache.Preloaded` / `PreloadedCRC` which are populated only by tests' `seedCachedMidi` and the production `PreloadClient` (called from `world.startingFn`). Existing tests that construct a player without seeding the cache will exercise the silent-no-op guard, which is the same behavior as before NAI-16 (no writeOut).

If any pre-existing test fails, **investigate before proceeding** — likely cause is a test that exercised PlaySong/PlayJingle indirectly (script handler fires) and was relying on the absence-pin semantics. Such a test must be updated to either seed the cache or assert the silent-no-op.

- [ ] **Step 8: Run vet + build**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`

Expected: clean.

- [ ] **Step 9: Verify acceptance gates from spec § Acceptance gates**

Run these verification greps:

```bash
# Gate 4: Op constants registered + writeOut callers wired.
rg -n "OpMidiSong|OpMidiJingle" pkg/io/protocol/game/server/prot.go modules/world/

# Gate 5: cache.Preloaded reads only at intended sites.
rg -n "cache.Preloaded|cache.PreloadedCRC" pkg/ modules/

# Gate 6: PreloadClient called exactly once in production.
rg -n "PreloadClient" modules/ cmd/

# Gate 7: S7h-D1 deferred-comment sites cleared.
rg -n "deferred \(S7h-D1\)" modules/world/

# Gate 8: NoWriteOut renames complete.
rg -n "NoWriteOut" modules/world/
```

Expected:
- Gate 4: 4+ hits (2 declarations in prot.go; 2 writeOut calls in player_script.go)
- Gate 5: writes only in `pkg/cache/preloaded.go`; reads in `modules/world/player_script.go` (4 — two `Preloaded[key]`, one `PreloadedCRC[key]`, one `Preloaded[name+".mid"]`) plus seeds in `player_script_test.go`
- Gate 6: exactly 1 hit (`modules/world/world.go`)
- Gate 7: 0 hits
- Gate 8: 0 hits

If any gate trips, fix the issue before commit.

- [ ] **Step 10: Commit**

```bash
git add modules/world/player_script.go modules/world/player_script_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-16 Task 5 — wire MIDI write path; retire S7h-D1

(*Player).PlaySong and (*Player).PlayJingle now perform PRELOADED
lookup + encoder call + p.writeOut after name normalization. Silent
no-op on missing PRELOADED entry (mirrors TS Player.ts:1908-1913
and :1922-1924 guards).

Test mutations:
- Renames TestPlaySongNoWriteOut → TestPlaySongWritesOut (positive
  pin). Same for jingle.
- Adds TestPlay{Song,Jingle}MissingFromPreloadedReturnsSilently
  pinning the TS `if (song && crc)` / `if (jingle)` guards.
- Adds TestPlaySongSongSeededButCRCMissingReturnsSilently pinning
  the `||` conjunction in the song-side guard.
- New seedCachedMidi(t, name, data, crc) test helper seeds both
  cache.Preloaded and cache.PreloadedCRC under name and registers
  t.Cleanup for both entries.

Retires deviation S7h-D1. Active deviation count: 16 → 15.
EOF
)"
```

---

## Task 6: NAI-16 close

**Files:**
- Modify: `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md` (mark NAI-16 entry resolved)
- Single chore commit summarizing the sub-spec

**Goal:** Update the followups memory entry, retire S7h-D1 from the deviation count, produce the close commit with the standard `chore(script): NAI-16 closed — ...` subject + `Closes memory: nai_followups` trailer per `close_commit_memory_trailer` memory.

- [ ] **Step 1: Final whole-module test sanity**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`

Expected: all green.

- [ ] **Step 2: Final vet sanity**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...`

Expected: clean.

- [ ] **Step 3: Update `nai_followups.md`'s NAI-16 entry**

Open `~/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/nai_followups.md`. Find the "## From S7h (2026-04-24)" section and the "### NAI-16-midi-encoders: MIDI_SONG + MIDI_JINGLE packet write activation" entry.

Add a resolution prefix block at the top of that subsection's body, immediately under the `### NAI-16-midi-encoders: ...` heading:

```markdown
**Resolved 2026-04-24 (NAI-16)** in commits Task 1 (`pkg/cache/preloaded.go`
+ `preloaded_test.go`), Task 2 (world.startingFn wire-in), Task 3 (OpMidiSong
+ OpMidiJingle in `prot.go`), Task 4 (`modules/world/midi_encoders.go` +
tests), Task 5 (`(*Player).PlaySong` / `PlayJingle` rewrites + flipped
absence-pins). S7h-D1 retired. Active deviation count: 16 → 15. See
`docs/superpowers/specs/2026-04-24-nai-16-midi-encoders-design.md` and
`docs/superpowers/plans/2026-04-24-nai-16-midi-encoders.md`.

---

_Original deferral body (preserved for historical context):_
```

(The existing body of the subsection is preserved unchanged below the `_Original..._` line.)

- [ ] **Step 4: Verify commit log shape**

Run: `git log --oneline main -8`

Confirm the five feat commits + one docs(spec) commit + one docs(plan) commit are all present and conventional (no `--amend`, no `--no-verify`, all signed-off only with `--no-gpg-sign` per global CLAUDE.md).

- [ ] **Step 5: Stage memory file + write the close commit**

```bash
git add -A  # picks up the memory edit if your repo tracks the memory dir; otherwise only stages git-tracked files

git commit --no-gpg-sign --allow-empty -m "$(cat <<'EOF'
chore(script): NAI-16 closed — MIDI encoders + PRELOADED registry

Closes the S7h-D1 deviation (MIDI_SONG / MIDI_JINGLE deferred
client-packet writes). (*Player).PlaySong / PlayJingle now perform
PRELOADED lookup + encoder + p.writeOut, mirroring TS Player.playSong
/ playJingle. PRELOADED registry walks data/pack/client/{maps,songs,
jingles} eagerly at world.startingFn. Two new wire opcodes, two
new encoders, two flipped absence-pins, three new miss-path pins.

Active deviation count: 16 → 15.

Crosses three packages (pkg/cache, pkg/io/protocol/game/server,
modules/world). Hidden coupling with future RebuildNormalEncoder /
RebuildGetMapsHandler ports addressed at registry layer (full 3-dir
walk); consumer-side ports are out-of-scope follow-ups.

Closes memory: nai_followups
EOF
)"
```

The `--allow-empty` flag accommodates the case where the memory file lives outside the repo and there are no tracked-file changes for this commit. If the memory file IS tracked, `git add -A` picks it up and `--allow-empty` is harmless.

- [ ] **Step 6: Smoke-test handoff (user-launched per `smoke_test_server_handoff` memory)**

The Java-client smoke runs `[label,music_playbyregion]` against a user-launched server. Claude must NOT start the server in a background bash (sandbox isolation prevents the host Java client from reaching it).

Hand off to the user with:

> "NAI-16 closed at HEAD=`<commit-sha>`. Please run the server yourself with `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml` and exercise `[label,music_playbyregion]`. Expected: client-side audio plays for the regions; no startup error from `cache.PreloadClient`. If `data/pack/client/{maps,songs,jingles}` are missing in your env, startup fails loudly with the wrapped error — that's intentional fail-fast."

---

## Task ordering and review checkpoints

**Subagent-driven execution flow** (per `execution_mode_default` memory):

1. **Task 1** → fresh subagent → at completion, dispatch single review subagent (code-quality + spec-compliance combined since Task 1 is self-contained registry + tests).
2. **Task 2** → fresh subagent → small task; combine review with Task 3.
3. **Task 3** → fresh subagent → quick review (Task 2+3 reviewed together).
4. **Task 4** → fresh subagent → review per `rsbuf_roundtrip_tests` shape.
5. **Task 5** → fresh subagent → most complex; full per-task review.
6. **Task 6** (close) → main session (no subagent); apply close commit + memory update.

**Final two-stage review** (per user's session-prompt cadence requirement):

After Task 5 commits but before Task 6 close:

- **Stage 1: Code-quality review** — `superpowers:requesting-code-review` or `feature-dev:code-reviewer` agent. Surface DRY/YAGNI/style issues across all five tasks' diffs.
- **Stage 2: Whole-impl fidelity review** — separate agent. Verify every spec requirement has a code witness (per `plan_test_coverage_crosscheck`); verify TS source-line refs are accurate; verify deviation count truly went 16 → 15 (no silent new deviations).

---

## Self-review (this section is for the plan author, not the implementer)

**Spec coverage crosscheck:**

| Spec requirement | Plan task |
|---|---|
| § Scope (C) bullet 1 — `pkg/cache/preloaded.go` registry | Task 1 |
| § Scope bullet 2 — `cache.PreloadClient` wired in `world.startingFn` | Task 2 |
| § Scope bullet 3 — `OpMidiSong` / `OpMidiJingle` in prot.go | Task 3 |
| § Scope bullet 4 — `modules/world/midi_encoders.go` | Task 4 |
| § Scope bullet 5 — wire write path into `(*Player).PlaySong`/`PlayJingle` | Task 5 |
| § Scope bullet 6 — `pkg/cache/preloaded_test.go` hybrid fixture | Task 1 (Step 1) |
| § Scope bullet 7 — `midi_encoders_test.go` rsbuf_roundtrip_tests | Task 4 (Step 1) |
| § Scope bullet 8 — flip 2 absence-pins; add 3 miss-path tests | Task 5 (Step 3) |
| § Scope bullet 9 — retire S7h-D1; count 16 → 15 | Task 6 |
| All 20 tests in spec § Test strategy | Tasks 1, 4, 5 (counted: 9+6+5=20 ✓) |
| § Acceptance gates 1-10 | Task 5 Step 9 + Task 6 Steps 1-2 |

No gaps.

**Placeholder scan:** zero "TBD" / "TODO" / "implement later" / "fill in details" / "similar to Task N" / "add appropriate error handling" / "write tests for the above" patterns. Every step has either exact commands or complete code blocks.

**Type consistency:**
- `Preloaded` (Task 1) → `cache.Preloaded` (Task 5) → consistent.
- `PreloadedCRC` (Task 1) → `cache.PreloadedCRC` (Task 5) → consistent.
- `PreloadClient(baseDir string) error` (Task 1) → called as `cache.PreloadClient("data/pack/client")` (Task 2) → signature and arg match.
- `OpMidiSong`, `OpMidiJingle` (Task 3) → `gameserver.OpMidiSong`, `gameserver.OpMidiJingle` (Task 5) → naming and aliasing consistent.
- `encodeMidiSong(buf, name, crc, length)` (Task 4) → called as `encodeMidiSong(buf, name, crc, uint32(len(song)))` (Task 5) → arg types match (`uint32`/`uint32`).
- `encodeMidiJingle(buf, delay, data)` with `delay uint16` (Task 4) → called as `encodeMidiJingle(buf, uint16(delay), jingle)` (Task 5) → cast at call site, types align.
- `seedCachedMidi(t, name, data, crc)` (Task 5) → used by `TestPlaySongWritesOut` and `TestPlayJingleWritesOut` → consistent signature.

**Test helper coverage** (per `plan_helper_coverage` memory):
- `seedCachedMidi`: consumers are `TestPlaySongWritesOut` (Task 5) + `TestPlayJingleWritesOut` (Task 5). Both seed all four args; Cleanup logic deletes both maps. Verified.
- `resetPreloadedForTest`: consumers are all 9 tests in `preloaded_test.go`; each calls it as the first statement. Cleanup logic clears both maps. Verified.

**Spec § Acceptance gates** map directly to Task 5 Step 9's verification commands. No gate left unchecked.

No issues found in self-review.
