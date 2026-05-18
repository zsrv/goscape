# LoginClient interface refactor + deferred FU tests

**Date:** 2026-05-18
**Predecessor:** [2026-05-18-playerloading-design.md](2026-05-18-playerloading-design.md)
**Retires TODOs:**
- `NAI-PLAYERLOADING-FU-LOGIN-INTEGRATION-TESTS` (`modules/world/tick.go:180`)
- `NAI-PLAYERLOADING-FU-LOGOUT-INTEGRATION-TESTS` (`modules/world/server.go:880`)
- `NAI-PLAYERLOADING-FU-DISCONNECT-INTEGRATION-TESTS` (`modules/world/server.go:914`)
- `NAI-PLAYERLOADING-FU-AUTOSAVE-INTEGRATION-TEST` (`modules/world/tick.go:57`)

## 1. Problem

`*LoginClient` (`modules/world/login_client.go:15`) is a concrete struct wrapping a real gRPC client. The four follow-up integration tests for the NAI-PLAYERLOADING login/logout/disconnect/autosave RPC paths were all deferred on the same blocker: tests cannot stub `Server.loginClient` because there is no interface seam. Production behaviour is already correct and exercised by the smoke pack; what is missing is unit-level integration coverage that asserts the *contents* of the RPC requests fired on each lifecycle transition.

## 2. Goal

Introduce an interface seam at the `LoginClient` boundary, then land the four deferred test groups in a single slice.

Out of scope: any production behaviour change, any new RPCs, any change to `cmd/goscape/app/modules.go` wiring beyond what the type rename forces.

## 3. Design

### 3.1 Interface shape — rename concrete, expose interface (Option A)

The concrete struct is renamed `grpcLoginClient` (unexported); the exported name `LoginClient` becomes the interface. This is idiomatic Go (cf. `io.Reader` / `bytes.Buffer`).

```go
// LoginClient is the world-side interface to the login service.
// Production impl: grpcLoginClient. Test impl: fakeLoginClient (login_client_fake_test.go).
type LoginClient interface {
    WorldStartup(ctx context.Context, nodeID int32, profile string)
    PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error)
    PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error)
    PlayerAutosave(ctx context.Context, req *loginpb.PlayerAutosaveRequest)
    PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest)
    Close() error
}
```

All six current methods stay on the interface. `Close` is included because `World.Close` calls it during shutdown — keeping it on the interface lets the field type stay uniformly `LoginClient` everywhere (no second handle for lifecycle).

`NewLoginClient(addr string, log *slog.Logger) (LoginClient, error)` — same name, return type widens from `*LoginClient` to `LoginClient`. Constructor returns `&grpcLoginClient{...}`.

Field types change from `*LoginClient` to `LoginClient` in:
- `Server.loginClient` (`modules/world/server.go:52`)
- `World.loginClient` (`modules/world/world.go:22`)

`World.GetLoginClient() LoginClient` (was `*LoginClient`). `cmd/goscape/app/modules.go` already uses it only as opaque handoff to `NewWorldService` — no change there beyond the type.

### 3.2 Fake placement

`modules/world/login_client_fake_test.go` — package-internal, test-only file.

```go
type fakeLoginClient struct {
    mu sync.Mutex

    // Captured requests (per RPC, last call wins for sync RPCs;
    // channels for async RPCs so tests can select-with-timeout).
    worldStartupCalls    []worldStartupCall
    lastPlayerLoginReq   *loginpb.PlayerLoginRequest
    lastPlayerLogoutReq  *loginpb.PlayerLogoutRequest
    autosaveReqs         chan *loginpb.PlayerAutosaveRequest
    forceLogoutReqs      chan *loginpb.PlayerForceLogoutRequest

    // Canned responses for sync RPCs.
    playerLoginResp *loginpb.PlayerLoginResponse
    playerLoginErr  error
    playerLogoutResp *loginpb.PlayerLogoutResponse
    playerLogoutErr error

    closed bool
}
```

- **Sync RPCs** (`PlayerLogin`, `PlayerLogout`): record last request under `mu`, return canned response.
- **Async/fire-and-forget RPCs** (`PlayerAutosave`, `PlayerForceLogout`): push to channel; tests `select` with timeout. Buffered to depth ~16 so tests don't have to drain in lockstep.
- **WorldStartup**: append to slice (called once per `Server.starting`); slice is sufficient.
- **Close**: set `closed=true`, return nil.

The async RPCs in production are called inside `go func()` blocks. The fake's method body itself is synchronous — it just records and returns — so the channel push happens on the caller's goroutine (the production goroutine), which is the right granularity for tests to observe.

### 3.3 Tests to land

One test group per FU TODO. All four go into `modules/world/server_test.go` or sibling `_test.go` files, following the existing patterns.

#### FU-LOGIN-INTEGRATION-TESTS (`tick.go:180` cluster — processLogins)

Drive the `processLogins` path with a fake `LoginClient` returning each branch's response. Assert the captured `PlayerLoginRequest` carries the expected `NodeID`, `Profile`, `Username`, `Password`, `UID`, `Socket`, `RemoteAddress`, `Reconnecting`, `HasSave=false`.

Branches to cover (per the existing TODO block):
- `LOGIN_RESULT_OK` → player added, RS2 reply byte = 2
- `LOGIN_RESULT_NEW_PLAYER` → player added, reply byte = 2 (or whatever `loginResultToRS2` maps it to — read from production)
- `LOGIN_RESULT_INVALID_USER_OR_PASS` → no player added, reply byte = 3
- `LOGIN_RESULT_SERVER_OFFLINE` (RPC error path) → no player added, reply byte = `OpLoginServerOffline.Opcode`

#### FU-LOGOUT-INTEGRATION-TESTS (`server.go:884` — removePlayerOnTick)

Construct a `Server` with a fake login client, add a player with a non-empty username and seeded inventory state, call `removePlayerOnTick(p)`. Wait on a small synchronisation point (fake's mutex or a sync channel — see §3.4) then assert the captured `PlayerLogoutRequest` has the expected `NodeID`, `Profile`, `Username`, and a non-nil `Save` payload whose first bytes match the SAV magic.

Negative: with `loginClient == nil` or `p.username == ""`, no RPC fires (assert recorded request is still nil).

#### FU-DISCONNECT-INTEGRATION-TESTS (`server.go:918` — removePlayerOnDisconnect)

Same fixture as logout test but call `removePlayerOnDisconnect(p)`. Assert that `PlayerForceLogout` channel receives a request with the expected fields, **and** that `PlayerLogout` was not called (recorded `lastPlayerLogoutReq` stays nil). Confirms the "no save on ungraceful disconnect" contract.

#### FU-AUTOSAVE-INTEGRATION-TEST (`tick.go:57` cluster + `autosavePlayers`)

Construct a `Server` with a fake login client and 2 active players (non-empty usernames). Tick `PlayerSaveRate` times. Assert:
- Each player produces exactly one `PlayerAutosaveRequest` on the channel.
- Each request carries the expected `NodeID`, `Profile`, `Username`, and a non-nil `Save`.
- A second pass of `PlayerSaveRate - 1` ticks fires nothing further; the `PlayerSaveRate`-th tick fires both again.

### 3.4 Synchronisation: no `time.Sleep`

`removePlayerOnTick` spawns a goroutine for the RPC. Tests must wait on the fake's channel (or a `sync.WaitGroup` exposed by the fake) before asserting. Specifically:

- For async RPCs (`PlayerAutosave`, `PlayerForceLogout`): the fake's channel push is the synchronisation point.
- For the `removePlayerOnTick` goroutine (sync RPC inside async goroutine): give the fake a `playerLogoutFired chan struct{}` that closes/sends after `lastPlayerLogoutReq` is recorded. Test does `select { case <-fired: case <-time.After(2*time.Second): t.Fatal("timeout") }`.

Race detector (`go test -race ./modules/world/...`) must run clean.

### 3.5 Fixture reuse

The four tests share most of their fixture setup. Factor into one helper:

```go
func newFakeLoginServer(t *testing.T) (*Server, *fakeLoginClient) {
    t.Helper()
    fake := newFakeLoginClient()
    srv := newTestServer(t, withLoginClient(fake))
    return srv, fake
}
```

If `newTestServer` doesn't yet exist in the form needed, prefer extending the existing test-fixture pattern in `server_test.go` over inventing a new shape.

## 4. PR shape

Single slice, two commits:

1. **refactor:** introduce `LoginClient` interface, rename concrete to `grpcLoginClient`, switch field types. Zero behaviour change. Verified by existing tests + `-race`.
2. **test:** add `fakeLoginClient` + the 4 test groups. Retire the 4 TODO comments inline in the same commit.

A close commit (`chore(close):`) follows after both land.

## 5. Verification gates

- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...`
- `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/...`
- Full repo: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./...`
- Smoke pack: 12 OK / 0 ERR baseline holds (touches nothing the pack exercises, but cheap to confirm).

## 6. Risk notes

- **`Close()` on the interface** — test fakes must implement it. Trivial (return nil) but worth noting.
- **Fire-and-forget contract** — `PlayerAutosave` / `PlayerForceLogout` log on error in production; fakes must not surface errors through return values (the methods have no return). Test channels observe the *captured request*, not an error path.
- **Goroutine timing** — `removePlayerOnTick` spawns a goroutine inline. Tests use the fake's sync channel, never `time.Sleep`.
- **No interface bloat creep** — six methods is the upper bound. Any new LoginClient RPC added later will require adding it to the interface plus the fake; that's the intended cost.

## 7. Deviation tags anticipated

None expected — this is a mechanical interface extraction with no semantic deviation from the TS reference. If a fake-related deviation surfaces during implementation, tag inline with `NAI-PLAYERLOADING-LC-D-*`.
