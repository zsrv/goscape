package ondemand

import (
	"context"
	"net/http"
	"strings"

	"github.com/coder/websocket"
)

// WebSocketHandler upgrades HTTP requests with `Upgrade: websocket` to a
// WebSocket and bridges binary frames into the world module's TCP connection
// handler. Mirrors web.ts:125-127 in Engine-TS.
//
// # Origin allowlist
//
// PORTING-EXCEPTION (rev244-b3-ws-origin): TS 244 (9aadcec4) has the entire
// open/origin-check block commented out as a TODO (web.ts:125-154). goscape
// retains its pre-existing origin check (AllowedOrigins allowlist enforced
// before upgrade handshake) as a deliberate security improvement — the TS
// upstream TODO acknowledges this is important but defers implementation.
// The configured AllowedOrigins slice is enforced BEFORE the upgrade
// handshake. An empty slice ⇒ allow all (mirrors TS WEB_CORS_ALLOWED_ORIGINS
// empty-default at Environment.ts:13). A non-empty slice ⇒ the request's
// Origin header must exactly match one entry; otherwise the request is
// rejected with 403 before any upgrade is attempted. TS terminates the
// connection AFTER the upgrade (web.ts:127-129) in the 225 baseline; the
// 244 pin comments this out entirely. See docs/PORTING.md.
//
// # Fallthrough
//
// Requests without an `Upgrade: websocket` header (case-insensitive) are
// delegated to RootHandler so plain GET / requests still hit the existing
// static dispatch chain. This lets us register a single GET / route when
// the WS bridge is enabled.
//
// After upgrade, the WS connection is wrapped via websocket.NetConn so that
// binary frames stream through as raw bytes — the world module's
// connection state machine reads RS2 framing off the net.Conn as if it were
// a TCP connection. HandleConn takes ownership of the wrapped net.Conn and
// blocks for the lifetime of the connection.
func (a *OnDemand) WebSocketHandler(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		a.RootHandler(w, r)
		return
	}

	// Pre-upgrade origin allowlist (see doc comment above).
	if !a.originAllowed(r.Header.Get("Origin")) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// We enforce our own origin allowlist (empty-default-allow-all to
		// mirror TS); InsecureSkipVerify disables coder/websocket's stricter
		// same-host default so the pre-upgrade gate above is authoritative.
		InsecureSkipVerify: true,
	})
	if err != nil {
		a.log.Debug("websocket upgrade failed", "err", err, "sourceIPs", a.clientIP(r))
		return
	}

	// Wrap the WebSocket as net.Conn. Binary frames flow as raw bytes; the
	// world module's per-connection state machine is unaware of the WS
	// framing. We pass context.Background() because the wrapped conn lives
	// for the duration of HandleConn (which owns close), not r.Context()
	// (which is cancelled when this handler returns per http.Hijacker docs).
	netConn := websocket.NetConn(context.Background(), wsConn, websocket.MessageBinary)

	// NOTE: websocket.NetConn internally calls c.SetReadLimit(-1) to disable
	// per-message read limits (see coder/websocket netconn.go:49) because
	// netConn streams across messages. We re-apply our configured limit
	// AFTER NetConn so the cap mirrors TS maxPayloadLength: 2_000 at
	// web.ts:125 for inbound traffic.
	if a.cfg.WebSocket.MaxPayloadBytes > 0 {
		wsConn.SetReadLimit(a.cfg.WebSocket.MaxPayloadBytes)
	}

	// HandleConn takes ownership of netConn (including closing it) and
	// blocks for the lifetime of the connection.
	a.worldConn.HandleConn(netConn)
}

// originAllowed implements the empty-default-allow-all origin policy. An
// empty AllowedOrigins slice permits any Origin (including missing); a
// non-empty slice requires exact match against the Origin header.
func (a *OnDemand) originAllowed(origin string) bool {
	if len(a.cfg.WebSocket.AllowedOrigins) == 0 {
		return true
	}
	for _, allowed := range a.cfg.WebSocket.AllowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}
