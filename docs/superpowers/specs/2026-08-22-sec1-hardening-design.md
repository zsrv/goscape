# SEC1 — Security hardening batch 1 (M-1, M-2, M-7, M-8, M-12)

Source: goscape security review 2026-08-20 (rev-274 @ d509f14a). This batch
addresses five Medium findings the user selected. Criticals/Highs are
deliberately out of scope here and remain open.

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
| SEC1-D3 | Outbound socket writes happen on a per-client writer goroutine behind a bounded queue (`maxOutboundQueueSlots`=64 frames, `maxOutboundQueueBytes`=256 KiB). Queue overflow or a write exceeding `tcp_server_write_timeout` closes the connection. TS `socket.write` is async with an unbounded Node buffer and no disconnect. | Node's async write never stalls the event loop; Go's blocking `net.Conn.Write` on the tick goroutine did. The cap is the goscape equivalent of "never block the tick" plus protection against unbounded memory. | Permanent. Cap values are constants; revisit if legitimate clients hit them (log line `outbound queue overflow`). |

M-7, M-8, M-12 touch goscape-only surfaces (log redaction, portal, Helm); no TS behaviour exists to diverge from.

## Non-goals / known residuals

- Rate limiters still key on `RemoteAddr` (M-9).
- `node_production` default unchanged (M-5).
- `networkPolicy.enabled` stays `false` by default (belongs to C-1's fix).
