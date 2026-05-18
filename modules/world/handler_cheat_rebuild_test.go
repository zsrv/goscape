package world

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
)

// TestHandleClientCheat_Rebuild_NoContentPath_PrivateError pins that
// the handler short-circuits when ContentPath is empty: private message
// to invoker only, no broadcast, no worker engagement.
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
	if !bytes.Contains(out, []byte("content-path")) {
		t.Errorf("error message should mention content-path; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("must not emit 'Rebuilding scripts...' when content-path is empty; got %q", out)
	}
}

// TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress pins
// that the handler returns immediately, enqueues a request to the
// worker (rebuildBusy true while packFn blocks), and emits "Rebuilding
// scripts..." to the invoker.
func TestHandleClientCheat_Rebuild_DispatchesAndBroadcastsInProgress(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Block packFn so the worker stays busy across the assertion.
	release := make(chan struct{})
	entered := make(chan struct{})
	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		close(entered)
		<-release
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(release)
		<-s.rebuildResult // drain so worker can complete
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")
	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...' start message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("must not emit 'Rebuilt:' before tick drains result; got %q", out)
	}

	// Confirm worker is in fact busy (i.e., dispatchRebuildRequest fed it).
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("packFn never entered — dispatch did not reach worker")
	}

	s.rebuildMu.Lock()
	busy := s.rebuildBusy
	invoker := s.rebuildManualInvoker
	s.rebuildMu.Unlock()
	if !busy {
		t.Errorf("expected rebuildBusy=true while packFn blocks")
	}
	if invoker != p {
		t.Errorf("expected rebuildManualInvoker == p; got %v", invoker)
	}
}

// TestHandleClientCheat_Rebuild_Async_HappyPath_DrainsAndReloads pins
// the full async-success path: dispatch → worker runs packFn → result
// drained on tick → reloadFn invoked → "Rebuilt: ..." emitted to invoker.
func TestHandleClientCheat_Rebuild_Async_HappyPath_DrainsAndReloads(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.packFn = func(srcDir, outDir, dataPackDir string) error { return nil }

	var reloadCalls atomic.Int32
	var reloadArg atomic.Bool
	s.reloadFn = func(clearInvs bool) error {
		reloadCalls.Add(1)
		reloadArg.Store(clearInvs)
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")

	// Wait for the worker to post a result, then run the tick drain
	// directly (mirrors the top-of-tick select in runTickLoopWithRate).
	select {
	case r := <-s.rebuildResult:
		s.handleRebuildResult(r)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...'; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("missing 'Rebuilt:' success message; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuild failed")) {
		t.Errorf("happy path must not emit 'Rebuild failed'; got %q", out)
	}
	if c := reloadCalls.Load(); c != 1 {
		t.Errorf("expected reloadFn called once; got %d", c)
	}
	if !reloadArg.Load() {
		t.Errorf("expected reloadFn(true); got reloadFn(false)")
	}
}

// TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailureAndSkipsReload
// pins that packFn errors arrive at the tick drain as a failure message
// and reloadFn is NOT invoked.
func TestHandleClientCheat_Rebuild_Async_PackError_BroadcastsFailureAndSkipsReload(t *testing.T) {
	p, conn := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	s.cfg.NodeProduction = false
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"
	p.staffModLevel = 4
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		return errors.New("pack boom")
	}

	var reloadCalls atomic.Int32
	s.reloadFn = func(clearInvs bool) error {
		reloadCalls.Add(1)
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	received := drainConn(t, conn)
	dispatchCheat(t, p, "rebuild")

	select {
	case r := <-s.rebuildResult:
		s.handleRebuildResult(r)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	p.client.flushWrite()
	out := <-received

	if !bytes.Contains(out, []byte("Rebuilding scripts...")) {
		t.Errorf("missing 'Rebuilding scripts...'; got %q", out)
	}
	if !bytes.Contains(out, []byte("Rebuild failed: pack boom")) {
		t.Errorf("missing 'Rebuild failed: pack boom'; got %q", out)
	}
	if bytes.Contains(out, []byte("Rebuilt:")) {
		t.Errorf("must not emit 'Rebuilt:' on packFn error; got %q", out)
	}
	if c := reloadCalls.Load(); c != 0 {
		t.Errorf("expected reloadFn NOT called on pack error; got %d", c)
	}
}
