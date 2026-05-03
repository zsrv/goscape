package world

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/script"
)

func TestChebDist(t *testing.T) {
	tests := []struct {
		name                   string
		ax, az, bx, bz, expect int
	}{
		{"same tile", 5, 5, 5, 5, 0},
		{"adjacent N", 5, 5, 5, 4, 1},
		{"diagonal", 5, 5, 6, 6, 1},
		{"two tiles E", 5, 5, 7, 5, 2},
		{"asymmetric 3x1", 5, 5, 8, 6, 3},
		{"negative direction", 10, 10, 7, 7, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := chebDist(tc.ax, tc.az, tc.bx, tc.bz)
			if got != tc.expect {
				t.Errorf("chebDist(%d,%d,%d,%d) = %d, want %d",
					tc.ax, tc.az, tc.bx, tc.bz, got, tc.expect)
			}
		})
	}
}

func TestTargetKindString(t *testing.T) {
	loc := entitypkg.NewLoc(0, 1, 1, 1, 1, entitypkg.LifecycleForever, 0, 0, 0)
	obj := entitypkg.NewObj(0, 1, 1, entitypkg.LifecycleForever, 0, 1)
	npc := &Npc{}
	plr := &Player{}

	tests := []struct {
		name   string
		target entity
		expect string
	}{
		{"loc", loc, "Loc"},
		{"obj", obj, "Obj"},
		{"npc", npc, "Npc"},
		{"player", plr, "Player"},
		{"nil", nil, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := targetKindString(tc.target)
			if got != tc.expect {
				t.Errorf("targetKindString(%T) = %q, want %q",
					tc.target, got, tc.expect)
			}
		})
	}
}

// capturingHandler is a slog.Handler that retains every Record passed to
// Handle so tests can assert on emitted frames.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// newCapturingLogger returns a logger and the handler so tests can pull
// records back out. The logger emits at Debug level (matching production
// usage of s.log.Debug for instrumentation).
func newCapturingLogger() (*slog.Logger, *capturingHandler) {
	h := &capturingHandler{}
	return slog.New(h), h
}

// findRecord returns the first record with the given message, or nil.
func findRecord(records []slog.Record, msg string) *slog.Record {
	for i := range records {
		if records[i].Message == msg {
			return &records[i]
		}
	}
	return nil
}

// attrValue extracts the value of attribute `key` from `r`. Returns
// (slog.Value{}, false) if not found.
func attrValue(r slog.Record, key string) (slog.Value, bool) {
	var found slog.Value
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

// requireAttr fails the test if `key` is missing from `r` or its value
// (compared via String()) doesn't match `want`.
func requireAttr(t *testing.T, r slog.Record, key, want string) {
	t.Helper()
	v, ok := attrValue(r, key)
	if !ok {
		t.Fatalf("record %q missing attr %q", r.Message, key)
	}
	if got := v.String(); got != want {
		t.Errorf("record %q attr %q = %q, want %q", r.Message, key, got, want)
	}
}

func TestRecordTryInteractBranch(t *testing.T) {
	tests := []struct {
		name              string
		slot, branch      int
		expectPre, expectPost int
	}{
		{"slot 0 writes pre", 0, 1, 1, 0},
		{"slot 1 writes post", 1, 3, 0, 3},
		{"slot 0 branch 4", 0, 4, 4, 0},
		{"slot 1 branch 2", 1, 2, 0, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Player{}
			p.interactCallSlot = tc.slot
			recordTryInteractBranch(p, tc.branch)
			if p.lastInteractBranchPre != tc.expectPre {
				t.Errorf("pre: got %d, want %d", p.lastInteractBranchPre, tc.expectPre)
			}
			if p.lastInteractBranchPost != tc.expectPost {
				t.Errorf("post: got %d, want %d", p.lastInteractBranchPost, tc.expectPost)
			}
		})
	}
}

func TestTryInteractBranchTrackingPerCallsite(t *testing.T) {
	// Each row drives tryInteract to a specific branch and asserts
	// the recorded branch id for both pre-step (slot=0) and
	// post-step (slot=1) calls. The fixture builds a player with a
	// Loc target and tweaks state per row to force the branch.
	tests := []struct {
		name           string
		setup          func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile)
		postSetup      func(p *Player)
		allowOpScenery bool
		wantBranch     int
	}{
		{
			name: "branch 1 (op + operable + scenery allowed)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				// player adjacent to loc → operable. opTrigger present
				// via registered script. allowOpScenery=true (so non-
				// PathingEntity Loc target qualifies).
				p.x, p.z = 99, 100
				registerOpLocScript(t, s, loc.Type(), 1, sf)
			},
			postSetup:      func(p *Player) {},
			allowOpScenery: true,
			wantBranch:     1,
		},
		{
			name: "branch 2 (ap + approach)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				// player 2 tiles away → approach but not operable. ap
				// trigger present, ap_range default 10.
				p.x, p.z = 98, 100
				p.apRange = 10
				registerApLocScript(t, s, loc.Type(), 1, sf)
			},
			postSetup:  func(p *Player) {},
			wantBranch: 2,
		},
		{
			name: "branch 3 (approach + ap nil)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 98, 100
				p.apRange = 10
				// no scripts registered: ap trigger nil → branch 3.
			},
			postSetup:  func(p *Player) {},
			wantBranch: 3,
		},
		{
			name: "branch 4 (operable + scenery allowed + op nil)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 99, 100
				// no scripts registered: op trigger nil; allowOpScenery
				// flips on; player is operable → branch 4 (NIH).
			},
			// SetInteraction resets apRange=10; fix after SetInteraction call.
			postSetup:      func(p *Player) { p.apRange = 0 },
			allowOpScenery: true,
			wantBranch:     4,
		},
		{
			// Pins the SECOND branch-2 site at interaction.go:379
			// (`nextTarget==nil && apRangeCalled` retry-no-op return false).
			// Skip tryFireApTrigger by setting interactionFired=true so
			// the gate is reached with the state set by postSetup.
			name: "branch 2 retry (apRangeCalled, nextTarget=nil)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 98, 100
				p.apRange = 10
				registerApLocScript(t, s, loc.Type(), 1, sf)
			},
			postSetup: func(p *Player) {
				p.interactionFired = true // skip tryFireApTrigger
				p.apRangeCalled = true    // force retry-arm guard
			},
			wantBranch: 2,
		},
		{
			name: "fallthrough (operable but allowOpScenery=false, no triggers)",
			setup: func(s *Server, p *Player, loc *entitypkg.Loc, sf *script.ScriptFile) {
				p.x, p.z = 99, 100
				// allowOpScenery=false; Loc target is non-PathingEntity;
				// branch 1 fails. No ap script. branch 2 fails. Not in
				// approach without ap_range>0 (default in fixture is 0).
				p.apRange = 0
				// branch 3 needs approach=true; with ap_range=0 it's
				// false. branch 4 needs allowOpScenery; false here.
				// Returns false at fallthrough → branch 0.
			},
			// SetInteraction resets apRange=10; fix after SetInteraction call.
			postSetup:      func(p *Player) { p.apRange = 0 },
			allowOpScenery: false,
			wantBranch:     0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+" pre-slot", func(t *testing.T) {
			s, p, loc, _ := makeOpLocFixture(t)
			s.scriptProvider = script.NewProvider() // empty; per-row setup re-seeds
			sf := buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 1)
			tc.setup(s, p, loc, sf)

			// Engage interaction so tryInteract has a target.
			p.SetInteraction(InteractionEngine, loc, 1, -1)
			tc.postSetup(p) // re-applies setup's apRange override after SetInteraction's reset

			p.interactCallSlot = 0
			_ = p.tryInteract(tc.allowOpScenery)
			if p.lastInteractBranchPre != tc.wantBranch {
				t.Errorf("pre: got branch %d, want %d", p.lastInteractBranchPre, tc.wantBranch)
			}
			if p.lastInteractBranchPost != 0 {
				t.Errorf("post unexpectedly set: %d", p.lastInteractBranchPost)
			}
		})
		t.Run(tc.name+" post-slot", func(t *testing.T) {
			s, p, loc, _ := makeOpLocFixture(t)
			s.scriptProvider = script.NewProvider()
			sf := buildPOpLocScript(script.TriggerOpLoc1, loc.Type(), 1)
			tc.setup(s, p, loc, sf)
			p.SetInteraction(InteractionEngine, loc, 1, -1)
			tc.postSetup(p) // re-applies setup's apRange override after SetInteraction's reset

			p.interactCallSlot = 1
			_ = p.tryInteract(tc.allowOpScenery)
			if p.lastInteractBranchPost != tc.wantBranch {
				t.Errorf("post: got branch %d, want %d", p.lastInteractBranchPost, tc.wantBranch)
			}
			if p.lastInteractBranchPre != 0 {
				t.Errorf("pre unexpectedly set: %d", p.lastInteractBranchPre)
			}
		})
	}
}

// registerOpLocScript registers a [oploc<N>,<typeID>] script via the
// existing TriggerOpLoc1 + LookupKeyForType convention. The OP-side
// trigger is computed inline as `script.TriggerOpLoc1 + (op-1)` since
// goscape only exposes apLocTriggerForOp; the OP variant is the AP
// variant + 7 (TS offset convention; see interaction_trigger.go:194-198).
func registerOpLocScript(t *testing.T, s *Server, typeID int, op int, sf *script.ScriptFile) {
	t.Helper()
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	trigger := script.TriggerOpLoc1 + script.ServerTriggerType(op-1)
	sf.LookupKey = script.LookupKeyForType(trigger, typeID)
	s.scriptProvider.Register(sf)
}

func registerApLocScript(t *testing.T, s *Server, typeID int, op int, sf *script.ScriptFile) {
	t.Helper()
	if s.scriptProvider == nil {
		s.scriptProvider = script.NewProvider()
	}
	trigger, ok := apLocTriggerForOp(op)
	if !ok {
		t.Fatalf("apLocTriggerForOp(%d) returned ok=false", op)
	}
	sf.LookupKey = script.LookupKeyForType(trigger, typeID)
	s.scriptProvider.Register(sf)
}

func TestInteractionFrameB_EmittedWhenTargetSetAndNodeDebugTrue(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true
	s.scriptProvider = script.NewProvider() // empty; force fallthrough/branch 3

	// Place player 2 tiles away — approach distance with default ap_range=10
	// (set by SetInteraction). Target Loc; no scripts → branch 3 pre-step,
	// then post-step pathToTarget no-op (Loc has no waypoints generated for
	// shape-blind path in this fixture; that's acceptable here — the test
	// only verifies frame emission, not pathing correctness).
	p.x, p.z = 98, 100

	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.uid = 12345

	p.processInteraction()

	rec := findRecord(h.snapshot(), "interaction tick")
	if rec == nil {
		t.Fatal("expected one 'interaction tick' record; got none")
	}
	requireAttr(t, *rec, "player_uid", "12345")
	requireAttr(t, *rec, "target_kind", "Loc")
	if v, ok := attrValue(*rec, "target_x"); !ok || v.Int64() != 100 {
		t.Errorf("target_x: got %v, want 100", v)
	}
	if v, ok := attrValue(*rec, "target_z"); !ok || v.Int64() != 100 {
		t.Errorf("target_z: got %v, want 100", v)
	}
	if _, ok := attrValue(*rec, "cheb_dist"); !ok {
		t.Errorf("cheb_dist missing")
	}
	if _, ok := attrValue(*rec, "branch_pre"); !ok {
		t.Errorf("branch_pre missing")
	}
	if _, ok := attrValue(*rec, "branch_post"); !ok {
		t.Errorf("branch_post missing")
	}
	if _, ok := attrValue(*rec, "waypoint_idx"); !ok {
		t.Errorf("waypoint_idx missing")
	}
	if _, ok := attrValue(*rec, "target_still_set"); !ok {
		t.Errorf("target_still_set missing")
	}
}

func TestInteractionFrameB_SuppressedWhenNoTargetAtEntry(t *testing.T) {
	s, p, _, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = true

	// p.target is nil (default after newTestPlayer); processInteraction
	// short-circuits at the first guard. No frame should emit.
	p.processInteraction()

	if rec := findRecord(h.snapshot(), "interaction tick"); rec != nil {
		t.Errorf("unexpected 'interaction tick' record: %v", rec)
	}
}

func TestInteractionFrameB_SuppressedWhenNodeDebugFalse(t *testing.T) {
	s, p, loc, _ := makeOpLocFixture(t)
	logger, h := newCapturingLogger()
	s.log = logger
	s.cfg.NodeDebug = false
	s.scriptProvider = script.NewProvider()

	p.x, p.z = 98, 100
	p.SetInteraction(InteractionEngine, loc, 1, -1)
	p.processInteraction()

	if rec := findRecord(h.snapshot(), "interaction tick"); rec != nil {
		t.Errorf("unexpected 'interaction tick' record: %v", rec)
	}
}
