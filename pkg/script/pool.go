package script

import "sync"

// scriptBuffers bundles the three fixed-capacity buffers every ScriptState
// carries, pooled as one unit (PERF-3). Profiling on the idle singleplayer
// server (2026-07-09) attributed 71% of allocation churn to Init allocating
// these fresh per run (~26 KB: 8 KB IntStack + 16 KB StringStack + Frames).
//
// TS allocates fresh arrays per init (ScriptRunner.ts:66-119,
// ScriptState.ts:39-146) and never reuses them; pooling is a Go-only
// optimization. It is behavior-invisible because stack reads are strictly
// SP-disciplined — PopInt/PopString (state.go) only read slots pushed
// earlier in the same run — so stale values in a recycled intStack are
// unobservable. Everything with read-before-write zero-init semantics
// (IntLocals, StringLocals, Arrays, the state struct itself) is NOT pooled
// and keeps coming from fresh make/new. See
// docs/superpowers/specs/2026-07-09-scriptstate-buffer-pool-design.md.
type scriptBuffers struct {
	intStack    []int
	stringStack []string
	frames      []Frame
}

var buffersPool = sync.Pool{
	New: func() any {
		return &scriptBuffers{
			intStack:    make([]int, StackCapacity),
			stringStack: make([]string, StackCapacity),
			frames:      make([]Frame, FrameCapacity),
		}
	},
}

// Release recycles s's stack buffers into the pool. Call ONLY when s is
// provably unreferenced: the Finished/Aborted dispatch arms in
// modules/world, after OnScriptFinishedOrAborted's pointer-identity guard
// has cleared any matching activeScript. Never call it on a suspended
// state — its buffers stay live (held via Player/Npc activeScript or the
// world script queue) until it terminates through those same arms. A missed
// Release is safe: the buffers just fall to the GC as before pooling.
//
// stringStack and frames are cleared before pooling so recycled bundles pin
// no string/*ScriptFile/locals references; intStack is intentionally left
// dirty (slots above SP are never read). The state's slice fields and buf
// are nil'd so use-after-release panics on a nil slice instead of silently
// aliasing a recycled buffer, and so a double Release is a no-op.
func Release(s *ScriptState) {
	if s == nil || s.buf == nil {
		return
	}
	b := s.buf
	clear(b.stringStack)
	clear(b.frames)
	s.IntStack = nil
	s.StringStack = nil
	s.Frames = nil
	s.buf = nil
	buffersPool.Put(b)
}
