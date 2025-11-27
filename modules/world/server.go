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
	"strconv"
	"time"

	"github.com/zsrv/goscape/internal/dskit/signals"
	"github.com/zsrv/goscape/pkg/cache"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/io/protocol/login"
	util "github.com/zsrv/goscape/pkg/util/jstring"
)

// SignalHandler used by Server.
type SignalHandler interface {
	// Loop starts the signals handler. This method is blocking, and returns
	// only after signal is received, or Stop is called.
	Loop()

	// Stop blocked Loop method.
	Stop()
}

type Server struct {
	// TODO: put WS server here too

	cfg         Config // TODO: make a TCP/WS server specific config struct later? or one for each?
	handler     SignalHandler
	tcpListener net.Listener

	log *slog.Logger
}

func NewServer(cfg Config, logger *slog.Logger) (*Server, error) {
	tcpListener, err := net.Listen(cfg.TCPListenNetwork, net.JoinHostPort(cfg.TCPListenAddress, strconv.Itoa(cfg.TCPListenPort)))
	if err != nil {
		return nil, fmt.Errorf("failed to create tcp listener: %w", err)
	}

	logger.Info("tcp server listening", "addr", tcpListener.Addr())

	handler := cfg.SignalHandler
	if handler == nil {
		handler = signals.NewHandler(logger)
	}

	return &Server{
		cfg:         cfg,
		handler:     handler,
		tcpListener: tcpListener,

		log: logger,
	}, nil
}

func (s *Server) Run() error {
	errChan := make(chan error, 1)

	// Wait for a signal
	go func() {
		s.handler.Loop()
		select {
		case errChan <- nil:
		default:
		}
	}()

	// TODO: WS support
	go func() {
		err := s.serveTCP()
		if errors.Is(err, net.ErrClosed) { // TODO: verify if this is appropriate - does errclosed only happen when server closes the conn, not client?
			err = nil
		}

		select {
		case errChan <- err:
		default:
		}
	}()

	return <-errChan
}

// Stop unblocks Run().
func (s *Server) Stop() {
	s.handler.Stop()
}

func (s *Server) Shutdown() {
	_, cancel := context.WithTimeout(context.Background(), s.cfg.ServerGracefulShutdownTimeout)
	defer cancel() // releases resources if httpServer.Shutdown completes before timeout elapses. TODO: revisit this statement
	// TODO: can we even use ctx here if tcplistener doesn't accept one and this is the proper way to shut down?
	_ = s.tcpListener.Close() // TODO: revisit, compare to what http server shutdown does
	// TODO: need to close listener but also close client connections
	// https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/
}

func (s *Server) serveTCP() error {
	defer s.tcpListener.Close()

	// Accept incoming connections in a loop
	for {
		conn, err := s.tcpListener.Accept()
		if err != nil {
			return fmt.Errorf("failed to accept connection: %w", err)
		}

		// Handle the connection in a new goroutine for concurrency
		go s.handleTCPConn(conn)
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.log.Info("connection closed", "remote_addr", conn.RemoteAddr())
	}()

	//c := newClient(conn, s, s.log)
	c := newClient(conn, s.log)

	seed := packet.NewPacket(make([]byte, 8))
	seed.P4(rand.Uint32())
	seed.P4(rand.Uint32())

	c.bufw.Write(seed.Bytes())
	c.bufw.Flush()

	buf := make([]byte, 64<<10) // TODO: sync.Pool, release after writing to c.in
	for {
		// Set a deadline to avoid hanging goroutines if clients disappear
		if err := c.conn.SetReadDeadline(time.Now().Add(s.cfg.TCPServerReadTimeout)); err != nil {
			s.log.Error("failed to set deadline", "error", err)
			return
		}

		n, err := c.bufr.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.log.Error("connection read error", "error", err)
			}
			return
		}

		msg := buf[:n]
		c.log.Info("received data", "num_bytes", len(msg), "data", fmt.Sprintf("%v", msg))
		c.in.Write(msg)

		//c.readRequest(msg)
		err = c.handleData()
		if err != nil {
			if errors.Is(err, protocol.ErrPayloadTooSmall) {
				c.log.Info("payload too small, waiting for more data2", "error", err)
				continue
			}
			c.log.Error("handleData error, closing connection", "error", err)
			return
		}
	}
}

func (c *client) handleData() error {
	switch c.state {
	case ClientStateLogin:
		return c.handleLogin()
	default:
		c.log.Info("unhandled client state", "state", c.state)
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
	case login.Op16.Opcode, login.Op18.Opcode:
		var req login.GameLogin

		pLen, ok := protocol.CheckPacketLength(c.in, login.Op16)
		if !ok {
			c.log.Info("partial packet data received, waiting for more", "opcode", login.Op16, "length", pLen)
			return protocol.ErrPayloadTooSmall
		}

		b := c.in.Next(pLen)
		if err := req.UnmarshalBinary(b); err != nil {
			return err
		}

		c.log.Info("unmarshalled Op16", "req", req)

		if req.Revision != 225 {
			// send 6, close conn
			return nil
		}

		if !slices.Equal(cache.CrcTable, req.ArchiveChecksums[:]) {
			//if cache.CrcBuffer32 != packet.GetCRC(req.ArchiveChecksums[:], 0, len(req.ArchiveChecksums)) {
			// send 6, close conn
			c.log.Info("invalid checksum", "crctable", cache.CrcTable, "reqsums", req.ArchiveChecksums)
			return nil
		}

		c.decryptor = io2.New(req.ISAACSeed)
		for i := range req.ISAACSeed {
			req.ISAACSeed[i] += 50
		}
		c.encryptor = io2.New(req.ISAACSeed)

		if len(req.Username) < 1 || len(req.Username) > 12 {
			// send 3, close conn
			return nil
		}

		if len(req.Password) < 1 || len(req.Password) > 20 {
			// send 3, close conn
			return nil
		}

		// TODO: check num of total players on world

		// TODO: check if user logging out

		safeName := util.ToSafeName(req.Username)

		// TODO

		//loginReq := loginserver.LoginReq{
		//	Username: safeName,
		//	Password: req.Password,
		//	UID:      int(req.UID),
		//	//Socket:        "",
		//	RemoteAddress: c.conn.RemoteAddr().String(),
		//	Reconnecting:  c.opcode == 18,
		//	HasSave:       c.opcode == 18, // TODO
		//}

		//loginResp, err := c.LoginClient.PlayerLogin(safeName, req.Password, int(req.UID), "", c.conn.RemoteAddr().String(), c.opcode == 18,
		//	c.opcode == 18, // TODO
		//)
		//if err != nil {
		//	return err
		//}

		//c.log.Info("loginResp", "loginResp", loginResp)

		c.log.Info("END OF LOGIN", "safename", safeName)

	}

	return nil
}
