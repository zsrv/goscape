# Friends-server bridge slice 4b — PrivateMessage delivery via stream

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the friends-server `PrivateMessage` RPC through slice 4a's `subscriptions` registry so a PM sent by sender's world is delivered to the recipient's open `SubscribeUpdates` stream, retiring `NAI-S1-D-PM-NO-DELIVERY` and opening `NAI-S4B-D-NO-INGAME-PM-EMIT`.

**Architecture:** Server-only code change (~6 LOC body) inside `modules/friends/handler.go`; tests on both sides; doc-only tag-hygiene edits in two world-side files. World-side runtime wiring was pre-built by slice 4a (`friends_subscriber.dispatch` already routes `*FriendsUpdate_PrivateMessage`; `FriendsDispatcher.OnPrivateMessage` already declared; `slogFriendsDispatcher.OnPrivateMessage` already logs).

**Tech Stack:** Go 1.x, gRPC, `pkg/friendspb` proto, existing `modules/friends/subscriptions` registry (drop-on-full per `NAI-S4A-D-DROP-ON-FULL`), existing `modules/world/friends_smoke_test.go` in-process e2e fixture.

**Reference spec:** `docs/superpowers/specs/2026-05-21-friends-server-bridge-slice4b-design.md`

---

## Task 1: Add `recordingFriendsDispatcher.privateCalls()` accessor + `waitForPrivate` helper

**Why first:** Subsequent world-side tests (Task 6) depend on a snapshot-safe accessor to the recorder's `private` slice. Slice 4a only added `friendCalls()`; PM tests need the symmetric `privateCalls()`. Land the test infra before any test that consumes it. No production behavior change.

**Files:**
- Modify: `modules/world/friends_subscriber_test.go`

- [ ] **Step 1: Add `privateCalls()` accessor**

In `modules/world/friends_subscriber_test.go`, immediately after the existing `friendCalls()` accessor (currently lines 53-57), insert:

```go
func (d *recordingFriendsDispatcher) privateCalls() []privateCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]privateCall(nil), d.private...)
}
```

- [ ] **Step 2: Verify package still compiles**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestFriendsSubscriber_DispatchesPrivate ./modules/world/`

Expected: PASS (this test already exists in slice 4a and exercises `d.private`).

- [ ] **Step 3: Commit**

```bash
git add modules/world/friends_subscriber_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: add recordingFriendsDispatcher.privateCalls() accessor

Symmetric with the existing friendCalls() accessor. Slice 4b's world-side
e2e PM-delivery test in friends_smoke_test.go needs a snapshot-safe view
into the private []privateCall slice.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: TDD red — write server-side `TestPrivateMessage_DeliveredToRecipient`

**Why now:** Drive the handler change from a failing test. The existing slice-1 test `TestHandler_PrivateMessage_NoOp_Slice1` will be deleted in Task 4 (its semantic is obsolete); add the replacement first so coverage is continuous.

**Files:**
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 1: Add the test**

Append to `modules/friends/handler_test.go` (after the existing `TestHandler_PrivateMessage_NoOp_Slice1` — do not delete the old test yet, Task 4 retires it):

```go
// TestPrivateMessage_DeliveredToRecipient pins slice 4b's contract:
// the server's PrivateMessage RPC routes the message into the target's
// open SubscribeUpdates stream as a PrivateMessageDelivery update.
// Mirrors TS FriendServer.sendPrivateMessage (FriendServer.ts:480-497).
func TestPrivateMessage_DeliveredToRecipient(t *testing.T) {
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.Register(1, 200, 0, 0) // recipient online so the subscription can attach

	// Recipient subscribes; drain initial empty snapshots.
	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 1, Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	// Sender (100) PMs recipient (200).
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         2,
		PmId:             0xCAFEBABE,
		Chat:             "hello",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	// Recipient's stream should see a PrivateMessageDelivery.
	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	got := pm.PrivateMessage
	if got.FromUsername37 != 100 {
		t.Errorf("FromUsername37 = %d, want 100", got.FromUsername37)
	}
	if got.StaffLvl != 2 {
		t.Errorf("StaffLvl = %d, want 2", got.StaffLvl)
	}
	if got.PmId != 0xCAFEBABE {
		t.Errorf("PmId = %#x, want 0xCAFEBABE", got.PmId)
	}
	if got.Chat != "hello" {
		t.Errorf("Chat = %q, want %q", got.Chat, "hello")
	}
}
```

- [ ] **Step 2: Run test, confirm it fails**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestPrivateMessage_DeliveredToRecipient ./modules/friends/`

Expected: FAIL — the test reaches `stream.recvWithin` after `PrivateMessage` and times out (current handler is a no-op; nothing is sent to the stream). Failure message will be `timed out waiting for update`.

- [ ] **Step 3: Commit the red test**

```bash
git add modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: red test for slice-4b PrivateMessage stream delivery (T2)

TDD red. TestPrivateMessage_DeliveredToRecipient pins the slice-4b
contract: PrivateMessage RPC routes the message into the target's open
SubscribeUpdates stream as a PrivateMessageDelivery update. Fails today
because the handler is a no-op (slice 1's NAI-S1-D-PM-NO-DELIVERY).
T3 makes it pass.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: TDD green — implement server-side `PrivateMessage` delivery

**Files:**
- Modify: `modules/friends/handler.go` (lines 147-162)

- [ ] **Step 1: Replace the no-op body**

In `modules/friends/handler.go`, replace the entire block from line 147 to line 162 (the `PrivateMessage` method and its preceding doc-comment) with:

```go
// PrivateMessage routes a PM from req.Username37 to req.TargetUsername37
// by pushing PrivateMessageDelivery into the target's open stream (if
// any). Mirrors TS FriendServer.sendPrivateMessage (FriendServer.ts:480-
// 497): silently no-ops when the target has no open stream (TS:
// `if (!socket) return Promise.resolve()`). The registry's send method
// already implements the no-op-on-absent-subscriber semantic.
//
// req.Coord is unused server-side (TS parity — recipient's world does
// not need it to deliver the chat overlay). req.WorldId is unused for
// routing because the registry is keyed solely by username37; cross-
// world routing therefore falls out for free.
//
// Persistence of the PM to private_chat is slice 6.
// NAI-S1-D-PM-NO-PERSISTENCE — slice 6 retires.
func (h *handler) PrivateMessage(_ context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	h.ensureWorld(req.WorldId)
	h.subs.send(req.TargetUsername37, &friendspb.FriendsUpdate{
		Update: &friendspb.FriendsUpdate_PrivateMessage{
			PrivateMessage: &friendspb.PrivateMessageDelivery{
				FromUsername37: req.Username37,
				StaffLvl:       req.StaffLvl,
				PmId:           req.PmId,
				Chat:           req.Chat,
			},
		},
	})
	return &emptypb.Empty{}, nil
}
```

Notes for the implementer:
- The previous doc-comment said `NAI-S1-D-PM-NO-DELIVERY — slice 4 retires.` — that line is gone in this rewrite. Retirement is recorded by its removal here plus the tag-cleanup commit in Task 8.
- The `h.log.Debug("friends-server received private message", ...)` call from the old body is intentionally removed (matches slice 4a's silent steady-state for broadcast helpers).
- `slog` import becomes unused in this file if no other `slog.*` call remains. Verify with the build in Step 2; if the import is now unused, delete it from the `import` block.

- [ ] **Step 2: Run the formerly-red test, expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestPrivateMessage_DeliveredToRecipient ./modules/friends/`

Expected: PASS.

If the build complains about unused `slog` import, remove the `"log/slog"` line from `modules/friends/handler.go`'s import block, then re-run. Verify no other test breakage:

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/friends/ -count=1`

Expected: all friends tests pass except `TestHandler_PrivateMessage_NoOp_Slice1` — that test should still pass (the new handler still returns `(empty, nil)` so the old assertion still holds). Task 4 retires it.

- [ ] **Step 3: Commit**

```bash
git add modules/friends/handler.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: route PrivateMessage to target's subscription stream (T3)

Replaces the slice-1 no-op body with subs.send(target, PrivateMessageDelivery{...}).
Mirrors TS FriendServer.sendPrivateMessage (FriendServer.ts:480-497).
Recipient's world-side subscriber (slice 4a) already routes the
delivery to FriendsDispatcher.OnPrivateMessage; the in-game packet
emit remains gated on NAI-182-D5.

Retires NAI-S1-D-PM-NO-DELIVERY (final tag cleanup in T8).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Retire `TestHandler_PrivateMessage_NoOp_Slice1` and add `TestPrivateMessage_NoSubscription`

**Why:** The old test asserts "returns OK with no observable side effect". The first half is still true; the second half is now false in the general case but trivially true when there's no recipient subscription. Rename/reframe the test to assert the more-precise contract.

**Files:**
- Modify: `modules/friends/handler_test.go` (existing `TestHandler_PrivateMessage_NoOp_Slice1` at line 268)

- [ ] **Step 1: Replace the test**

In `modules/friends/handler_test.go`, locate `TestHandler_PrivateMessage_NoOp_Slice1` (currently around lines 268-283). Replace the entire function (from the `// ...` comment line immediately above `func TestHandler_PrivateMessage_NoOp_Slice1`, if any, through the closing `}`) with:

```go
// TestPrivateMessage_NoSubscription pins TS-faithful silent-drop on
// absent recipient subscription. Mirrors FriendServer.ts:482-484
// (`if (!socket) return Promise.resolve()`). The registry's send method
// implements the no-op (subscriptions.go:85-87).
func TestPrivateMessage_NoSubscription(t *testing.T) {
	h := newTestHandler(t)
	// No SubscribeUpdates call for the target — registry is empty for
	// username37=0xBBBB.
	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		StaffLvl:         0,
		PmId:             1,
		Chat:             "hi",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}
}
```

- [ ] **Step 2: Confirm tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run 'TestPrivateMessage' ./modules/friends/ -count=1 -v`

Expected: both `TestPrivateMessage_DeliveredToRecipient` and `TestPrivateMessage_NoSubscription` PASS. No reference to `TestHandler_PrivateMessage_NoOp_Slice1` in the output.

- [ ] **Step 3: Commit**

```bash
git add modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: replace NoOp_Slice1 test with NoSubscription pin (T4)

The slice-1 "accepted-and-logged" assertion is obsolete: PrivateMessage
now routes via subs.send. The remaining TS-faithful contract worth
pinning is silent-drop when the recipient has no open subscription
(FriendServer.ts:482-484). Renamed accordingly.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Add server-side cross-world routing test

**Why:** Pins that registry routing is `username37`-keyed (world-agnostic). Test 5.3 in the spec.

**Files:**
- Modify: `modules/friends/handler_test.go`

- [ ] **Step 1: Add the test**

Append to `modules/friends/handler_test.go`:

```go
// TestPrivateMessage_CrossWorld pins that registry routing is keyed
// solely by username37, so a PM from a sender on world 1 reaches a
// recipient subscribed on world 20.
func TestPrivateMessage_CrossWorld(t *testing.T) {
	r, _ := newTestRepo(t)
	log := noopLogger()
	cfg := Config{NodeProfile: "main", WorldPlayerLimit: 100}
	h := &handler{repo: r, subs: newSubscriptions(log), cfg: cfg, log: log}
	r.InitializeWorld(1, 100)
	r.InitializeWorld(20, 100)
	r.Register(1, 100, 0, 0)  // sender on world 1
	r.Register(20, 200, 0, 0) // recipient on world 20

	stream := newTestStream(t)
	errc := make(chan error, 1)
	go func() {
		errc <- h.SubscribeUpdates(&friendspb.SubscribeUpdatesRequest{WorldId: 20, Username37: 200}, stream)
	}()
	t.Cleanup(func() {
		stream.cancel()
		<-errc
	})
	stream.recvWithin(t, 2*time.Second) // empty friendlist snapshot
	stream.recvWithin(t, 2*time.Second) // empty ignorelist snapshot

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1, // sender's world
		Username37:       100,
		TargetUsername37: 200,
		StaffLvl:         0,
		PmId:             0xDEADBEEF,
		Chat:             "cross-world hi",
		Coord:            0,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	u := stream.recvWithin(t, 2*time.Second)
	pm, ok := u.Update.(*friendspb.FriendsUpdate_PrivateMessage)
	if !ok {
		t.Fatalf("update = %T, want FriendsUpdate_PrivateMessage", u.Update)
	}
	if pm.PrivateMessage.PmId != 0xDEADBEEF {
		t.Errorf("PmId = %#x, want 0xDEADBEEF", pm.PrivateMessage.PmId)
	}
	if pm.PrivateMessage.Chat != "cross-world hi" {
		t.Errorf("Chat = %q, want %q", pm.PrivateMessage.Chat, "cross-world hi")
	}
}
```

- [ ] **Step 2: Run, expect PASS**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestPrivateMessage_CrossWorld ./modules/friends/ -count=1 -v`

Expected: PASS. (The implementation already handles this — the test pins behavior.)

- [ ] **Step 3: Commit**

```bash
git add modules/friends/handler_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
friends: pin cross-world PM routing via username37-keyed registry (T5)

A future "key registry by (world, player)" refactor would silently
break this; the test makes the failure loud.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: World-side e2e — `TestFriendsClient_E2E_PrivateMessageDelivery`

**Why:** Pins the full e2e path through grpc: sender world → friends-server → recipient stream → world-side subscriber → dispatcher.

**Files:**
- Modify: `modules/world/friends_smoke_test.go`

- [ ] **Step 1: Add a `waitForPrivate` helper alongside `waitForFriendlistEntry`**

Append to `modules/world/friends_smoke_test.go` (after `waitForFriendlistEntryWithWorld` at the bottom of the file):

```go
// waitForPrivate polls disp for any PrivateMessage call whose PmId
// matches pmId. Returns the captured call within d, or false on
// timeout.
func waitForPrivate(t *testing.T, disp *recordingFriendsDispatcher, d time.Duration, pmId uint32) (privateCall, bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, c := range disp.privateCalls() {
			if c.PmId == pmId {
				return c, true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return privateCall{}, false
}
```

- [ ] **Step 2: Add the e2e test**

Append to `modules/world/friends_smoke_test.go`:

```go
// TestFriendsClient_E2E_PrivateMessageDelivery pins slice 4b end-to-end:
// world's PrivateMessage RPC -> friends-server PrivateMessage handler
// -> subs.send -> recipient stream -> world-side subscriber dispatch
// -> FriendsDispatcher.OnPrivateMessage.
func TestFriendsClient_E2E_PrivateMessageDelivery(t *testing.T) {
	port := freePort(t)
	cfg := friends.Config{
		GRPCListenAddress:       "127.0.0.1",
		GRPCListenPort:          port,
		NodeProfile:             "main",
		WorldPlayerLimit:        100,
		Enable:                  true,
		GracefulShutdownTimeout: 5 * time.Second,
		SQLiteDSN:               filepath.Join(t.TempDir(), "friends.db"),
	}
	log := discardLogger()
	svc, err := friends.New(cfg, log)
	if err != nil {
		t.Fatalf("friends.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := svc.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := svc.AwaitRunning(ctx); err != nil {
		t.Fatalf("AwaitRunning: %v", err)
	}
	t.Cleanup(func() {
		svc.StopAsync()
		_ = svc.AwaitTerminated(context.Background())
	})

	addr := "127.0.0.1:" + strconv.Itoa(port)
	client, err := NewFriendsClient(addr, log)
	if err != nil {
		t.Fatalf("NewFriendsClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	client.WorldConnect(ctx, 10, "main")

	// Recipient (2222) logs in and subscribes.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 2222, PrivateChat: 0, StaffLvl: 0,
	})

	disp := &recordingFriendsDispatcher{}
	sub := newFriendsSubscriber(client, 10, 2222, disp, log)
	subCtx, subCancel := context.WithCancel(ctx)
	t.Cleanup(subCancel)
	go sub.run(subCtx)

	// Sender (1111) logs in.
	client.PlayerLogin(ctx, &friendspb.PlayerLoginRequest{
		WorldId: 10, Username37: 1111, PrivateChat: 0, StaffLvl: 0,
	})

	// Sender PMs recipient.
	client.PrivateMessage(ctx, &friendspb.PrivateMessageRequest{
		WorldId:          10,
		Username37:       1111,
		TargetUsername37: 2222,
		StaffLvl:         0,
		PmId:             0xCAFEBABE,
		Chat:             "e2e hi",
		Coord:            0,
	})

	got, ok := waitForPrivate(t, disp, 2*time.Second, 0xCAFEBABE)
	if !ok {
		t.Fatalf("recipient did not see PM with PmId 0xCAFEBABE within 2s; got %d calls", len(disp.privateCalls()))
	}
	if got.From != 1111 {
		t.Errorf("From = %d, want 1111", got.From)
	}
	if got.Target != 2222 {
		t.Errorf("Target = %d, want 2222", got.Target)
	}
	if got.Chat != "e2e hi" {
		t.Errorf("Chat = %q, want %q", got.Chat, "e2e hi")
	}
}
```

- [ ] **Step 3: Run the e2e test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race -run TestFriendsClient_E2E_PrivateMessageDelivery ./modules/world/ -count=1 -v -timeout 60s`

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: e2e smoke for slice-4b PM delivery (T6)

End-to-end: world PrivateMessage RPC -> friends-server handler -> stream
-> world subscriber -> dispatcher.OnPrivateMessage. Uses in-process
friends.Friends with t.TempDir() SQLite, freePort, recordingFriends-
Dispatcher. Pattern mirrors TestFriendsClient_E2E_SubscribeUpdatesStream.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Open `NAI-S4B-D-NO-INGAME-PM-EMIT` on the dispatcher

**Files:**
- Modify: `modules/world/bridges.go`

- [ ] **Step 1: Update the `FriendsDispatcher` interface doc-comment**

In `modules/world/bridges.go`, replace the existing doc-comment block above `type FriendsDispatcher interface` (currently lines 67-76) with:

```go
// FriendsDispatcher is the world-side sink for server -> world friends
// updates received over the SubscribeUpdates stream. Production impl
// (slogFriendsDispatcher, below) logs each event at Debug; the
// in-game ServerGameProt packet emit (UPDATE_FRIENDLIST /
// UPDATE_IGNORELIST / MESSAGE_PRIVATE writes to the player's client
// connection) is gated on NAI-182-D5 (the "social cluster"
// ServerGameProt deferral noted at tick.go:226).
//
// NAI-S4A-D-NO-INGAME-PACKET-EMIT — friendlist/ignorelist methods;
//   retires when NAI-182-D5 retires and OnFriendlistUpdate /
//   OnIgnorelistUpdate are wired through to player.write(...).
// NAI-S4B-D-NO-INGAME-PM-EMIT — OnPrivateMessage; retires when
//   NAI-182-D5 retires and the dispatcher is wired through to
//   player.write(MessagePrivate{...}) per TS World.ts:2000.
```

- [ ] **Step 2: Update `slogFriendsDispatcher.OnPrivateMessage` doc-comment**

In `modules/world/bridges.go`, locate `func (d *slogFriendsDispatcher) OnPrivateMessage(...)` (currently line 106). Prepend (immediately above the func line) the doc-comment:

```go
// OnPrivateMessage logs the inbound PM at Debug. The MESSAGE_PRIVATE
// ServerGameProt packet write to player.client (mirroring TS
// World.ts:2000 `player.write(new MessagePrivate(...))`) is gated on
// NAI-182-D5 (social-cluster ServerGameProt port) — see
// NAI-S4A-D-NO-INGAME-PACKET-EMIT on the interface for the parallel
// friendlist/ignorelist gating.
//
// NAI-S4B-D-NO-INGAME-PM-EMIT — retires when NAI-182-D5 retires and
// the dispatcher is wired through to player.write(MessagePrivate{...}).
```

- [ ] **Step 3: Build + re-run world tests**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 180s`

Expected: PASS (doc-only change; behavior unchanged).

- [ ] **Step 4: Commit**

```bash
git add modules/world/bridges.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: open NAI-S4B-D-NO-INGAME-PM-EMIT on dispatcher (T7)

Mirrors NAI-S4A-D-NO-INGAME-PACKET-EMIT (friendlist/ignorelist) for
the PM dispatch method. Both blocked on NAI-182-D5 (social-cluster
ServerGameProt port). Doc-only.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Retire `NAI-S1-D-PM-NO-DELIVERY` from remaining doc references

**Why:** Two references survive after T3: a stale exemplar in `modules/world/friends_client.go:20` and three carry-forward listings in resume notes. Clean up the live `.go` reference. The spec docs and resume notes are historical record; leave them.

**Files:**
- Modify: `modules/world/friends_client.go` (lines 18-21)

- [ ] **Step 1: Locate the stale reference**

The current doc-comment block in `modules/world/friends_client.go` reads (around lines 18-21):

```go
// All RPCs except Close and SubscribeUpdates are fire-and-forget: errors
// are logged via the embedded *slog.Logger and swallowed. The friends-server
// is best-effort by design (slice 1's NAI-S1-D-PM-NO-DELIVERY etc.);
// the world does not depend on its responses through slice 3.
```

- [ ] **Step 2: Replace with still-valid exemplar + drop slice-3 bound**

Replace those lines with:

```go
// All RPCs except Close and SubscribeUpdates are fire-and-forget: errors
// are logged via the embedded *slog.Logger and swallowed. The friends-
// server is best-effort by design — slice-1 and slice-2 deviation tags
// (e.g. NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS) document the posture; the
// world does not depend on its responses.
```

Rationale: `NAI-S1-D-PM-NO-DELIVERY` was the cited exemplar but it's just been retired (T3). `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` is permanent (per [[friends-server-slice4a-close]]) and captures the same "best-effort, world doesn't depend on response" posture. The "through slice 3" timestamp is now stale — drop it (the property remains true across all subsequent slices since the bridge interface hasn't changed shape).

- [ ] **Step 3: Verify world package still builds + tests pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/ -count=1 -timeout 180s`

Expected: PASS.

- [ ] **Step 4: Audit no other `NAI-S1-D-PM-NO-DELIVERY` references in live `.go` source**

Run: `grep -rn "NAI-S1-D-PM-NO-DELIVERY" --include="*.go" .`

Expected: zero matches. (References in `docs/` and `.claude/resume/` are historical and stay.)

If any match appears, investigate and remove it before continuing.

- [ ] **Step 5: Commit**

```bash
git add modules/world/friends_client.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
world: retire NAI-S1-D-PM-NO-DELIVERY exemplar in FriendsClient doc (T8)

The tag closed in T3 (PrivateMessage now routes via subs.send). Cite
NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS instead — permanent and captures the
same best-effort posture. Drop the stale "through slice 3" bound.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Full-project gate + smoke-pack

**Why:** Slice-4a discipline: structural changes get a full `go test -race ./...` + smoke-pack pass before closing.

**Files:** (none modified)

- [ ] **Step 1: Run full test suite with race detector**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./... -count=1 -timeout 600s`

Expected: all PASS. Duration ~150s based on slice 4a baseline.

If anything fails, stop and investigate. Common failure modes:
- Unused `slog` import in `modules/friends/handler.go` (T3 should have caught this; verify imports).
- Race between sender PM RPC and recipient subscribe — handler test should drain initial snapshots before asserting.

- [ ] **Step 2: Run smoke-pack**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape-cli smoke-pack --content-dir /home/owner/Code/github.com/LostCityRS/content`

Expected: `12 OK / 0 ERR / 0 SKIP` in approximately 8 seconds. The 12 stages are unrelated to friends-server but the baseline must hold (per slice 4a close).

- [ ] **Step 3: Confirm working tree**

Run: `git status --short`

Expected output (besides pre-existing Makefile WIP + untracked dotfiles): no unstaged `M` lines under `modules/`, `pkg/`, or `docs/superpowers/`.

- [ ] **Step 4: Confirm commit log**

Run: `git log --oneline 0863e23a..HEAD`

Expected: 9 commits — spec (2cdb1925) + T1..T8 in order. Each commit message references its task number except the spec.

This task has no commit of its own — it's a gate.

---

## Task 10: Write close memory + index entry

**Why:** Slice-close ritual per `[[friends-server-slice4a-close]]` precedent. Without it, slice 4c won't know what 4b changed.

**Files:**
- Create: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice4b_close.md`
- Modify: `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md` (prepend entry)

- [ ] **Step 1: Write the close memory file**

Write `friends_server_slice4b_close.md` with this content (substitute `<HEAD>` with the actual HEAD SHA from `git rev-parse --short HEAD` and `<COMMIT-RANGE>` with `2cdb1925..<HEAD>` after T9 passes):

```markdown
---
name: friends-server-slice4b-close
description: PrivateMessage delivery via stream shipped 2026-05-21; retires NAI-S1-D-PM-NO-DELIVERY, opens NAI-S4B-D-NO-INGAME-PM-EMIT
metadata:
  type: project
---

Slice 4b of 7 of the friends-server bridge arc shipped 2026-05-21 across <N> commits <COMMIT-RANGE> on top of [[friends-server-slice4a-close]]. Server-only code change inside `modules/friends/handler.go`: `PrivateMessage` no-op replaced with `h.subs.send(target, FriendsUpdate{PrivateMessage: PrivateMessageDelivery{...}})`; world-side wiring (`friends_subscriber.dispatch`, `FriendsDispatcher.OnPrivateMessage`, `slogFriendsDispatcher.OnPrivateMessage`) was pre-built in slice 4a.

Retires `NAI-S1-D-PM-NO-DELIVERY`. Opens `NAI-S4B-D-NO-INGAME-PM-EMIT` (`slogFriendsDispatcher.OnPrivateMessage` logs but does not write `MESSAGE_PRIVATE` ServerGameProt packet — blocked on NAI-182-D5; mirror of `NAI-S4A-D-NO-INGAME-PACKET-EMIT`).

Tests: 3 new server-side (`TestPrivateMessage_DeliveredToRecipient`, `TestPrivateMessage_NoSubscription`, `TestPrivateMessage_CrossWorld`); 1 new world-side e2e (`TestFriendsClient_E2E_PrivateMessageDelivery`); replaced `TestHandler_PrivateMessage_NoOp_Slice1` (obsolete assertion). `recordingFriendsDispatcher.privateCalls()` accessor + `waitForPrivate` helper added. `-race ./...` clean. `smoke-pack` 12 OK / 0 ERR holds.

Decision logged: Option B from the 4b resume — world-side in-game emit stays gated, new dedicated tag opened. Option A (wire NAI-182-D5 emit in 4b) was rejected to preserve slice-4a's discipline of "server-side fan-out is real, world-side ingame-emit is gated on the social-cluster ServerGameProt port."

Cross-world routing pinned by `TestPrivateMessage_CrossWorld` (sender world 1, recipient world 20, registry username37-keyed). A future "key registry by (world, player)" refactor would break loudly.

Slice 4b's mechanical simplicity (~150 LOC added, ~30 deleted) is a slice-4a dividend: pre-building the world-side dispatcher + subscriber dispatch arms in 4a meant 4b became server-only. The decomposition 4a/4b/4c continues to age well — 4c is now the last sub-slice (PlayerLoginResponse.Accepted handling, retires 2 tags `NAI-S1-D-PLAYERCAP-LOG-ONLY` + `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED`).

After 4b: still open `NAI-S1-D-LAZY-WORLDINIT` (permanent), `NAI-S1-D-PLAYERCAP-LOG-ONLY` (4c), `NAI-S1-D-PM-NO-PERSISTENCE` (6), `NAI-S2-D-PLAYERLOGIN-AT-PROCESSLOGINS` (permanent), `NAI-S2-D-PLAYERLOGOUT-BOTH-PATHS` (permanent), `NAI-S2-D-PLAYERLOGIN-IGNORES-ACCEPTED` (4c), `NAI-S3-D-*` (3, permanent), `NAI-S4A-D-DROP-ON-FULL` (permanent), `NAI-S4A-D-NO-INGAME-PACKET-EMIT` (NAI-182-D5), `NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM` (permanent), `NAI-S4B-D-NO-INGAME-PM-EMIT` (NAI-182-D5).
```

- [ ] **Step 2: Add MEMORY.md index entry**

In `/home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/MEMORY.md`, prepend a new line at the very top of the bullet list (above the existing `friends-server slice 4a close` line):

```markdown
- [friends-server slice 4b close](friends_server_slice4b_close.md) — PrivateMessage delivery via stream shipped 2026-05-21 on top of [[friends-server-slice4a-close]]; server-only change to `modules/friends/handler.go` (subs.send route); retires `NAI-S1-D-PM-NO-DELIVERY`; opens `NAI-S4B-D-NO-INGAME-PM-EMIT` (mirror of `NAI-S4A-D-NO-INGAME-PACKET-EMIT`, blocked on NAI-182-D5); 3 server tests + 1 world e2e; -race clean; smoke-pack 12 OK / 0 ERR holds
```

Keep the line under 200 chars (per MEMORY.md warning note).

- [ ] **Step 3: No git commit for memory files**

Memory files live outside the project repo (under `~/.claude/projects/.../memory/`) so there is no commit here. Verify with:

Run: `ls -la /home/owner/.claude/projects/-home-owner-Code-github-com-zsrv-goscape/memory/friends_server_slice4b_close.md`

Expected: file present.

---

## Self-review

**Spec coverage:**
- §2 forward map (handler change + tests + doc tags) → T3 + T7 + T8 + T2/T4/T5/T6.
- §4 server-side change → T3 (verbatim doc-comment text propagated).
- §5 three server tests → T2, T4, T5.
- §6 world-side e2e + `privateCalls()` accessor + `waitForPrivate` helper → T1 + T6.
- §7 doc-only changes → T7 (open NAI-S4B-D-NO-INGAME-PM-EMIT on bridges.go) + T8 (retire stale exemplar in friends_client.go).
- §9 architectural notes → encoded in test design (no-sub silent drop in T4; cross-world in T5; Option B in T7).
- §10 deviation inventory → T7 opens, T3 retires.

**Placeholder scan:** None.

**Type consistency:** `PrivateMessageDelivery` fields (`FromUsername37`, `StaffLvl`, `PmId`, `Chat`) match the proto (verified against `pkg/friendspb/friends.pb.go:926-993`). `subs.send(uint64, *FriendsUpdate)` signature matches `subscriptions.go:81`. `privateCall` struct fields (`Target`, `From`, `StaffLvl`, `PmId`, `Chat`) match `friends_subscriber_test.go:30-35`. `newFriendsSubscriber(client, worldID, username37, dispatcher, log)` signature matches existing `friends_smoke_test.go:177` call site.

**Task ordering:** T1 lands the test accessor before T6 consumes it. T2 (red) precedes T3 (green). T4 retires the obsolete test only after T3 makes the new green. T7 (open new tag) and T8 (retire old tag) are independent doc changes that could run in either order; sequence is fine. T9 gate runs after all code changes. T10 close-memory is documentation only.
