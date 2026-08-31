# Chat Kafka-Only Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `public_chat`/`private_chat` tables from the goscape central DB and make the telemetry event pipeline (Kafka → ClickHouse) the only chat sink, on all 5 rev branches.

**Architecture:** Public chat: the world module's existing `ChatMessageEvent` emission is renamed to `PublicChatEvent`, enriched with `session_uuid`+`coord`, and the friends-server `PublicMessage` RPC (whose only job was the DB insert) is retired end-to-end. Private chat: the friends `PrivateMessage` handler keeps its TS account-resolution silent-drop semantics but emits a `PlayerInputEnvelope{PrivateChatEvent}` instead of inserting. A `000002_drop_chat` migration drops both tables.

**Tech Stack:** Go 1.26+, buf (protobuf codegen via `make protos`), golang-migrate (embedded iofs migrations), sqlite (`modernc`) + postgres backends.

**Spec:** `docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md` (read it first; it records the approved decisions and the fidelity-divergence rationale).

## Global Constraints

- Every `go` invocation: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go ...`; race runs additionally need `CGO_ENABLED=1`.
- Every commit: `git commit --no-gpg-sign`, message trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- This is a **documented TS-fidelity divergence**: every removal site's replacement comment must cite the spec path (the code blocks below already do — keep those citations).
- Delivery guarantee is fire-and-forget per the existing `telemetry.Emitter` contract ("must not block") — do NOT add blocking/retry logic.
- Do NOT touch `000001_init.up.sql`; the drop is a new `000002` migration.
- Tasks 1–5 run on branch `rev-274` in `~/Code/github.com/zsrv/goscape`. Tasks 6–9 run in the sibling worktrees (`goscape-rev254`, `goscape-rev245.2`, `goscape-rev244`, `goscape-rev225`).
- Never inspect generated artifacts across worktrees; regenerate protos inside whichever worktree you're editing (`make protos` there).
- After editing files in a sibling worktree, verify with `git -C <worktree> status` that the edits actually landed there (sandbox writes to sibling worktrees may be blocked or silently no-op; if blocked, rerun the failing command with sandbox off).
- Tests: never call `t.Context()` inside `t.Cleanup` callbacks.

---

### Task 1: `PublicChatEvent` proto rename + single enriched world emission

Public chat currently has TWO sinks in `handleMessagePublic`: a `ChatMessageEvent` telemetry emission (~line 388) and a `friendsBridge.PublicMessage` audit call (~line 428) that becomes a DB row. This task renames/extends the event and merges both sites into one emission at the audit-call position. The bridge plumbing itself is deleted in Task 2.

**Files:**
- Modify: `proto/events/v1/world.proto`
- Regenerate: `pkg/eventspb/world.pb.go` (via `make protos` — never hand-edit)
- Modify: `modules/world/handlers_game.go` (~lines 385–430, `handleMessagePublic`)
- Modify: `modules/world/handler_message_public_test.go`

**Interfaces:**
- Consumes: existing `telemetry.Get().EmitWorld(*eventspb.WorldEnvelope)`, `coordgrid.PackCoord(level, x, z) int`, `p.sessionOrHeadless() string`, `p.accountID int64`.
- Produces: `eventspb.PublicChatEvent{Text, SessionUuid string; Coord int32}`, oneof wrapper `eventspb.WorldEnvelope_PublicChat{PublicChat *eventspb.PublicChatEvent}`, accessor `env.GetPublicChat()`. Test helper `captureEmitter` (world package) with `worldEnvs`/`playerInputEnvs` slices — Tasks 2–3 reuse it.

- [ ] **Step 1: Edit the proto**

In `proto/events/v1/world.proto`, change the oneof entry (field number 103 stays):

```proto
    PublicChatEvent public_chat = 103;
```

and replace the `ChatMessageEvent` message with:

```proto
// PublicChatEvent is the only record of a public-chat utterance — chat
// is Kafka-only (documented TS divergence: TS persists a public_chat DB
// row instead; spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md).
message PublicChatEvent {
  // Field 1 was ChatMessageEvent.Channel; the enum is retired (these
  // revisions are 2004-05 era — clan chat does not exist).
  reserved 1;
  // WordPack-decoded, UNFILTERED text (TS sets player.logMessage BEFORE
  // the wordenc filter, MessagePublicHandler.ts:32). Kept on field 2 so
  // in-flight Kafka messages decode across the rename.
  string text = 2;
  // Player.session per-login UUID; 'headless' when unassigned (TS
  // Player.ts ctor default). No session-validity gate.
  string session_uuid = 3;
  // coordgrid.PackCoord(level, x, z) at utterance.
  int32 coord = 4;
}
```

- [ ] **Step 2: Regenerate + confirm the expected breakage**

Run: `make protos && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | head -20`
Expected: compile FAILS only in `modules/world/handlers_game.go` (references to `eventspb.ChatMessageEvent`, `WorldEnvelope_Chat`, `ChatMessageEvent_CHANNEL_PUBLIC`). If anything else breaks, stop and list it before proceeding.

- [ ] **Step 3: Rewrite the tests to pin the new single-emission contract**

In `modules/world/handler_message_public_test.go`:

Replace the imports block's `encfilter`/`wordpack` set with (keep existing, add three):

```go
import (
	"bytes"
	"testing"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/telemetry"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)
```

Add the shared capture emitter (mutexed — the friends smoke tests in Task 3 capture from a gRPC server goroutine). Put it in this file; it is package-scoped:

```go
// captureEmitter records emitted envelopes for assertions. Safe for
// cross-goroutine use (smoke tests capture from gRPC handler goroutines).
type captureEmitter struct {
	mu              sync.Mutex
	worldEnvs       []*eventspb.WorldEnvelope
	playerInputEnvs []*eventspb.PlayerInputEnvelope
}

func (c *captureEmitter) EmitAuth(*eventspb.AuthEnvelope) {}
func (c *captureEmitter) EmitWorld(env *eventspb.WorldEnvelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.worldEnvs = append(c.worldEnvs, env)
}
func (c *captureEmitter) EmitPlayerInput(env *eventspb.PlayerInputEnvelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playerInputEnvs = append(c.playerInputEnvs, env)
}
func (c *captureEmitter) EmitWealth(*eventspb.WealthEnvelope) {}

// publicChats returns the captured WorldEnvelopes carrying a PublicChatEvent.
func (c *captureEmitter) publicChats() []*eventspb.WorldEnvelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []*eventspb.WorldEnvelope
	for _, e := range c.worldEnvs {
		if e.GetPublicChat() != nil {
			out = append(out, e)
		}
	}
	return out
}
```

(add `"sync"` to the imports.)

Replace `commonMessagePublicSetup` — it now installs the capture emitter instead of recording bridges:

```go
// commonMessagePublicSetup wires a player against a server and installs
// a capture telemetry emitter. Chat is Kafka-only (spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md): the
// friends-bridge audit path is retired, so assertions read the emitter.
func commonMessagePublicSetup(t *testing.T) (*Player, *captureEmitter) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	p.session = "uuid-sess-1"
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)
	return p, cap
}
```

Rewrite `TestHandleMessagePublic_FiresFriendsBridge` as `TestHandleMessagePublic_EmitsPublicChatEvent`:

```go
// TestHandleMessagePublic_EmitsPublicChatEvent pins that a valid
// public-chat utterance emits exactly one PublicChatEvent with the
// (session_uuid, coord, decoded message) tuple TS used to persist to
// public_chat (World.ts:1567-1574 @2e3bcf43). Chat is Kafka-only —
// documented TS divergence, spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md.
func TestHandleMessagePublic_EmitsPublicChatEvent(t *testing.T) {
	p, cap := commonMessagePublicSetup(t)
	// Move the player to a known coord so PackCoord output is deterministic.
	p.level, p.x, p.z = 0, 3210, 3210

	payload := packPublicChatPayload(0, 0, "hi")
	if err := handleMessagePublic(p, payload); err != nil {
		t.Fatalf("handleMessagePublic: %v", err)
	}

	envs := cap.publicChats()
	if len(envs) != 1 {
		t.Fatalf("PublicChatEvent envelopes: got %d, want 1", len(envs))
	}
	got := envs[0].GetPublicChat()
	// rev-254 A3: session UUID, not username
	if got.SessionUuid != "uuid-sess-1" {
		t.Errorf("SessionUuid: got %q, want uuid-sess-1 (rev-254 A3 re-key; TS World.ts:1567-1574 @2e3bcf43)", got.SessionUuid)
	}
	wantCoord := int32(coordgrid.PackCoord(0, 3210, 3210))
	if got.Coord != wantCoord {
		t.Errorf("Coord: got %d, want %d", got.Coord, wantCoord)
	}
	if got.Text != "Hi" { // wordpack.Unpack applies sentence-case to "hi"
		t.Errorf("Text: got %q, want %q (sentence-cased)", got.Text, "Hi")
	}
	if envs[0].AccountId != p.accountID {
		t.Errorf("AccountId: got %d, want %d", envs[0].AccountId, p.accountID)
	}
}
```

Rewrite the three `TestPublicChatLog_SessionKeyed` subtests with the same substitution pattern — `rec.publicMsgs` → `cap.publicChats()`, `.sessionUUID` → `.GetPublicChat().SessionUuid`. Keep every existing behavioral pin and its TS citation comment:

```go
func TestPublicChatLog_SessionKeyed(t *testing.T) {
	t.Run("session_uuid_carried", func(t *testing.T) {
		p, cap := commonMessagePublicSetup(t)
		p.level, p.x, p.z = 0, 3200, 3200
		payload := packPublicChatPayload(0, 0, "hello")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		envs := cap.publicChats()
		if len(envs) != 1 {
			t.Fatalf("PublicChatEvent envelopes: got %d, want 1", len(envs))
		}
		if got := envs[0].GetPublicChat().SessionUuid; got != "uuid-sess-1" {
			t.Errorf("SessionUuid: got %q, want uuid-sess-1 (keyed by session UUID, not username)", got)
		}
	})

	t.Run("empty_session_emits_as_headless", func(t *testing.T) {
		// No session gate: TS only gates on logMessage != null. An
		// unassigned session ("" in Go) relays as 'headless' (the TS
		// Player.ts:311 ctor default).
		p, cap := commonMessagePublicSetup(t)
		p.session = ""
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		envs := cap.publicChats()
		if len(envs) != 1 {
			t.Fatalf("PublicChatEvent envelopes: got %d, want 1 (no session gate, TS World.ts:629-631 @2e3bcf43)", len(envs))
		}
		if got := envs[0].GetPublicChat().SessionUuid; got != "headless" {
			t.Errorf("SessionUuid: got %q, want headless (TS ctor default)", got)
		}
		// In-world propagation must still fire.
		if p.chatBytes == nil {
			t.Errorf("p.chatBytes: got nil, want non-nil (Chat must fire regardless of session)")
		}
	})

	t.Run("headless_session_still_emits", func(t *testing.T) {
		// 'headless' sessions emit too — no session-validity gate.
		p, cap := commonMessagePublicSetup(t)
		p.session = "headless"
		payload := packPublicChatPayload(0, 0, "hi")
		if err := handleMessagePublic(p, payload); err != nil {
			t.Fatalf("handleMessagePublic: %v", err)
		}
		if got := len(cap.publicChats()); got != 1 {
			t.Errorf("PublicChatEvent envelopes: got %d, want 1 (no session gate at 254)", got)
		}
	})
}
```

In `TestHandleMessagePublic_AppliesWordEncFilterToChatBytes`, replace the trailing audit-log assertion block (the `rec.publicMsgs` checks) with:

```go
	// The emitted event MUST carry the unfiltered text (mirrors TS
	// player.logMessage at MessagePublicHandler.ts:32, set BEFORE filtering).
	envs := cap.publicChats()
	if len(envs) != 1 {
		t.Fatalf("expected 1 PublicChatEvent, got %d", len(envs))
	}
	if got := envs[0].GetPublicChat().Text; got != "Anal" {
		t.Errorf("event text: got %q, want %q (unfiltered, sentence-cased)", got, "Anal")
	}
```

(and change its setup line to `p, cap := commonMessagePublicSetup(t)`.)

- [ ] **Step 4: Run the world tests to verify they fail for the right reason**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run 'MessagePublic|PublicChat' -v 2>&1 | tail -20`
Expected: compile error in `handlers_game.go` (still references the deleted `ChatMessageEvent` identifiers) — the implementation hasn't been updated yet.

- [ ] **Step 5: Merge the two emission sites in `handleMessagePublic`**

In `modules/world/handlers_game.go`:

(a) DELETE the early NAI-Phase2 emission block — the comment starting `// NAI-Phase2: emit ChatMessageEvent (public-channel only …` and the entire `telemetry.Get().EmitWorld(&eventspb.WorldEnvelope{ … ChatMessageEvent … })` call under it.

(b) REPLACE the audit block — the long comment starting `// Audit-log to friends-server with the UNFILTERED decoded text …` down through the closing brace of `{ s := p.client.server; coord := …; s.friendsBridge.PublicMessage(…) }` — with:

```go
	// Chat is Kafka-only (documented TS divergence, spec
	// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md). TS
	// stores player.logMessage (UNFILTERED, MessagePublicHandler.ts:32,
	// BEFORE filter), drains it once per tick (World.ts:630 @2e3bcf43)
	// and persists {session_uuid, coord, message} to public_chat via the
	// friend server (World.ts:1567-1574). goscape emits one
	// PublicChatEvent inline instead — same tuple, same
	// no-session-gate posture ('headless' sessions emit too,
	// World.ts:629-631); the public_chat table and the PublicMessage
	// RPC are retired.
	{
		s := p.client.server
		coord := coordgrid.PackCoord(p.level, p.x, p.z)
		telemetry.Get().EmitWorld(&eventspb.WorldEnvelope{
			SchemaVersion: 1,
			EventId:       uuid.NewString(),
			Ts:            timestamppb.Now(),
			WorldId:       int32(s.cfg.NodeID),
			AccountId:     p.accountID,
			Payload: &eventspb.WorldEnvelope_PublicChat{
				PublicChat: &eventspb.PublicChatEvent{
					Text:        decoded,
					SessionUuid: p.sessionOrHeadless(),
					Coord:       int32(coord),
				},
			},
		})
	}
```

Note this KEEPS the `s.friendsBridge` field and interface untouched (Task 2's job) — only the call site goes.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ 2>&1 | tail -5`
Expected: PASS (whole package — nothing else asserted the bridge call from this handler; if another test fails on `publicMsgs` being empty, that test belongs to Task 2's plumbing and should be examined, not blindly deleted).

- [ ] **Step 7: Commit**

```bash
git add proto/events/v1/world.proto pkg/eventspb/world.pb.go modules/world/handlers_game.go modules/world/handler_message_public_test.go
git commit --no-gpg-sign -m "feat(events): PublicChatEvent replaces ChatMessageEvent; single enriched emission

Renames the world chat event, retires the Channel enum (no clan chat in
2004-05 era; field 1 reserved), adds session_uuid+coord, and merges the
NAI-Phase2 emission with the friends-bridge audit call site into one
telemetry emission. Spec: docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Retire the `PublicMessage` plumbing end-to-end

After Task 1 nothing calls `friendsBridge.PublicMessage`. Delete the whole chain: bridge interface method → grpc client method → friends RPC → repository insert, plus the proto surface and every test of the deleted pieces.

**Files:**
- Modify: `proto/friends/friends.proto` (delete `rpc PublicMessage` + `message PublicMessageRequest`)
- Regenerate: `pkg/friendspb/*.pb.go` (via `make protos`)
- Modify: `modules/world/bridges.go` (interface method ~lines 35–47, `noopBridges` impl ~line 325, `grpcFriendsBridge` impl ~lines 502–513)
- Modify: `modules/world/friends_client.go` (interface method ~line 56, impl ~lines 226–229)
- Modify: `modules/world/bridges_test.go` (`recordedPublicMessageCall`, `publicMsgs` field, recording impl, the `b.PublicMessage("alice", 0, "msg")` line in `TestNoopBridgesAllMethods`, any `publicMsgs` assertions)
- Modify: `modules/world/friends_smoke_test.go` (delete `TestFriendsClient_E2E_PublicMessagePersistsRow`, ~lines 956–1045)
- Modify: `modules/friends/handler.go` (delete the `PublicMessage` handler)
- Modify: `modules/friends/repository.go` (delete `LogPublicMessage`)
- Modify: `modules/friends/repository_test.go` (delete `TestRepository_LogPublicMessage_PersistsRow`, `TestRepository_LogPublicMessage_AppendOnly`, `TestRepository_LogPublicMessage_EmptyMessageAllowed`, `TestLogPublicMessage_TSRowShape`)

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing — pure removal. After this task `grep -rn "PublicMessage" --include="*.go" modules/ pkg/friendspb/` must return zero hits (generated code included).

- [ ] **Step 1: Delete the proto surface and regenerate**

In `proto/friends/friends.proto` delete the `rpc PublicMessage(...)` line together with its doc comment block (~lines 28–35), and the whole `message PublicMessageRequest { … }` (~lines 150–170). Then:

Run: `make protos && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... 2>&1 | head -30`
Expected: compile errors ONLY at the four known Go sites (bridges.go, friends_client.go, friends/handler.go, and test files). These errors are the deletion worklist.

- [ ] **Step 2: Delete the Go plumbing, guided by the compile errors**

- `modules/world/bridges.go`: remove the `PublicMessage(sessionUUID string, coord int, message string)` interface method and its full doc comment; remove `func (noopBridges) PublicMessage(...)`; remove `func (b *grpcFriendsBridge) PublicMessage(...)`.
- `modules/world/friends_client.go`: remove `PublicMessage` from the client interface and the `grpcFriendsClient.PublicMessage` method.
- `modules/friends/handler.go`: remove the `PublicMessage` handler and its doc comment (the gRPC service interface regenerated in Step 1 no longer declares it).
- `modules/friends/repository.go`: remove `LogPublicMessage` and its doc comment.
- If the compiler flags a now-unused import anywhere, remove it.

- [ ] **Step 3: Delete the tests of the deleted surface**

- `modules/world/bridges_test.go`: remove `recordedPublicMessageCall`, the `publicMsgs` field, `recordingBridges.PublicMessage`, and the single line `b.PublicMessage("alice", 0, "msg")` inside `TestNoopBridgesAllMethods`. Run `grep -n publicMsgs modules/world/*.go` — must be empty.
- `modules/world/friends_smoke_test.go`: delete the whole `TestFriendsClient_E2E_PublicMessagePersistsRow` function.
- `modules/friends/repository_test.go`: delete the four `LogPublicMessage` test functions listed above (including the `// --- public_chat persistence (follow-up post-slice-7) ---` banner).
- Any fake friends-service impl in tests that the compiler flags for a missing/extra `PublicMessage` method: remove that method.

- [ ] **Step 4: Verify no survivors, then run the tests**

Run: `grep -rn "PublicMessage" --include="*.go" modules/ pkg/friendspb/ | grep -v "PrivateMessage"`
Expected: no output.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/... ./pkg/... 2>&1 | tail -10`
Expected: PASS. (`public_chat` DB tests were deleted; the table itself is dropped in Task 4.)

- [ ] **Step 5: Commit**

```bash
git add -A proto/friends pkg/friendspb modules/world modules/friends
git commit --no-gpg-sign -m "refactor(friends): retire PublicMessage RPC end-to-end

The RPC's only job was the public_chat insert; with chat Kafka-only
(spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md) the
bridge method, grpc client method, RPC, request message, handler, and
LogPublicMessage are all dead. Pure removal.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Private chat — emit `PrivateChatEvent` from the friends handler

Replace the `private_chat` INSERT with a telemetry emission while preserving the TS resolve-both-accounts silent-drop semantics (`FriendServer.ts:266-284`). The recipient's account id exists only here — that's why emission is friends-side, not world-side.

**Files:**
- Modify: `proto/events/v1/player_input.proto` (add `coord` to `PrivateChatEvent`)
- Regenerate: `pkg/eventspb/player_input.pb.go` (via `make protos`)
- Modify: `modules/friends/repository.go` (`LogPrivateMessage` → `ResolvePrivateMessageEndpoints`)
- Modify: `modules/friends/handler.go` (`PrivateMessage` handler)
- Modify: `modules/friends/handler_test.go` (new emission tests + capture emitter)
- Modify: `modules/friends/repository_test.go` (reshape the `LogPrivateMessage` tests)
- Modify: `modules/world/friends_smoke_test.go` (rewrite `TestFriendsClient_E2E_PrivateMessagePersistsRow` → `…EmitsEvent`)

**Interfaces:**
- Consumes: `seedAccount(t, db *gamedb.DB, username37 uint64) int64` (repository_test.go:1030), `h.repos.db`, `errAccountMissing`, `r.accountID(ctx, username37) (int64, bool, error)`, the `captureEmitter` from Task 1 (world package; friends gets its own copy below).
- Produces: `(*Repository) ResolvePrivateMessageEndpoints(ctx context.Context, from, to uint64) (int64, int64, error)`; `eventspb.PrivateChatEvent` gains `Coord int32`.

- [ ] **Step 1: Extend the proto**

In `proto/events/v1/player_input.proto`, replace `PrivateChatEvent`:

```proto
// PrivateChatEvent is the only record of a PM — chat is Kafka-only
// (documented TS divergence: TS persists a private_chat DB row instead;
// spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md).
// Envelope account_id = resolved sender account id.
message PrivateChatEvent {
  int64 recipient_account_id = 1;
  string text = 2;
  // coordgrid packed coord of the sender at send time (wire field
  // PrivateMessageRequest.coord; TS persisted it in the private_chat row).
  int32 coord = 3;
}
```

Run: `make protos && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./...`
Expected: builds clean (purely additive field).

- [ ] **Step 2: Write the failing handler tests**

In `modules/friends/handler_test.go`, add a friends-package capture emitter and two tests:

```go
// captureEmitter records emitted PlayerInputEnvelopes for assertions.
type captureEmitter struct {
	mu   sync.Mutex
	envs []*eventspb.PlayerInputEnvelope
}

func (c *captureEmitter) EmitAuth(*eventspb.AuthEnvelope)   {}
func (c *captureEmitter) EmitWorld(*eventspb.WorldEnvelope) {}
func (c *captureEmitter) EmitPlayerInput(env *eventspb.PlayerInputEnvelope) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.envs = append(c.envs, env)
}
func (c *captureEmitter) EmitWealth(*eventspb.WealthEnvelope) {}

func (c *captureEmitter) snapshot() []*eventspb.PlayerInputEnvelope {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*eventspb.PlayerInputEnvelope(nil), c.envs...)
}

// TestPrivateMessage_EmitsPrivateChatEvent pins the Kafka-only PM
// record (spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md):
// a delivered PM emits exactly one PlayerInputEnvelope{PrivateChatEvent}
// keyed by the RESOLVED account ids (TS FriendServer.ts:266-284 resolve
// step), with the request's coord and text; no private_chat row exists.
func TestPrivateMessage_EmitsPrivateChatEvent(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h := newTestHandler(t)
	fromID := seedAccount(t, h.repos.db, 0xAAAA)
	toID := seedAccount(t, h.repos.db, 0xBBBB)

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          7,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		StaffLvl:         0,
		PmId:             1,
		Chat:             "hi there",
		Coord:            12345,
	}); err != nil {
		t.Fatalf("PrivateMessage: %v", err)
	}

	envs := cap.snapshot()
	if len(envs) != 1 {
		t.Fatalf("emitted %d envelopes, want 1", len(envs))
	}
	env := envs[0]
	pc := env.GetPrivateChat()
	if pc == nil {
		t.Fatalf("payload = %T, want PrivateChat", env.Payload)
	}
	if env.AccountId != fromID {
		t.Errorf("AccountId = %d, want %d (resolved sender)", env.AccountId, fromID)
	}
	if env.WorldId != 7 {
		t.Errorf("WorldId = %d, want 7", env.WorldId)
	}
	if pc.RecipientAccountId != toID {
		t.Errorf("RecipientAccountId = %d, want %d", pc.RecipientAccountId, toID)
	}
	if pc.Text != "hi there" {
		t.Errorf("Text = %q, want %q", pc.Text, "hi there")
	}
	if pc.Coord != 12345 {
		t.Errorf("Coord = %d, want 12345", pc.Coord)
	}
}

// TestPrivateMessage_MissingAccount_NoEmit pins that the TS silent-drop
// (FriendServer.ts:266-284: either endpoint unresolvable → no insert,
// no delivery, successful result) also means NO event: an undelivered
// PM leaves no record anywhere.
func TestPrivateMessage_MissingAccount_NoEmit(t *testing.T) {
	cap := &captureEmitter{}
	telemetry.Set(cap)
	t.Cleanup(telemetry.Reset)

	h := newTestHandler(t)
	seedAccount(t, h.repos.db, 0xAAAA) // recipient 0xBBBB deliberately absent

	if _, err := h.PrivateMessage(t.Context(), &friendspb.PrivateMessageRequest{
		WorldId:          1,
		Profile:          "main",
		Username37:       0xAAAA,
		TargetUsername37: 0xBBBB,
		Chat:             "hi",
	}); err != nil {
		t.Fatalf("PrivateMessage: %v (silent drop must return success)", err)
	}
	if n := len(cap.snapshot()); n != 0 {
		t.Errorf("emitted %d envelopes, want 0 (silent drop)", n)
	}
}
```

Add imports to the test file as needed: `"sync"`, `"github.com/zsrv/goscape/pkg/eventspb"`, `"github.com/zsrv/goscape/pkg/telemetry"`. Note `seedAccount` lives in `repository_test.go` — same package, already visible.

- [ ] **Step 3: Run them to verify they fail**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ -run 'TestPrivateMessage_EmitsPrivateChatEvent|TestPrivateMessage_MissingAccount_NoEmit' -v`
Expected: `TestPrivateMessage_EmitsPrivateChatEvent` FAILS with `emitted 0 envelopes, want 1` (handler still inserts, doesn't emit). `_NoEmit` may already pass — fine; it pins the negative.

- [ ] **Step 4: Reshape the repository method**

In `modules/friends/repository.go`, replace `LogPrivateMessage` entirely with:

```go
// ResolvePrivateMessageEndpoints resolves both PM endpoints to account
// ids — the resolve step of TS FriendServer.ts:266-284
// (executeTakeFirstOrThrow on from and to). Either endpoint missing →
// errAccountMissing: the handler drops the PM silently, matching the
// TS throw-and-catch. The TS insert into private_chat is retired —
// chat is Kafka-only (documented divergence, spec
// docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md); the
// caller emits a PrivateChatEvent with the resolved ids instead.
func (r *Repository) ResolvePrivateMessageEndpoints(ctx context.Context, from, to uint64) (int64, int64, error) {
	fromID, ok, err := r.accountID(ctx, from)
	if err != nil {
		return 0, 0, fmt.Errorf("ResolvePrivateMessageEndpoints: %w", err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("ResolvePrivateMessageEndpoints from %d: %w", from, errAccountMissing)
	}
	toID, ok, err := r.accountID(ctx, to)
	if err != nil {
		return 0, 0, fmt.Errorf("ResolvePrivateMessageEndpoints: %w", err)
	}
	if !ok {
		return 0, 0, fmt.Errorf("ResolvePrivateMessageEndpoints to %d: %w", to, errAccountMissing)
	}
	return fromID, toID, nil
}
```

If `gamedb.IsForeignKeyViolation` (used only by the deleted insert's race path) now has no callers in this file, drop the import if the compiler says it's unused — check other call sites first with `grep -rn IsForeignKeyViolation modules/friends/`.

- [ ] **Step 5: Rewire the handler**

In `modules/friends/handler.go`, replace the `PrivateMessage` body's log-call block. The doc comment keeps its TS citations but the persistence sentence changes; full replacement:

```go
// PrivateMessage resolves both endpoint accounts, emits the PM's
// PrivateChatEvent (chat is Kafka-only — documented TS divergence,
// spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md;
// TS FriendServer.ts:266-284 inserts a private_chat row here instead),
// and routes a PrivateMessageDelivery to the target's open stream.
// Either endpoint missing → the PM is dropped silently — no event, no
// delivery, successful result (TS throws inside the message handler
// and the outer catch swallows it). Other resolve failures keep the
// codes.Internal posture.
//
// req.Coord rides the event (otherwise unused for routing). req.WorldId
// is unused for routing because the registry is keyed solely by
// (profile, username37); cross-world routing therefore falls out for free.
func (h *handler) PrivateMessage(ctx context.Context, req *friendspb.PrivateMessageRequest) (*emptypb.Empty, error) {
	repo := h.repo()
	h.ensureWorld(req.WorldId)
	fromID, toID, err := repo.ResolvePrivateMessageEndpoints(ctx, req.Username37, req.TargetUsername37)
	if err != nil {
		if errors.Is(err, errAccountMissing) {
			return &emptypb.Empty{}, nil
		}
		return nil, status.Errorf(codes.Internal, "ResolvePrivateMessageEndpoints: %v", err)
	}
	telemetry.Get().EmitPlayerInput(&eventspb.PlayerInputEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.NewString(),
		Ts:            timestamppb.Now(),
		AccountId:     fromID,
		WorldId:       req.WorldId,
		Payload: &eventspb.PlayerInputEnvelope_PrivateChat{
			PrivateChat: &eventspb.PrivateChatEvent{
				RecipientAccountId: toID,
				Text:               req.Chat,
				Coord:              req.Coord,
			},
		},
	})
	h.subs.send(h.profile(), req.TargetUsername37, &friendspb.FriendsUpdate{
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

Add imports to `modules/friends/handler.go`: `"github.com/google/uuid"`, `"google.golang.org/protobuf/types/known/timestamppb"`, `"github.com/zsrv/goscape/pkg/eventspb"`, `"github.com/zsrv/goscape/pkg/telemetry"`.

- [ ] **Step 6: Reshape the repository tests**

In `modules/friends/repository_test.go`:

- `TestRepository_LogPrivateMessage_AppendOnly`, `TestRepository_LogPrivateMessage_RespectsProfile`, `TestRepository_LogPrivateMessage_EmptyMessageAllowed`, `TestLogPrivateMessage_BothExist_PersistsIDKeyedRow`: DELETE — they pin the retired insert (row shape, append-only, profile column). Their surviving concern — resolution — is covered by the two replacements below.
- Replace `TestLogPrivateMessage_MissingEndpoint_ErrAccountMissing` with:

```go
// TestResolvePrivateMessageEndpoints_MissingEndpoint pins the TS
// silent-drop precondition (FriendServer.ts:266-284): either endpoint
// unresolvable → errAccountMissing.
func TestResolvePrivateMessageEndpoints_MissingEndpoint(t *testing.T) {
	r, db := newTestRepo(t)

	seedAccount(t, db, 1) // only 'from' exists
	if _, _, err := r.ResolvePrivateMessageEndpoints(t.Context(), 1, 2); !errors.Is(err, errAccountMissing) {
		t.Fatalf("missing to: got %v, want errAccountMissing", err)
	}
	if _, _, err := r.ResolvePrivateMessageEndpoints(t.Context(), 3, 1); !errors.Is(err, errAccountMissing) {
		t.Fatalf("missing from: got %v, want errAccountMissing", err)
	}
}

// TestResolvePrivateMessageEndpoints_BothExist pins that both ids come
// back resolved (the ids TS used to key the private_chat row by).
func TestResolvePrivateMessageEndpoints_BothExist(t *testing.T) {
	r, db := newTestRepo(t)

	fromWant := seedAccount(t, db, 1)
	toWant := seedAccount(t, db, 2)
	from, to, err := r.ResolvePrivateMessageEndpoints(t.Context(), 1, 2)
	if err != nil {
		t.Fatalf("ResolvePrivateMessageEndpoints: %v", err)
	}
	if from != fromWant || to != toWant {
		t.Errorf("resolved = (%d, %d), want (%d, %d)", from, to, fromWant, toWant)
	}
}
```

`newTestRepo(t) (*Repository, *gamedb.DB)` (repository_test.go:72) and `seedAccount` already exist in this file — no new helpers needed.

- [ ] **Step 7: Rewrite the smoke test**

In `modules/world/friends_smoke_test.go`, rewrite `TestFriendsClient_E2E_PrivateMessagePersistsRow` as `TestFriendsClient_E2E_PrivateMessageEmitsEvent`: keep the entire harness (config, boot, both `seedAccount`-equivalent inserts, the `client.PrivateMessage(...)` call) but:

- Before booting the friends service, install the capture emitter (the world-package `captureEmitter` from Task 1 — mutexed precisely for this cross-goroutine case): `cap := &captureEmitter{}; telemetry.Set(cap); t.Cleanup(telemetry.Reset)`.
- Delete everything from the `sql.Open` re-open block through the final row assertion (the second-DB polling machinery — there is no row anymore).
- The RPC is synchronous, so after `client.PrivateMessage(ctx, …)` returns, assert directly:

```go
	var pmEnvs []*eventspb.PlayerInputEnvelope
	cap.mu.Lock()
	for _, e := range cap.playerInputEnvs {
		if e.GetPrivateChat() != nil {
			pmEnvs = append(pmEnvs, e)
		}
	}
	cap.mu.Unlock()
	if len(pmEnvs) != 1 {
		t.Fatalf("PrivateChatEvent envelopes: got %d, want 1", len(pmEnvs))
	}
	pc := pmEnvs[0].GetPrivateChat()
	if pc.Coord != 42 || pc.Text != "persisted" {
		t.Errorf("event = (coord %d, %q), want (42, %q)", pc.Coord, pc.Text, "persisted")
	}
	// Sender/recipient ride as RESOLVED account ids (TS 274 re-key).
	if pmEnvs[0].AccountId == 0 || pc.RecipientAccountId == 0 {
		t.Errorf("account ids = (%d, %d), want both non-zero resolved ids",
			pmEnvs[0].AccountId, pc.RecipientAccountId)
	}
```

Update the doc comment: the test now pins "a client.PrivateMessage call against a real in-process friends.Friends emits the PM's PrivateChatEvent (Kafka-only chat, spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md); delivery is pinned by TestFriendsClient_E2E_PrivateMessageDelivery."

- [ ] **Step 8: Run the tests to verify they pass**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/friends/ ./modules/world/ 2>&1 | tail -5`
Expected: PASS, including the Step-2 tests.

- [ ] **Step 9: Commit**

```bash
git add proto/events/v1/player_input.proto pkg/eventspb/player_input.pb.go modules/friends modules/world/friends_smoke_test.go
git commit --no-gpg-sign -m "feat(friends): PrivateMessage emits PrivateChatEvent instead of DB insert

Resolve-both-accounts silent-drop semantics preserved (TS
FriendServer.ts:266-284) via new ResolvePrivateMessageEndpoints; the
private_chat INSERT is retired. PrivateChatEvent gains coord. Spec:
docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Drop migration, migrate tests, comment sweep, docs note

**Files:**
- Create: `pkg/gamedb/migrations/sqlite/000002_drop_chat.up.sql`
- Create: `pkg/gamedb/migrations/postgres/000002_drop_chat.up.sql`
- Modify: `pkg/gamedb/migrate_test.go` (expected-tables list line ~27; FK-cascade fixtures lines ~70/~82; new dropped-tables test)
- Modify: `modules/world/login_username_test.go` (comment ~line 54)
- Modify: `docs/PORTING.md` (divergence entry)

**Interfaces:**
- Consumes: `gamedb.Migrate` picks up any `migrations/{sqlite,postgres}/*.sql` via `//go:embed migrations` + iofs — new files need no registration. Existing migrations are up-only; do NOT add a `.down.sql`.
- Produces: schema version 2 with neither chat table.

- [ ] **Step 1: Write the failing migrate test**

Add to `pkg/gamedb/migrate_test.go` (match the file's existing sqlite setup helper — the same one the expected-tables test uses):

```go
// TestMigrate_ChatTablesDropped pins migration 000002: chat is
// Kafka-only (spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md),
// so a fully-migrated schema has neither public_chat nor private_chat.
func TestMigrate_ChatTablesDropped(t *testing.T) {
	db := migratedTestDB(t) // same helper TestMigrate_CreatesAllTables uses
	for _, tbl := range []string{"public_chat", "private_chat"} {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tbl,
		).Scan(&n); err != nil {
			t.Fatalf("sqlite_master query: %v", err)
		}
		if n != 0 {
			t.Errorf("table %s still exists, want dropped", tbl)
		}
	}
}
```

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/ -run TestMigrate_ChatTablesDropped -v`
Expected: FAIL — both tables still exist.

- [ ] **Step 2: Add the migration (both backends, identical content)**

`pkg/gamedb/migrations/sqlite/000002_drop_chat.up.sql` and `pkg/gamedb/migrations/postgres/000002_drop_chat.up.sql`:

```sql
-- Chat is Kafka-only: public_chat/private_chat are retired (documented
-- TS divergence — TS persists chat to these tables; goscape emits
-- PublicChatEvent/PrivateChatEvent telemetry instead; spec
-- docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md).
-- Destroys existing chat rows by design (approved: no backfill).
DROP TABLE IF EXISTS public_chat;
DROP TABLE IF EXISTS private_chat;
```

- [ ] **Step 3: Fix the collateral in migrate_test.go**

- Remove `"private_chat", "public_chat"` from the expected-tables list (~line 27).
- The FK-cascade fixture (~line 70 `INSERT INTO private_chat …` and the ~line 82 expectation `{"private_chat", "account_id", owner, 0}`): delete both entries. The remaining fixtures still exercise the cascade behavior; do not invent a replacement table.

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/gamedb/... -v 2>&1 | tail -10`
Expected: PASS including `TestMigrate_ChatTablesDropped`.

- [ ] **Step 4: Comment sweep + docs**

- `modules/world/login_username_test.go` ~line 54: the comment says chat log tables `(public_chat / private_chat) can key on the per-login UUID` — reword to reference the PublicChatEvent session key instead, e.g. `so the public-chat telemetry record (PublicChatEvent.session_uuid — chat is Kafka-only, spec docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md) can key on the per-login UUID`.
- Run `grep -rn "public_chat\|private_chat" --include="*.go" --include="*.md" . | grep -v docs/superpowers | grep -v pb.go` and fix any remaining stale comment (expect: none beyond the above; migrations 000001/000002 legitimately mention the tables).
- `docs/PORTING.md`: add an entry to the deviations/divergences section (match the surrounding entry format):

> **Chat persistence (public_chat / private_chat) — DIVERGENCE.** TS persists public chat (`FriendServer.ts:286-297`) and PMs (`FriendServer.ts:266-284`) to central-DB tables. goscape retires both tables (migration `000002_drop_chat`) and emits `PublicChatEvent` (world module, `MESSAGE_PUBLIC` handler) / `PrivateChatEvent` (friends `PrivateMessage` handler) telemetry events instead — Kafka-only, fire-and-forget. The TS resolve-both-accounts silent-drop for PMs is preserved (`ResolvePrivateMessageEndpoints`). A deployment without a telemetry emitter records chat nowhere. Approved in `docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md`.

- [ ] **Step 5: Commit**

```bash
git add pkg/gamedb modules/world/login_username_test.go docs/PORTING.md
git commit --no-gpg-sign -m "feat(gamedb): 000002_drop_chat — retire public_chat/private_chat

Chat is Kafka-only; drops both tables on sqlite+postgres, reworks
migrate tests, sweeps stale comments, documents the divergence in
docs/PORTING.md. Spec: docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: rev-274 verification gate

**Files:** none (verification only; fix-forward anything it surfaces).

- [ ] **Step 1: Full build + test**

Run: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go build ./... && GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./... 2>&1 | tail -15`
Expected: all packages PASS.

- [ ] **Step 2: Race detector on touched packages**

Run: `CGO_ENABLED=1 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./modules/world/... ./modules/friends/... ./pkg/gamedb/... ./pkg/telemetry/... 2>&1 | tail -10`
Expected: PASS, no race reports (the capture emitters are mutexed for exactly this run).

- [ ] **Step 3: Generated-files check**

Run: `make check-generated-files && git status --short`
Expected: no diff — protos were regenerated, not hand-edited. If `git status` shows modified `.pb.go` files, the regen was stale: commit the refreshed files with a `chore(protos)` commit.

- [ ] **Step 4: Reference sweep**

Run: `grep -rn "ChatMessageEvent\|LogPublicMessage\|LogPrivateMessage\|PublicMessageRequest" --include="*.go" . | grep -v docs/`
Expected: no output. Any hit is an unfinished edit from Tasks 1–3 — fix it there.

---

### Tasks 6–9: Port to rev-254, rev-245.2, rev-244, rev-225

One task per branch, executed in this order (nearest branch first; each port may inform the next). Worktrees: `~/Code/github.com/zsrv/goscape-rev254`, `…-rev245.2`, `…-rev244`, `…-rev225`. The target-state code is Tasks 1–4 of this plan plus the rev-274 commits (list them with `git log --oneline rev-274 -6`); this section defines the per-branch adaptation procedure and known deltas.

**Interfaces (all four tasks):**
- Consumes: the five rev-274 commits from Tasks 1–4 (+ any Task 5 fix-forward commit).
- Produces: each branch compiles, tests green, with the identical event schema (`PublicChatEvent`/`PrivateChatEvent` proto shapes MUST be byte-identical across branches) and its own adapted module/migration deletions.

**Per-branch known deltas (from the central-DB consolidation; verify each in-tree before adapting):**
- **rev-254:** same session_uuid public-chat re-key as 274 (A3 landed there). Expect near-COPYABLE; the friends handler/repo may differ in minor comment pins (`@2e3bcf43` refs are 254's engine pin — keep the branch's own pins).
- **rev-245.2 / rev-244:** 6-column `public_chat` (profile/world consumed by the handler) and the public-chat key is the **username**, not session UUID (the A3 re-key is a 254+ change — `bridges.go` on those branches has `PublicMessage(username string, …)` or equivalent). Adaptation rule: the deletion set is the same; for the emission, populate `PublicChatEvent.session_uuid` from the branch's `Player` session field **if the branch's Player carries one**, else leave it empty (`""`) and rely on the envelope `account_id` — record which case applied in the port commit message. Do NOT add a username field to the event; the schema stays identical across branches.
- **rev-225:** pre-B5 schema; friends server surface is older. Verify whether the `PublicMessage` RPC and both tables exist in the branch's `friends.proto` / migrations before deleting; the table columns differ but the drop migration content is identical. **The telemetry platform builds against `../goscape-rev225`** (`replace` directive) — after this port, the platform stops compiling until its own follow-up plan lands; that is expected and out of scope here.

**Procedure per branch (repeat as Task 6=rev-254, 7=rev-245.2, 8=rev-244, 9=rev-225):**

- [ ] **Step 1: Baseline sanity**

Run (substituting the worktree): `git -C ~/Code/github.com/zsrv/goscape-rev254 status --short && git -C ~/Code/github.com/zsrv/goscape-rev254 log --oneline -1`
Expected: clean tree on the branch tip. If dirty, STOP and report.

- [ ] **Step 2: COPYABLE check for the event protos**

Run: `git diff rev-254 rev-274~6 -- proto/events/v1/world.proto proto/events/v1/player_input.proto` (adjust `~6` so it points at the pre-Task-1 rev-274 commit; find it with `git log --oneline rev-274 -8`).
If the diff is EMPTY (branch matches the pre-change base): `git -C <worktree> checkout rev-274 -- proto/events/v1/world.proto proto/events/v1/player_input.proto`. If NOT empty: hand-apply the Task 1/Task 3 proto edits onto the branch's own files, preserving branch-local comments.

- [ ] **Step 3: Regenerate protos inside the worktree**

Run: `make -C <worktree> protos` (regenerate there — never copy `.pb.go` across worktrees). Repeat after the friends.proto edit below.

- [ ] **Step 4: Adapt the four rev-274 change-sets** (world emission merge; PublicMessage retirement incl. the branch's `friends.proto`; friends PrivateMessage emission + `ResolvePrivateMessageEndpoints`; drop migration + migrate-test rework). For each, open the branch's counterpart file, apply the Task 1–4 target code adapted to the branch's shapes (per the known-delta notes above), and check the migration number: `ls <worktree>/pkg/gamedb/migrations/sqlite/` — use the next free number if the branch isn't at 000001.

- [ ] **Step 5: Branch gate**

Run in the worktree: `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -run '^$' ./...` (compile-all) then `GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/... ./pkg/gamedb/... 2>&1 | tail -5`
Expected: compile clean, tests PASS. Ignore stale IDE/LSP diagnostics during the port; trust only these commands.

- [ ] **Step 6: Commit on the branch** (same commit granularity as rev-274 — four commits, or one squashed port commit if the branch adaptation was mechanical; cite the spec and note the session_uuid decision for 245.2/244):

```bash
git -C <worktree> add -A
git -C <worktree> status --short   # verify exactly the intended files staged
git -C <worktree> commit --no-gpg-sign -m "feat(chat): Kafka-only chat port from rev-274 (<branch>)

<per-branch adaptation notes>
Spec: docs/superpowers/specs/2026-07-07-chat-kafka-only-design.md

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Out of scope (separate follow-up plan)

The `goscape-telemetry-platform` workstream (format_schemas sync, ClickHouse migration for `public_chat.*` columns, demo seeder + `pgwrite.go`, dashboard sweep, rev-225 rebuild) — spec §7. Plan it after Task 9 lands, since the platform builds against the rev-225 worktree.
