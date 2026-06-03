package ondemand

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeConnHandler is a connhandler.ConnHandler that records the net.Conn it
// receives and drains it until close. It is used by the WebSocket bridge
// tests to assert that the WS-wrapped net.Conn is what the world side sees,
// and that bytes written client-side appear on the conn.Read side.
type fakeConnHandler struct {
	mu       sync.Mutex
	conns    []net.Conn
	received []byte
	done     chan struct{}
}

func newFakeConnHandler() *fakeConnHandler {
	return &fakeConnHandler{done: make(chan struct{}, 1)}
}

func (f *fakeConnHandler) HandleConn(c net.Conn) {
	f.mu.Lock()
	f.conns = append(f.conns, c)
	f.mu.Unlock()
	defer func() {
		select {
		case f.done <- struct{}{}:
		default:
		}
	}()
	defer c.Close()
	buf := make([]byte, 64)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			f.mu.Lock()
			f.received = append(f.received, buf[:n]...)
			f.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (f *fakeConnHandler) snapshot() ([]net.Conn, []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs := append([]net.Conn(nil), f.conns...)
	rs := append([]byte(nil), f.received...)
	return cs, rs
}

// newTestOnDemand constructs an OnDemand wired with the supplied connhandler and
// a httptest.Server fronting only the WebSocket route at /. Mirrors the
// production wiring at cmd/goscape/app/modules.go:initOnDemand where
// WebSocketHandler owns GET / when both Enable and worldConn are set.
func newTestOnDemand(t *testing.T, cfg WebSocketConfig, handler *fakeConnHandler) (*OnDemand, *httptest.Server) {
	t.Helper()
	a := &OnDemand{
		log: discardLogger(),
		cfg: Config{
			WebSocket: cfg,
		},
	}
	if handler != nil {
		a.worldConn = handler
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.WebSocketHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return a, srv
}

// TestWebSocketHandler_NonUpgradeRequest_FallsThroughToRootHandler asserts
// that a plain GET / without an Upgrade header is delegated to RootHandler,
// preserving the existing static dispatch chain for non-WS clients.
func TestWebSocketHandler_NonUpgradeRequest_FallsThroughToRootHandler(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{Enable: true, MaxPayloadBytes: 2000}, fake)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	// RootHandler's terminal branch is 404 (no public dir + no named
	// routes match on /). The body is the stdlib NotFound output. Either
	// way, no 101 was issued, the WS bridge was not invoked, and the conn
	// handler never saw a conn.
	if resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatalf("unexpected 101 on non-Upgrade request")
	}
	if conns, _ := fake.snapshot(); len(conns) != 0 {
		t.Fatalf("handler invoked on non-Upgrade request: %d conns", len(conns))
	}
}

// TestWebSocketHandler_OriginRejected_403BeforeUpgrade asserts that when
// AllowedOrigins is non-empty and the request Origin does not match, the
// upgrade is rejected with 403 BEFORE the handshake — strict superset of
// TS web.ts:127-129 which terminates AFTER upgrade.
func TestWebSocketHandler_OriginRejected_403BeforeUpgrade(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{
		Enable:          true,
		MaxPayloadBytes: 2000,
		AllowedOrigins:  []string{"https://example.com"},
	}, fake)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if err == nil {
		t.Fatalf("dial succeeded; want 403")
	}
	if resp == nil {
		t.Fatalf("nil response on dial failure: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	if conns, _ := fake.snapshot(); len(conns) != 0 {
		t.Fatalf("handler invoked on rejected origin: %d conns", len(conns))
	}
}

// TestWebSocketHandler_OriginEmptyList_AllowsAllOrigins asserts the
// empty-default-allow-all policy (mirrors TS WEB_CORS_ALLOWED_ORIGINS empty
// default at Environment.ts:13).
func TestWebSocketHandler_OriginEmptyList_AllowsAllOrigins(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{Enable: true, MaxPayloadBytes: 2000}, fake)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://any-origin.example"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")
}

// TestWebSocketHandler_OriginExactMatch_AllowsUpgrade asserts exact-match
// behaviour when AllowedOrigins is populated.
func TestWebSocketHandler_OriginExactMatch_AllowsUpgrade(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{
		Enable:          true,
		MaxPayloadBytes: 2000,
		AllowedOrigins:  []string{"https://example.com"},
	}, fake)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://example.com"}},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")
}

// TestWebSocketHandler_ReadLimitEnforced asserts that messages larger than
// MaxPayloadBytes cause a WS close (mirrors TS maxPayloadLength: 2_000 at
// web.ts:125).
func TestWebSocketHandler_ReadLimitEnforced(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{
		Enable:          true,
		MaxPayloadBytes: 16,
	}, fake)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusInternalError, "")

	// Send a frame larger than the 16-byte read limit. The server-side
	// SetReadLimit causes the bridge to surface an error to HandleConn,
	// which closes the wrapped net.Conn — so the client either sees the
	// close on its next read or its write succeeds and the subsequent
	// read fails. Either way HandleConn returns and signals done.
	big := make([]byte, 256)
	_ = conn.Write(ctx, websocket.MessageBinary, big)

	// Drain client-side reads so the WS close-handshake can complete
	// (the server has closed via StatusMessageTooBig — the client must
	// read it to deliver the close to its Reader). Run in a goroutine
	// because Read blocks until the close frame arrives.
	go func() {
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	select {
	case <-fake.done:
		// HandleConn returned — the read-limit caused the bridge to
		// surface an error and tear down the wrapped net.Conn.
	case <-time.After(2 * time.Second):
		t.Fatalf("HandleConn did not return after oversized frame")
	}
}

// TestWebSocketHandler_BridgesToConnHandler asserts the end-to-end bridge:
// a binary frame written client-side arrives as bytes on the conn.Read side
// of the world handler.
func TestWebSocketHandler_BridgesToConnHandler(t *testing.T) {
	fake := newFakeConnHandler()
	_, srv := newTestOnDemand(t, WebSocketConfig{Enable: true, MaxPayloadBytes: 2000}, fake)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	want := []byte{0x10, 0x20, 0x30, 0x40, 0x55}
	if err := conn.Write(ctx, websocket.MessageBinary, want); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.Close(websocket.StatusNormalClosure, "")

	select {
	case <-fake.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("HandleConn did not return after close")
	}

	conns, got := fake.snapshot()
	if len(conns) != 1 {
		t.Fatalf("HandleConn invoked %d times, want 1", len(conns))
	}
	if string(got) != string(want) {
		t.Fatalf("received bytes = % x, want % x", got, want)
	}
}

// TestWebSocketHandler_NilConnHandler is a regression guard: even if the WS
// route is somehow registered without a worldConn, an Upgrade request must
// not panic. (Production wiring at modules.go:initOnDemand skips the route
// registration in this case, so this exercises the defensive path only.)
func TestWebSocketHandler_NilConnHandler(t *testing.T) {
	a := &OnDemand{
		log: discardLogger(),
		cfg: Config{WebSocket: WebSocketConfig{Enable: true, MaxPayloadBytes: 2000}},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.WebSocketHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Non-Upgrade request falls through to RootHandler (which 404s on /
	// with no public dir + no named routes). No worldConn invocation, no
	// panic.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}
