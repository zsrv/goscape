# NAI-129 — L7 zone-broadcast UID/slot filter fix

**Status:** spec
**Date:** 2026-05-08
**Predecessor:** NAI-128 Stage 3 close (`a653fbc`) bound the rat-loot residual to layer L7 (post-add zone-broadcast gap).
**Tech stack:** Go 1.26+
**Cadence:** full (brainstorm → spec → plan → subagent-driven TDD with two-stage review).

## §1 — Problem & diagnosis

NAI-128 Stage 3 smoke (2026-05-08) logged two valid `worldVarsView.AddObj` calls during a Citizen kill — `typeID=526 bones`, `typeID=995 coins`, `receiverID=2232170497`, `duration=200` — but the client showed no loot.

**Root cause:** producer and per-player delivery filter use different identifier-spaces for private-drop receiver matching.

| Site | Source | Identifier space |
|---|---|---|
| Producer (handler) | `pkg/script/handlers_obj.go:109` — `objAddCommon(s, "OBJ_ADD", s.Self.UID())` | UID (`composeUID(username37, slot)`) |
| Producer (worldVarsView) | `modules/world/server_varp.go:170` — `w.s.AddObj(obj, receiverID)`; `obj.ReceiverID = receiverID` | UID |
| Queueing | `pkg/zone/zone.go:267` — `ReceiverID: receiverID` on `ZoneEventFollows` | UID |
| Filter (PartialFollows per-tick) | `modules/world/player_zone.go:76` — `if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.slot` | **slot** ❌ |
| Filter (FullFollows replay on zone-load) | `modules/world/player_zone.go:41` — `if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.slot` | **slot** ❌ |

UID values fit `int` but never collide with slot (1..2047): `composeUID` is `((username37 & 0x1FFFFF) << 11) | slot`, so a non-zero `username37` produces a value ≥ 2048. Every private-drop Follows event silently filters out for every player.

**TS canonical comparator** is UID-space at both ends:

- `Engine-TS/src/engine/zone/Zone.ts:138` — `obj.receiver64 !== player.hash64` (writeFullFollows replay)
- `Engine-TS/src/engine/zone/Zone.ts:190` — `event.receiver64 !== player.hash64` (writePartialFollows per-tick)

**Why no test caught it:** `TestPartialFollowsFiltersByReceiverID` (`modules/world/player_zone_test.go:159`) seeds the producer and the player with the same `slot` value — slot/slot in the fixture, UID/slot in production. The fixture passes self-consistently through the buggy comparator.

**Public-receiver paths unaffected:** the comparator short-circuits on `e.ReceiverID == zone.PublicReceiver` (and the public Enclosed buffer flows through `Shared()` which has no receiver gate at all). That is why public death-drops have always worked while private drops never have.

## §2 — Architecture / fix

**Production change (2 sites):**

```go
// modules/world/player_zone.go:41 — writeFullFollows replay
- if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.slot {
+ if obj.ReceiverID != zone.PublicReceiver && obj.ReceiverID != p.uid {

// modules/world/player_zone.go:76 — writePartialFollows per-tick
- if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.slot {
+ if e.ReceiverID != zone.PublicReceiver && e.ReceiverID != p.uid {
```

`player_zone.go` is in package `world`, so `p.uid` is accessible directly; no need to route through the `Player.UID()` accessor.

**Doc-comment touch-ups (no behavior change):**

- `pkg/zone/event.go:30` — `ReceiverID int` field comment: clarify "UID-space (mirrors TS `receiver64`); `PublicReceiver = -1` for public Enclosed events and public Follows."
- `modules/world/world_zone.go:123` — `Server.AddObj` doc: align with UID-space convention ("`receiverID == zone.PublicReceiver` for public; otherwise the receiver's UID per `composeUID`").
- `modules/world/player_zone.go:14-20, 67-69` — replay/per-tick doc comments: add "compares `obj.ReceiverID` against `p.uid` per TS `Zone.ts:138, 190`".

**No symbol rename.** `ReceiverID` (vs TS `receiver64`) — Go convention favors `ReceiverID`; the bigint vs int representational difference is doc-commented but does not motivate a rename (~30 sites for cosmetic gain).

## §3 — Test plan

### Existing rewrites (1)

`TestPartialFollowsFiltersByReceiverID` (`modules/world/player_zone_test.go:159`) — convert from slot/slot to UID/UID:

- Seed two `Player` fixtures with distinct `username37` values so `composeUID` produces distinct UIDs even when slots are adjacent.
- Producer call sites: `s.AddObj(objA, uidA)`, `s.AddObj(objB, uidB)` (pass UID, not slot).
- Assert player A's encoded output contains exactly the obj for `uidA`; not `uidB`.
- Add a third public-receiver obj (`s.AddObj(objPub, zone.PublicReceiver)`) to confirm the public path stays unaffected.

### New tests (4 — full pin)

| Test | File | Path | Pin shape |
|---|---|---|---|
| `TestPartialFollowsDeliversPrivateDropToOwnerByUID` | `modules/world/player_zone_test.go` | `writePartialFollows` filter | Positive: owner with matching UID receives an `OBJ_ADD` packet. Driven via `s.AddObj` → `ComputeShared` → `writePartialFollows` → `flushWrite`, mirroring the pattern of existing `TestPartialFollowsFiltersByReceiverID`. |
| `TestPartialFollowsHidesPrivateDropFromNonOwnerByUID` | same | `writePartialFollows` filter | Absence-pin: a second player at a different UID standing in the same zone receives nothing for that obj. |
| `TestFullFollowsReplaysPrivateDropToOwnerByUID` | same | `writeFullFollows` replay loop | Positive: a player loading a zone where a private drop with matching UID already exists gets the obj in the replay. Driven via `s.AddObj` followed by `writeFullFollows` (player not yet in `loadedZones`). |
| `TestFullFollowsHidesPrivateDropFromNonOwnerInReplay` | same | `writeFullFollows` replay loop | Absence-pin: a second player loading the same zone with a different UID does not get the obj in the replay. |

### Fixture utility extension

`newZoneTestPlayer` currently writes `p.slot = slot` only. Extend it (or introduce a sibling `newZoneTestPlayerWithUID`) to also assign `p.uid = composeUID(username37, slot)` from a per-test `username37`. Use distinct `username37` values across tests so adjacent-slot players still get distinct UIDs.

### Regression check

- `pkg/zone/zone_test.go` unit tests assert queueing semantics (event type, bytes), not the per-player filter — unchanged.
- `modules/world/nai128_rat_loot_test.go` asserts G6 records the right `obj.add` invocations. It does not assert client-side delivery, so it stays green on both pre- and post-fix code. No change needed.

## §4 — Smoke handoff

User-launched per `smoke_test_server_handoff`. Steps:

1. Build & run: `CGO_ENABLED=0 GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go run -trimpath ./cmd/goscape --config.file config.yaml`
2. Login Lumbridge newbieman; attack a Citizen until kill.
3. Verify client UI: bones (id=526, ×1) + coins (id=995, ×3) appear under the rat at the kill tile.
4. Verify NodeDebug log still shows G1..G6 firing. After fix, the same `receiverID=2232170497` value should flow unchanged — the gateways stay live as permanent diagnostics per `nodedebug_gateway_probe_pattern`.

**Pass criterion:** loot visible to the killing player; NodeDebug G6 still records the two `obj.add` lines with the right type/count/receiverID.

**Failure routing:** if client still shows no loot post-fix, the mechanical UID/slot match would not be at fault per TS `Zone.ts:138, 190`. Open NAI-130 with the new symptom; do not regress this fix.

## §5 — Risk register

| Risk | Likelihood | Mitigation |
|---|---|---|
| `p.uid == -1` at filter time before `addPlayer` runs | Very low — `updateZones` runs from `processOut` which is called over `s.playerLoop`, populated only at `addPlayer` (`server.go:696`); UID is composed before the player joins the loop | Verified pre-flight; no defensive gate needed. |
| `composeUID` collisions producing UID == slot | Zero — `composeUID = ((username37 & 0x1FFFFF) << 11) \| slot`; for any non-zero `username37` the result ≥ 2048; for `username37 == 0` the result equals slot, but `username37 == 0` is not a valid logged-in player | None needed; documented for completeness. |
| Other private-receiver `ReceiverID` filter sites missed | None — `rg "ReceiverID" modules/world pkg/zone` returns only the two fix sites + `PublicReceiver` sentinel checks at queue-time | Verified pre-flight. |
| Test fixture parity miss in unrelated suites | None — `addObjToZone` in `npc_hunt_entities_test.go` uses public receivers (`category` parameter is unrelated to `ReceiverID`); only `player_zone_test.go` private-receiver path needs the fixture rewrite | Audit complete. |

## §6 — TS-fidelity & deviations

**No new deviations.** This fix retires an unintentional divergence — `ReceiverID` filter was always meant to be UID-space per TS `Zone.ts:138, 190`. No deviation tag.

**Existing related deviations** (kept, unaffected by this sub-spec):

- `NAI-115-D2` — `duration` accepted but not honored on private drops (no despawn-after-N-ticks scheduler). Orthogonal; rat loot smoke does not exercise duration.

## §7 — Out of scope

- `NAI-115-D2` despawn scheduler (still deferred).
- Per-zone obj cap (`OBJS=129` TODO at `pkg/zone/zone.go:251`).
- `RevealObj` tradeability gating (TODO at `pkg/zone/zone.go:324`).
- `ReceiverID → Receiver` symbol rename (cosmetic, deferred indefinitely).
- Beyond rat-loot: any additional smoke-surfaced divergences route to NAI-130+ per `smoke_surfaces_adjacent_divergences`.

## §8 — Close criteria

1. `go test ./...` green.
2. Production diff is exactly the 2-line filter change + doc comments — no incidental code motion.
3. Smoke (§4) confirms bones+coins visible to the killing player.
4. Close commit body cites: pre/post-fix smoke G6 lines + `Closes memory:` trailer per `close_commit_memory_trailer`.
