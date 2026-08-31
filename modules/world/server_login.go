package world

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"slices"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/cache"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	loginreq "github.com/zsrv/goscape/pkg/io/protocol/login/req"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

func (c *client) handleData() error {
	switch c.state {
	case ClientStateLogin:
		return c.handleLogin()
	default:
		c.log.Warn("unhandled client state", "state", c.state)
		return errors.New("unhandled client state")
	}
}

func (c *client) handleLogin() error {
	opcode, err := c.in.Peek(1)
	if err != nil {
		c.log.Error("failed to read opcode", "opcode", opcode)
		return errors.New("failed to read opcode")
	}

	switch opcode[0] {
	default:
		return fmt.Errorf("unexpected opcode in login state: %d", opcode[0])

	case 14:
		// Op 14 — checklogin handshake (World.ts:2104-2127 @2e3bcf43).
		// Total wire input: opcode(1) + payload(1) = 2 bytes.
		// Payload is the _loginServer discriminator byte — read and discarded by TS.
		// Reply (17 bytes total on the happy path, sent in three TS calls):
		//   8x0x00            — TS: client.send([0,0,0,0,0,0,0,0])
		//   0x00              — TS: client.send([0])
		//   p4(rand & 0x00ffffff) || p4(rand) — 8-byte seed (World.ts:2123-2126).
		// The seed is generated fresh per call and NOT stored — the client echoes
		// its own ISAAC seeds inside the RSA block at op 16/18.
		// First word masked to 24 bits (TS: Math.random() * 0x00ffffff) so high byte
		// is always 0x00 — byte-faithful to the TS upstream.
		if c.in.Len() < 2 {
			return protocol.ErrPayloadTooSmall
		}
		// goscape consumes opcode + the _loginServer byte up front; TS
		// reads _loginServer only after the rate-limit check below —
		// the byte is discarded either way, so the wire behavior is
		// identical.
		c.in.Next(2)

		// TS sends the 8 zero bytes BEFORE the rate-limit check
		// (World.ts:2105); a limited client still receives them,
		// followed by the [16] reject.
		zeros := packet.NewPacket(make([]byte, 0, 8))
		zeros.P4(0)
		zeros.P4(0)
		c.write(zeros.Bytes())

		// rev-254 A4 — per-address login rate limit (World.ts:2107-2117
		// @2e3bcf43): gated on NODE_PRODUCTION && threshold > 0;
		// increments on EVERY op-14 attempt (before the comparison, so
		// rejected attempts keep the window armed); attempts >= limit →
		// reply byte 16 ("login attempts exceeded") + close. Keyed by
		// the bare remote IP (TS client.remoteAddress carries no port).
		if s := c.server; s != nil && s.cfg.NodeProduction && s.cfg.NodeRatelimitAddressLogin > 0 {
			host, _ := splitHostPort(c.conn.RemoteAddr().String())
			if s.loginAddressAttempts.bump(host, loginAddressAttemptTTL) >= s.cfg.NodeRatelimitAddressLogin {
				return c.sendLoginError(loginresp.OpTooManyAttempts.Opcode)
			}
		}

		reply := packet.NewPacket(make([]byte, 0, 9))
		reply.P1(0)                          // 0x00 separator byte
		reply.P4(rand.Uint32() & 0x00ffffff) // first seed word, 24-bit-masked
		reply.P4(rand.Uint32())              // second seed word, full 32-bit
		c.write(reply.Bytes())
		if err := c.flushWrite(); err != nil {
			return fmt.Errorf("op14: flush failed: %w", err)
		}
		return nil

	case 15:
		// Op 15 — OnDemand connection entry (World.ts:2240-2242 at 244 pin 9aadcec4).
		// Total wire input: opcode(1), no payload.
		// Transitions client.state to 2 (ClientStateOndemand) and sends 8 zero bytes.
		// After this reply, the connection's read-loop routes all subsequent bytes
		// to s.onDemand.onClientData via *clientODAdapter (TcpServer.ts:35-37).
		c.in.Next(1) // consume opcode byte
		c.state = ClientStateOndemand
		c.write(make([]byte, 8)) // 8 zero bytes — TS: client.send(new Uint8Array(8))
		if err := c.flushWrite(); err != nil {
			return fmt.Errorf("op15: flush failed: %w", err)
		}
		return nil

	case loginreq.OpReqInitGameConnection.Opcode, loginreq.OpReqGameReconnect.Opcode:
		var req loginreq.GameLogin

		pLen, ok := protocol.CheckPacketLength(c.in, loginreq.OpReqInitGameConnection)
		if !ok {
			applog.Trace(c.log, "partial packet data received, waiting for more", "opcode", loginreq.OpReqInitGameConnection, "length", pLen)
			return protocol.ErrPayloadTooSmall
		}

		// TS gates the revision (World.ts:2157-2158) and CRC (2167-2170)
		// checks on the cleartext header BEFORE calling rsadec (2176), so a
		// stale-revision or bad-CRC client never burns RSA CPU — the same
		// rationale the 225 rate-limit gates used (removed at 244). Decode
		// the header first, validate rev+CRC, then decrypt the RSA tail. L37.
		r := packet.NewPacket(c.in.Next(pLen))
		if err := req.UnmarshalHeader(r); err != nil {
			// Malformed cleartext header — tell client it's out of date.
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}
		c.lowMemory = req.LowMemory

		// rev-274: the revision arrives as g1 with a 0xff→g2 escape
		// (World.ts:2136-2138 @dee467c8) — decoded in UnmarshalHeader.
		// Mismatch → reply 6 "RuneScape has been updated!" + close
		// (World.ts:2140-2143).
		if req.Revision != expectedRevision {
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		crcSnap := cache.CRC()
		// PORTING-EXCEPTION (rev244-b3-crc-compare): per-slot compare vs TS's
		// CrcBuffer32 hash-of-36-bytes (World.ts:2170) — strictly stronger,
		// wire-identical. Empty cache → empty Table → all logins rejected
		// until B6 produces a real cache. See pkg/cache/crctable.go.
		if !slices.Equal(crcSnap.Table, req.ArchiveChecksums[:]) {
			// LOG-1: full CRC tables are bulky and only useful at debug time.
			c.log.Info("invalid checksum", "remote_addr", c.conn.RemoteAddr())
			c.log.Debug("invalid checksum detail", "crc_table", crcSnap.Table, "req_checksums", req.ArchiveChecksums)
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		rsaKey := protocol.DefaultRSAKey
		if c.server != nil && c.server.rsaKey != nil {
			rsaKey = c.server.rsaKey
		}
		if err := req.UnmarshalRSA(r, rsaKey); err != nil {
			// RSA failure or malformed encrypted block — out of date.
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		// LOG-1: full req struct (incl. CRC table + password hash + ISAAC
		// seed) at Info is noisy per-login. Demote to Debug; keep contextual
		// Info-level success log at the end of handleLogin (line 955-ish).
		// SEC1 M-7: GameLogin.LogValue redacts password/seed/CRC table.
		c.log.Debug("unmarshalled OpReqInitGameConnection", "req", req)

		c.decryptor = io2.New(req.ISAACSeed)
		for i := range req.ISAACSeed {
			req.ISAACSeed[i] += 50
		}
		c.encryptor = io2.New(req.ISAACSeed)

		// rev-254 A4 — per-device login rate limit (World.ts:2172-2184
		// @2e3bcf43): sits AFTER the RSA block is decoded (uid/username/
		// password read, ISAAC ciphers armed) and BEFORE the username
		// length validation — i.e. before any credential verification.
		// Exceeded → reply byte 16 + close. See deviceLoginLimited.
		if c.deviceLoginLimited(req.UID) {
			return c.sendLoginError(loginresp.OpTooManyAttempts.Opcode)
		}

		if len(req.Username) < 1 || len(req.Username) > 12 {
			return c.sendLoginError(loginresp.OpInvalidUsernameOrPassword.Opcode)
		}

		if len(req.Password) < 1 || len(req.Password) > 20 {
			return c.sendLoginError(loginresp.OpInvalidUsernameOrPassword.Opcode)
		}

		safeName := util.ToSafeName(req.Username)

		// TS World.ts:2188-2192 — reject when world is past its
		// configured connected-player cap. Uses NodeMaxConnected
		// (default 1000) which mirrors Environment.NODE_MAX_CONNECTED.
		if c.server != nil && c.server.getTotalPlayers() > c.server.cfg.NodeMaxConnected {
			return c.sendLoginError(loginresp.OpServerFull.Opcode)
		}

		// world-tick-3: TS processLogins gate (World.ts:884-890) rejects
		// new logins during the final 50-tick (~30s) pre-shutdown window
		// via forceLogout(player, 14) — a single-byte 14 reply followed
		// by socket close. goscape's handleLogin sends OpUpdateInProgress
		// (opcode 14 in pkg/io/protocol/login/resp/resp.go), the same
		// "The server is being updated. Please wait 1 minute and try
		// again." byte the Java client renders. Placement at the world-
		// state-rejects layer (alongside ServerFull above) instead of
		// inside processLogins keeps the reject inside the login
		// handshake — goscape's processLogins runs AFTER sendLoginOK,
		// so this earlier gate is the TS-faithful pre-login-OK position.
		if c.server != nil && c.server.shutdownSoon() {
			return c.sendLoginError(loginresp.OpUpdateInProgress.Opcode)
		}

		// TS World.ts:2194-2199 — reject while the prior session for
		// this username is still completing its logout. goscape models
		// the TS logoutRequests set with the per-player loggingOut
		// flag (player.go:310, player.go:710); a username is "still
		// logging out" iff a live player slot is occupied by an entry
		// with loggingOut=true.
		if c.server != nil && c.server.isUsernameLoggingOut(safeName) {
			return c.sendLoginError(loginresp.OpDuplicate.Opcode)
		}

		reconnecting := opcode[0] == loginreq.OpReqGameReconnect.Opcode
		c.reconnecting = reconnecting

		var reply byte
		if c.server != nil && c.server.loginClient != nil {
			loginReq := &loginpb.PlayerLoginRequest{
				NodeId:        int32(c.server.cfg.NodeID),
				Profile:       c.server.cfg.NodeProfile,
				NodeMembers:   c.server.cfg.NodeMembers,
				Username:      safeName,
				Password:      req.Password,
				Uid:           int32(req.UID),
				RemoteAddress: c.conn.RemoteAddr().String(),
				Reconnecting:  reconnecting,
				HasSave:       false,
			}

			var err error
			reply, err = c.callPlayerLoginRPC(loginReq, safeName)
			if err != nil {
				return c.sendLoginError(reply)
			}
		} else {
			// login server not configured — reject with try again
			reply = loginresp.OpTryAgain.Opcode
		}

		// Non-accepting replies: send the byte and close the connection.
		switch reply {
		case loginresp.OpOK.Opcode, loginresp.OpReconnectOK.Opcode:
			// accepted — fall through to post-login handling below.
			// (254 removed the 18/19 staff replies; the staff tier is the
			// second byte of sendLoginOK's [2, min(staff,2), 1] reply.)
		case loginresp.OpHopTimer.Opcode:
			// rev-254 A4 — the hop timer is the only reject with a
			// payload: [21, min(255, remaining/1000)] (TS World.ts:
			// 1861-1866 @2e3bcf43). c.hopRemainingMs was cached by
			// callPlayerLoginRPC from the HOP_TIMER gRPC response.
			return c.sendLoginHopTimer(c.hopRemainingMs)
		default:
			return c.sendLoginError(reply)
		}

		c.log.Info("login accepted", "safename", safeName, "reply", reply, "reconnecting", reconnecting)
		return c.sendLoginOK()

	}
}

// callPlayerLoginRPC runs the PlayerLogin RPC against c.server.loginClient,
// maps the result to the RS2 wire reply byte, and caches accepted-session
// fields on c. Returns (reply, nil) on success; (loginresp.OpLoginServerOffline.Opcode, err)
// on RPC error so the caller can fail-fast via sendLoginError. Extracted from
// handleLogin to enable unit testing with a stubbed LoginClient.
func (c *client) callPlayerLoginRPC(req *loginpb.PlayerLoginRequest, safeName string) (byte, error) {
	// arch-29.3 fix wave (reviewer Critical): reject logins until the
	// WorldStartup registration has succeeded. WorldStartup blanket-clears
	// logged_in for this node+profile; a login admitted while the
	// background registration retry is still pending would have its LIVE
	// session's flag wiped by the eventually-successful retry, falsifying
	// the duplicate-login guard. A not-yet-registered world and a down
	// login server are operationally the same, so reuse the identical
	// offline reject (opcode 8). This also makes the steady-state variant
	// unreachable: the gate only opens after the wipe (worldStartupCall),
	// and the retry loop exits on first success.
	if !c.server.worldStartupDone.Load() {
		err := errors.New("world startup registration pending; rejecting login")
		c.log.Warn("PlayerLogin rejected: WorldStartup registration still pending",
			slog.String("username", safeName))
		return loginresp.OpLoginServerOffline.Opcode, err
	}
	// CTX-1: bound the login RPC by Server.bridgesCtx + bridgeCallTimeout so
	// Shutdown cancels in-flight calls promptly and a hung login server
	// doesn't deadlock the per-connection goroutine. Mirrors the pattern
	// shipped in Arc 19 R3 for friends/login bridges.
	ctx, cancel := context.WithTimeout(c.server.bridgesCtx, bridgeCallTimeout)
	defer cancel()
	resp, err := c.server.loginClient.PlayerLogin(ctx, req)
	if err != nil {
		c.log.Warn("PlayerLogin RPC failed", "error", err)
		// login-server-5: TS LoginServer's rejectLoginForSafety
		// (LoginServer.ts:115-124) — fired from the LoginServer.ts:287-290
		// reconnect-save / 346-347 missing-save-with-logout / 364-367
		// corrupt-save sites — sends response 7 which World.ts:1857-1861
		// maps to wire opcode 11 ("Login server rejected session. Please
		// try again."). goscape's login handler at modules/login/handler.go
		// :181/184/224/236 surfaces those four paths as codes.DataLoss;
		// translate that specific status back to OpLoginServerRejected
		// here so the world sends opcode 11 instead of opcode 8 (offline).
		// Any non-DataLoss error keeps the prior OpLoginServerOffline
		// posture — a real transport / Internal failure IS the login server
		// being unreachable, which is what opcode 8 reports.
		if st, ok := status.FromError(err); ok && st.Code() == codes.DataLoss {
			return loginresp.OpLoginServerRejected.Opcode, err
		}
		return loginresp.OpLoginServerOffline.Opcode, err
	}
	c.log.Debug("PlayerLogin RPC response", "result", resp.GetResult())

	result := resp.GetResult()
	reply := loginResultToRS2(result)

	// rev-254 A4 — cache the hop-timer remainder so handleLogin's reject
	// dispatch can render [21, min(255, remaining/1000)] (the only login
	// reject carrying a payload byte; World.ts:1861-1866 @2e3bcf43).
	if result == loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER {
		c.hopRemainingMs = resp.GetRemainingMs()
	}

	// Only cache session details if the login was accepted.
	if result == loginpb.LoginResult_LOGIN_RESULT_OK ||
		result == loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER ||
		result == loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
		c.staffModLevel = resp.GetStaffModLevel()
		c.members = resp.GetMembers()
		c.username = safeName
		c.savePayload = resp.GetSave()
		c.sessionUUID = resp.GetSessionUuid()
		c.accountID = int64(resp.GetAccountId())
	}
	return reply, nil
}

// loginResultToRS2 maps a gRPC LoginResult enum to the RS2 wire response byte
// that the Java client understands.
func loginResultToRS2(result loginpb.LoginResult) byte {
	switch result {
	case loginpb.LoginResult_LOGIN_RESULT_OK:
		return loginresp.OpOK.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER:
		return loginresp.OpOK.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK:
		return loginresp.OpReconnectOK.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS:
		return loginresp.OpInvalidUsernameOrPassword.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN:
		return loginresp.OpDuplicate.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED:
		return loginresp.OpBanned.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER:
		return loginresp.OpNeedMembersAccount.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS:
		return loginresp.OpTooManyAttempts.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_IP_BANNED:
		return loginresp.OpLoginServerRejected.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED:
		// TS response 8 → byte 16 "too many attempts" (World.ts:1851-1855 @2e3bcf43).
		return loginresp.OpTooManyAttempts.Opcode
	case loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER:
		// rev-254: the hop timer moved off the response-6/byte-9 rendering
		// (pre-254 LoginServer) onto its own response 10 → wire
		// [21, min(255, remaining/1000)] (TS LoginServer.ts:327-346 +
		// World.ts:1861-1866 @2e3bcf43). The remaining-seconds payload
		// byte is appended by sendLoginHopTimer, not here.
		return loginresp.OpHopTimer.Opcode
	default:
		// UNSPECIFIED / unknown future values
		return loginresp.OpIPLimit.Opcode
	}
}

// disconnectSessionLogEvent classifies a per-conn read-side error into the
// TS-faithful ENGINE session-log message (and optional extra args). Mirrors
// the three TcpServer.ts:44-67 handlers:
//
//   - s.on('close') → "TCP socket closed"   (io.EOF / net.ErrClosed)
//   - s.on('timeout') → "TCP socket timeout" (net.Error.Timeout())
//   - s.on('error')   → "TCP socket error"   (any other error, with err.Error()
//     joined per AddSessionLog's TS args-join quirk)
//
// goscape collapses TS's three socket event handlers into one read-error
// branch because Go's net.Conn surfaces all three failure modes through a
// single Read() return — the discrimination happens by error type rather
// than by event source.
func disconnectSessionLogEvent(err error) (string, []string) {
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return "TCP socket closed", nil
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return "TCP socket timeout", nil
	}
	return "TCP socket error", []string{err.Error()}
}
