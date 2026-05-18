# NAI-214: wire `LoginBridgeMod` to login-server gRPC

**Date:** 2026-05-18
**Tech stack:** Go 1.26+ per `[[go_version]]`. No new deps; `google.golang.org/grpc`, `google.golang.org/protobuf/types/known/timestamppb`, `google.golang.org/protobuf/types/known/emptypb` all already direct-required by `modules/login` + `modules/world/login_client.go`.
**Cadence:** Light — single-package modification under `modules/world/`, ~120 LOC production (2 RPC adapters + 1 bridge type + 1 default-flip), ~6 new tests. No proto changes. No DB migrations. No new module.
**Predecessor close memory:** `[[loginclient_interface_close]]` shipped the `LoginClient` interface refactor at bf228320 (2026-05-18); this spec extends that interface with two methods and adds a real `LoginBridgeMod` impl. The interface extension is the deferred bridge-mod half of `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD`.

---

## §1. Scope

`modules/world/bridges.go:27` declares `LoginBridgeMod` with two methods (`NotifyPlayerBan`, `NotifyPlayerMute`). All five call sites today resolve to `noopBridges{}`:

- `handler_reportabuse.go:50` — auto-ban on out-of-range report reason
- `handler_reportabuse.go:60` — auto-mute on offender flag
- `handler_message_private.go:42` — auto-ban on spam detection
- `handlers_game.go:1187` — `::ban` cheat
- `handlers_game.go:1206` — `::mute` cheat

The login-server side is already in place: `proto/login/login.proto` defines `PlayerBan` + `PlayerMute` RPCs, `modules/login/handler.go` implements both (writes `banned_until` / `muted_until` to SQLite), and `modules/login/handler_test.go` has unit tests. What's missing is purely the *world-side wire-up*: world's `LoginClient` interface doesn't expose those RPCs, and `loginBridgeMod` defaults to `noopBridges{}`.

This spec extends the world's `LoginClient` interface with two fire-and-forget RPC methods, adds a `loginGRPCBridgeMod` adapter implementing `LoginBridgeMod` by delegating to a `LoginClient`, and flips `NewServer`'s default so any production `Server` constructed with a non-nil `LoginClient` automatically routes bridge calls through gRPC. Tests continue to override via `installRecordingBridges` post-construction unchanged.

**In scope:**
- Add `PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest)` and `PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest)` to the `LoginClient` interface in `modules/world/login_client.go`. Both return `void` (fire-and-forget like `PlayerAutosave` / `PlayerForceLogout`).
- Implement both on `grpcLoginClient` using the same log-and-swallow pattern as `PlayerAutosave` (`login_client.go:78-86`).
- Extend `fakeLoginClient` (`login_client_fake_test.go`) with sync-RPC-capture-under-`mu` for `PlayerBan` + `PlayerMute`, matching the `VerifySave`/`LoadSave` precedent from `[[loginclient_interface_close]]`.
- Add `loginGRPCBridgeMod` type to `modules/world/bridges.go` (kept in same file — adds ~25 LOC, well under the 200-line "split file" heuristic). Wraps a `LoginClient`; each `Notify*` translates `time.Time → *timestamppb.Timestamp` and fires `go b.client.Player{Ban,Mute}(context.Background(), ...)`.
- Add package-level helper `defaultLoginBridgeMod(client LoginClient, log *slog.Logger) LoginBridgeMod` returning `&loginGRPCBridgeMod{...}` when `client != nil` else `noopBridges{}`. Extracted from `NewServer` for unit-testability without the full server bring-up.
- Flip default in `NewServer` (`server.go:272`): replace `s.loginBridgeMod = noopBridges{}` with `s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)`.
- Retire `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` deviation tag in `bridges.go:23-26` (comment block update).
- ~6 new tests across `login_client_test.go`, `bridges_test.go` (or new `login_bridge_test.go`), and `server_test.go`.

**Out of scope:**
- **Audit log.** TS `LoginServer.ts:489,501` has `// todo: audit log`; goscape's `modules/login/handler.go` PlayerBan/Mute matches that TS no-op. Adding an audit table is a separate slice on the login-server side, orthogonal to bridge wire-up.
- **Retries / circuit breaker.** PlayerBan/Mute are best-effort moderation actions; if the login server is unreachable, the bridge logs a warn and the action effectively no-ops. Matches `PlayerAutosave` policy (`login_client.go:81-85`). Adding retry policy would require a coordinated decision across all 4 fire-and-forget RPCs.
- **In-process kick on ban.** TS `World.notifyPlayerBan` (`World.ts:2270-2285`) kicks the banned player from world memory (`other.logout(); other.client.close()`) *before* posting to login. The goscape equivalent already lives at the `::ban` cheat call site in `handlers_game.go:1187` (synchronous remove-from-world logic adjacent to the bridge call). No new in-process logic needed in the bridge itself; bridge is a pure RPC fan-out.
- **`FriendsBridge` real impl.** Separate slice (the `NAI-72-D-FRIENDS-SERVER-BRIDGE` half). The friends server is a distinct dskit module; this slice does not touch it.
- **`LoggerBridge` changes.** Already has a real impl (`slogLoggerBridge`); not in scope.
- **`PlayerBanResponse` / `PlayerMuteResponse` body parsing.** Both are empty messages (`proto/login/login.proto:91,98`). The fire-and-forget pattern ignores the response entirely.
- **A new CLI flag.** Bridge is unconditional given `loginClient != nil` (which is already gated by `--login.address` in the existing config).
- **Touching `modules/login`, `proto/login/login.proto`, `pkg/loginpb`, `cmd/goscape/app/*`, or any code outside `modules/world/`.** The login-server side is complete.
- **A new deviation tag.** This spec retires one and adds none.

---

## §2. Layout

| File | State | Purpose |
|---|---|---|
| `modules/world/login_client.go` | MODIFIED | Add `PlayerBan` + `PlayerMute` to the `LoginClient` interface and `grpcLoginClient` impl. ~30 LOC added, same shape as existing `PlayerAutosave` block (lines 78-86). |
| `modules/world/login_client_fake_test.go` | MODIFIED | Extend `fakeLoginClient` to capture `PlayerBan` + `PlayerMute` RPC payloads under existing `mu`. Mirrors `VerifySave`/`LoadSave` capture pattern. |
| `modules/world/bridges.go` | MODIFIED | Add `loginGRPCBridgeMod` type, `_ LoginBridgeMod = (*loginGRPCBridgeMod)(nil)` compile-time interface check, and `defaultLoginBridgeMod(client, log)` package-level helper. Update doc comment on `LoginBridgeMod` to remove the `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` "real impl deferred" sentence and add a one-line pointer to `loginGRPCBridgeMod`. `noopBridges` retained unchanged (still used for standalone-world fallback + tests). |
| `modules/world/bridges_test.go` | MODIFIED | Add `TestLoginGRPCBridgeMod_NotifyPlayerBan_FiresRPC`, `_Mute_FiresRPC`, and `_FireAndForget_DoesNotBlock`. Existing `recordingBridges` tests unchanged. |
| `modules/world/server.go` | MODIFIED | Replace `s.loginBridgeMod = noopBridges{}` (line 272) with `s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)`. One-line change. |
| `modules/world/login_client_test.go` | MODIFIED | Add `TestGRPCLoginClient_PlayerBan_LogsErrorOnFailure` + `_Mute_` variant. Uses an in-package stub `LoginServiceClient` (mirrors existing `TestGRPCLoginClient_PlayerAutosave_LogsErrorOnFailure` if present; otherwise adopts the same shape as login-server-side handler tests with a dial-buffer connection). |

No new files unless `login_bridge_test.go` is preferred over extending `bridges_test.go` — author's call at implementation time; both meet the "one bridge type per file" threshold for splitting.

No changes to:
- `proto/login/login.proto`, `pkg/loginpb/*` (RPCs already exist)
- `modules/login/*` (handler already implements PlayerBan/Mute)
- `cmd/goscape/app/*` (NewServer signature unchanged)
- `go.mod` / `go.sum` (no new deps)

---

## §3. Architecture

### §3.1 `LoginClient` interface extension

Current interface (`login_client.go:16-23`):

```go
type LoginClient interface {
    WorldStartup(ctx context.Context, nodeID int32, profile string)
    PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error)
    PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error)
    PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
    PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest)
    Close() error
}
```

Extended (two new methods, same return shape as `PlayerAutosave`/`PlayerForceLogout`):

```go
type LoginClient interface {
    // ...existing methods...
    PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest)
    PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest)
    Close() error
}
```

Fire-and-forget rationale: `PlayerBanResponse` / `PlayerMuteResponse` are empty messages with no information for the caller. Errors are best-effort (logged via `c.log.Warn(...)`) and not returned — consistent with the two existing fire-and-forget RPCs. Bridge call sites are inside synchronous handler/cheat paths and must not block on network I/O.

### §3.2 `grpcLoginClient` impls

```go
func (c *grpcLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
    if _, err := c.client.PlayerBan(ctx, req); err != nil {
        c.log.Warn("PlayerBan RPC failed",
            slog.String("username", req.Username),
            slog.Any("err", err),
        )
    }
}

func (c *grpcLoginClient) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) {
    if _, err := c.client.PlayerMute(ctx, req); err != nil {
        c.log.Warn("PlayerMute RPC failed",
            slog.String("username", req.Username),
            slog.Any("err", err),
        )
    }
}
```

Identical to `PlayerAutosave` at `login_client.go:78-86` and `PlayerForceLogout` at `login_client.go:88-95` modulo type names. Same log-key conventions.

### §3.3 `loginGRPCBridgeMod` adapter

Added to `modules/world/bridges.go` directly below the `noopBridges` block:

```go
// loginGRPCBridgeMod is the production LoginBridgeMod impl. Translates
// moderation actions into gRPC RPCs against the login server. Calls are
// fired in a goroutine so packet handlers and the tick loop never block
// on network I/O — mirrors the goroutine fan-out used by autosave
// (server.go:1018) and force-logout (server.go:982).
type loginGRPCBridgeMod struct {
    client LoginClient
    log    *slog.Logger
}

func (b *loginGRPCBridgeMod) NotifyPlayerBan(staff, username string, until time.Time) {
    go b.client.PlayerBan(context.Background(), &loginpb.PlayerBanRequest{
        Staff:    staff,
        Username: username,
        Until:    timestamppb.New(until),
    })
}

func (b *loginGRPCBridgeMod) NotifyPlayerMute(staff, username string, until time.Time) {
    go b.client.PlayerMute(context.Background(), &loginpb.PlayerMuteRequest{
        Staff:    staff,
        Username: username,
        Until:    timestamppb.New(until),
    })
}

var _ LoginBridgeMod = (*loginGRPCBridgeMod)(nil)
```

Why `context.Background()` rather than a derived context with deadline: matches existing fire-and-forget calls at `server.go:982,1018`. A short deadline would silently truncate slow networks during legitimate moderation; an operator dialling `--login.address` to a hung server has bigger problems than a moderation log line. The goroutine outlives the synchronous caller but completes when gRPC returns (or the underlying conn closes on shutdown).

### §3.4 `NewServer` default flip

Current (`server.go:271-273`):

```go
s.friendsBridge = noopBridges{}
s.loginBridgeMod = noopBridges{}
s.loggerBridge = NewSlogLoggerBridge(s.log)
```

Updated (`loginBridgeMod` only, delegating to a package-level helper for testability — see §4.4):

```go
s.friendsBridge = noopBridges{}
s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)
s.loggerBridge = NewSlogLoggerBridge(s.log)
```

The helper lives in `bridges.go` next to the bridge type it returns.

Standalone-world mode (`--login.address` unset → `loginClient == nil` per existing `cmd/goscape/app/modules.go` plumbing) keeps the noop fallback so moderation cheats degrade gracefully rather than panicking on a nil dereference.

`newTestServer` in `server_test.go:300-326` constructs `Server{}` directly and explicitly sets `s.loginBridgeMod = noopBridges{}`. That line stays — tests have always been the source of their own bridge state, and the production default flip doesn't reach them.

### §3.5 Doc-comment retirement

Current `bridges.go:23-26`:

```go
// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). The existing LoginClient is auth-only; this is a
// separate moderation channel. Real impl deferred via
// NAI-72-D-LOGIN-SERVER-BRIDGE-MOD.
```

Replaced with:

```go
// LoginBridgeMod mirrors TS World.loginThread.postMessage('player_ban'/
// 'player_mute', ...). Production impl is loginGRPCBridgeMod (below),
// which delegates to LoginClient.PlayerBan / .PlayerMute.
```

No file other than `bridges.go` references the tag string. Memory file `[[loginclient_interface_close]]` already lists this as the deferred half — the new close memory at slice end will cite this spec as the retirement point.

---

## §4. Test plan

### §4.1 `LoginClient` extension tests (`login_client_test.go`)

| Test | Setup | Assertion |
|---|---|---|
| `TestGRPCLoginClient_PlayerBan_PassesRequest` | bufconn gRPC server registering a `loginpb.LoginServiceServer` stub that records the inbound `PlayerBanRequest` | After `client.PlayerBan(ctx, req)` returns, the stub recorded `(Staff, Username, Until.AsTime())` matching the request. |
| `TestGRPCLoginClient_PlayerBan_LogsErrorOnFailure` | bufconn server registers a `PlayerBan` impl that returns `status.Error(codes.Unavailable, "down")`; client uses a `slog.Handler` capturing emitted records | After the call returns, exactly one warn record was emitted with key `"PlayerBan RPC failed"` and the username matches. The call does not panic. |
| `TestGRPCLoginClient_PlayerMute_PassesRequest` | symmetric | symmetric |
| `TestGRPCLoginClient_PlayerMute_LogsErrorOnFailure` | symmetric | symmetric |

The bufconn pattern matches the precedent in `modules/login/handler_test.go` (uses a real in-process gRPC dial). If `login_client_test.go` does not already host bufconn scaffolding, the slice author may either lift it from `modules/login/handler_test.go` or use a simpler approach: inject a fake `loginpb.LoginServiceClient` directly into `grpcLoginClient.client` (the struct field is unexported but in-package). The simpler approach is preferred per Go testing convention.

### §4.2 Bridge tests (`bridges_test.go`)

| Test | Setup | Assertion |
|---|---|---|
| `TestLoginGRPCBridgeMod_NotifyPlayerBan_FiresRPC` | `fakeLoginClient` (with new `PlayerBan` capture); `bridge := &loginGRPCBridgeMod{client: fake, log: discardLogger()}`; call `bridge.NotifyPlayerBan("alice", "evilbob", t0)` | `fake.WaitForPlayerBan(t, ...)` (sync helper added to fake — see §4.3) records exactly one call with `Staff="alice"`, `Username="evilbob"`, `Until.AsTime().Equal(t0)`. |
| `TestLoginGRPCBridgeMod_NotifyPlayerMute_FiresRPC` | symmetric | symmetric |
| `TestLoginGRPCBridgeMod_FireAndForget_DoesNotBlock` | `fakeLoginClient` with a `blockPlayerBan chan struct{}` gate; `bridge.NotifyPlayerBan(...)` from main goroutine | The synchronous call returns within 50ms (well before the gate is signalled). After the gate closes, the captured call appears. Validates the `go` fan-out. |

### §4.3 Fake extension (`login_client_fake_test.go`)

`fakeLoginClient` (`login_client_fake_test.go:15-45`) uses buffered channels of capacity 16 for both existing fire-and-forget RPCs (`autosaveReqs`, `forceLogoutReqs`). PlayerBan/Mute follow the same shape exactly:

- Add fields:
  ```go
  playerBanReqs  chan *loginpb.PlayerBanRequest
  playerMuteReqs chan *loginpb.PlayerMuteRequest
  ```
- Init in `newFakeLoginClient` with `make(chan *loginpb.PlayerBanRequest, 16)` and the muted variant.
- Add methods:
  ```go
  func (f *fakeLoginClient) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) {
      select {
      case f.playerBanReqs <- req:
      default:
      }
  }
  ```
  Mirror exactly the `PlayerAutosave` impl at `login_client_fake_test.go:86-94`. Drop-on-full matches that precedent (tests draining lazily).
- `PlayerMute` symmetric.

No `Wait*` helper needed — tests select on the channel with a timeout, same as existing tests that drain `autosaveReqs`. The gate-channel variant in §4.2's `_FireAndForget_DoesNotBlock` test is a one-off: that test wraps a *separate* tiny fake (not `fakeLoginClient`) whose `PlayerBan` blocks on a `<-gate` before recording, so the bridge's `go` fan-out is observable. Keeps the main fake clean.

### §4.4 Default-flip test (`bridges_test.go`)

`NewServer` opens a TCP listener and loads the full cache (`server.go:236-330` — locTypes, params, objTypes, gamemap, invTypes, dbTable/Row, varp/vars/varn, etc.). That's far too heavy for a test asserting one type-of-default decision.

Extract the default-selection logic into a package-level helper:

```go
// defaultLoginBridgeMod returns the production LoginBridgeMod for the
// given LoginClient: a goroutine-fanout gRPC adapter when client != nil,
// otherwise noopBridges{}. Called from NewServer; broken out for
// testability without spinning up the full Server.
func defaultLoginBridgeMod(client LoginClient, log *slog.Logger) LoginBridgeMod {
    if client != nil {
        return &loginGRPCBridgeMod{client: client, log: log}
    }
    return noopBridges{}
}
```

`NewServer` calls `s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.log)`. Test in `bridges_test.go`:

```go
func TestDefaultLoginBridgeMod_NonNilClient_ReturnsGRPCBridge(t *testing.T) {
    got := defaultLoginBridgeMod(newFakeLoginClient(), discardLogger())
    if _, ok := got.(*loginGRPCBridgeMod); !ok {
        t.Fatalf("defaultLoginBridgeMod: got %T, want *loginGRPCBridgeMod", got)
    }
}

func TestDefaultLoginBridgeMod_NilClient_ReturnsNoop(t *testing.T) {
    got := defaultLoginBridgeMod(nil, discardLogger())
    if _, ok := got.(noopBridges); !ok {
        t.Fatalf("defaultLoginBridgeMod: got %T, want noopBridges", got)
    }
}
```

### §4.5 Call-site smoke

No new tests needed. Existing tests in `handler_reportabuse_test.go`, `handler_message_private_test.go`, `handler_cheats_supermod_test.go` already install `recordingBridges` post-construction and assert bridge calls; the production default flip does not reach them. Run the full `modules/world` test suite to confirm no regression — those tests stay green by construction.

### §4.6 Race detector

Slice closes only after `go test -race ./modules/world/...` is clean. The `go` fan-out in `loginGRPCBridgeMod.NotifyPlayer*` and the fake's `mu`-protected capture are the new concurrency surfaces.

---

## §5. Implementation order

1. **Interface + grpcLoginClient impl** (`login_client.go`): extend `LoginClient` with PlayerBan/Mute, add `grpcLoginClient.PlayerBan`/`.PlayerMute`. Compile only; tests in §4.1 land next.
2. **fakeLoginClient extension** (`login_client_fake_test.go`): add PlayerBan/Mute capture + helpers. Unblocks §4.2 + §4.4.
3. **§4.1 tests**: assert `grpcLoginClient.PlayerBan`/`.PlayerMute` against an in-process gRPC stub or in-package fake. Land before bridge impl.
4. **`loginGRPCBridgeMod` impl + `defaultLoginBridgeMod` helper** (`bridges.go`): new type + compile-time interface check + helper; doc-comment retirement.
5. **§4.2 + §4.4 tests**: assert bridge → LoginClient delegation against fake, and helper selects the right type for nil/non-nil client.
6. **`NewServer` default flip** (`server.go:272`): one-line swap to call the helper.
8. **Full `-race` run** + smoke-pack 12 OK / 0 ERR baseline check (defensive; this slice doesn't touch packing, but the gate is cheap and the memory record from `[[content-watcher-replay-close]]` shows the value of always running it).
9. **Commit ladder** (target: 4–6 commits):
   - `world: extend LoginClient with PlayerBan + PlayerMute RPCs`
   - `world: capture PlayerBan + PlayerMute in fakeLoginClient`
   - `world: add loginGRPCBridgeMod adapter`
   - `world: default loginBridgeMod from non-nil LoginClient` (retires NAI-72-D-LOGIN-SERVER-BRIDGE-MOD tag in same commit)
   - Optional: split fakeLoginClient extension or test files if the diff bloats one commit too far.

---

## §6. Risks and edge cases

- **`time.Time` zero-value passthrough.** `timestamppb.New(time.Time{})` produces an `1-01-01T00:00:00Z` proto. None of the 5 call sites pass a zero time (`time.Now().Add(...)` in all five), but if a future caller does, the login server will write a 1-year ban literally. Bridge does not defensively coerce — pass-through honors caller intent. Document in the bridge doc-comment: "callers must compute the absolute deadline before invocation".
- **gRPC fan-out leaks on shutdown.** `go b.client.PlayerBan(...)` may outlive the synchronous caller. Same risk exists today for `PlayerAutosave` / `PlayerForceLogout` and has not surfaced as a problem. No new shutdown coordination needed.
- **`recordingBridges` test-side coverage.** `recordingBridges` (`bridges_test.go:75-80`) already records `NotifyPlayerBan`/`Mute` calls. Tests that install it (e.g., `handler_reportabuse_test.go`, `handler_cheats_supermod_test.go`) test the *handler→bridge* layer in isolation. Tests of the *bridge→RPC* layer are new in §4.2 and don't overlap. The combination (handler → bridge → RPC) is end-to-end and untested at the unit level — covered by the smoke-pack baseline + manual exercise of the `::ban` cheat against a real login server. No new e2e test needed for this slice; that's the scope of a separate observability follow-up if ever wanted.
- **`NewServer` signature unchanged.** `loginClient` is already a parameter (since `[[loginclient_interface_close]]`). The default flip is purely internal. `cmd/goscape/app/modules.go` does not need to change.
- **No deviation tag introduced.** TS posts the message via `loginThread.postMessage` (a worker-thread MessagePort, not gRPC). goscape uses gRPC throughout. This is an architectural substitution that predates this slice (existing `LoginClient` already replaces TS `LoginThread`); no new tag required for the same substitution applied to two more methods.
- **`PlayerBan` in `proto/login/login.proto:85-89` carries no `node_id` or `node_time`.** TS sends both for audit-trail context (`LoginClient.ts:138-150`). goscape's existing proto deviates here, presumably consciously when the proto was authored. This spec does not introduce or change that deviation.

---

## §7. Exit criteria

- All §4 tests green.
- `go test -race ./modules/world/...` clean.
- Existing `modules/world` test suite passes unmodified except for the additions in §2.
- Smoke-pack 12 OK / 0 ERR holds (defensive — no packing surface touched, but baseline gate always runs).
- `NAI-72-D-LOGIN-SERVER-BRIDGE-MOD` deviation tag removed from `bridges.go`. No new tags introduced.
- Close-memory entry written under `memory/nai214_login_moderation_bridge_close.md` and indexed in `MEMORY.md`, citing the commit range and any emergent issues found during implementation.
