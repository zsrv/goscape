package world

import (
	"net"
	"testing"
)

// TestNewServerDoesNotBind pins arch-29.8: NewServer must not bind the TCP
// listener — two Servers configured for the same port can coexist until
// Listen() is called. Resource acquisition belongs to the service's
// starting phase (world.go's startingBody), not to construction, so a
// failed init of a LATER module in the dskit DAG never leaks an
// already-bound socket from this one.
//
// Uses the rev-225 reference cache via ref225CacheDir (see
// testdata_path_test.go) because NewServer performs full cache loading
// (loc/obj/npc/etc. types, plus encfilter.Load(cfg.CachePath) for
// wordenc); skips when the reference checkout is unavailable, mirroring
// TestNewServer_LoadsWordencFilter in server_wordenc_test.go.
func TestNewServerDoesNotBind(t *testing.T) {
	cachePath := ref225CacheDir(t)

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
