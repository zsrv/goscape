package world

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
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
)

type client struct {
	conn          net.Conn
	log           *slog.Logger
	bufr          *bufio.Reader
	bufw          *bufio.Writer
	in            *packet.Packet
	encryptor     *io2.Isaac
	decryptor     *io2.Isaac
	server        *Server
	writeTimeout  time.Duration
	state         ClientState
	opcode        int
	waiting       int
	staffModLevel int32
	members       bool
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

// flushWrite sets the write deadline and flushes the buffered writer.
func (c *client) flushWrite() error {
	if c.writeTimeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
			return err
		}
	}
	return c.bufw.Flush()
}

// sendLoginError writes a single-byte login rejection code to the client,
// flushes it, and returns errCloseConn to signal the connection should close.
func (c *client) sendLoginError(code byte) error {
	c.bufw.WriteByte(code)
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
