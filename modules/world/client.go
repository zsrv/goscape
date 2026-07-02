package world

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/tapper"
	applog "github.com/zsrv/goscape/pkg/util/log"
)

// errCloseConn signals that the connection should be closed cleanly after a
// handled rejection (e.g. wrong revision, invalid credentials). It is not a
// real I/O error and should not be logged as one.
var errCloseConn = errors.New("close connection")

// maxClientInBufSize caps the incoming buffer to match the client's own limit,
// preventing unbounded memory growth from a misbehaving or malicious client.
const maxClientInBufSize = 65535

type ClientState int

const (
	ClientStateClosed ClientState = -1
	ClientStateLogin  ClientState = 0
	ClientStateGame   ClientState = 1
	// ClientStateOndemand marks a connection that has completed the op-15
	// handshake and is now serving OnDemand cache requests.
	// TS uses literal 2 for this state (TcpServer.ts:35-37, client.state===2
	// check; World.ts:2241: client.state = 2). Value must match TS exactly.
	ClientStateOndemand ClientState = 2
)

type client struct {
	conn         net.Conn
	log          *slog.Logger
	bufr         *bufio.Reader
	bufw         *bufio.Writer
	in           *packet.Packet
	inMu         sync.Mutex // guards in, opcode, waiting between reader goroutine and tick goroutine
	player       *Player    // nil until sendLoginOK; owned exclusively by tick goroutine after login
	encryptor    *io2.Isaac
	decryptor    *io2.Isaac
	server       *Server
	writeTimeout time.Duration
	state        ClientState
	opcode       int
	waiting      int
	// staffModLevel is the moderator/admin tier set from the login server's
	// gRPC response (server.go:546 — resp.GetStaffModLevel()). Copied onto
	// Player at newPlayer(). Read by handleClientCheat for the staff-only
	// command gates (::tele, ::getcoord) and by Chat for the rights byte.
	staffModLevel int32
	members       bool
	// username is the safe-form ("snake_case") account name from the RS2 login
	// packet, set after successful login at server.go's login handler. Copied
	// onto Player.username at newPlayer(); also drives Player.username37 (the
	// base37-long encoding consumed by the appearance buffer's name field).
	username string
	// lowMemory carries the client's low-memory capability bit from the
	// RS2 login packet (LoginRequest.LowMemory, parsed at server.go's
	// req.UnmarshalBinary). Copied onto Player at newPlayer(). Read by
	// script opcodes that trigger client audio loads (MIDI_SONG, MIDI_JINGLE).
	lowMemory bool
	// reconnecting carries whether the client used OpReqGameReconnect (opcode 18)
	// vs. OpReqInitGameConnection (opcode 16). Set at server.go's login-opcode
	// branch. Copied onto Player at newPlayer(). Read by buildArea.ShouldRebuild
	// to skip a full rebuild on actual reconnects.
	reconnecting bool
	// savePayload is the optional SAV bytes returned by the login server
	// on PlayerLogin (resp.GetSave()). Read once by processLogins on the
	// tick goroutine to populate the freshly-constructed Player via
	// LoadSave. Nil for fresh accounts; arbitrary length for returning
	// players.
	savePayload []byte
	// sessionUUID is the per-login session correlation key returned by
	// the login server's PlayerLogin RPC (resp.GetSessionUuid()). Copied
	// onto Player.session at newPlayer(). Empty for paths that bypass
	// the login bridge (standalone world, unit tests); tick.go's
	// "headless" fallback then applies. Slice 7 of friends-server bridge
	// arc.
	sessionUUID string
	// accountID is the persistent DB account.id returned by the login
	// server's PlayerLogin RPC (resp.GetAccountId(), int32 on the wire).
	// Widened to int64 to match the eventspb AccountId field across all
	// telemetry envelopes. Copied onto Player.accountID at newPlayer();
	// the world emits 0 for connections that bypass the login bridge
	// (unit tests, standalone world). NAI-Phase2 backfill.
	accountID int64
	// hopRemainingMs is the hop-timer cooldown remainder (milliseconds)
	// from the login server's PlayerLogin response, cached by
	// callPlayerLoginRPC ONLY when result == LOGIN_RESULT_HOP_TIMER
	// (rev-254 A4). Consumed once by handleLogin's reject dispatch to
	// render the 2-byte [21, min(255, remaining/1000)] wire reply
	// (sendLoginHopTimer). Always > 0 when set — the login server only
	// emits HOP_TIMER when remaining > 0 (TS LoginServer.ts:334).
	hopRemainingMs int64
	// tap is the seam handle owned by the tapper dskit module;
	// nil on tests that construct a client without a Server. Tap calls are
	// always gated on (c.tap != nil && c.sessionID != "").
	tap tapper.Tapper
	// sessionID is the per-login session correlation key for the tap
	// pipeline, freshly minted in sendLoginOK. Distinct from sessionUUID
	// (which is the friends-bridge correlation). Empty before login;
	// stays set across teardown so the defer in handleTCPConn can fire
	// SessionEnded before the field is cleared.
	sessionID string
	// teardownRefs counts the goroutines that may still touch this
	// client's pooled buffers: the conn goroutine (always, from
	// newClient) and the tick goroutine (from successful login in
	// sendLoginOK until removePlayerOnTick). The last owner out
	// returns the buffers to their pools — releasing on the conn
	// goroutine alone recycled bufw into a NEW connection while the
	// tick was still flushing the old player's frames into it
	// (arch-28.4b).
	teardownRefs atomic.Int32
	connRefOnce  sync.Once
	tickRefOnce  sync.Once
}

func newClient(conn net.Conn, writeTimeout time.Duration /*server *World,*/, logger *slog.Logger) *client {
	c := &client{
		log: logger,

		//server: server,
		conn:         conn,
		bufr:         getBufioReader64k(conn), // Wrap the connection with a buffered reader
		bufw:         getBufioWriter64k(conn), // Wrap the connection with a buffered writer
		writeTimeout: writeTimeout,

		in: packet.Alloc(65536),

		state: ClientStateLogin,

		opcode: -1,
	}
	// The conn goroutine is always an owner of the pooled buffers, from
	// construction until its handleTCPConn defer runs (arch-28.4b).
	c.teardownRefs.Store(1)
	return c
}

// dropRef releases one buffer owner; the last one returns the pooled
// buffers. Callers use dropConnRef/dropTickRef (idempotent per side).
func (c *client) dropRef() {
	if c.teardownRefs.Add(-1) == 0 {
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
	}
}

// dropConnRef releases the conn goroutine's ref on the pooled buffers.
// Idempotent — safe to call more than once for the same client.
func (c *client) dropConnRef() { c.connRefOnce.Do(c.dropRef) }

// dropTickRef releases the tick goroutine's ref on the pooled buffers.
// Idempotent — safe to call more than once for the same client (the idle-
// logout and disconnect paths can both land on the same player).
func (c *client) dropTickRef() { c.tickRefOnce.Do(c.dropRef) }

// bufferData appends data to the incoming buffer, returning false and discarding
// the data if it would exceed maxClientInBufSize.
func (c *client) bufferData(data []byte) bool {
	if c.in.Len()+len(data) > maxClientInBufSize {
		return false
	}
	c.in.Write(data)
	return true
}

func (c *client) write(data []byte) {
	// TODO: return error?
	c.bufw.Write(data)
	applog.Trace(c.log, "sent data", "opcode", c.opcode, "num_bytes", len(data), "data", fmt.Sprintf("%v", data))
}

// flushWrite sets the write deadline and flushes the buffered writer.
func (c *client) flushWrite() error {
	if c.writeTimeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return err
		}
	}
	return c.bufw.Flush()
}

// sendLoginOK queues the player for world registration on the next tick,
// sends the 3-byte login-accepted reply [2, min(staffModLevel,2), 1], and
// transitions to ClientStateGame.
// Actual slot assignment happens in processLogins (next tick).
func (c *client) sendLoginOK() error {
	if c.server != nil {
		p := newPlayer(c)
		// sendLoginOK runs on a per-connection goroutine concurrently with
		// the tick goroutine that may be running Reload; read invTypes from
		// the atomic snapshot. DEVIATION-NAI-C-CONFIGS-ATOMIC-SWAP.
		if cfgs := c.server.loginConfigs(); cfgs.invTypes != nil {
			p.SetAppearanceInv(cfgs.invTypes.Worn)
		}
		// tick co-owns the buffers until removePlayerOnTick (arch-28.4b).
		// Taking the ref before publishing to appendNewPlayer preserves the
		// owner-ref-before-publication invariant even if a future same-tick
		// removal path appears.
		c.teardownRefs.Add(1)
		c.server.appendNewPlayer(p)
		c.player = p
	}

	if c.tap != nil && c.tap.Enabled() {
		c.sessionID = uuid.NewString()
		c.tap.SessionStarted(c.accountID, c.sessionID, time.Now())
	}

	// TS World.ts:946-950 @43e02957 (254): always opcode 2, then
	// min(staffModLevel, 2), then 1 (mouse tracking can only be enabled on
	// login — the 254 client reads staffmodlevel then mouseTracked after
	// response code 2, Client.java:2451-2452 @2e629784). The 245.2
	// three-way opcode fork (19 supermod / 18 mod / 2 normal) is gone.
	//
	// Reconnecting clients take this same path: goscape never emits wire
	// byte 15 — RECONNECT_OK is internal-only (loginResultToRS2) and the
	// resync happens via p.reconnecting → onReconnect. TS's single-byte
	// [15] send (World.ts:880 @43e02957) is its in-world session-
	// replacement branch, which goscape's login-server-owned reconnect
	// design does not have (see the M25-27 reconnect notes).
	c.bufw.WriteByte(loginresp.OpOK.Opcode)
	c.bufw.WriteByte(byte(min(c.staffModLevel, 2)))
	c.bufw.WriteByte(1)
	if err := c.flushWrite(); err != nil {
		return fmt.Errorf("failed to flush login OK: %w", err)
	}
	c.state = ClientStateGame
	return nil
}

// sendLoginError writes a single-byte login rejection code to the client,
// flushes it, and returns errCloseConn to signal the connection should close.
func (c *client) sendLoginError(code byte) error {
	c.bufw.WriteByte(code)
	c.log.Debug("send login error", "opcode", c.opcode, "num_bytes", c.in.Len(), "data", code)
	c.flushWrite() // best-effort; connection is closing regardless
	return errCloseConn
}

// sendLoginHopTimer writes the 2-byte hop-timer login reject
// [21, min(255, remainingMs/1000)], flushes it, and returns errCloseConn.
// Mirrors TS World.ts:1861-1866 @2e3bcf43:
//
//	} else if (reply === 10) {
//	    // hop timer
//	    const { remaining } = msg;
//	    client.send(Uint8Array.from([21, Math.min(255, remaining! / 1000)]));
//	    client.close();
//
// JS Uint8Array.from truncates the float toward zero, so Go's integer
// division (remainingMs/1000) is byte-identical for the positive values
// the login server emits. The <0 clamp is unreachable in practice
// (LoginServer.ts:334 only sends response 10 when remaining > 0) but
// keeps a zero/unset RemainingMs from wrapping to 255 via byte(-1).
func (c *client) sendLoginHopTimer(remainingMs int64) error {
	secs := remainingMs / 1000
	if secs > 255 {
		secs = 255
	}
	if secs < 0 {
		secs = 0
	}
	c.bufw.WriteByte(loginresp.OpHopTimer.Opcode)
	c.bufw.WriteByte(byte(secs))
	c.log.Debug("send login hop timer", "opcode", c.opcode, "remaining_ms", remainingMs, "seconds_byte", secs)
	c.flushWrite() // best-effort; connection is closing regardless
	return errCloseConn
}

// clientODAdapter adapts *client to the odClient interface consumed by onDemand.
// It is wired by the connection read-loop in state ClientStateOndemand.
//
// send writes data through the buffered writer and flushes immediately.
// Race safety: after the op-15 handshake transitions state to ClientStateOndemand,
// the connection's read goroutine only reads — it never writes again. All
// sends come from the onDemand cycle goroutine via this adapter, making bufw
// access single-writer and race-safe without additional locking.
//
// close calls conn.Close() which causes the read goroutine's bufr.Read to
// return an error, triggering the deferred cleanup (release, player remove,
// log). This mirrors TS client.close() → socket.destroy() (TcpServer.ts:57).
type clientODAdapter struct {
	c *client
}

func (a *clientODAdapter) send(data []byte) error {
	a.c.write(data)
	return a.c.flushWrite()
}

func (a *clientODAdapter) close() {
	a.c.conn.Close()
}

/////////////

var (
	bufioReader64kPool sync.Pool
	bufioWriter64kPool sync.Pool
)

func getBufioReader64k(r io.Reader) *bufio.Reader {
	if v := bufioReader64kPool.Get(); v != nil {
		br := v.(*bufio.Reader)
		br.Reset(r)
		return br
	}
	return bufio.NewReaderSize(r, 64<<10)
}

func putBufioReader64k(b *bufio.Reader) {
	b.Reset(nil)
	bufioReader64kPool.Put(b)
}

func getBufioWriter64k(w io.Writer) *bufio.Writer {
	if v := bufioWriter64kPool.Get(); v != nil {
		bw := v.(*bufio.Writer)
		bw.Reset(w)
		return bw
	}
	return bufio.NewWriterSize(w, 64<<10)
}

func putBufioWriter64k(b *bufio.Writer) {
	b.Reset(nil)
	bufioWriter64kPool.Put(b)
}

var readBuf64kPool = sync.Pool{
	New: func() any { return make([]byte, 64<<10) },
}

func getReadBuf64k() []byte {
	return readBuf64kPool.Get().([]byte)
}

func putReadBuf64k(b []byte) {
	readBuf64kPool.Put(b)
}
