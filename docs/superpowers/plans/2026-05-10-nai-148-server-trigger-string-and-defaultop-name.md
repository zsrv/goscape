# NAI-148 — `ServerTriggerType.String()` + TS-faithful `defaultOp` debug name — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close `NAI-147-D-TRIGGER-NAME-NUMERIC` by porting TS `ServerTriggerType.toString` (151-entry lowered-name table) and adding a `tsTriggerForOpFire` mapper that bridges goscape's op-slot/sentinel `targetOp` namespace to the TS AP\*/OP\* `ServerTriggerType` namespace, so `defaultOp`'s `cfg.NodeDebug=true` chat emits TS-faithful trigger names like `[opnpc1,test_npc]` instead of `[8,test_npc]`.

**Architecture:** Two-layer fix. (1) `pkg/script/trigger.go` gains a `serverTriggerNames` map (151 entries, gaps `{22,23,29,30,50,51,57,58,78,79,85,86,106,107,113,114,115}` omitted) and a `String()` method with a `"trigger_<N>"` fallback for unknown values. (2) `modules/world/interaction.go` gains a private `tsTriggerForOpFire(target entity, targetOp int) script.ServerTriggerType` helper that resolves sentinels first (TS L1086 — sentinel wins over target type) then disambiguates numeric op-slots `1..5` via target type-switch. `defaultOp` then prints `tsTriggerForOpFire(p.target, p.targetOp).String()`.

**Tech Stack:** Go 1.26+ (`go_version.md`). No new dependencies. All work in `pkg/script/` and `modules/world/`.

**Spec:** `docs/superpowers/specs/2026-05-10-nai-148-server-trigger-string-and-defaultop-name-design.md` (commit `35e0d26`).

**Cadence:** 15-100 LOC band per `compressed_cadence.md` — separate spec + plan docs, single combined Sonnet reviewer at end-of-impl. Subagent-driven-development per `execution_mode_default.md`.

---

## File Structure

| File                                                    | Action  | Responsibility                                                                                            |
|---------------------------------------------------------|---------|------------------------------------------------------------------------------------------------------------|
| `pkg/script/trigger.go`                                 | Modify  | Append `serverTriggerNames` map + `String()` method + `fmt` import.                                        |
| `pkg/script/trigger_test.go`                            | Create  | Table-driven `TestServerTriggerType_String` covering enum spread + fallback.                               |
| `modules/world/interaction.go`                          | Modify  | Add `tsTriggerForOpFire`; wire `defaultOp` to use it; retire `NAI-147-D-TRIGGER-NAME-NUMERIC` doc-comment. |
| `modules/world/interaction_default_op_debug_test.go`    | Modify  | Append `TestTsTriggerForOpFire`; update `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue` wire assertion.  |

Total estimated delta: ~210 LOC production (~170 of which is mechanical map data) + ~140 LOC test.

---

## Pre-flight verification (controller, not implementer)

Before dispatching T1, the controller (parent) verifies these premises against HEAD `35e0d26`:

- `pkg/script/trigger.go` has no existing `String()` method or `fmt` import.
- `pkg/script/trigger_test.go` does not exist.
- `modules/world/movement_consts.go:45` declares `entity` interface (private, package `world`).
- `modules/world/interaction.go:36-45` declares `targetOpLocT/U`, `targetOpNpcT/U`, `targetOpPlayerT/U`, `targetOpObjT/U` constants.
- `modules/world/interaction.go:472` is the line being modified (currently `p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))`).
- `modules/world/interaction_default_op_debug_test.go:32-56` is the test being modified (currently asserts `[8,test_npc]`).

If any premise has shifted, re-evaluate before dispatch (per `controller_preflight.md`).

---

## Task 1 — `ServerTriggerType.String()` + table

**Files:**
- Modify: `pkg/script/trigger.go`
- Create: `pkg/script/trigger_test.go`

- [ ] **Step 1.1: Add the failing test file**

Create `pkg/script/trigger_test.go`:

```go
package script

import "testing"

// TestServerTriggerType_String pins the TS-faithful lowered enum name
// returned by (ServerTriggerType).String(). Mirrors TS
// LostCityRS/Engine-TS/.../ServerTriggerType.ts:166-170 toString
// (`ServerTriggerType[trigger].toLowerCase()`).
func TestServerTriggerType_String(t *testing.T) {
	cases := []struct {
		name string
		in   ServerTriggerType
		want string
	}{
		// Constant-prefix free of underscore.
		{"proc", TriggerProc, "proc"},
		{"label", TriggerLabel, "label"},
		{"debugproc", TriggerDebugProc, "debugproc"},

		// AP/OP families (1-5 + U + T).
		{"apnpc1", TriggerApNpc1, "apnpc1"},
		{"apnpcu", TriggerApNpcU, "apnpcu"},
		{"apnpct", TriggerApNpcT, "apnpct"},
		{"opnpc1", TriggerOpNpc1, "opnpc1"},
		{"opnpct", TriggerOpNpcT, "opnpct"},
		{"oploc1", TriggerOpLoc1, "oploc1"},
		{"oploct", TriggerOpLocT, "oploct"},
		{"opobj1", TriggerOpObj1, "opobj1"},
		{"opobjt", TriggerOpObjT, "opobjt"},
		{"opplayer1", TriggerOpPlayer1, "opplayer1"},
		{"opplayert", TriggerOpPlayerT, "opplayert"},
		{"opheld1", TriggerOpHeld1, "opheld1"},
		{"opheldt", TriggerOpHeldT, "opheldt"},

		// AI prefix retains underscore.
		{"ai_apnpc1", TriggerAiApNpc1, "ai_apnpc1"},
		{"ai_opnpc5", TriggerAiOpNpc5, "ai_opnpc5"},
		{"ai_aploc1", TriggerAiApLoc1, "ai_aploc1"},
		{"ai_opplayer1", TriggerAiOpPlayer1, "ai_opplayer1"},
		{"ai_queue4", TriggerAiQueue4, "ai_queue4"},
		{"ai_queue20", TriggerAiQueue20, "ai_queue20"},
		{"ai_timer", TriggerAiTimer, "ai_timer"},
		{"ai_walktrigger", TriggerAiWalkTrigger, "ai_walktrigger"},
		{"ai_spawn", TriggerAiSpawn, "ai_spawn"},
		{"ai_despawn", TriggerAiDespawn, "ai_despawn"},

		// Compound underscore retention.
		{"if_button", TriggerIfButton, "if_button"},
		{"if_close", TriggerIfClose, "if_close"},
		{"inv_button1", TriggerInvButton1, "inv_button1"},
		{"inv_buttond", TriggerInvButtonD, "inv_buttond"},

		// Single-token specials.
		{"queue", TriggerQueue, "queue"},
		{"softtimer", TriggerSoftTimer, "softtimer"},
		{"timer", TriggerTimer, "timer"},
		{"walktrigger", TriggerWalkTrigger, "walktrigger"},
		{"login", TriggerLogin, "login"},
		{"logout", TriggerLogout, "logout"},
		{"tutorial", TriggerTutorial, "tutorial"},
		{"advancestat", TriggerAdvanceStat, "advancestat"},
		{"mapzone", TriggerMapZone, "mapzone"},
		{"mapzoneexit", TriggerMapZoneExit, "mapzoneexit"},
		{"zone", TriggerZone, "zone"},
		{"zoneexit", TriggerZoneExit, "zoneexit"},
		{"changestat", TriggerChangeStat, "changestat"},

		// Unknown-value fallback.
		{"unknown_high", ServerTriggerType(9999), "trigger_9999"},
		{"gap_22", ServerTriggerType(22), "trigger_22"},
		{"unknown_negative", ServerTriggerType(-1), "trigger_-1"},
	}

	for _, c := range cases {
		got := c.in.String()
		if got != c.want {
			t.Errorf("%s: ServerTriggerType(%d).String() = %q, want %q", c.name, int(c.in), got, c.want)
		}
	}
}
```

- [ ] **Step 1.2: Run test — expect FAIL (no String method)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestServerTriggerType_String -v
```

Expected: build failure or compilation error — `c.in.String()` undefined (or `String() string` returns the default `%!s(script.ServerTriggerType=N)` if Go's default Stringer isn't satisfied; explicit failure either way).

- [ ] **Step 1.3: Add the table + method to `pkg/script/trigger.go`**

Edit `pkg/script/trigger.go`. After the existing `package script` declaration on line 1, add an `import` block. After the closing `)` of the const block (currently line 173), append the map and method.

Final file state — full content of `pkg/script/trigger.go`:

```go
package script

import "fmt"

// ServerTriggerType identifies which event type a script is bound to.
// Numeric values match TS ServerTriggerType.ts exactly.
type ServerTriggerType int

const (
	TriggerProc      ServerTriggerType = 0
	TriggerLabel     ServerTriggerType = 1
	TriggerDebugProc ServerTriggerType = 2

	TriggerApNpc1 ServerTriggerType = 3
	TriggerApNpc2 ServerTriggerType = 4
	TriggerApNpc3 ServerTriggerType = 5
	TriggerApNpc4 ServerTriggerType = 6
	TriggerApNpc5 ServerTriggerType = 7
	TriggerApNpcU ServerTriggerType = 8
	TriggerApNpcT ServerTriggerType = 9
	TriggerOpNpc1 ServerTriggerType = 10
	TriggerOpNpc2 ServerTriggerType = 11
	TriggerOpNpc3 ServerTriggerType = 12
	TriggerOpNpc4 ServerTriggerType = 13
	TriggerOpNpc5 ServerTriggerType = 14
	TriggerOpNpcU ServerTriggerType = 15
	TriggerOpNpcT ServerTriggerType = 16

	TriggerAiApNpc1 ServerTriggerType = 17
	TriggerAiApNpc2 ServerTriggerType = 18
	TriggerAiApNpc3 ServerTriggerType = 19
	TriggerAiApNpc4 ServerTriggerType = 20
	TriggerAiApNpc5 ServerTriggerType = 21
	TriggerAiOpNpc1 ServerTriggerType = 24
	TriggerAiOpNpc2 ServerTriggerType = 25
	TriggerAiOpNpc3 ServerTriggerType = 26
	TriggerAiOpNpc4 ServerTriggerType = 27
	TriggerAiOpNpc5 ServerTriggerType = 28

	TriggerApObj1 ServerTriggerType = 31
	TriggerApObj2 ServerTriggerType = 32
	TriggerApObj3 ServerTriggerType = 33
	TriggerApObj4 ServerTriggerType = 34
	TriggerApObj5 ServerTriggerType = 35
	TriggerApObjU ServerTriggerType = 36
	TriggerApObjT ServerTriggerType = 37
	TriggerOpObj1 ServerTriggerType = 38
	TriggerOpObj2 ServerTriggerType = 39
	TriggerOpObj3 ServerTriggerType = 40
	TriggerOpObj4 ServerTriggerType = 41
	TriggerOpObj5 ServerTriggerType = 42
	TriggerOpObjU ServerTriggerType = 43
	TriggerOpObjT ServerTriggerType = 44

	TriggerAiApObj1 ServerTriggerType = 45
	TriggerAiApObj2 ServerTriggerType = 46
	TriggerAiApObj3 ServerTriggerType = 47
	TriggerAiApObj4 ServerTriggerType = 48
	TriggerAiApObj5 ServerTriggerType = 49
	TriggerAiOpObj1 ServerTriggerType = 52
	TriggerAiOpObj2 ServerTriggerType = 53
	TriggerAiOpObj3 ServerTriggerType = 54
	TriggerAiOpObj4 ServerTriggerType = 55
	TriggerAiOpObj5 ServerTriggerType = 56

	TriggerApLoc1 ServerTriggerType = 59
	TriggerApLoc2 ServerTriggerType = 60
	TriggerApLoc3 ServerTriggerType = 61
	TriggerApLoc4 ServerTriggerType = 62
	TriggerApLoc5 ServerTriggerType = 63
	TriggerApLocU ServerTriggerType = 64
	TriggerApLocT ServerTriggerType = 65
	TriggerOpLoc1 ServerTriggerType = 66
	TriggerOpLoc2 ServerTriggerType = 67
	TriggerOpLoc3 ServerTriggerType = 68
	TriggerOpLoc4 ServerTriggerType = 69
	TriggerOpLoc5 ServerTriggerType = 70
	TriggerOpLocU ServerTriggerType = 71
	TriggerOpLocT ServerTriggerType = 72

	TriggerAiApLoc1 ServerTriggerType = 73
	TriggerAiApLoc2 ServerTriggerType = 74
	TriggerAiApLoc3 ServerTriggerType = 75
	TriggerAiApLoc4 ServerTriggerType = 76
	TriggerAiApLoc5 ServerTriggerType = 77
	TriggerAiOpLoc1 ServerTriggerType = 80
	TriggerAiOpLoc2 ServerTriggerType = 81
	TriggerAiOpLoc3 ServerTriggerType = 82
	TriggerAiOpLoc4 ServerTriggerType = 83
	TriggerAiOpLoc5 ServerTriggerType = 84

	TriggerApPlayer1 ServerTriggerType = 87
	TriggerApPlayer2 ServerTriggerType = 88
	TriggerApPlayer3 ServerTriggerType = 89
	TriggerApPlayer4 ServerTriggerType = 90
	TriggerApPlayer5 ServerTriggerType = 91
	TriggerApPlayerU ServerTriggerType = 92
	TriggerApPlayerT ServerTriggerType = 93
	TriggerOpPlayer1 ServerTriggerType = 94
	TriggerOpPlayer2 ServerTriggerType = 95
	TriggerOpPlayer3 ServerTriggerType = 96
	TriggerOpPlayer4 ServerTriggerType = 97
	TriggerOpPlayer5 ServerTriggerType = 98
	TriggerOpPlayerU ServerTriggerType = 99
	TriggerOpPlayerT ServerTriggerType = 100

	TriggerAiApPlayer1 ServerTriggerType = 101
	TriggerAiApPlayer2 ServerTriggerType = 102
	TriggerAiApPlayer3 ServerTriggerType = 103
	TriggerAiApPlayer4 ServerTriggerType = 104
	TriggerAiApPlayer5 ServerTriggerType = 105
	TriggerAiOpPlayer1 ServerTriggerType = 108
	TriggerAiOpPlayer2 ServerTriggerType = 109
	TriggerAiOpPlayer3 ServerTriggerType = 110
	TriggerAiOpPlayer4 ServerTriggerType = 111
	TriggerAiOpPlayer5 ServerTriggerType = 112

	TriggerQueue     ServerTriggerType = 116
	TriggerAiQueue1  ServerTriggerType = 117
	TriggerAiQueue2  ServerTriggerType = 118
	TriggerAiQueue3  ServerTriggerType = 119
	TriggerAiQueue4  ServerTriggerType = 120
	TriggerAiQueue5  ServerTriggerType = 121
	TriggerAiQueue6  ServerTriggerType = 122
	TriggerAiQueue7  ServerTriggerType = 123
	TriggerAiQueue8  ServerTriggerType = 124
	TriggerAiQueue9  ServerTriggerType = 125
	TriggerAiQueue10 ServerTriggerType = 126
	TriggerAiQueue11 ServerTriggerType = 127
	TriggerAiQueue12 ServerTriggerType = 128
	TriggerAiQueue13 ServerTriggerType = 129
	TriggerAiQueue14 ServerTriggerType = 130
	TriggerAiQueue15 ServerTriggerType = 131
	TriggerAiQueue16 ServerTriggerType = 132
	TriggerAiQueue17 ServerTriggerType = 133
	TriggerAiQueue18 ServerTriggerType = 134
	TriggerAiQueue19 ServerTriggerType = 135
	TriggerAiQueue20 ServerTriggerType = 136

	TriggerSoftTimer ServerTriggerType = 137
	TriggerTimer     ServerTriggerType = 138
	TriggerAiTimer   ServerTriggerType = 139

	TriggerOpHeld1 ServerTriggerType = 140
	TriggerOpHeld2 ServerTriggerType = 141
	TriggerOpHeld3 ServerTriggerType = 142
	TriggerOpHeld4 ServerTriggerType = 143
	TriggerOpHeld5 ServerTriggerType = 144
	TriggerOpHeldU ServerTriggerType = 145
	TriggerOpHeldT ServerTriggerType = 146

	TriggerIfButton   ServerTriggerType = 147
	TriggerIfClose    ServerTriggerType = 148
	TriggerInvButton1 ServerTriggerType = 149
	TriggerInvButton2 ServerTriggerType = 150
	TriggerInvButton3 ServerTriggerType = 151
	TriggerInvButton4 ServerTriggerType = 152
	TriggerInvButton5 ServerTriggerType = 153
	TriggerInvButtonD ServerTriggerType = 154

	TriggerWalkTrigger   ServerTriggerType = 155
	TriggerAiWalkTrigger ServerTriggerType = 156

	TriggerLogin       ServerTriggerType = 157
	TriggerLogout      ServerTriggerType = 158
	TriggerTutorial    ServerTriggerType = 159
	TriggerAdvanceStat ServerTriggerType = 160
	TriggerMapZone     ServerTriggerType = 161
	TriggerMapZoneExit ServerTriggerType = 162
	TriggerZone        ServerTriggerType = 163
	TriggerZoneExit    ServerTriggerType = 164
	TriggerChangeStat  ServerTriggerType = 165
	TriggerAiSpawn     ServerTriggerType = 166
	TriggerAiDespawn   ServerTriggerType = 167
)

// serverTriggerNames mirrors TS ServerTriggerType reverse-mapping
// (Engine-TS/.../ServerTriggerType.ts:1-162 enum keys, lowercased per
// `ServerTriggerType[trigger].toLowerCase()` at L168). Numeric gaps in
// the TS enum {22,23,29,30,50,51,57,58,78,79,85,86,106,107,113,114,115}
// are intentionally absent — TS reverse-mapping returns undefined for
// those, mapping cleanly onto String()'s "trigger_<N>" fallback.
var serverTriggerNames = map[ServerTriggerType]string{
	TriggerProc:      "proc",
	TriggerLabel:     "label",
	TriggerDebugProc: "debugproc",

	TriggerApNpc1: "apnpc1",
	TriggerApNpc2: "apnpc2",
	TriggerApNpc3: "apnpc3",
	TriggerApNpc4: "apnpc4",
	TriggerApNpc5: "apnpc5",
	TriggerApNpcU: "apnpcu",
	TriggerApNpcT: "apnpct",
	TriggerOpNpc1: "opnpc1",
	TriggerOpNpc2: "opnpc2",
	TriggerOpNpc3: "opnpc3",
	TriggerOpNpc4: "opnpc4",
	TriggerOpNpc5: "opnpc5",
	TriggerOpNpcU: "opnpcu",
	TriggerOpNpcT: "opnpct",

	TriggerAiApNpc1: "ai_apnpc1",
	TriggerAiApNpc2: "ai_apnpc2",
	TriggerAiApNpc3: "ai_apnpc3",
	TriggerAiApNpc4: "ai_apnpc4",
	TriggerAiApNpc5: "ai_apnpc5",
	TriggerAiOpNpc1: "ai_opnpc1",
	TriggerAiOpNpc2: "ai_opnpc2",
	TriggerAiOpNpc3: "ai_opnpc3",
	TriggerAiOpNpc4: "ai_opnpc4",
	TriggerAiOpNpc5: "ai_opnpc5",

	TriggerApObj1: "apobj1",
	TriggerApObj2: "apobj2",
	TriggerApObj3: "apobj3",
	TriggerApObj4: "apobj4",
	TriggerApObj5: "apobj5",
	TriggerApObjU: "apobju",
	TriggerApObjT: "apobjt",
	TriggerOpObj1: "opobj1",
	TriggerOpObj2: "opobj2",
	TriggerOpObj3: "opobj3",
	TriggerOpObj4: "opobj4",
	TriggerOpObj5: "opobj5",
	TriggerOpObjU: "opobju",
	TriggerOpObjT: "opobjt",

	TriggerAiApObj1: "ai_apobj1",
	TriggerAiApObj2: "ai_apobj2",
	TriggerAiApObj3: "ai_apobj3",
	TriggerAiApObj4: "ai_apobj4",
	TriggerAiApObj5: "ai_apobj5",
	TriggerAiOpObj1: "ai_opobj1",
	TriggerAiOpObj2: "ai_opobj2",
	TriggerAiOpObj3: "ai_opobj3",
	TriggerAiOpObj4: "ai_opobj4",
	TriggerAiOpObj5: "ai_opobj5",

	TriggerApLoc1: "aploc1",
	TriggerApLoc2: "aploc2",
	TriggerApLoc3: "aploc3",
	TriggerApLoc4: "aploc4",
	TriggerApLoc5: "aploc5",
	TriggerApLocU: "aplocu",
	TriggerApLocT: "aploct",
	TriggerOpLoc1: "oploc1",
	TriggerOpLoc2: "oploc2",
	TriggerOpLoc3: "oploc3",
	TriggerOpLoc4: "oploc4",
	TriggerOpLoc5: "oploc5",
	TriggerOpLocU: "oplocu",
	TriggerOpLocT: "oploct",

	TriggerAiApLoc1: "ai_aploc1",
	TriggerAiApLoc2: "ai_aploc2",
	TriggerAiApLoc3: "ai_aploc3",
	TriggerAiApLoc4: "ai_aploc4",
	TriggerAiApLoc5: "ai_aploc5",
	TriggerAiOpLoc1: "ai_oploc1",
	TriggerAiOpLoc2: "ai_oploc2",
	TriggerAiOpLoc3: "ai_oploc3",
	TriggerAiOpLoc4: "ai_oploc4",
	TriggerAiOpLoc5: "ai_oploc5",

	TriggerApPlayer1: "applayer1",
	TriggerApPlayer2: "applayer2",
	TriggerApPlayer3: "applayer3",
	TriggerApPlayer4: "applayer4",
	TriggerApPlayer5: "applayer5",
	TriggerApPlayerU: "applayeru",
	TriggerApPlayerT: "applayert",
	TriggerOpPlayer1: "opplayer1",
	TriggerOpPlayer2: "opplayer2",
	TriggerOpPlayer3: "opplayer3",
	TriggerOpPlayer4: "opplayer4",
	TriggerOpPlayer5: "opplayer5",
	TriggerOpPlayerU: "opplayeru",
	TriggerOpPlayerT: "opplayert",

	TriggerAiApPlayer1: "ai_applayer1",
	TriggerAiApPlayer2: "ai_applayer2",
	TriggerAiApPlayer3: "ai_applayer3",
	TriggerAiApPlayer4: "ai_applayer4",
	TriggerAiApPlayer5: "ai_applayer5",
	TriggerAiOpPlayer1: "ai_opplayer1",
	TriggerAiOpPlayer2: "ai_opplayer2",
	TriggerAiOpPlayer3: "ai_opplayer3",
	TriggerAiOpPlayer4: "ai_opplayer4",
	TriggerAiOpPlayer5: "ai_opplayer5",

	TriggerQueue:     "queue",
	TriggerAiQueue1:  "ai_queue1",
	TriggerAiQueue2:  "ai_queue2",
	TriggerAiQueue3:  "ai_queue3",
	TriggerAiQueue4:  "ai_queue4",
	TriggerAiQueue5:  "ai_queue5",
	TriggerAiQueue6:  "ai_queue6",
	TriggerAiQueue7:  "ai_queue7",
	TriggerAiQueue8:  "ai_queue8",
	TriggerAiQueue9:  "ai_queue9",
	TriggerAiQueue10: "ai_queue10",
	TriggerAiQueue11: "ai_queue11",
	TriggerAiQueue12: "ai_queue12",
	TriggerAiQueue13: "ai_queue13",
	TriggerAiQueue14: "ai_queue14",
	TriggerAiQueue15: "ai_queue15",
	TriggerAiQueue16: "ai_queue16",
	TriggerAiQueue17: "ai_queue17",
	TriggerAiQueue18: "ai_queue18",
	TriggerAiQueue19: "ai_queue19",
	TriggerAiQueue20: "ai_queue20",

	TriggerSoftTimer: "softtimer",
	TriggerTimer:     "timer",
	TriggerAiTimer:   "ai_timer",

	TriggerOpHeld1: "opheld1",
	TriggerOpHeld2: "opheld2",
	TriggerOpHeld3: "opheld3",
	TriggerOpHeld4: "opheld4",
	TriggerOpHeld5: "opheld5",
	TriggerOpHeldU: "opheldu",
	TriggerOpHeldT: "opheldt",

	TriggerIfButton:   "if_button",
	TriggerIfClose:    "if_close",
	TriggerInvButton1: "inv_button1",
	TriggerInvButton2: "inv_button2",
	TriggerInvButton3: "inv_button3",
	TriggerInvButton4: "inv_button4",
	TriggerInvButton5: "inv_button5",
	TriggerInvButtonD: "inv_buttond",

	TriggerWalkTrigger:   "walktrigger",
	TriggerAiWalkTrigger: "ai_walktrigger",

	TriggerLogin:       "login",
	TriggerLogout:      "logout",
	TriggerTutorial:    "tutorial",
	TriggerAdvanceStat: "advancestat",
	TriggerMapZone:     "mapzone",
	TriggerMapZoneExit: "mapzoneexit",
	TriggerZone:        "zone",
	TriggerZoneExit:    "zoneexit",
	TriggerChangeStat:  "changestat",
	TriggerAiSpawn:     "ai_spawn",
	TriggerAiDespawn:   "ai_despawn",
}

// String returns the TS-faithful lowered enum name (e.g. TriggerOpNpc1
// returns "opnpc1", TriggerAiQueue4 returns "ai_queue4"). Mirrors TS
// ServerTriggerType.toString at Engine-TS/.../ServerTriggerType.ts:166-170:
//
//	ServerTriggerType[trigger].toLowerCase()
//
// Unknown values return "trigger_<N>" rather than panicking. TS would
// throw on `undefined.toLowerCase()`; Go's nil-handling and the
// debug-only call site (defaultOp under cfg.NodeDebug) make a sentinel
// safer (DEVIATION-NAI-148-D-STRING-FALLBACK).
func (t ServerTriggerType) String() string {
	if name, ok := serverTriggerNames[t]; ok {
		return name
	}
	return fmt.Sprintf("trigger_%d", int(t))
}
```

- [ ] **Step 1.4: Run test — expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/ -run TestServerTriggerType_String -v
```

Expected: PASS — all 46 table rows succeed.

- [ ] **Step 1.5: Run package + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./pkg/script/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./pkg/script/...
```

Expected: all green; no vet warnings.

- [ ] **Step 1.6: Commit**

```bash
git add pkg/script/trigger.go pkg/script/trigger_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(script): NAI-148 T1 — ServerTriggerType.String() + name table

Port TS ServerTriggerType.toString (Engine-TS/.../ServerTriggerType.ts:
166-170 — `ServerTriggerType[trigger].toLowerCase()`). 151-entry
serverTriggerNames map covers all declared Trigger* constants;
unknown values fall back to `trigger_<N>` per
DEVIATION-NAI-148-D-STRING-FALLBACK (TS would throw on
`undefined.toLowerCase()` — JS implicit error not portable).

TestServerTriggerType_String pins the table by representative spread
(46 rows): proc/label/debugproc, AP/OP families with U/T sentinels,
AI_ underscore retention, IF_/INV_ compound underscores, single-token
specials, and the unknown-value fallback (incl. gap value 22).

Closes nothing yet — T2 + T3 wire defaultOp.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2 — `tsTriggerForOpFire` mapper

**Files:**
- Modify: `modules/world/interaction.go`
- Modify: `modules/world/interaction_default_op_debug_test.go`

- [ ] **Step 2.1: Write the failing test**

Append to `modules/world/interaction_default_op_debug_test.go` (after the existing test functions, end of file):

```go
// TestTsTriggerForOpFire pins the goscape-targetOp → TS ServerTriggerType
// mapping used by defaultOp's debug chat. TS Player.ts:1093 emits
// `ServerTriggerType[targetOp+7]` where targetOp is the AP* trigger set
// by setInteraction; +7 maps AP* → OP*. Goscape stores targetOp as an
// op-slot int (1..5) or as one of the targetOp{Loc,Npc,Player,Obj}{T,U}
// sentinels (interaction.go:36-45). This helper bridges both namespaces.
//
// Sentinels override target type (TS L1086 — APNPCT/APPLAYERT/APLOCT/
// APOBJT all evaluate independent of target type); numeric op-slots
// disambiguate via target type-switch.
func TestTsTriggerForOpFire(t *testing.T) {
	s := newTestServer(t)
	npc := makeInteractionNpc(t, s, 1, 100, 100, 0)
	loc := entitypkg.NewLoc(0, 100, 100, 1, 1, entitypkg.LifecycleForever, 42, 10, 0)
	obj := entitypkg.NewObj(0, 100, 100, entitypkg.LifecycleForever, 42, 1)
	other, otherWait := makeInteractionPlayer(t, s, 100, 100, 0)
	defer otherWait()

	cases := []struct {
		name     string
		target   entity
		targetOp int
		want     script.ServerTriggerType
	}{
		// Numeric op-slots — disambiguate via target type.
		{"npc_slot1", npc, 1, script.TriggerOpNpc1},
		{"npc_slot5", npc, 5, script.TriggerOpNpc5},
		{"loc_slot2", loc, 2, script.TriggerOpLoc2},
		{"obj_slot3", obj, 3, script.TriggerOpObj3},
		{"player_slot4", other, 4, script.TriggerOpPlayer4},

		// Sentinels — match by targetOp regardless of target type.
		{"npc_T_sentinel", npc, targetOpNpcT, script.TriggerOpNpcT},
		{"npc_U_sentinel", npc, targetOpNpcU, script.TriggerOpNpcU},
		{"loc_T_sentinel", loc, targetOpLocT, script.TriggerOpLocT},
		{"obj_U_sentinel", obj, targetOpObjU, script.TriggerOpObjU},
		{"player_T_sentinel", other, targetOpPlayerT, script.TriggerOpPlayerT},

		// Sentinel/target-type mismatch — sentinel wins (TS-faithful).
		{"player_with_NpcT_sentinel", other, targetOpNpcT, script.TriggerOpNpcT},
		{"npc_with_LocT_sentinel", npc, targetOpLocT, script.TriggerOpLocT},

		// Fallback — nil target / out-of-range slot.
		{"nil_target", nil, 1, script.ServerTriggerType(-1)},
		{"bad_slot_99", npc, 99, script.ServerTriggerType(-1)},
	}

	for _, c := range cases {
		got := tsTriggerForOpFire(c.target, c.targetOp)
		if got != c.want {
			t.Errorf("%s: tsTriggerForOpFire(%T, %d) = %v (%q), want %v (%q)",
				c.name, c.target, c.targetOp, int(got), got.String(), int(c.want), c.want.String())
		}
	}
}
```

- [ ] **Step 2.2: Run test — expect FAIL (helper undefined)**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTsTriggerForOpFire -v
```

Expected: build failure — `tsTriggerForOpFire` undefined.

- [ ] **Step 2.3: Add the helper to `modules/world/interaction.go`**

Locate the `defaultOpDebugname` function in `modules/world/interaction.go` (currently lines 482-540). Insert `tsTriggerForOpFire` immediately AFTER `defaultOpDebugname` (i.e. after the closing `}` of `defaultOpDebugname`, before any subsequent function).

Code to insert (verbatim):

```go
// tsTriggerForOpFire returns the TS-faithful OP* ServerTriggerType for the
// given target/targetOp pair, used only by defaultOp's debug chat
// (NodeDebug-gated).
//
// TS Player.ts:1093 emits ServerTriggerType[targetOp+7] where targetOp is
// the AP* trigger set by setInteraction; +7 maps AP* -> OP*. Goscape stores
// targetOp as an op-slot int (1..5) or one of the targetOp{Loc,Npc,Player,Obj}
// {T,U} sentinels (interaction.go:36-45). This helper bridges both namespaces.
//
// Sentinel matches dispatch by targetOp alone (TS L1086 — APNPCT/APPLAYERT/
// APLOCT/APOBJT all evaluate independent of target type). Numeric op-slots
// disambiguate via target type. Returns ServerTriggerType(-1) when target is
// nil or unrecognised, or targetOp is out-of-range — goscape defensive; TS
// would throw via `undefined.toLowerCase()` (DEVIATION-NAI-148-D-OPFIRE-FALLBACK).
func tsTriggerForOpFire(target entity, targetOp int) script.ServerTriggerType {
	switch targetOp {
	case targetOpLocT:
		return script.TriggerOpLocT
	case targetOpLocU:
		return script.TriggerOpLocU
	case targetOpNpcT:
		return script.TriggerOpNpcT
	case targetOpNpcU:
		return script.TriggerOpNpcU
	case targetOpPlayerT:
		return script.TriggerOpPlayerT
	case targetOpPlayerU:
		return script.TriggerOpPlayerU
	case targetOpObjT:
		return script.TriggerOpObjT
	case targetOpObjU:
		return script.TriggerOpObjU
	}
	if targetOp < 1 || targetOp > 5 {
		return script.ServerTriggerType(-1)
	}
	offset := script.ServerTriggerType(targetOp - 1)
	switch target.(type) {
	case *Npc:
		return script.TriggerOpNpc1 + offset
	case *entitypkg.Loc:
		return script.TriggerOpLoc1 + offset
	case *entitypkg.Obj:
		return script.TriggerOpObj1 + offset
	case *Player:
		return script.TriggerOpPlayer1 + offset
	}
	return script.ServerTriggerType(-1)
}
```

No new imports required for this step — `entitypkg`, `script` are already imported in `interaction.go` (verify: top of file currently imports `github.com/zsrv/goscape/pkg/entity` aliased as `entitypkg` and `github.com/zsrv/goscape/pkg/script`).

- [ ] **Step 2.4: Run test — expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestTsTriggerForOpFire -v
```

Expected: PASS — all 14 table rows succeed.

- [ ] **Step 2.5: Run package + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./modules/world/...
```

Expected: all green; no vet warnings. (At this point `defaultOp` still emits the numeric form; `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue` is unchanged and still passes against `[8,test_npc]`.)

- [ ] **Step 2.6: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_default_op_debug_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-148 T2 — tsTriggerForOpFire targetOp namespace bridge

Add private helper that maps goscape's op-slot/sentinel targetOp
namespace (interaction.go:36-45) to the TS AP*/OP* ServerTriggerType
namespace expected by defaultOp's debug chat. Sentinels match by
targetOp alone (TS Player.ts:1086 — APNPCT et al. evaluate
independent of target type); numeric op-slots 1..5 disambiguate via
target type-switch over *Npc/*entitypkg.Loc/*entitypkg.Obj/*Player.

Defensive fallback returns ServerTriggerType(-1) on nil/unknown
target or out-of-range slot per
DEVIATION-NAI-148-D-OPFIRE-FALLBACK (TS would throw on
`undefined.toLowerCase()`).

TestTsTriggerForOpFire pins 14 cases: 5 numeric op-slots across all
four entity types; 5 sentinel matches; 2 sentinel/target-type
mismatches (sentinel wins, TS-faithful); 2 fallback rows.

Caller wiring (defaultOp) ships in T3.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3 — Wire `defaultOp`; update existing T4 fixture

**Files:**
- Modify: `modules/world/interaction.go`
- Modify: `modules/world/interaction_default_op_debug_test.go`

- [ ] **Step 3.1: Update the existing test to assert the TS-faithful name**

Edit `modules/world/interaction_default_op_debug_test.go`. Two surgical changes inside `TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue` (currently lines 32-56):

Find this block (currently lines 44-49):

```go
	// Goscape stores targetOp as an op-slot (1-5), not a ServerTriggerType
	// enum value. The +7 offset is preserved from TS Player.ts:1092
	// (NAI-147-D-TRIGGER-NAME-NUMERIC). For op-slot=1, emitted value = 1+7=8.
	// The script import is kept to anchor the deviation documentation.
	_ = script.TriggerOpNpc1 // deviation doc anchor — see NAI-147-D-TRIGGER-NAME-NUMERIC
	wantDebug := []byte("No trigger for [" + strconv.Itoa(1+7) + ",test_npc]")
```

Replace with:

```go
	// NAI-148 closed NAI-147-D-TRIGGER-NAME-NUMERIC: defaultOp emits
	// the TS-faithful lowered trigger name via tsTriggerForOpFire +
	// ServerTriggerType.String(). For an Npc target with targetOp=1
	// (op-slot 1), the helper resolves to script.TriggerOpNpc1 → "opnpc1".
	wantDebug := []byte("No trigger for [opnpc1,test_npc]")
```

Then, at the top of the file, locate the file-level header comment:

```go
// NAI-147 T4 — TS Player.ts:1076-1093 debug chat under NodeDebug.
// Numeric trigger-name fallback per NAI-147-D-TRIGGER-NAME-NUMERIC.
```

Replace with:

```go
// NAI-147 T4 — TS Player.ts:1076-1093 debug chat under NodeDebug.
// NAI-148 — TS-faithful trigger names via ServerTriggerType.String()
// + tsTriggerForOpFire (closed NAI-147-D-TRIGGER-NAME-NUMERIC).
```

Note on `strconv`: this import remains used by `TestDefaultOp_DebugnameNpc_FallbackToTypeId` (line 126: `strconv.Itoa(npc.typeId)`). Do NOT remove the import.

Note on `script`: this import remains used by lines 86, 104 (`&script.ScriptFile{...}`) and by the new `TestTsTriggerForOpFire` (T2). Do NOT remove the import.

- [ ] **Step 3.2: Run modified test — expect FAIL**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue -v
```

Expected: FAIL — wire bytes contain `[8,test_npc]` (`defaultOp` still emits numeric); `wantDebug = "No trigger for [opnpc1,test_npc]"` not found.

- [ ] **Step 3.3: Wire `defaultOp` to use `tsTriggerForOpFire`**

Edit `modules/world/interaction.go`. Find the `defaultOp` body (currently lines 467-477):

```go
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
			debugname := defaultOpDebugname(p, s)
			p.MessageGame(fmt.Sprintf("No trigger for [%d,%s]", p.targetOp+7, debugname))
		}
	}
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}
```

Replace the inner `p.MessageGame(...)` line (currently line 472) with:

```go
			trigger := tsTriggerForOpFire(p.target, p.targetOp)
			p.MessageGame(fmt.Sprintf("No trigger for [%s,%s]", trigger, debugname))
```

Final body of `defaultOp`:

```go
func defaultOp(p *Player, opTrigger, apTrigger *script.ScriptFile) {
	if p.client != nil && p.client.server != nil {
		s := p.client.server
		if s.cfg.NodeDebug && opTrigger == nil && apTrigger == nil {
			debugname := defaultOpDebugname(p, s)
			trigger := tsTriggerForOpFire(p.target, p.targetOp)
			p.MessageGame(fmt.Sprintf("No trigger for [%s,%s]", trigger, debugname))
		}
	}
	p.MessageGame("Nothing interesting happens.")
	p.waypointIndex = -1 // TS Player.ts:1096 — clearWaypoints()
}
```

Then update the doc-comment block immediately above `defaultOp` (currently lines 456-466). Find this block:

```go
// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// NAI-147 T4 closes NAI-78-D-DEBUG-MSG-DEFERRED: under cfg.NodeDebug
// (TS !NODE_PRODUCTION analogue) and both triggers nil, emit the TS
// L1076-1093 debug chat. NAI-147-D-TRIGGER-NAME-NUMERIC: trigger name
// emitted in numeric form because pkg/script.ServerTriggerType has no
// String() table — adding a 50+ entry name table for one debug-only
// chat is over-investment.
```

Replace with:

```go
// defaultOp implements the NIH (Not-Implemented-Here) fallback fired by
// tryInteract branch 4 when the player reaches operable distance but no
// [op…] script is registered. Mirrors LostCityRS/Engine-TS
// Player.ts:1072-1097.
//
// NAI-147 T4 closed NAI-78-D-DEBUG-MSG-DEFERRED: under cfg.NodeDebug
// (TS !NODE_PRODUCTION analogue) and both triggers nil, emit the TS
// L1076-1093 debug chat.
//
// NAI-148 closed NAI-147-D-TRIGGER-NAME-NUMERIC: trigger name now
// resolved via tsTriggerForOpFire(p.target, p.targetOp).String() —
// emits TS-faithful lowered name (e.g. "opnpc1") instead of the
// numeric `targetOp+7` form. tsTriggerForOpFire bridges goscape's
// op-slot/sentinel namespace to TS's AP*/OP* ServerTriggerType.
```

- [ ] **Step 3.4: Run modified test — expect PASS**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue -v
```

Expected: PASS — wire bytes contain `[opnpc1,test_npc]`.

- [ ] **Step 3.5: Run full module + adjacency fence**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test ./modules/world/ -run TestDefaultOp_ -v
```

Expected: all green. The 15 `TestDefaultOp_*` tests in `interaction_default_op_debug_test.go` all pass — 14 unchanged tests (which only assert `,debugname]` substrings, not the trigger-name half) plus the updated NoTrigger test.

- [ ] **Step 3.6: Run full repo with race detector + vet**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go test -race ./...
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache go vet ./...
```

Expected: all green; no vet warnings; no `NAI-147-D-TRIGGER-NAME-NUMERIC` references remain in production code (verify next step).

- [ ] **Step 3.7: Verify deviation-tag retirement**

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg "NAI-147-D-TRIGGER-NAME-NUMERIC" pkg/ modules/
```

Expected: zero matches in production code. (The tag may still appear in `docs/superpowers/specs/2026-05-10-nai-147-*-design.md` and the spec just landed for NAI-148 — those are intentional historical references, not active deviations.)

```
GOPATH=$TMPDIR/go GOCACHE=$TMPDIR/go-cache rg "DEVIATION-NAI-148" pkg/ modules/
```

Expected: exactly two matches — `DEVIATION-NAI-148-D-STRING-FALLBACK` in `pkg/script/trigger.go` (above `String()`) and `DEVIATION-NAI-148-D-OPFIRE-FALLBACK` in `modules/world/interaction.go` (above `tsTriggerForOpFire`).

- [ ] **Step 3.8: Commit**

```bash
git add modules/world/interaction.go modules/world/interaction_default_op_debug_test.go
git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(world): NAI-148 T3 — wire defaultOp to TS-faithful trigger name

Replace `[%d,%s] targetOp+7` with `[%s,%s]
tsTriggerForOpFire(p.target, p.targetOp)`. Mirrors TS Player.ts:1093
(`ServerTriggerType[targetOp+7].toLowerCase()`) — for an OpNpc1 click
on a rat, defaultOp now emits `No trigger for [opnpc1,rat]` instead
of `No trigger for [8,rat]`.

Update doc-comment to retire the NAI-147-D-TRIGGER-NAME-NUMERIC
deviation note (replaced by the closure crumb pointing at
tsTriggerForOpFire). Update TestDefaultOp_NoTriggerEmitsDebug_NodeDebugTrue
wire assertion + drop the `_ = script.TriggerOpNpc1` deviation
anchor; replace the file-level header comment with NAI-148 closure
note.

Closes NAI-147-D-TRIGGER-NAME-NUMERIC.

Closes memory: nai_followups.md NAI-147-D-TRIGGER-NAME-NUMERIC

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Post-T3 — End-of-impl review (controller, single Sonnet reviewer)

After T3 commits land, the controller dispatches a single combined Sonnet code-review subagent (per `superpowers_code_reviewer_model.md` — never Opus) covering all three commits as one bundle. Reviewer brief:

- **Correctness:** verify the 151-entry `serverTriggerNames` table against `Engine-TS/.../ServerTriggerType.ts:1-162` for any missing entries or wrong lowercased name.
- **TS-fidelity of `tsTriggerForOpFire`:** confirm sentinels-first semantics match TS L1086 (sentinels evaluate independent of target type).
- **Doc-comment retirement:** confirm `NAI-147-D-TRIGGER-NAME-NUMERIC` is fully retired in production code (only the historical NAI-147 spec retains it).
- **No leaked deviations:** confirm both DEVIATION-NAI-148-D-* tags are correctly placed and rationalised inline.

Reviewer fixups land as follow-up commits (T3-fixup style) before close.

## Post-review — Close commit

After review fixups (if any) land, controller writes a `chore(close): NAI-148` commit per `close_commit_memory_trailer.md`. Body summarises the bundle, lists tracked deviations, applies pattern-memory crumbs, and ends with `Closes memory: nai_followups.md NAI-147-D-TRIGGER-NAME-NUMERIC`.

No PRIMARY smoke required (per spec §7); SECONDARY pins are the test surface above.

---

## Memory / pattern crumbs to apply at close

- `compressed_cadence` — 15-100 LOC band confirmed at close.
- `superpowers_code_reviewer_model` — Sonnet reviewer used.
- `defensive_gate_doc_comment_label` — both deviations labelled inline at the function whose behavior diverges.
- `true_to_ts_gate` — both tracked with rationale + no follow-up.
- `close_commit_memory_trailer` — close commit ends with `Closes memory:` trailer.
- `verify_implementer_claims` — controller verifies fresh build/test green at each task HEAD before approving.
- `feedback_subagent_wt_path` — `git status` after each commit confirms clean main working tree.
- `plan_runnable_test_fixtures` — controller mentally executed both test fixtures at plan-write; identifiers cross-checked against existing helpers (`makeInteractionNpc`, `makeInteractionPlayer`, `entitypkg.NewLoc`, `entitypkg.NewObj`, `targetOp{Npc,Loc,Obj,Player}{T,U}`).
