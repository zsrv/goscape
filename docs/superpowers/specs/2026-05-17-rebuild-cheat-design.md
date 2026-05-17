# `::rebuild` cheat — in-process PackAll + Reload

**Date:** 2026-05-17
**Tech stack:** Go 1.26+ per `[[go_version]]`.
**Cadence:** Compressed per `[[compressed_cadence]]` — ~100 LOC. One spec doc, no formal sub-spec review cycle; direct TDD.
**Execution mode:** in-line TDD (compressed).
**TS canonical source:** `Engine-TS/src/network/game/client/handler/ClientCheatHandler.ts` L151-153 dispatch + `Engine-TS/src/engine/World.ts` L1813-1819 (`rebuild()`) + `Engine-TS/src/cache/DevThread.ts` L92-98 (worker-thread `processChangedFiles`).

---

## §1. Scope

Wire the `::rebuild` staff cheat in `modules/world/handlers_game.go` to invoke `packall.PackAll` in-process against a configured content directory, then call `s.Reload(true)` to load the freshly-packed data. Synchronous on the tick goroutine; mirrors `::reload`'s execution posture.

**Why this matters now.** `[[nai212_close]]` flagged this as a deferred follow-up. Today a developer modifying script source has to: stop the server, run `goscape-cli pack` (or PackAll equivalent) against `Content`, restart. With `::rebuild`, the developer can: edit a script, type `::rebuild` in the running game, and see the change without a restart cycle.

**In scope:**
- New config field `ContentPath` (CLI flag `--world.content-path`, yaml `content_path`).
- New `rebuild` case in the dev-block dispatch of `handleClientCheat`.
- Synchronous PackAll → Reload pipeline on the tick goroutine.
- Player UX: "Rebuilding scripts..." on start, "Rebuilt: <elapsed>." on success, "Rebuild failed: …" on error (private MessageGame).
- Failure isolation: a failing PackAll does not call Reload; CachePath is left in whatever partial state PackAll wrote (matches TS half-swap posture).
- Unit tests mirroring the `::reload` cheat test patterns.

**Out of scope:**
- Async / worker-goroutine execution. TS uses a `Worker` thread (DevThread); goscape ports the synchronous semantics first. The tick blocks for the duration of pack + reload (~7s on real Content), matching TS's blocking-main-thread reload pattern. An async variant is deferred — would require a coordination channel + tick-loop integration.
- File-watch auto-rebuild (TS `trackDir` watching `${BUILD_SRC_DIR}/maps` etc.). Manual `::rebuild` is the MVP trigger.
- The `::reload` ↔ `::rebuild` dependency direction. `::rebuild` calls Reload itself; `::reload` is unchanged.
- Friend-server `RELAY_RELOAD` integration. Reload's existing `clearInvs` parameter is forwarded as `true` (cheat default).
- New deviation tags. The design is straightforward sync-port; no semantic deviations from the TS pipeline that warrant a permanent tag.

---

## §2. Layout

| File | Purpose |
|---|---|
| `modules/world/config.go` (MODIFIED) | Add `ContentPath string` field + flag/yaml binding. Default `""` (cheat returns an error if invoked with no content path). |
| `modules/world/handlers_game.go` (MODIFIED) | New `case "rebuild":` arm in `handleClientCheat`'s dev-block switch (next to `case "reload":`). |
| `modules/world/handlers_game_test.go` or new `modules/world/handler_cheats_rebuild_test.go` (NEW) | Three tests: happy path, missing ContentPath, PackAll failure. |

No changes to `pkg/packall/`, `pkg/pack/`, or `modules/asset/`.

---

## §3. Invocation surface

Cheat command (in-game): `::rebuild` (no arguments).

Config flag: `--world.content-path=<dir>`. Default `""`. When empty, `::rebuild` returns a private error message and does not invoke PackAll.

---

## §4. Architecture

### §4.1 Handler arm

In `handleClientCheat`'s developer-block switch (gated by `!cfg.NodeProduction && staffModLevel >= 4`, the same block that hosts `reload`), add:

```go
case "rebuild":
    // TS ClientCheatHandler.ts:151-153. Mirrors the synchronous
    // dispatch pattern of "reload" above. Block the tick goroutine
    // for the duration of the pack + reload (matches TS's blocking
    // main-thread posture).
    if p.client.server.cfg.ContentPath == "" {
        p.MessageGame("Rebuild failed: --world.content-path is not configured.")
        return nil
    }
    p.MessageGame("Rebuilding scripts...")
    start := time.Now()
    cachePath := p.client.server.cfg.CachePath
    if err := packall.PackAll(p.client.server.cfg.ContentPath, cachePath, cachePath); err != nil {
        p.client.server.log.Error("rebuild cheat: PackAll failed", "err", err)
        p.MessageGame("Rebuild failed: " + err.Error())
        return nil
    }
    if err := p.client.server.Reload(true); err != nil {
        p.client.server.log.Error("rebuild cheat: Reload failed", "err", err)
        p.MessageGame("Rebuild failed: reload returned error (see server log).")
        return nil
    }
    p.MessageGame(fmt.Sprintf("Rebuilt: %s.", time.Since(start).Round(time.Millisecond)))
    return nil
```

### §4.2 Staff gate

The existing dev-block switch is gated on `!NodeProduction && staffModLevel >= 4` (developer block — see `handlers_game.go` around the `reload` case). `::rebuild` lives inside the same gate; no separate access check. This also means production servers cannot use `::rebuild` even with staff level 4 — same posture as `::reload`.

### §4.3 Retire the NAI-190-D5 carryforward

`handlers_game.go` carries a block comment `DEVIATION-NAI-190-D5-CARRYFORWARD` that lists `rebuild` as the last unported cheat. Update that comment to mark `rebuild` ported, and either retire the carryforward entirely (if no other items remain) or shrink its body.

### §4.4 Path arguments to PackAll

`packall.PackAll(srcDir, outDir, dataPackDir)`:
- `srcDir` = `s.cfg.ContentPath`
- `outDir` = `s.cfg.CachePath`
- `dataPackDir` = `s.cfg.CachePath`

Setting `outDir == dataPackDir == CachePath` means PackAll writes server-side files (server/*.dat) AND the entity-type cache that `Reload()` immediately reads. RunServerCompiler internally reads entity types from `dataPackDir`, which after our prior stages have written equals the (possibly stale) CachePath; this is acceptable because PackConfigs (stage 1) re-writes server-side configs before RunServerCompiler (stage 3) reads them.

### §4.5 Failure semantics

PackAll error → Reload is NOT called → CachePath is left in whatever partial state PackAll wrote. Mirrors `NAI-190-D2-HALF-SWAP` philosophy: don't roll back, surface the error, operator decides next step. Player gets a private "Rebuild failed: …" message; server log gets the full error.

Reload error after successful PackAll → CachePath is fresh but in-memory state is partially swapped per existing `NAI-190-D2-HALF-SWAP` semantics. Player gets a private "Rebuild failed: reload returned error" message.

### §4.6 Import surface

`modules/world` newly imports `github.com/zsrv/goscape/pkg/packall`. `packall` already imports every per-stage pack subpackage, so `modules/world` transitively depends on them after this change. This bloats the binary by the pack-time code paths (sprites/audio/maps/etc.), which is acceptable — these were already in scope when the operator decided to run a content-rebuild-capable server. No import cycle: `packall` does not import `modules/world`.

---

## §5. Test strategy

Three tests in `handler_cheats_rebuild_test.go` mirroring the `::reload` cheat test triplet (`TestHandleClientCheat_Reload_*`):

| Test | What it pins |
|---|---|
| `TestHandleClientCheat_Rebuild_Dispatches` | Happy path: with `ContentPath=<fake fixture>`, `CachePath=<tmp>`, staff level ≥ 2, `::rebuild` ends with a private `Rebuilt: <duration>.` MessageGame and Reload is invoked. Use `seedMinimalPackFixture` (from cmd_pack_test.go in goscape-cli — we'll need a copy or shared helper) so PackAll succeeds. |
| `TestHandleClientCheat_Rebuild_NoContentPath_PrivateErrorMessage` | With `ContentPath=""`, `::rebuild` sends "Rebuild failed: --world.content-path is not configured." and never calls PackAll. |
| `TestHandleClientCheat_Rebuild_PackAllFailure_PrivateErrorMessage` | With `ContentPath=<empty tmp dir>` (PackAll fails immediately on missing `pack/obj.pack`), `::rebuild` sends "Rebuild failed: …" and does not call Reload. |

The fake-fixture helper duplicates `seedMinimalPackFixture` from `cmd/goscape-cli/cmd_pack_test.go`; copy-paste rather than introducing a shared test fixture package (YAGNI; one-off use).

Each test uses the existing `dispatchCheat` helper, `drainConn` for private-message capture, and `io2.New(...)` for the encryptor seed.

---

## §6. Open questions / deferrals

- **Async variant.** Not in this spec. Listed as the natural next follow-up. Trigger: developer complaints about tick-freeze during rebuild.
- **File-watch auto-rebuild.** Not in this spec. TS uses `fs.watch`; goscape would need `github.com/fsnotify/fsnotify`. Trigger: developer demand.
- **Persistent player state across rebuild.** Reload's existing semantics (clearInvs=true) wipe inv state. This is TS-faithful for `::rebuild` (which calls TS-equivalent `World.rebuild` → dev_reload → World.reload).

---

## §7. Acceptance

- `go test ./modules/world/...` passes.
- New `--world.content-path` flag visible in `goscape --help`.
- Manual smoke (operator):
  1. `goscape --target=world --world.cache-path=/some/dir --world.content-path=/path/to/Content`.
  2. Log in as staff (modlvl ≥ 2). Modify a script in Content/scripts/. Type `::rebuild`. Expect "Rebuilding scripts..." then "Rebuilt: Ns." messages. Reload runs; new script is live.
