# SEC1 — Security hardening batch 1 (M-1, M-2, M-7, M-8, M-12)

Source: goscape security review 2026-08-20 (rev-274 @ 60db51ba). This batch
addresses five Medium findings the user selected. Criticals/Highs are
deliberately out of scope here and remain open.

**rev-225 back-port note.** This branch carries the same batch (commits
`6208ff06`, `2f7291f1`, `5253149c`, `69bfe5e5`, `79f44af5`), user-directed,
with two adaptations.

1. **SEC1-D3's producer-side yield is N/A here.** rev-225's world has no
   in-process OnDemand TCP path at all — no `ClientStateOndemand`, no
   `odClient`/`clientODAdapter`, no `modules/world/ondemand.go`; cache
   downloads are served by the separate `modules/ondemand` HTTP server, which
   never touches this writer. rev-274's `serveClient` slice-end, the
   `roundRobin`/`pump` per-pass progress bound, `backloggedRetryInterval`, the
   resumable per-client cursor, the adapter's `backlogged()`/`close()`
   conversions and `TestOnDemandPumpYieldsWhenBacklogged` therefore have no
   counterpart on this branch and were skipped, as was rev-254's re-derived
   `cycle`/`sendBatch` equivalent (inherited by rev-245.2 and rev-244). The
   game client is the only producer, so the hard caps (512 frames / 1 MiB) are
   the sole bound. `outboundWriter.Backlogged()` is kept and still pinned by
   `TestOutboundWriter_Backlogged`, with a comment recording that it has no
   in-world consumer on this pin so a future producer inherits the contract
   unchanged. Caps, soft thresholds, `outboundWriter`, `closeConn()` and
   everything else in M-2 are identical to rev-274.
2. **M-1's `tickOnce` extraction boundary was re-derived.** rev-225 tracks no
   `cycleStats`, so there is no CYCLE stat or `snapshotCycleStats` to leave
   behind in the loop; `currentTick++`, `stampTick`, the quit-select and the
   nextTick/drift/sleep bookkeeping stay in `runTickLoopWithRate`, and
   `tickOnce()` runs `processClientsIn` through `processSessionLogs`. The move
   was verified line-for-line pure. `crashSavePlayers` ranges `s.playerLoop`
   (exactly as this branch's `autosavePlayers` does) rather than the later
   revisions' `PlayerList` iterator.

M-7, M-8 and M-12 are byte-identical to rev-274's versions apart from
`cmd/goscape/Dockerfile` and `modules/ondemand/config.go`, where only the
equivalent hunks (`USER 65532:65532`, the `DebugStatusEnabled` field and flag)
were applied onto this branch's own file.

## Scope

| ID | Finding | Fix shape |
|----|---------|-----------|
| M-1 | `[login]`/`[logout]` trigger panics are unrecovered → process dies, no save | per-player recover around both trigger calls; tick-loop crash handler autosaves every player before re-panicking |
| M-2 | Slow-reading client blocks the tick goroutine (blocking `bufw.Flush` on `net.Conn`) | per-client async writer goroutine fed by a bounded queue; queue overflow / write timeout closes the conn; tick never blocks on the socket |
| M-7 | Plaintext password logged at Debug (`server_login.go:167`) | `slog.LogValuer` on `loginreq.GameLogin` redacting password, ISAAC seed, checksums |
| M-8 | Portal: no CSRF on public forms; no security headers; only `ReadHeaderTimeout`; state-changing GET `/verify-email` | double-submit CSRF cookie for anonymous forms; session rotation on login; header middleware; server timeouts + body cap; two-step verify |
| M-12 | Helm: no pod hardening, SA token mounted, `--config.expand-env` only with postgres, public `/debug/status`, hiscore port open to all when Kong fronts it | hardened defaults in `values.yaml`; always `--config.expand-env=true`; `ondemand.debug_status_enabled` (default false); hiscore ingress restricted to the Kong proxy when `createGatewayConfig`; explicit `USER` in Dockerfile (image stays `debug-nonroot` per user) |

Out of scope (user did not select): M-9 trusted-proxy config, image digest pinning is included as an *optional* value only.

## Deviations from Engine-TS (fidelity gate)

| ID | Description | Rationale | Closure plan |
|----|-------------|-----------|--------------|
| SEC1-D1 | `[login]` and `[logout]` trigger scripts run under `recoverPlayer`; a panic force-disconnects that player instead of propagating. TS has no per-player catch there — an exception propagates to `cycle()`'s catch and the process exits. | One player's broken save/script state must not take the world down. | Permanent (security hardening). |
| SEC1-D2 | On any unrecovered panic in the tick loop, goscape fires `PlayerAutosave` for every online player, waits ≤ `playerSaveFlushTimeout`, then re-panics. TS `cycle()` catch logs and `process.exit(1)` without saving. | Bounded data loss on crash. Process still dies, so crash semantics for supervisors are unchanged. | Permanent. |
| SEC1-D3 | Outbound socket writes happen on a per-client writer goroutine behind a bounded queue (`maxOutboundQueueSlots`=512 frames, `maxOutboundQueueBytes`=1 MiB). Queue overflow or a write exceeding `tcp_server_write_timeout` closes the connection; with `tcp_server_write_timeout` <= 0 the per-write deadlines are off, so `Close` stamps a 250 ms `fallbackDrainTimeout` on the socket — freeing any parked write and bounding the whole drain under one absolute budget while still delivering queued bytes. TS `socket.write` is async with an unbounded Node buffer and no disconnect. | Node's async write never stalls the event loop; Go's blocking `net.Conn.Write` on the tick goroutine did. The cap is the goscape equivalent of "never block the tick" plus protection against unbounded memory. Which cap binds depends on frame size: a stalled game client trips the 1 MiB byte cap, but the OnDemand pump emits one ~506-byte chunk per send, so 512 slots are exhausted at only ~260 KiB and the SLOT cap is what it would trip. The OnDemand pump therefore does not rely on the caps at all: it consults the soft high-water mark (`outboundHighWater` = 512 KiB, `outboundHighWaterSlots` = 256) via `outboundWriter.Backlogged` and ends the client's slice when it is set — **the OnDemand pump yields at the soft high-water mark and retries next pass, so a slow downloader is served slowly instead of disconnected** (`cq.active` and pending requests untouched, connection not closed; `pump`'s per-pass bound keeps the pass terminating and `run` re-arms after one tick). Memory budget: the hard cap is per client (1 MiB), so worst-case queue memory is 1 MiB × concurrently-stalled clients against the Helm `2Gi` memory limit; stalled clients die at `tcp_server_write_timeout`, so steady state is far below that — change either number with the other in mind. | Permanent. Cap values are constants; revisit if legitimate clients hit them (log line `outbound queue overflow`). |

M-7, M-8, M-12 touch goscape-only surfaces (log redaction, portal, Helm); no TS behaviour exists to diverge from.

## Non-goals / known residuals

- Rate limiters still key on `RemoteAddr` (M-9).
- `node_production` default unchanged (M-5).
- `networkPolicy.enabled` stays `false` by default (belongs to C-1's fix).
