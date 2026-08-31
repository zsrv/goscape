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
	"uuid"

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
	conn      net.Conn
	log       *slog.Logger
	bufr      *bufio.Reader
	bufw      *bufio.Writer
	in        *packet.Packet
	inMu      sync.Mutex // guards in, opcode, waiting between reader goroutine and tick goroutine
	player    *Player    // nil until sendLoginOK; owned exclusively by tick goroutine after login
	encryptor *io2.Isaac
	decryptor *io2.Isaac
	server    *Server
	state     ClientState
	opcode    int
	waiting   int
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
	//
	// The refcount models those two owners (conn + tick) plus the OnDemand
	// pump goroutine (onDemand.cycle -> onDemand.send -> clientODAdapter.send),
	// which participates transiently: tryRef/dropRef bracket each send
	// instead of holding a standing ref like conn/tick. This closes the
	// arch-28 residual where a pure-OnDemand connection's in-flight pump
	// send could race the conn goroutine's defer pool-returning the
	// buffers (arch-29.1 fidelity backport — see PORTING.md).
	teardownRefs atomic.Int32
	connRefOnce  sync.Once
	tickRefOnce  sync.Once
	// out is the asynchronous socket writer that bufw drains into
	// (SEC1 M-2). Nothing but out's own goroutine calls conn.Write, so a
	// stalled peer can no longer hold the tick goroutine inside a socket
	// write. Nil only for clients built by hand in tests that never went
	// through newClient; closeConn tolerates that.
	out *outboundWriter
}

func newClient(conn net.Conn, writeTimeout time.Duration /*server *World,*/, logger *slog.Logger) *client {
	// SEC1 M-2: bufw flushes into the outbound writer, not straight into
	// the socket, so no flush on the tick goroutine can block on the peer.
	out := newOutboundWriter(conn, writeTimeout, logger)
	c := &client{
		log: logger,

		//server: server,
		conn: conn,
		bufr: getBufioReader64k(conn), // Wrap the connection with a buffered reader
		bufw: getBufioWriter64k(out),  // Wrap the outbound writer with a buffered writer
		out:  out,

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

// tryRef acquires a transient buffer reference iff the client is still
// live (refcount > 0). The OnDemand pump takes one around each send so
// dropRef's pool return can never race an in-flight frame flush. CAS
// loop: a plain Add would resurrect a client whose buffers were already
// returned.
func (c *client) tryRef() bool {
	for {
		v := c.teardownRefs.Load()
		if v <= 0 {
			return false
		}
		if c.teardownRefs.CompareAndSwap(v, v+1) {
			return true
		}
	}
}

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

// flushWrite hands bufw's bytes to the outbound writer. Never blocks on
// the socket (SEC1 M-2): the writer goroutine owns write deadlines, so
// the only errors here are "queue overflowed" and "already closed".
func (c *client) flushWrite() error {
	return c.bufw.Flush()
}

// flushWriteOrClose flushes the buffered writer; on failure it closes the
// conn so the reader goroutine exits and tears the connection down through
// the normal disconnect path (TS: socket error event → close). bufio's
// error is sticky, so without the close a dead connection lingered,
// silently receiving nothing, until the read-side timeout.
// Since SEC1 M-2 a socket write failure surfaces asynchronously — the
// writer goroutine closes the conn itself and marks the writer failed, so
// the *next* flush is the one that reports it here. Either way the
// connection ends up closed and the read loop tears it down.
func (c *client) flushWriteOrClose() {
	if err := c.flushWrite(); err != nil {
		c.closeConn()
	}
}

// closeConn is the one way to close a client's connection: it stops new
// writes, drains already-flushed frames (bounded by writeTimeout) and
// then closes the socket, so a logout byte flushed just before close is
// still delivered (DEVIATION SEC1-D3, see outbound.go). Idempotent and
// safe from any goroutine; it never blocks on the network. The reader
// goroutine unblocks when the socket finally closes and runs the normal
// teardown.
//
// Writer goroutines are deliberately NOT tracked by tcpWg: at shutdown
// Server.Shutdown calls closeLiveConns first, which hard-closes every
// socket, so each writer fails its next write (or its drain) and exits
// promptly on its own rather than holding the wait group open for one
// write timeout per connection.
func (c *client) closeConn() {
	if c.out != nil {
		c.out.Close()
		return
	}
	_ = c.conn.Close()
}

// sendLoginOK queues the player for world registration on the next tick,
// sends the login-accepted byte, and transitions to ClientStateGame.
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
		c.sessionID = uuid.New().String()
		c.tap.SessionStarted(c.accountID, c.sessionID, time.Now())
	}

	// TS World.ts:943-949 (244 pin 9aadcec4):
	//   staffModLevel >= 2 → 19 (supermod/admin)
	//   staffModLevel >= 1 → 18 (mod — right-click report abuse)
	//   else              → 2  (normal)
	if c.staffModLevel >= 2 {
		c.bufw.WriteByte(loginresp.OpLoginOKSupermod.Opcode)
	} else if c.staffModLevel >= 1 {
		c.bufw.WriteByte(loginresp.OpLoginOKWithRights.Opcode)
	} else {
		c.bufw.WriteByte(loginresp.OpOK.Opcode)
	}
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

// clientODAdapter adapts *client to the odClient interface consumed by onDemand.
// It is wired by the connection read-loop in state ClientStateOndemand.
//
// send writes data through the buffered writer and flushes immediately.
// Race safety: after the op-15 handshake transitions state to ClientStateOndemand,
// the connection's read goroutine only reads — it never writes again. All
// sends come from the onDemand cycle goroutine via this adapter, making bufw
// access single-writer and race-safe without additional locking.
//
// close calls closeConn(), which drains any already-flushed frames and then
// closes the socket; that causes the read goroutine's bufr.Read to return an
// error, triggering the deferred cleanup (release, player remove, log). This
// mirrors TS client.close() → socket.destroy() (TcpServer.ts:57).
type clientODAdapter struct {
	c *client
}

func (a *clientODAdapter) send(data []byte) error {
	// arch-29.1: transient ref brackets the write+flush so teardown's
	// pool return waits out an in-flight frame. A refused send means the
	// connection is closing; the onDemand cycle treats it like any send
	// error.
	if !a.c.tryRef() {
		return net.ErrClosed
	}
	defer a.c.dropRef()
	a.c.write(data)
	return a.c.flushWrite()
}

func (a *clientODAdapter) close() {
	a.c.closeConn()
}

// backlogged reports whether the connection's outbound queue has passed
// its soft high-water mark, so the pump should end this client's slice
// and retry on a later pass (SEC1 M-2 / DEVIATION SEC1-D3). Nil-safe:
// a client constructed without a writer (unit tests, parse-only paths)
// is never backlogged.
func (a *clientODAdapter) backlogged() bool {
	if a.c == nil || a.c.out == nil {
		return false
	}
	return a.c.out.Backlogged()
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
