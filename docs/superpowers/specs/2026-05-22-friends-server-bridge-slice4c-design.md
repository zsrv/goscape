# Friends-server bridge — slice 4c design: world acts on PlayerLoginResponse.Accepted

**Date:** 2026-05-22
**Slice:** 4c of 7 (friends-server bridge arc; slice 4 decomposed into 4a/4b/4c)
**Predecessor:** slice 4b (close commit `add114bb`, retired `NAI-S1-D-PM-NO-DELIVERY`; see `[[friends-server-slice4b-close]]`)
**Closes:** `NAI-S1-D-PLAYERCAP-LOG-ONLY`, `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED`
**Opens:** none

## 1. Scope

Surface the friends-server's `PlayerLoginResponse.Accepted` field to the world. Today the world's `FriendsClient.PlayerLogin` is void and the response is discarded at `modules/world/friends_client.go:86-92`. Slice 4c changes the interface to take a callback and wires the call site at `modules/world/tick.go:170-177` to log a warn on `Accepted=false`.

The server side is already correct: `modules/friends/handler.go:61-77` returns `PlayerLoginResponse{Accepted: false}` when the per-world cap is reached. Slice 4c does **not** change server behavior; it only retires the doc-comment deferral note. The cap-rejection path is already covered by `TestHandler_PlayerLogin_PlayerCapAccepted_False` (`modules/friends/handler_test.go:96-118`).

Slice 4c closes the entire slice-4 cluster (4a + 4b + 4c). Remaining slice-2 and slice-1 deviation tags after this slice are either permanent (TS-faithful posture decisions), future-slice (slices 5/6/7), or blocked on NAI-182-D5 (the social-cluster ServerGameProt port).

## 2. Forward map (what changes)

| File | Change | Notes |
|---|---|---|
| `modules/world/friends_client.go` | **signature change** + impl change | `FriendsClient.PlayerLogin` gains `onResponse func(accepted bool)` parameter; `grpcFriendsClient.PlayerLogin` invokes the callback after the RPC returns. Doc-comments retire `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED`. |
| `modules/world/friends_client_fake_test.go` | **signature change** | `fakeFriendsClient.PlayerLogin` matches the new interface; calls the callback after pushing the request onto the channel. Adds a configurable `playerLoginAccepted` field for test control (defaults to `true`). |
| `modules/world/friends_client_test.go` | **changed** | Update `TestGRPCFriendsClient_LogsErrorOnFailure` PlayerLogin entry to pass `nil` callback (or assert callback receives `false` on RPC error). Add `TestGRPCFriendsClient_PlayerLogin_InvokesCallback` covering accepted-true and accepted-false. |
| `modules/world/tick.go` | **changed** | Call site at lines 170-177 passes a callback that logs warn on `accepted=false`. Retire both `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` doc-comments. |
| `modules/world/tick_friends_login_test.go` | **changed / additive** | Update existing tests to drain the new fake's behavior. Add `TestProcessLogins_FriendsPlayerLogin_LogsWarnOnRejection` capturing slog output via a test handler. |
| `modules/friends/handler.go` | **doc-only** | Retire `NAI-S1-D-PLAYERCAP-LOG-ONLY` annotation on `PlayerLogin`. Remove the "Slice 4c surfaces Accepted to callers" forward-reference (now satisfied). |
| `modules/world/friends_smoke_test.go` | **additive** | New `TestFriendsClient_E2E_PlayerLoginCapRejected` boots a real friends server with `WorldPlayerLimit: 1`, logs in two players, asserts the second callback fires with `accepted=false`. |

LOC estimate: ~130 added, ~30 deleted, plus a mechanical signature update at every PlayerLogin call site (1 production site + ~5 fake-test sites in `friends_smoke_test.go`).

## 3. Architectural decision: callback shape, TS-faithful behavior

### Shape: callback (not return value)

The world-side `FriendsClient` posture is fire-and-forget for every RPC except `SubscribeUpdates` (`modules/world/friends_client.go:18-23`). The slice-2 deviation tags `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` and `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` are permanent and document this posture for the bridge as a whole.

Switching `PlayerLogin` to a synchronous return value would force the tick loop's `processLogins` to either (a) block on the friends RPC every login, or (b) launch a goroutine and wait inside it. Both compromise the fire-and-forget property. A callback preserves the existing `go s.friendsClient.PlayerLogin(...)` shape — the callback runs in the same goroutine that issued the RPC, after the response is received.

Three rejected alternatives:

- **Return value** (`PlayerLogin(ctx, req) (accepted bool)`): forces synchronous shape; contradicts the fire-and-forget convention used by every other void RPC on this interface.
- **Separate method** (`PlayerLoginWithResponse(...)` alongside void `PlayerLogin`): bifurcates the API for one caller. Not justified by adoption.
- **Channel return** (`PlayerLogin(...) <-chan bool`): exotic; no precedent in this codebase; same blocking concern as return value when the channel is read.

The callback is `func(accepted bool)`. Errors (RPC failure, server down) are still logged inside `grpcFriendsClient.PlayerLogin` and the callback is invoked with `accepted=false` so the caller sees a uniform "rejected-or-error" signal. This matches a TS-faithful read of "if the friends-server can't confirm acceptance, treat as not-accepted".

### Behavior on `Accepted=false`: log warn, do not interrupt login

TS canonical (`Engine-TS/src/server/friend/FriendServer.ts:128-132`):

```typescript
if (!(await this.repository.register(world, username37, privateChat, message.staffLvl))) {
    // TODO handle this better?
    // console.error(`[Friends]: World ${world} is full`);
    return;
}
```

The TS server returns early on cap-reached. There is **no signal back to the world**; the TS world has no idea its login was rejected by the friends-server. The world's own player cap (separate check in goscape at `tick.go:154-159`) is the single source of truth for "world full" from the player-facing perspective.

The TS-faithful behavior on goscape's side is therefore: **log a warn, continue the login flow unchanged**. We do not refuse the login. We do not skip the subscriber start. The subscriber start on a not-registered player is a benign no-op: the friends-server's `subscriptions` registry (slice 4a) accepts the stream and tracks it per-(world, username37) regardless of presence-table membership, so the subscriber sits there receiving nothing relevant until the player's `friendsSubCancel` is invoked on logout/disconnect.

### Why the subscriber stays unconditional

Two reasons:

1. **Cleanup orthogonality.** Keeping the subscriber start at the tick site (line 181-186) outside any callback closure means the player-struct fields `friendsSubCancel` and `friendsSub` are written from the single tick goroutine, with no happens-before concern between the RPC callback goroutine and the subscriber-start. The logout/disconnect paths that read `friendsSubCancel` (in `client.go` and `tick.go`) continue to do so unchanged.
2. **No observable cost.** The subscriber holds one open gRPC stream, two goroutines (recv loop + supervisor), and a slot in the server's `subscriptions` map. On a cap-rejected player, this is wasted work — but only until the next logout/disconnect, which is bounded by the world's own auth flow accepting or rejecting the player on its own merits.

Alternative considered: gate the subscriber start on `accepted=true` via a `sync.Once`-protected flag the callback sets. Rejected — adds synchronization complexity for marginal benefit. If a future slice introduces metrics that care about wasted subscribers, gating can be added then with a clear motivation.

## 4. Interface change

`modules/world/friends_client.go`:

```go
type FriendsClient interface {
    WorldConnect(ctx context.Context, worldID int32, profile string)
    // PlayerLogin registers the player on the friends server. onResponse is
    // invoked once after the RPC completes: accepted=true on success,
    // accepted=false on cap-reached or RPC error. May be nil.
    PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool))
    PlayerLogout(ctx context.Context, req *friendspb.PlayerLogoutRequest)
    // ... (rest unchanged)
}
```

`grpcFriendsClient.PlayerLogin` impl:

```go
// PlayerLogin registers the player on the friends server. The response's
// Accepted field is surfaced to onResponse (slice 4c retires
// NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED). onResponse may be nil.
func (c *grpcFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
    resp, err := c.client.PlayerLogin(ctx, req)
    if err != nil {
        c.log.Warn("PlayerLogin RPC failed",
            slog.Uint64("username37", req.Username37),
            slog.Any("err", err),
        )
        if onResponse != nil {
            onResponse(false)
        }
        return
    }
    if onResponse != nil {
        onResponse(resp.GetAccepted())
    }
}
```

`fakeFriendsClient.PlayerLogin` impl (`friends_client_fake_test.go`):

```go
// playerLoginAccepted is what onResponse will be called with. Defaults to
// true; tests set false to simulate cap-rejection. Read under mu.
playerLoginAccepted bool // initialised true in newFakeFriendsClient

// ...

func (f *fakeFriendsClient) PlayerLogin(ctx context.Context, req *friendspb.PlayerLoginRequest, onResponse func(accepted bool)) {
    select {
    case f.playerLoginReqs <- req:
    default:
    }
    f.mu.Lock()
    accepted := f.playerLoginAccepted
    f.mu.Unlock()
    if onResponse != nil {
        onResponse(accepted)
    }
}
```

## 5. Call site change

`modules/world/tick.go` lines 166-177 become:

```go
// NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS: register the player on
// the friends server when they enter the world. Mirrors TS's
// PLAYER_LOGIN-on-world-entry semantics.
if s.friendsClient != nil && p.username != "" {
    username37 := p.username37
    worldID := int32(s.cfg.NodeID)
    go s.friendsClient.PlayerLogin(context.Background(), &friendspb.PlayerLoginRequest{
        WorldId:     worldID,
        Username37:  username37,
        PrivateChat: int32(p.privateChat),
        StaffLvl:    p.staffModLevel,
    }, func(accepted bool) {
        if !accepted {
            // TS-faithful: friends-server cap rejection is logged on the
            // world side; the world's login flow is not interrupted. The
            // world's own player cap (addPlayer above) is the single
            // source of truth for "world full" from the client's view.
            s.log.Warn("friends-server rejected player login (cap reached or RPC error)",
                slog.Int("world_id", int(worldID)),
                slog.Uint64("username37", username37),
            )
        }
    })
}
```

The subscriber-start block at lines 179-186 is unchanged.

## 6. Tests

### 6.1 Server-side: no new tests

`TestHandler_PlayerLogin_PlayerCapAccepted_False` (`modules/friends/handler_test.go:96-118`) already pins the server returning `Accepted=false` when the cap is reached.

### 6.2 World-side unit tests (new)

**`TestGRPCFriendsClient_PlayerLogin_InvokesCallback`** in `friends_client_test.go`:

Two sub-cases driven by the existing `mockFriendsPBClient` mechanism:

- Mock returns `PlayerLoginResponse{Accepted: true}`, no err → callback receives `true`.
- Mock returns `PlayerLoginResponse{Accepted: false}`, no err → callback receives `false`.

Use a channel + 1-second timeout to receive the callback value, matching the codebase's existing `chan + select` test style.

**Update to `TestGRPCFriendsClient_LogsErrorOnFailure`**: the PlayerLogin entry passes `nil` for the callback (the test asserts the error-logging contract, not the callback). Add a separate assertion at the end of the PlayerLogin case (or a new test) that when callback is non-nil and the RPC errors, the callback receives `false`. The simplest split: leave the existing log-on-error test passing `nil`, and add a focused `TestGRPCFriendsClient_PlayerLogin_CallbackFalseOnError` that asserts `accepted=false` when the mock returns an error.

### 6.3 World-side integration test (new)

**`TestProcessLogins_FriendsPlayerLogin_LogsWarnOnRejection`** in `tick_friends_login_test.go`:

Set `fake.playerLoginAccepted = false`, run `processLogins`, drain `playerLoginReqs`, capture warn-level slog output via a test handler installed on `s.log`. Assert a record containing "friends-server rejected player login" with username37 and world_id attributes.

Use a `slog.NewTextHandler` writing into a `bytes.Buffer` (or a custom handler that pushes records onto a channel) to make the assertion deterministic. Existing tests use `discardLogger()`; this one needs a capturing logger.

Two existing tests are updated mechanically:

- `TestProcessLogins_FiresFriendsPlayerLogin`: no behavior change; `playerLoginAccepted` defaults to true so the existing assertion path is preserved. Drain the channel as before.
- `TestProcessLogins_EmptyUsername_NoFriendsRPC`: no change.

### 6.4 World-side e2e test (new)

**`TestFriendsClient_E2E_PlayerLoginCapRejected`** in `friends_smoke_test.go`:

Boots a real `friends.Friends` service with `WorldPlayerLimit: 1`, dials it through `NewFriendsClient`, calls `WorldConnect(worldID=10, profile="main")`, then issues two `PlayerLogin` RPCs:

1. First player (username37=1001): callback receives `true`.
2. Second player (username37=1002): callback receives `false`.

The callbacks are synchronised via a `chan bool` per call with a 5-second timeout. Mirrors the pattern in `TestFriendsClient_E2E_SubscribeUpdatesStream`.

### 6.5 Call-site sweep tests

Every existing test that constructs `fakeFriendsClient` and exercises a code path that ends up calling `PlayerLogin` needs to compile under the new signature. The fake's `PlayerLogin` change matches the interface mechanically — no test logic changes for cases that don't care about the callback. The sweep is a compile-driven mechanical update; the `playerLoginAccepted=true` default preserves existing assertions.

## 7. Sequencing

The signature change is atomic — it touches the interface, the production impl, the fake, the call site, and the existing tests in one task. Splitting across tasks leaves the package half-compiled. Subsequent tasks add behavior and tests on top of the new shape.

The doc-comment retirements (server-side `NAI-S1-D-PLAYERCAP-LOG-ONLY`; world-side `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` in both `friends_client.go:85` and `tick.go:168-169`) are doc-only and grouped into a tail task per the deviation-tag-retirement convention.

## 8. Deviation tags retired by slice 4c

| Tag | Site | Action |
|---|---|---|
| `NAI-S1-D-PLAYERCAP-LOG-ONLY` | `modules/friends/handler.go:59` doc-comment | Doc-only retirement — server already returns `Accepted` correctly; comment removed / updated. |
| `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` | `modules/world/friends_client.go:85` + `modules/world/tick.go:168-169` | Code change retires both: response is now passed through to the callback. |

## 9. Tags remaining open after slice 4c

- `NAI-S1-D-LAZY-WORLDINIT` (permanent, TS-faithful)
- `NAI-S1-D-PM-NO-PERSISTENCE` (slice 6 closes)
- `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent)
- `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent)
- `NAI-S3-D-USERNAME37-NOT-ACCOUNTID` (permanent, spec-only)
- `NAI-S3-D-NO-IN-MEMORY-CACHE` (permanent, spec-only)
- `NAI-S3-D-NO-LIST-CAPS` (permanent, spec-only)
- `NAI-S4A-D-DROP-ON-FULL` (permanent)
- `NAI-S4A-D-NO-INGAME-PACKET-EMIT` (blocked on NAI-182-D5)
- `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` (permanent)
- `NAI-S4B-D-NO-INGAME-PM-EMIT` (blocked on NAI-182-D5)

After slice 4c, the friends-server bridge arc has slices 5 (cross-world RELAY_*), 6 (chat logging), and 7 (per-login UUID) remaining. Slice 4c is the last cleanup of slice-1 and slice-2 carry-forwards.

## 10. Out of scope

- The MESSAGE_PRIVATE / UPDATE_FRIENDLIST / UPDATE_IGNORELIST ServerGameProt packet emits remain gated on NAI-182-D5 (not part of slice 4).
- Server-side `PlayerLogin` behavior is unchanged. The doc-comment retirement is the only server-side edit.
- The world's own player-cap check (`addPlayer` at `tick.go:154-159`) is unchanged. Slice 4c surfaces the friends-server's separate cap, not the world's.
- No new server-side test is added; existing coverage is sufficient.

## 11. References

- TS canonical: `Engine-TS/src/server/friend/FriendServer.ts:115-142` (PLAYER_LOGIN handler with early-return on cap-rejection).
- Server impl: `modules/friends/handler.go:55-77` (returns `Accepted` correctly today).
- Server test: `modules/friends/handler_test.go:96-118`.
- World interface: `modules/world/friends_client.go:23-39`.
- World impl: `modules/world/friends_client.go:84-93`.
- World fake: `modules/world/friends_client_fake_test.go:68-73`.
- World call site: `modules/world/tick.go:166-186`.
- World existing tests: `modules/world/tick_friends_login_test.go`.
- Slice 4b predecessor close: `[[friends-server-slice4b-close]]`.
