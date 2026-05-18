package world

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestRebuildWorker_Request_RunsPackAndPostsResult pins the happy path:
// dispatchRebuildRequest → worker reads, runs packFn, posts success result.
func TestRebuildWorker_Request_RunsPackAndPostsResult(t *testing.T) {
	s := newTestServer(t)
	s.cfg.ContentPath = "/content"
	s.cfg.CachePath = "/cache"

	var calls atomic.Int32
	var gotSrc, gotOut, gotPack string
	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		calls.Add(1)
		gotSrc, gotOut, gotPack = srcDir, outDir, dataPackDir
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

	s.dispatchRebuildRequest()

	select {
	case r := <-s.rebuildResult:
		if r.err != nil {
			t.Fatalf("expected nil err, got %v", r.err)
		}
		if r.duration <= 0 {
			t.Errorf("expected duration > 0, got %v", r.duration)
		}
		if r.invoker != nil {
			t.Errorf("expected nil invoker (no manual sender), got %v", r.invoker)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}

	if c := calls.Load(); c != 1 {
		t.Errorf("expected packFn called once, got %d", c)
	}
	if gotSrc != "/content" || gotOut != "/cache" || gotPack != "/cache" {
		t.Errorf("packFn args: src=%q out=%q pack=%q; want /content,/cache,/cache",
			gotSrc, gotOut, gotPack)
	}
}

// TestRebuildWorker_PackError_PostsErrorResult pins that packFn errors
// propagate to rebuildResult.err verbatim. handleRebuildResult (tested
// elsewhere) is responsible for not calling reloadFn on error.
func TestRebuildWorker_PackError_PostsErrorResult(t *testing.T) {
	s := newTestServer(t)
	wantErr := errors.New("boom")
	s.packFn = func(srcDir, outDir, dataPackDir string) error { return wantErr }

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runRebuildWorker()
	}()
	t.Cleanup(func() {
		close(s.quit)
		<-done
	})

	s.dispatchRebuildRequest()

	select {
	case r := <-s.rebuildResult:
		if !errors.Is(r.err, wantErr) {
			t.Errorf("expected errors.Is err == wantErr; got %v", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for rebuildResult")
	}
}

// TestRebuildWorker_BusyCoalesce_PendingTriggersSecondRun pins the
// busy-state coalesce: a second dispatch while busy sets pending; on
// pack completion, worker re-queues itself and runs packFn exactly one
// more time. A third dispatch arriving during the first pack is
// coalesced into the same "pending" slot — not a third run.
func TestRebuildWorker_BusyCoalesce_PendingTriggersSecondRun(t *testing.T) {
	s := newTestServer(t)

	release1 := make(chan struct{})
	release2 := make(chan struct{})
	entered := make(chan struct{}, 2)
	var calls atomic.Int32

	s.packFn = func(srcDir, outDir, dataPackDir string) error {
		n := calls.Add(1)
		entered <- struct{}{}
		switch n {
		case 1:
			<-release1
		case 2:
			<-release2
		default:
			t.Errorf("unexpected third packFn invocation")
		}
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

	// Dispatch req1; wait until packFn entered (so worker is busy).
	s.dispatchRebuildRequest()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first packFn entry")
	}

	// Dispatch req2 + req3 while busy. Both should coalesce into a
	// single pending slot (one re-run after release1).
	s.dispatchRebuildRequest()
	s.dispatchRebuildRequest()

	// Release packFn #1; drain result1; expect packFn #2 to enter.
	close(release1)
	select {
	case <-s.rebuildResult:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result #1")
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second packFn entry (coalesced pending)")
	}

	// Release packFn #2; drain result2.
	close(release2)
	select {
	case <-s.rebuildResult:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for result #2")
	}

	// Give the worker a window to spuriously re-run. It must not.
	select {
	case <-entered:
		t.Errorf("packFn ran a third time; coalesce broken (calls=%d)", calls.Load())
	case <-time.After(200 * time.Millisecond):
	}

	if c := calls.Load(); c != 2 {
		t.Errorf("expected exactly 2 packFn calls, got %d", c)
	}
}
