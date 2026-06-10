package world

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// ref225CacheDir is defined in testdata_path_test.go — it resolves the
// Server225_2 reference cache (rev-225 component layout) and skips when
// the reference checkout is unavailable. The repo's own data/pack is not
// used here: the git common-dir fallback can resolve a different revision's
// cache (e.g. 245.2-format) that rev-225's decoders can no longer read.

func TestReload_FreshLoad_PopulatesAllRegistries(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload returned err: %v", err)
	}
	if s.paramTypes == nil || len(s.paramTypes.Configs) == 0 {
		t.Errorf("paramTypes empty post-reload")
	}
	if s.objTypes == nil || len(s.objTypes.Configs) == 0 {
		t.Errorf("objTypes empty post-reload")
	}
	if s.locTypes == nil || len(s.locTypes.Configs) == 0 {
		t.Errorf("locTypes empty post-reload")
	}
	if s.npcTypes == nil || len(s.npcTypes.Configs) == 0 {
		t.Errorf("npcTypes empty post-reload")
	}
	if s.invTypes == nil || len(s.invTypes.Configs) == 0 {
		t.Errorf("invTypes empty post-reload")
	}
	if s.varpTypes == nil || s.varsTypes == nil || s.varnTypes == nil {
		t.Errorf("var*Types empty post-reload")
	}
	if s.enumTypes == nil || s.structTypes == nil {
		t.Errorf("enum/struct types empty post-reload")
	}
	if s.seqTypes == nil || s.spotanimTypes == nil || s.idkTypes == nil {
		t.Errorf("seq/spotanim/idk types empty post-reload")
	}
	if s.mesanimTypes == nil || s.dbTableTypes == nil || s.dbRowTypes == nil || s.dbTableIndex == nil {
		t.Errorf("mesanim/dbtable/dbrow/dbtableindex empty post-reload")
	}
	if s.huntTypes == nil || s.componentTypes == nil {
		t.Errorf("hunt/component types empty post-reload")
	}
}

func TestReload_PreservesIdentitySwap(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	if err := s.Reload(true); err != nil {
		t.Fatalf("first Reload: %v", err)
	}
	objBefore := s.objTypes
	locBefore := s.locTypes
	if err := s.Reload(true); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	if s.objTypes == objBefore {
		t.Errorf("s.objTypes pointer unchanged across reloads (expected fresh instance)")
	}
	if s.locTypes == locBefore {
		t.Errorf("s.locTypes pointer unchanged across reloads (expected fresh instance)")
	}
}

// newTestServerWithCachePath builds a fresh Server using the real
// objtype loaders against cachePath. Mirrors NewServer's loader
// sequence (modules/world/server.go) minus tick / TCP setup.
// Used only by reload tests that need a fully-populated registry set.
func newTestServerWithCachePath(t *testing.T, cachePath string) *Server {
	t.Helper()
	s := newTestServer(t)
	s.cfg.CachePath = cachePath
	s.cfg.NodeDebug = true
	s.gamemap = nil // reload's GameMap re-injection step is gated on s.gamemap != nil; tested separately in T7
	return s
}

func TestResizeVarShared_CountUnchanged_ReturnsInputs(t *testing.T) {
	oldVars := []int32{10, 20, 30}
	oldStrs := []string{"a", "b", "c"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt},
		{Type: objtype.ScriptVarTypeInt},
		{Type: objtype.ScriptVarTypeInt},
	}
	newVars, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
	// No allocation expected — pointer-identity check.
	if &newVars[0] != &oldVars[0] {
		t.Errorf("expected pass-through on count match (no realloc)")
	}
	if newStrs[0] != "a" || newStrs[2] != "c" {
		t.Errorf("strs not preserved on pass-through: %v", newStrs)
	}
}

func TestResizeVarShared_CountGrew_ClobbersAllNonStringSlots(t *testing.T) {
	oldVars := []int32{10, 20, 30}
	oldStrs := []string{"a", "b", "c"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt}, // i=0: was 10 → clobbered to 0
		{Type: objtype.ScriptVarTypeInt}, // i=1: was 20 → clobbered to 0
		{Type: objtype.ScriptVarTypeInt}, // i=2: was 30 → clobbered to 0
		{Type: objtype.ScriptVarTypeObj}, // i=3: net-new, OBJ default = -1
		{Type: objtype.ScriptVarTypeLoc}, // i=4: net-new, non-INT non-STRING default = -1
	}
	newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
	want := []int32{0, 0, 0, -1, -1}
	for i, v := range want {
		if newVars[i] != v {
			t.Errorf("newVars[%d]: got %d, want %d (DEVIATION-NAI-190-D3-CANDIDATE clobber-after-copy)", i, newVars[i], v)
		}
	}
}

func TestResizeVarShared_StringType_KeepsCopiedValue(t *testing.T) {
	oldVars := []int32{0, 0, 0}
	oldStrs := []string{"hello", "world", "foo"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString},
		{Type: objtype.ScriptVarTypeString}, // net-new STRING slot
	}
	_, newStrs := resizeVarShared(oldVars, oldStrs, cfgs)
	if newStrs[0] != "hello" || newStrs[1] != "world" || newStrs[2] != "foo" {
		t.Errorf("STRING slots clobbered: %v (expected [hello world foo \"\"])", newStrs)
	}
	if newStrs[3] != "" {
		t.Errorf("net-new STRING slot non-empty: %q", newStrs[3])
	}
}

func TestResizeVarShared_NilConfigSlot_Skipped(t *testing.T) {
	oldVars := []int32{10}
	oldStrs := []string{"x"}
	cfgs := []*objtype.VarSharedType{
		{Type: objtype.ScriptVarTypeInt},
		nil, // defensive
		{Type: objtype.ScriptVarTypeInt},
	}
	newVars, _ := resizeVarShared(oldVars, oldStrs, cfgs)
	if len(newVars) != 3 {
		t.Fatalf("newVars len: got %d, want 3", len(newVars))
	}
	if newVars[1] != 0 {
		t.Errorf("nil-config slot should remain zero: got %d", newVars[1])
	}
}

func TestReconcileInvs_Shared_RebuildsFreshFromType(t *testing.T) {
	sentinel := &inventory.Inventory{} // distinguishable from FromType output
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(5, map[int]int{3: objtype.InvTypeScopeShared}),
	}
	fresh := reconcileInvs(nil, invTypes)
	if fresh[3] == sentinel {
		t.Errorf("SHARED id 3 not replaced with fresh inv (still sentinel)")
	}
	if fresh[3] == nil {
		t.Errorf("SHARED id 3 missing fresh inv")
	}
}

func TestReconcileInvs_Temp_DeletesFromAllPlayers(t *testing.T) {
	sentinel := &inventory.Inventory{}
	p1 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
	p2 := &Player{invs: map[int]*inventory.Inventory{7: sentinel}}
	players := []*Player{nil, p1, p2} // index 0 is nil per goscape's slot-1-indexed convention
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(10, map[int]int{7: objtype.InvTypeScopeTemp}),
	}
	_ = reconcileInvs(players, invTypes)
	if _, ok := p1.invs[7]; ok {
		t.Errorf("p1.invs[7] should be deleted")
	}
	if _, ok := p2.invs[7]; ok {
		t.Errorf("p2.invs[7] should be deleted")
	}
}

func TestReconcileInvs_Perm_LeftUntouched(t *testing.T) {
	sentinel := &inventory.Inventory{}
	p1 := &Player{invs: map[int]*inventory.Inventory{9: sentinel}}
	invTypes := &objtype.InvTypeConfigs{
		Configs: makeInvConfigs(10, map[int]int{9: objtype.InvTypeScopePerm}),
	}
	_ = reconcileInvs([]*Player{p1}, invTypes)
	if p1.invs[9] != sentinel {
		t.Errorf("SCOPE_PERM inv reconciled (should be untouched)")
	}
}

func TestReconcileInvs_NilInvTypes_ReturnsEmptyMap(t *testing.T) {
	fresh := reconcileInvs(nil, nil)
	if fresh == nil {
		t.Fatal("expected empty non-nil map, got nil")
	}
	if len(fresh) != 0 {
		t.Errorf("expected empty map, got %d entries", len(fresh))
	}
}

// makeInvConfigs builds a []*objtype.InvType of size n with default
// InvTypeScopePerm, overriding specific ids per the scopes map.
func makeInvConfigs(n int, scopes map[int]int) []*objtype.InvType {
	configs := make([]*objtype.InvType, n)
	for i := range n {
		configs[i] = &objtype.InvType{Scope: objtype.InvTypeScopePerm}
	}
	for id, scope := range scopes {
		configs[id].Scope = scope
	}
	return configs
}

func TestReload_ScriptCount_NodeDebug_SuccessBroadcast(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	s.cfg.NodeDebug = true
	var captured []string
	s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(captured) == 0 {
		t.Fatal("expected broadcast on NodeDebug=true success path")
	}
	last := captured[len(captured)-1]
	if !strings.HasPrefix(last, "Loaded ") || !strings.HasSuffix(last, " scripts.") {
		t.Errorf("broadcast: got %q, want \"Loaded N scripts.\"", last)
	}
}

// To exercise the scripts-failure-only branch, we need a cache where
// every loader except scripts succeeds. Copy real cache without
// server/script.{dat,idx}.
func TestReload_ScriptCount_NodeDebug_FailureBroadcast_PartialCache(t *testing.T) {
	cacheDir := copyCacheExcept(t, ref225CacheDir(t), "server/script.dat", "server/script.idx")
	s := newTestServerWithCachePath(t, cacheDir)
	s.cfg.NodeDebug = true
	s.scriptProvider = script.NewProvider()
	var captured []string
	s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
	_ = s.Reload(true) // earlier loaders succeed; only scripts.Load fails
	if len(captured) == 0 {
		t.Fatal("expected broadcast on NodeDebug=true script-failure path")
	}
	last := captured[len(captured)-1]
	if last != "There was an issue while reloading scripts." {
		t.Errorf("broadcast: got %q, want failure message", last)
	}
}

func TestReload_NotNodeDebug_DoesNotBroadcast(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	s.cfg.NodeDebug = false
	var captured []string
	s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(captured) != 0 {
		t.Errorf("NodeDebug=false should not broadcast; got %v", captured)
	}
}

func TestReload_ClearInvsTrue_RebuildsSharedInvs(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	sentinel := &inventory.Inventory{}
	s.invs = map[int]*inventory.Inventory{0xDEAD: sentinel}
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, leaked := s.invs[0xDEAD]; leaked {
		t.Errorf("sentinel at id 0xDEAD leaked through clearInvs=true")
	}
	// Find any SCOPE_SHARED id in the real cache and assert it's populated.
	sharedFound := false
	for id, inv := range s.invTypes.Configs {
		if inv == nil || inv.Scope != objtype.InvTypeScopeShared {
			continue
		}
		if s.invs[id] == nil {
			t.Errorf("SHARED inv id %d not populated post-reload", id)
		}
		sharedFound = true
		break
	}
	if !sharedFound {
		t.Skip("no SCOPE_SHARED inv in real cache; cannot pin")
	}
}

func TestReload_ClearInvsFalse_LeavesInvsUntouched(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	sentinel := &inventory.Inventory{}
	s.invs = map[int]*inventory.Inventory{42: sentinel}
	if err := s.Reload(false); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if s.invs[42] != sentinel {
		t.Errorf("clearInvs=false should leave existing invs untouched")
	}
}

func TestReload_ClearInvsTrue_DeletesTempScopeFromPlayer(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	// Suppress BroadcastMes → writeOut path; players[1] has no encryptor.
	s.broadcastMesFunc = func(string) {}
	if err := s.Reload(false); err != nil {
		t.Fatalf("priming reload: %v", err)
	}
	tempID := -1
	for id, inv := range s.invTypes.Configs {
		if inv != nil && inv.Scope == objtype.InvTypeScopeTemp {
			tempID = id
			break
		}
	}
	if tempID < 0 {
		t.Skip("no SCOPE_TEMP inv in real cache")
	}
	p := &Player{invs: map[int]*inventory.Inventory{tempID: {}}}
	s.players[1] = p
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if _, ok := p.invs[tempID]; ok {
		t.Errorf("SCOPE_TEMP inv id %d not deleted from player.invs", tempID)
	}
}

func TestReload_VarSharedStringSlot_PreservedAcrossReload(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	if err := s.Reload(true); err != nil {
		t.Fatalf("priming reload: %v", err)
	}
	if len(s.vars) == 0 {
		t.Skip("no vars in real cache")
	}
	// Find a STRING-typed slot if any (those survive the clobber loop).
	stringSlot := -1
	for i, v := range s.varsTypes.Configs {
		if v != nil && v.Type == objtype.ScriptVarTypeString {
			stringSlot = i
			break
		}
	}
	if stringSlot < 0 {
		t.Skip("no STRING vars in real cache; covered by unit test instead")
	}
	s.varsStrings[stringSlot] = "marker"
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if s.varsStrings[stringSlot] != "marker" {
		t.Errorf("STRING var %d was clobbered: %q", stringSlot, s.varsStrings[stringSlot])
	}
}

func TestReload_CRCRegen_OverwritesGlobalCrcBuffer(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	// Pre-seed a sentinel snapshot. Reload's cache.MakeCRCs() must
	// publish a fresh snapshot — the sentinel's pointer must no longer
	// be observable via cache.CRC().
	sentinel := &cache.CRCSnapshot{
		Bytes: []byte{0xDE, 0xAD, 0xBE, 0xEF},
		Table: []uint32{0xDEAD},
	}
	cache.SetCRCForTest(sentinel)
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	post := cache.CRC()
	if post == sentinel {
		t.Errorf("CRC snapshot not regenerated post-reload (still pointing at sentinel)")
	}
	if len(post.Table) == 0 {
		t.Errorf("CRC().Table empty post-reload")
	}
}

func TestReload_GameMapTypesReInjected(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	gm := gamemap.New(s.log)
	s.gamemap = gm
	if err := s.Reload(true); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if gm.LocTypesForTest() != s.locTypes {
		t.Errorf("GameMap loc types not re-injected post-reload (DEVIATION-NAI-190-D1)")
	}
	if gm.ObjTypesForTest() != s.objTypes {
		t.Errorf("GameMap obj types not re-injected post-reload (DEVIATION-NAI-190-D1)")
	}
}

func TestReload_PreStep3LoaderError_LeavesRegistriesUnmutated(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	if err := s.Reload(true); err != nil {
		t.Fatalf("priming reload: %v", err)
	}
	varpBefore := s.varpTypes
	objBefore := s.objTypes
	locBefore := s.locTypes

	// Point at an empty tempdir so the FIRST loader (LoadVarpTypes) fails.
	s.cfg.CachePath = t.TempDir()
	err := s.Reload(true)
	if err == nil {
		t.Fatal("expected error from missing varp.dat")
	}
	if s.varpTypes != varpBefore {
		t.Errorf("varpTypes mutated despite pre-step-3 error (DEVIATION-NAI-190-D2 contract violated)")
	}
	if s.objTypes != objBefore {
		t.Errorf("objTypes mutated despite pre-step-3 error")
	}
	if s.locTypes != locBefore {
		t.Errorf("locTypes mutated despite pre-step-3 error")
	}
}

func TestReload_MidPipelineLoaderError_LeavesHalfSwapped_SkipPin(t *testing.T) {
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	if err := s.Reload(true); err != nil {
		t.Fatalf("priming reload: %v", err)
	}
	objBefore := s.objTypes

	// Construct a partial cache: copy real cache MINUS server/dbrow.dat
	// (step 5 loader; unlike mesanim, dbrow has no ErrNotExist leniency).
	// Reload will succeed through step 3 (swap) and fail at step 5.
	// objTypes must be the NEW instance (mutated); locTypes also new.
	// This documents DEVIATION-NAI-190-D2-HALF-SWAP.
	cacheDir := copyCacheExcept(t, ref225CacheDir(t), "server/dbrow.dat")
	s.cfg.CachePath = cacheDir
	err := s.Reload(true)
	if err == nil {
		t.Fatal("expected mid-pipeline error")
	}
	// Per memory skip_pin_full_struct_capture: capture verbatim, not inferred.
	t.Logf("DEVIATION-NAI-190-D2-HALF-SWAP captured state post-error:\n"+
		"  s.objTypes=%p (before=%p)\n"+
		"  err: %v",
		s.objTypes, objBefore, err)
	if s.objTypes == objBefore {
		t.Errorf("expected post-step-3 swap to have taken effect before step-5 failure")
	}
	t.Skip("DEVIATION-NAI-190-D2-HALF-SWAP: half-swap is the documented contract; this test pins the observed shape but does not enforce it across future refactors.")
}

// --- NAI-190 T9: ::reload cheat integration tests ---

// TestHandleClientCheat_Reload_Dispatches pins that the "reload" case in the
// dev-block fixed-cmd switch calls (*Server).Reload(true) and, when NodeDebug
// is true, broadcasts the success "Loaded N scripts." message.
// TS ClientCheatHandler.ts:149-150.
func TestHandleClientCheat_Reload_Dispatches(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	p.client.server = s
	s.cfg.NodeDebug = true
	s.cfg.NodeProduction = false
	p.staffModLevel = 4
	var captured []string
	s.broadcastMesFunc = func(msg string) { captured = append(captured, msg) }

	// dispatchCheat sends 1 ctrlHeld byte + GJStrLF("reload") — no "::" prefix
	// (the Java client strips it before sending; TS doc L524).
	dispatchCheat(t, p, "reload")

	if len(captured) == 0 {
		t.Fatal("::reload did not broadcast (cheat not wired or Reload failed)")
	}
	if !strings.HasPrefix(captured[len(captured)-1], "Loaded ") {
		t.Errorf("expected success broadcast; got %q", captured[len(captured)-1])
	}
}

// TestHandleClientCheat_Reload_ErrorPath_LogsAndPrivateMes pins that when
// (*Server).Reload returns an error, handleClientCheat swallows it (returns
// nil) and sends a private MessageGame("Reload failed: ...") to the player.
func TestHandleClientCheat_Reload_ErrorPath_LogsAndPrivateMes(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.CachePath = t.TempDir() // empty dir → all loaders fail
	s.cfg.NodeProduction = false
	p.staffModLevel = 4
	// encryptor required: MessageGame → writeOut → c.encryptor.GetNext()
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "reload")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Reload failed")) {
		t.Errorf("expected private 'Reload failed' message; got %q", out)
	}
}

// TestHandleClientCheat_Reload_DefaultsClearInvsTrue pins that the cheat
// always passes clearInvs=true to Reload (TS L149-150 default). A sentinel
// inventory in s.invs must be absent after the call.
func TestHandleClientCheat_Reload_DefaultsClearInvsTrue(t *testing.T) {
	p, _ := newTestPlayer(t)
	s := newTestServerWithCachePath(t, ref225CacheDir(t))
	p.client.server = s
	s.cfg.NodeProduction = false
	p.staffModLevel = 4
	sentinel := &inventory.Inventory{}
	s.invs = map[int]*inventory.Inventory{0xCAFE: sentinel}

	dispatchCheat(t, p, "reload")

	if _, leaked := s.invs[0xCAFE]; leaked {
		t.Errorf("::reload should default clearInvs=true (sentinel at 0xCAFE leaked)")
	}
}

// copyCacheExcept copies all files from src to a t.TempDir, OMITTING
// the listed relative paths.
func copyCacheExcept(t *testing.T, src string, omit ...string) string {
	t.Helper()
	dst := t.TempDir()
	omitSet := make(map[string]bool)
	for _, p := range omit {
		omitSet[p] = true
	}
	err := filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, _ := filepath.Rel(src, path)
		if omitSet[rel] {
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyCacheExcept: %v", err)
	}
	return dst
}
