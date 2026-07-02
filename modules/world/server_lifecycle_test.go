package world

import (
	"net"
	"path/filepath"
	"testing"
)

// TestNewServerDoesNotBind pins arch-29.8: NewServer must not bind the TCP
// listener — two Servers configured for the same port can coexist until
// Listen() is called. Resource acquisition belongs to the service's
// starting phase (world.go's startingBody), not to construction, so a
// failed init of a LATER module in the dskit DAG never leaks an
// already-bound socket from this one.
//
// Uses the Server274-ref pack (see ref274CacheDir in testdata_path_test.go)
// because NewServer performs full cache loading (loc/obj/npc/etc. types);
// skips when the reference checkout is unavailable, mirroring
// TestNewServer_LoadsWordencFilter in server_wordenc_test.go.
func TestNewServerDoesNotBind(t *testing.T) {
	cachePath := ref274CacheDir(t)
	// encfilter.Load() (invoked by NewServer) resolves data/raw/wordenc
	// relative to cwd — switch to the repo root so it's reachable.
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	t.Chdir(repoRoot)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	cfg := Config{
		CachePath:        cachePath,
		TCPListenNetwork: "tcp",
		TCPListenAddress: "127.0.0.1",
		TCPListenPort:    lis.Addr().(*net.TCPAddr).Port,
	}
	s, err := NewServer(cfg, nil, nil, discardLogger(), nil)
	if err != nil {
		t.Fatalf("NewServer must succeed without binding: %v", err)
	}
	if s.tcpListener != nil {
		t.Fatal("NewServer must not bind s.tcpListener; construction should defer acquisition to Listen()")
	}

	// The port above is already held by lis — Listen() must fail with it
	// occupied, proving Listen() (not NewServer) is what actually binds.
	if err := s.Listen(); err == nil {
		t.Fatal("Listen on an occupied port must fail")
	}
}
