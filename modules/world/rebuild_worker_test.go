package world

import (
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
