package script

import "testing"

// newPoolTestScript builds a minimal one-instruction script (immediate
// RETURN) with a few locals declared, mirroring the fixture idiom of
// modules/world/world_script_queue_test.go.
func newPoolTestScript() *ScriptFile {
	return &ScriptFile{
		Name:             "pool_test",
		Opcodes:          []Opcode{OpReturn},
		IntOperands:      []int32{0},
		StringOperands:   []string{""},
		InstructionCount: 1,
		IntLocalCount:    4,
		StringLocalCount: 2,
	}
}

func frameIsZero(f Frame) bool {
	return f.Script == nil && f.PC == 0 && f.IntLocals == nil && f.StringLocals == nil
}

func TestReleaseClearsAndNils(t *testing.T) {
	s := Init(newPoolTestScript(), nil, false, []int{7}, []string{"a"})
	s.PushInt(42)
	s.PushString("secret")
	s.Frames[0] = Frame{Script: s.Script, PC: 9, IntLocals: []int{1}}
	b := s.buf
	if b == nil {
		t.Fatal("Init did not attach pooled buffers (s.buf == nil)")
	}

	Release(s)

	if s.IntStack != nil || s.StringStack != nil || s.Frames != nil || s.buf != nil {
		t.Fatal("Release must nil the state's slice fields and buf")
	}
	for i, v := range b.stringStack {
		if v != "" {
			t.Fatalf("stringStack[%d] not cleared: %q", i, v)
		}
	}
	for i, f := range b.frames {
		if !frameIsZero(f) {
			t.Fatalf("frames[%d] not cleared: %+v", i, f)
		}
	}

	Release(s)   // double release must be a no-op
	Release(nil) // nil must be a no-op
}

func TestReleaseNoopOnUnpooledState(t *testing.T) {
	// Literal &ScriptState{} fixtures exist throughout the test suites;
	// Release must tolerate them.
	s := &ScriptState{}
	Release(s) // must not panic
}

func TestInitReusesReleasedBuffers(t *testing.T) {
	// sync.Pool gives no hard reuse guarantee (a background GC empties
	// it), so accept reuse on ANY of 5 attempts; all-5 misses means the
	// pool wiring is broken.
	reused := false
	for range 5 {
		s1 := Init(newPoolTestScript(), nil, false, nil, nil)
		p := &s1.IntStack[0]
		Release(s1)
		s2 := Init(newPoolTestScript(), nil, false, nil, nil)
		if &s2.IntStack[0] == p {
			reused = true
		}
		Release(s2)
	}
	if !reused {
		t.Fatal("no buffer reuse observed across 5 Release→Init cycles")
	}
}

// The spec's stale-leak pin: nothing from a released run may be observable
// in a state built on the recycled buffers.
func TestRecycledStateLeaksNothing(t *testing.T) {
	s1 := Init(newPoolTestScript(), nil, false, []int{111, 222}, []string{"stale-local"})
	s1.PushInt(31337)
	s1.PushString("stale-stack")
	s1.Frames[0] = Frame{Script: s1.Script, PC: 77, IntLocals: []int{9}}
	Release(s1)

	s2 := Init(newPoolTestScript(), nil, false, nil, nil)
	defer Release(s2)

	if got := s2.PopInt(); got != 0 {
		t.Errorf("empty-stack PopInt on recycled state = %d, want 0 (underflow default)", got)
	}
	if got := s2.PopString(); got != "" {
		t.Errorf("empty-stack PopString on recycled state = %q, want \"\"", got)
	}
	for i, v := range s2.IntLocals {
		if v != 0 {
			t.Errorf("IntLocals[%d] = %d, want 0 (locals must be fresh allocations)", i, v)
		}
	}
	for i, v := range s2.StringLocals {
		if v != "" {
			t.Errorf("StringLocals[%d] = %q, want \"\" (locals must be fresh allocations)", i, v)
		}
	}
	if s2.FrameSP != 0 {
		t.Errorf("FrameSP = %d, want 0", s2.FrameSP)
	}
	if !frameIsZero(s2.Frames[0]) {
		t.Errorf("Frames[0] not zero on recycled state: %+v", s2.Frames[0])
	}
}

func BenchmarkInitRelease(b *testing.B) {
	sf := newPoolTestScript()
	b.ReportAllocs()
	for b.Loop() {
		Release(Init(sf, nil, false, nil, nil))
	}
}

// Regression gate on the win itself: pre-pool Init allocates ~26.5 KB/op
// (two 1024-cap stacks + 50 frames). Post-pool steady state must be under
// 4 KiB/op (struct + locals only). Generous threshold — not flaky.
func TestInitReleaseAllocBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark-backed gate; skipped in -short")
	}
	res := testing.Benchmark(BenchmarkInitRelease)
	if bpo := res.AllocedBytesPerOp(); bpo > 4096 {
		t.Fatalf("Init+Release allocates %d B/op, want ≤4096 (buffer-pool regression; pre-pool ≈26500)", bpo)
	}
}
