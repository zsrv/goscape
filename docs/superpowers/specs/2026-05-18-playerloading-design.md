# PlayerLoading Port — SAV Codec + RPC Wire-up

**Scope:** Port the Engine-TS `PlayerLoading` codec (decode + verify) and
`Player.save()` (encode) to goscape's world module, and wire the codec
into the existing gRPC `LoginService` plumbing so that player state
flows end-to-end between the (out-of-process) login server and the
in-process `Player` struct.

**Source:** `LostCityRS/Engine-TS/src/engine/entity/PlayerLoading.ts`
(160 LOC) and `Player.ts:190-270` (the `save()` method, ~80 LOC).

**Out of scope:** The TS-port of the login server itself
(`LoginServer.ts` / `LoginThread.ts`), filesystem reads/writes,
hiscore export, `wouldResetSaveFile` guard, batch migration of
historical SAV files, reconnect `HasSave` optimization.

---

## Why now

The gRPC `LoginService` and `LoginClient` are already wired into world
startup; `PlayerLogin` returns an optional `save: bytes`, and
`PlayerAutosave`/`PlayerLogout`/`PlayerForceLogout` accept `save: bytes`
on the request side. None of the codec exists yet, so the `save` field
is dropped on receive and never produced on send. The goscape world
currently bootstraps every player from hardcoded defaults at
`tick.go:157` with an explicit "save-file load + restore is a future
sub-spec" doc-comment. This slice closes that loop.

---

## Architecture

```
Login server (existing, OOP)            World side (this slice)
┌──────────────────────┐               ┌──────────────────────┐
│ data/players/*.sav   │               │ Player struct        │
│        ↑↓             │   gRPC bytes  │        ↑↓             │
│ fs read/write        │ ◄───────────► │ player_load.go       │
│                      │               │ player_save.go       │
└──────────────────────┘               └──────────────────────┘
```

The codec is the only new logic. Three wiring touchpoints close the
loop:

1. **Login decode** — after `PlayerLogin` returns an accepted result,
   if `resp.Save` is present and `VerifySave`s, decode bytes into the
   freshly-built `Player`; otherwise apply empty-save bootstrap.
2. **Autosave** — every 1500 ticks (TS `PLAYER_SAVERATE`), iterate
   `s.playerLoop`, call `loginClient.PlayerAutosave` with each
   player's `p.Save()` bytes.
3. **Logout** — in `removePlayer(p)`, call `PlayerLogout` with
   `p.Save()` bytes before clearing.

---

## Components

### Files

```
modules/world/
├── player_load.go      ← VerifySave + LoadSave (decode + empty-save bootstrap)
├── player_save.go      ← (*Player).Save() — encode current Player to SAV bytes
├── player_save_test.go ← byte-pin tests against v1..v6 fixtures
├── testdata/playerloading/
│   ├── v1.sav          ← TS-generated fixture (pre-playtime-overflow-fix)
│   ├── v2.sav          ← v1 + 4-byte playtime
│   ├── v3.sav          ← v2 + afkZones + lastAfkZone
│   ├── v4.sav          ← v3 + packed chat modes
│   ├── v5.sav          ← v4 + per-inv size field
│   ├── v6.sav          ← v5 + lastLoginTime (current)
│   ├── empty.sav       ← zero-byte (bootstrap path; can be a sentinel,
│   │                    not actually read from disk)
│   └── README.md       ← tsx generator script source + invocation
```

### Public API (package `world`)

```go
const (
    SavMagic   uint16 = 0x2004
    SavVersion uint16 = 6
)

// VerifySave reports whether sav has a valid magic, a supported
// version, and a matching trailing CRC. Mirrors PlayerLoading.verify
// (PlayerLoading.ts:16-29).
func VerifySave(sav []byte) bool

// LoadSave populates p from sav. If len(sav) < 2 it applies the
// empty-save bootstrap (21 stats=0, baseLevels=1, levels=1; hitpoints
// at level 10 with matching XP). Mirrors PlayerLoading.load
// (PlayerLoading.ts:31-159). Returns an error on magic mismatch,
// unsupported version, or CRC mismatch.
func LoadSave(p *Player, sav []byte) error

// Save serializes p to a fresh SAV byte slice at the current version.
// Inventories iterate over typeIds in ascending order (deviation
// NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID). Mirrors Player.save()
// (Player.ts:190-270).
func (p *Player) Save() []byte
```

### Sentinel errors

```go
var (
    ErrSavInvalidMagic   = errors.New("playerloading: invalid save magic")
    ErrSavUnsupportedVer = errors.New("playerloading: unsupported save version")
    ErrSavCorrupt        = errors.New("playerloading: incorrect save checksum")
)
```

### Internal helpers

- All three operate over `*packet.Packet` from `pkg/io/packet/`.
  `LoadSave` builds a Packet from `sav` and reads via `G1/G2/G4/G8`;
  `Save` writes via `P1/P2/P4/P8`.
- TS `g4s` (signed 32-bit) → `int32(p.G4())` at read site;
  `p.G4()` returns `uint32`.
- TS `g8` → already `uint64`; cast to `int64` for `lastLoginTime`.
- `packet.GetCRC` reused (it's `crc32.ChecksumIEEE`).

### Touched non-codec files

| File | Change |
|---|---|
| `modules/world/tick.go` | Remove the empty-save bootstrap at line 157 (it moves into `LoadSave`). Add autosave call at top of tick for-body, gated on `tick % PlayerSaveRate == 0 && tick > 0`. |
| `modules/world/server.go` | Capture `resp.GetSave()` post-`PlayerLogin` (around line 729); call `LoadSave(p, savePayload)` after `newPlayer` and before `addPlayer`. Split `removePlayer(p)` (line 835) into `removePlayerOnTick` (calls `PlayerLogout` with `p.Save()`) and `removePlayerOnDisconnect` (calls `PlayerForceLogout` — no save bytes, ungraceful). Update both call sites (`tick.go:305` and `server.go:545`). |

---

## Data flow

### Login decode

Today in `server.go` ~line 695, `PlayerLogin` is called; only
`result/staffModLevel/members/username` are extracted from the
response, and `resp.Save` is dropped. After this change the accept-path
captures `resp.GetSave()` on the client struct:

```go
if result == OK || result == NEW_PLAYER || result == RECONNECT_OK {
    c.staffModLevel = resp.GetStaffModLevel()
    c.members       = resp.GetMembers()
    c.username      = safeName
    c.savePayload   = resp.GetSave()   // NEW — bytes or nil
}
```

When the Player struct is later constructed via `newPlayer(c)`, the
caller immediately decodes:

```go
p := newPlayer(c)
if err := LoadSave(p, c.savePayload); err != nil {
    c.log.Warn("LoadSave failed, falling back to empty bootstrap",
        slog.String("username", c.username), slog.Any("err", err))
    _ = LoadSave(p, nil) // bootstrap via empty-save path
}
```

The empty-save bootstrap moves out of `tick.go:157` and becomes fully
owned by `LoadSave`. Single entry point, mirrors TS
`PlayerLoading.load`.

### Autosave

In `tick.go`, top of for-body:

```go
if s.tick%PlayerSaveRate == 0 && s.tick > 0 {
    s.autosavePlayers()
}
```

```go
const PlayerSaveRate = 1500 // ~15 min at 600ms ticks; matches TS World.PLAYER_SAVERATE
```

`autosavePlayers` iterates `s.playerLoop`, builds a
`PlayerAutosaveRequest` per player, fires non-blocking RPCs:

```go
func (s *Server) autosavePlayers() {
    if s.loginClient == nil {
        return
    }
    for _, p := range s.playerLoop {
        if p.username == "" {
            continue
        }
        save := p.Save()
        req := &loginpb.PlayerAutosaveRequest{
            Profile: s.cfg.NodeProfile,
            Username: p.username,
            Save: save,
        }
        go s.loginClient.PlayerAutosave(context.Background(), req)
    }
}
```

`LoginClient.PlayerAutosave` is already best-effort with a warn-log on
RPC failure (no error returned to caller).

### Logout

`removePlayer(p)` is called from two contexts:

| Site | Goroutine | Trigger |
|---|---|---|
| `tick.go:305` | Tick goroutine | Graceful logout (`p.LoggingOut()`) |
| `server.go:545` | Per-conn goroutine | Ungraceful disconnect (defer-on-conn-close) |

`p.Save()` reads ~30 Player fields. The tick goroutine owns those
fields and mutates them every tick. **Calling `Save()` from the
per-conn goroutine would race** with tick.

Resolution: `removePlayer` gains a `caller` parameter (or two thin
wrappers `removePlayerOnTick(p)` and `removePlayerOnDisconnect(p)`).
The tick-goroutine variant runs the logout RPC with `p.Save()` bytes;
the disconnect variant calls `PlayerForceLogout` instead (no save
bytes, ungraceful-loss-of-state).

Tick-goroutine variant:

```go
func (s *Server) removePlayerOnTick(p *Player) {
    if s.loginClient != nil && p.username != "" {
        save := p.Save()  // safe: tick goroutine owns p
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
            defer cancel()
            _, err := s.loginClient.PlayerLogout(ctx, &loginpb.PlayerLogoutRequest{
                NodeId:   int32(s.cfg.NodeID),
                Profile:  s.cfg.NodeProfile,
                Username: p.username,
                Save:     save,
            })
            if err != nil {
                s.log.Warn("PlayerLogout RPC failed",
                    slog.String("username", p.username), slog.Any("err", err))
            }
        }()
    }
    s.removePlayerInternal(p)
}
```

Disconnect variant calls the existing `LoginClient.PlayerForceLogout`
(already best-effort with internal warn-log) — last-segment state
since the most recent autosave is lost.

Per-conn-goroutine variant:

```go
func (s *Server) removePlayerOnDisconnect(p *Player) {
    if s.loginClient != nil && p.username != "" {
        go s.loginClient.PlayerForceLogout(context.Background(),
            &loginpb.PlayerForceLogoutRequest{
                NodeId:   int32(s.cfg.NodeID),
                Profile:  s.cfg.NodeProfile,
                Username: p.username,
            })
    }
    s.removePlayerInternal(p)
}
```

`removePlayerInternal` is the existing slot/zone cleanup body
(unchanged). The two call sites — `tick.go:305` and `server.go:545` —
update to call the appropriate variant. This makes the goroutine
ownership explicit at the call site.

The save-RPC goroutine fires before slot/zone cleanup so a slow
login-server RPC doesn't block tick progression. The `5s` timeout is
conservative; no `PlayerForceLogout` fallback after `PlayerLogout`
failure on the tick path (see deviation tag below).

### Reconnect

`PlayerLoginRequest.HasSave` stays hardcoded `false`. The reconnect
optimization (skip resending save bytes if world already holds them)
is a future slice — tagged.

---

## Error handling

### Decode-path errors

| Condition | TS | Go |
|---|---|---|
| `len(sav) < 2` | bootstrap, return ok | bootstrap, return nil |
| magic mismatch | throw 'Invalid save file' | return `ErrSavInvalidMagic` |
| version > 6 | throw 'Unsupported save version' | return `ErrSavUnsupportedVer` |
| CRC mismatch | throw 'Incorrect save checksum' | return `ErrSavCorrupt` |
| version == 0 | TS would attempt decode and likely OOB-read | reject (defensive); treat as unsupported |

Call-site policy in `server.go` post-login: **log + fall back to
empty bootstrap**, do not reject the login (see deviation
`NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP`).

### Encode-path errors

`Save()` cannot fail. Returns `[]byte` (not `([]byte, error)`).
Matches TS `save(): Uint8Array`.

### Out-of-range and sentinel handling

- Body `255` on read → `-1` (TS PlayerLoading.ts:76-78): port verbatim.
- Inv slot `id - 1` on read; drop if resulting `id == -1`: port verbatim.
- Count `255` on read → read extended `int32`: port verbatim.

### RPC-failure on encode side

- `PlayerAutosave` — already best-effort, warn-log on failure.
- `PlayerLogout` — fire-and-forget goroutine; warn-log on failure. No
  `PlayerForceLogout` fallback (see deviation tag).

### CRC signedness note

TS reads CRC as `g4s` (signed int32) and compares against
`Packet.getcrc(...)`. Goscape stores/compares as `uint32` and reads
into `int32` only for the signed-compare site. Bit pattern is
identical regardless of read-side signedness, so wire bytes match TS.
The implementation plan should pin this with a CRC fixture whose high
bit is set (negative-int32 when interpreted signed).

---

## Testing

### Byte-pin fixture tests (primary correctness gate)

For each `vN.sav`, `N ∈ {1..6}`:

```go
func TestLoadSave_V{N}_DecodesAllFields(t *testing.T) {
    raw := mustReadFile(t, "testdata/playerloading/v{N}.sav")
    p := newTestPlayer()
    if err := LoadSave(p, raw); err != nil { t.Fatal(err) }

    // Assert every field the version is supposed to persist
    if p.x != <expected> { t.Errorf(...) }
    // ... through afkZones[v3+], chat modes[v4+], lastLoginTime[v6+]
}
```

For v6 only (the version we encode):

```go
func TestSave_V6_RoundTripsBytePerfect(t *testing.T) {
    raw := mustReadFile(t, "testdata/playerloading/v6.sav")
    p := newTestPlayer()
    if err := LoadSave(p, raw); err != nil { t.Fatal(err) }
    got := p.Save()
    if !bytes.Equal(got, raw) {
        t.Errorf("Save() drift:\n got=%x\nwant=%x\nfirst-diff-at=%d",
            got, raw, firstDiff(got, raw))
    }
}
```

This is the critical byte-identity check vs TS-produced fixtures.
Locks the encoder to TS bit-for-bit.

### Negative tests

```
TestVerifySave_RejectsBadMagic           — flip byte 0, expect false
TestVerifySave_RejectsUnsupportedVer     — set version to 7, expect false
TestVerifySave_RejectsCorruptCRC         — flip a payload byte, expect false
TestLoadSave_EmptyByteSliceBootstraps    — sav=[], hitpoints==getExpByLevel(10)
TestLoadSave_NilSliceBootstraps          — sav=nil, equivalent to empty
TestLoadSave_BadMagicReturnsErr          — ErrSavInvalidMagic
TestLoadSave_VersionTooHigh_Err          — ErrSavUnsupportedVer
TestLoadSave_VersionZero_Err             — ErrSavUnsupportedVer (defensive)
TestLoadSave_CRCMismatch_Err             — ErrSavCorrupt
```

### Inv-ordering deviation pin

```
TestSave_InvsWrittenInTypeIDAscOrder — load player with invs added in
                                       non-sorted insertion order,
                                       Save(), parse out the inv
                                       section, assert typeIds appear
                                       ascending. Pins
                                       NAI-PLAYERLOADING-D-INVS-
                                       SORTED-BY-TYPEID.
```

### CRC high-bit pin

```
TestSave_CRCHighBitSet_RoundTrips — construct a Player whose
                                    serialized form yields a CRC with
                                    the high bit set; assert
                                    LoadSave(Save(p)) decodes p back
                                    successfully.
```

### Wire-up integration tests

```
TestLoginAcceptedWithSave_DecodesIntoPlayer
    — fake LoginClient returns OK + v6 SAV bytes; assert Player fields
      populated from save (not defaults).

TestLoginAcceptedWithoutSave_BootstrapsDefaults
    — fake returns OK + Save=nil; assert empty-bootstrap fields.

TestLoginAcceptedWithCorruptSave_FallsBackToBootstrap
    — fake returns OK + truncated SAV; assert defaults applied +
      warn-log emitted.

TestRemovePlayerOnTick_CallsLogoutWithSaveBytes
    — fake LoginClient captures the PlayerLogout call; assert Save
      bytes round-trip-load to the same Player state.

TestRemovePlayerOnDisconnect_CallsForceLogoutOnly
    — fake LoginClient captures calls; assert PlayerForceLogout was
      called and PlayerLogout was NOT called. Pins
      NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE.

TestTickAutosave_FiresAtRateMultiples
    — drive tick counter through 0..4500, assert PlayerAutosave called
      exactly at ticks 1500, 3000, 4500 (and not at tick 0).
```

### Existing test impact

`tick_test.go` empty-save bootstrap tests, if any, move to
`player_load_test.go` and re-target `LoadSave(p, nil)` rather than the
old in-tick init path.

---

## Fixture generation

`testdata/playerloading/README.md` documents the tsx script (committed
to Engine-TS tree, not goscape). The script:

1. Temporarily clamps `PlayerLoading.SAV_VERSION` to N for N ∈ 1..6.
2. For each version, comments out the version-gated `save()` blocks
   above N (e.g., for v1 skips the v2+ playtime-as-int32, the v3+
   afkZones, etc.).
3. Sorts `Player.invs` Map by typeId before serialization (matches
   Go's sort order — required for byte parity on the inv section).
4. Constructs a populated Player with deterministic values (fixed
   values per field, documented in the README), calls `save()`,
   writes bytes to `vN.sav`.
5. Restores `SAV_VERSION` and `save()` body to HEAD state.

The README enumerates the exact field values used so the fixtures are
reproducible from a fresh checkout.

---

## Deviation tags

| Tag | Where | Rationale |
|---|---|---|
| `NAI-PLAYERLOADING-D-INVS-SORTED-BY-TYPEID` | `(*Player).Save()` inv loop | TS iterates `this.invs` in Map insertion-order, which depends on which `getInventory` calls fired and in what order across the session. Non-portable. Goscape sorts ascending by typeId for deterministic, portable bytes. The TS fixture generator script sorts identically so fixture parity holds. Cross-ref: `[[smoke_pack_worldmap_stage_wiring]]`, `[[canonical_rs_worldmap_samples]]`. |
| `NAI-PLAYERLOADING-D-DECODE-ERR-FALLS-BACK-TO-BOOTSTRAP` | server.go post-login | TS LoginServer pre-verifies SAV before sending to world. Goscape doesn't yet (login-server-side port is a future sub-project), so corrupt SAV bytes log + fall back to empty bootstrap rather than rejecting login. Revisit when goscape login server gains a pre-verify pass. |
| `NAI-PLAYERLOADING-D-LOGOUT-NO-FORCE-FALLBACK` | `removePlayerOnTick` | TS does not call `LoginThread.forceLogout` if logout-save RPC fails. Goscape mirrors: `PlayerLogout` failure logs only, no `PlayerForceLogout` belt-and-braces. |
| `NAI-PLAYERLOADING-D-DISCONNECT-NO-SAVE` | `removePlayerOnDisconnect` | Per-conn-goroutine disconnect-cleanup calls `PlayerForceLogout` (no save bytes) rather than `PlayerLogout` (with save bytes), because `p.Save()` must run on the tick goroutine for thread-safety. Last-segment state since the most recent autosave is lost on ungraceful disconnect. Mitigation: 15-min autosave cadence caps the loss window. TS has the same window (its disconnect path also doesn't save). |
| `NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET` | tick.go autosave loop | TS posts to a worker thread (`LoginThread.postMessage` — async, no error). Goscape uses goroutine + best-effort RPC, no per-call back-pressure. If autosaves consistently fail for one player, no automatic remediation. |
| `NAI-PLAYERLOADING-D-HAS-SAVE-ALWAYS-FALSE` | server.go login req | `HasSave` field on `PlayerLoginRequest` is hardcoded false. The reconnect-flow optimization (skip resending bytes if world already holds them) is deferred. Future slice. |

Tags are pinned via inline comments and referenced in commit close
messages. None require runtime assertion tests (per project
convention).

---

## Slice size estimate

| Component | LOC |
|---|---|
| `player_load.go` (Verify, LoadSave, empty bootstrap, v1..v6 ladder) | ~140 |
| `player_save.go` (Save: encode v6) | ~100 |
| Wiring in `server.go` + `tick.go` | ~50 |
| `player_save_test.go` (6 decode pins + round-trip + 9 negatives + 1 inv-order + 1 CRC-high-bit + 5 wire-up) | ~250 |
| Fixtures (6 + 1 README) | ~50 (mostly binary) |
| **Total** | **~590** |

Larger than the original ~400 estimate once the integration-test
surface is counted, but still a single coherent slice.

---

## Out-of-scope follow-ups

- Goscape-side LoginServer port (TS `LoginServer.ts` /
  `LoginThread.ts` — fs SAV reads/writes, hiscore export, ban table).
- `wouldResetSaveFile` server-side guard.
- Reconnect `HasSave` optimization.
- Historical SAV migration tool (today: player auto-migrates v1..v5 to
  v6 on next login via load→save cycle).
- Hiscore export.
