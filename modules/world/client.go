package world

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
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
}

func newClient(conn net.Conn, writeTimeout time.Duration /*server *World,*/, logger *slog.Logger) *client {
	return &client{
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
	c.log.Debug("sent data", "opcode", c.opcode, "num_bytes", len(data), "data", fmt.Sprintf("%v", data))
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
		c.server.appendNewPlayer(p)
		c.player = p
	}

	if c.staffModLevel >= 1 {
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
