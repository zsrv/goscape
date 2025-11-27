package world

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"sync"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
)

type ClientState int

const (
	ClientStateClosed ClientState = -1
	ClientStateLogin  ClientState = 0
)

type client struct {
	log *slog.Logger

	//server *World
	conn net.Conn
	bufr *bufio.Reader
	bufw *bufio.Writer

	in *packet.Packet

	state     ClientState
	encryptor *io2.Isaac
	decryptor *io2.Isaac

	opcode  int // current opcode being read
	waiting int // number of bytes to wait for (if any)
}

func newClient(conn net.Conn /*server *World,*/, logger *slog.Logger) *client {
	return &client{
		log: logger,

		//server: server,
		conn: conn,
		bufr: getBufioReader64k(conn), // Wrap the connection with a buffered reader
		bufw: getBufioWriter64k(conn), // Wrap the connection with a buffered writer

		in: packet.NewPacket(make([]byte, 0, 64<<10)), // TODO: sync.Pool

		state: ClientStateLogin,

		opcode: -1,
	}
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
