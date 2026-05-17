package world

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// seedRebuildFixture writes the minimal source-content layout that
// lets packall.PackAll succeed end-to-end. Mirrors seedSmokeFixture in
// cmd/goscape-cli/cmd_smoke_pack_test.go (kept in-tree per the spec
// §5 YAGNI note — no shared test-fixture package).
func seedRebuildFixture(t *testing.T, dir string) {
	t.Helper()
	write := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	write("scripts/o.obj", "[bronze_sword]\nname=Bronze sword\n")
	write("pack/obj.pack", "0=bronze_sword\n")
	write("scripts/helper.rs2", "[proc,helper]\nreturn;\n")
	write("pack/script.pack", "0=[proc,helper]\n")
	write("scripts/i.inv", "[backpack]\n")
	write("pack/inv.pack", "0=backpack\n")
	write("scripts/n.varn", "[npc_hp]\ntype=int\n")
	write("pack/varn.pack", "0=npc_hp\n")
	write("scripts/s.vars", "[shared_xp]\ntype=int\n")
	write("pack/vars.pack", "0=shared_xp\n")
	write("scripts/d.dbtable", "[records]\n")
	write("pack/dbtable.pack", "0=records\n")
	write("pack/synth.pack", "")
	write("pack/anim.pack", "")
	write("pack/base.pack", "")
	write("pack/model.pack", "")
}

// TestHandleClientCheat_Rebuild_Dispatches pins the happy path: with a
// valid ContentPath fixture, `::rebuild` runs PackAll then Reload and
// emits "Rebuilding scripts..." and "Rebuilt: …" private messages.
//
// The minimal pack fixture produces only a subset of the entity-type
// caches Reload reads; we seed cacheDir with realCacheDir() contents
// first so Reload's loaders find a fully-populated cache after PackAll
// overwrites the subset that the fixture covers (mirrors TS DevThread
// flow where the existing cache is updated in place).
func TestHandleClientCheat_Rebuild_Dispatches(t *testing.T) {
	contentDir := t.TempDir()
	seedRebuildFixture(t, contentDir)
	cacheDir := copyCacheExcept(t, realCacheDir())

	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.NodeDebug = false
	s.cfg.ContentPath = contentDir
	s.cfg.CachePath = cacheDir
	s.gamemap = nil // matches newTestServerWithCachePath: skip GameMap re-injection
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...' start message; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("missing 'Rebuilt:' success message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("happy path should not emit 'Rebuild failed'; got %q", out)
	}
}

// TestHandleClientCheat_Rebuild_NoContentPath_PrivateError pins the
// graceful-failure case: empty ContentPath returns an explicit private
// MessageGame and does not invoke PackAll.
func TestHandleClientCheat_Rebuild_NoContentPath_PrivateError(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "" // unconfigured
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("missing 'Rebuild failed' message; got %q", out)
	}
	if !bytes.Contains(out, []byte("content-path")) && !bytes.Contains(out, []byte("content_path")) {
		t.Errorf("error message should mention content-path; got %q", out)
	}
}

// TestHandleClientCheat_Rebuild_PackAllFailure_PrivateError pins the
// PackAll-error path: ContentPath points at an empty dir (no
// pack/obj.pack etc.) so PackAll fails immediately; the cheat sends
// "Rebuild failed: …" and does not call Reload.
func TestHandleClientCheat_Rebuild_PackAllFailure_PrivateError(t *testing.T) {
	emptyContent := t.TempDir() // no fixture files
	cacheDir := t.TempDir()

	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = emptyContent
	s.cfg.CachePath = cacheDir
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...' start message; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("missing 'Rebuild failed' error message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("PackAll failure should NOT emit 'Rebuilt:' success; got %q", out)
	}
}
