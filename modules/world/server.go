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
	"sync"
	"time"

	"github.com/zsrv/goscape/internal/dskit/signals"
	"github.com/zsrv/goscape/pkg/cache"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	loginreq "github.com/zsrv/goscape/pkg/io/protocol/login/req"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
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
	handler     SignalHandler
	tcpListener net.Listener
	quit        chan interface{}
	log         *slog.Logger
	loginClient *LoginClient
	cfg         Config
	tcpWg       sync.WaitGroup
}

func NewServer(cfg Config, loginClient *LoginClient, logger *slog.Logger) (*Server, error) {
	tcpListener, err := net.Listen(cfg.TCPListenNetwork, net.JoinHostPort(cfg.TCPListenAddress, strconv.Itoa(cfg.TCPListenPort)))
	if err != nil {
		return nil, fmt.Errorf("failed to create tcp listener: %w", err)
	}

	logger.Info("tcp server listening", "addr", tcpListener.Addr())

	handler := cfg.SignalHandler
	if handler == nil {
		handler = signals.NewHandler(logger)
	}

	s := &Server{
		cfg:         cfg,
		handler:     handler,
		tcpListener: tcpListener,
		loginClient: loginClient,
		quit:        make(chan interface{}),

		log: logger,
	}
	s.tcpWg.Add(1)

	return s, nil
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

// Shutdown will block until the TCP listener has stopped accepting new clients and
// all handlers have returned.
func (s *Server) Shutdown() {
	close(s.quit)
	s.log.Debug("closing tcp listener")
	s.tcpListener.Close()
	s.log.Debug("waiting for tcp connections to close")
	s.tcpWg.Wait()
	s.log.Debug("all tcp connections closed")

	//_, cancel := context.WithTimeout(context.Background(), s.cfg.ServerGracefulShutdownTimeout)
	//defer cancel() // releases resources if httpServer.Shutdown completes before timeout elapses. TODO: revisit this statement
	//// TODO: can we even use ctx here if tcplistener doesn't accept one and this is the proper way to shut down?
	//_ = s.tcpListener.Close() // TODO: revisit, compare to what http server shutdown does
	//// TODO: need to close listener but also close client connections
	//// https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/
}

func (s *Server) serveTCP() error {
	defer s.tcpListener.Close() // TODO: put somewhere else? is this in the greenplace example?
	defer s.tcpWg.Done()

	// Accept incoming connections in a loop
	// Use a for loop so the server will accept each incoming connection,
	// handle it in a goroutine, and loop back around, ready to accept
	// the next connection
	for {
		// conn underlying type is net.TCPConn
		conn, err := s.tcpListener.Accept()
		if err != nil {
			// handshake between server and client failed, or the listener closed
			select {
			case <-s.quit:
				s.log.Debug("tcp listener closed")
				return nil
			default:
				return fmt.Errorf("failed to accept connection: %w", err)
			}
		}

		s.tcpWg.Add(1)

		// Handle the connection in a new goroutine for concurrency
		go func() {
			s.handleTCPConn(conn)
			s.tcpWg.Done()
		}()
	}
}

func (s *Server) handleTCPConn(conn net.Conn) {
	//c := newClient(conn, s, s.log)
	c := newClient(conn, s.cfg.TCPServerWriteTimeout, s.log)
	c.server = s

	// Fix 1: disable Nagle's algorithm so small game packets are sent immediately.
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetNoDelay(true); err != nil {
			s.log.Warn("failed to set TCP_NODELAY", "error", err)
		}
		if s.cfg.TCPKeepAlivePeriod > 0 {
			if err := tcpConn.SetKeepAlive(true); err != nil {
				s.log.Warn("failed to enable TCP keepalive", "error", err)
			} else if err := tcpConn.SetKeepAlivePeriod(s.cfg.TCPKeepAlivePeriod); err != nil {
				s.log.Warn("failed to set TCP keepalive period", "error", err)
			}
		}
	}

	defer func() {
		// Fix 7: log flush errors instead of silently discarding them.
		if err := c.flushWrite(); err != nil {
			s.log.Warn("failed to flush on connection close", "error", err, "remote_addr", conn.RemoteAddr())
		}
		c.in.Release()
		putBufioReader64k(c.bufr)
		putBufioWriter64k(c.bufw)
		conn.Close()
		s.log.Info("connection closed", "remote_addr", conn.RemoteAddr())
	}()

	seed := packet.NewPacket(make([]byte, 0, 8))
	seed.P4(rand.Uint32())
	seed.P4(rand.Uint32())

	c.write(seed.Bytes())
	// Fix 2: apply write deadline when flushing.
	if err := c.flushWrite(); err != nil {
		s.log.Error("failed to send seed", "error", err)
		return
	}

	buf := getReadBuf64k()
	defer putReadBuf64k(buf)
	for {
		// TODO: https://eli.thegreenplace.net/2020/graceful-shutdown-of-a-tcp-server-in-go/

		// Fix 6: skip the read deadline in debug-socket mode so long-running
		// bot/integration tests aren't killed by the normal timeout.
		if !s.cfg.NodeDebugSocket {
			if err := c.conn.SetReadDeadline(time.Now().Add(s.cfg.TCPServerReadTimeout)); err != nil {
				s.log.Error("failed to set read deadline", "error", err)
				return
			}
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

		// Fix 3: close the connection if incoming data would overflow the buffer.
		if !c.bufferData(msg) {
			c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
			return
		}

		//c.readRequest(msg)
		err = c.handleData()
		if err != nil {
			if errors.Is(err, protocol.ErrPayloadTooSmall) {
				c.log.Info("payload too small, waiting for more data2", "error", err)
				continue
			}
			// Fix 5: errCloseConn means a rejection was already sent — close quietly.
			if errors.Is(err, errCloseConn) {
				return
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
	case ClientStateGame:
		return c.handleGame()
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
	default:
		return fmt.Errorf("unexpected opcode in login state: %d", opcode[0])
	case loginreq.OpReqInitGameConnection.Opcode, loginreq.OpReqGameReconnect.Opcode:
		var req loginreq.GameLogin

		pLen, ok := protocol.CheckPacketLength(c.in, loginreq.OpReqInitGameConnection)
		if !ok {
			c.log.Info("partial packet data received, waiting for more", "opcode", loginreq.OpReqInitGameConnection, "length", pLen)
			return protocol.ErrPayloadTooSmall
		}

		b := c.in.Next(pLen)
		if err := req.UnmarshalBinary(b); err != nil {
			// RSA failure or malformed packet — tell client it's out of date.
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		c.log.Info("unmarshalled OpReqInitGameConnection", "req", req)

		if req.Revision != expectedRevision {
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		if !slices.Equal(cache.CrcTable, req.ArchiveChecksums[:]) {
			//if cache.CrcBuffer32 != packet.GetCRC(req.ArchiveChecksums[:], 0, len(req.ArchiveChecksums)) {
			c.log.Info("invalid checksum", "crc_table", cache.CrcTable, "req_checksums", req.ArchiveChecksums)
			return c.sendLoginError(loginresp.OpClientOutOfDate.Opcode)
		}

		c.decryptor = io2.New(req.ISAACSeed)
		for i := range req.ISAACSeed {
			req.ISAACSeed[i] += 50
		}
		c.encryptor = io2.New(req.ISAACSeed)

		// TODO: rate limit

		if len(req.Username) < 1 || len(req.Username) > 12 {
			return c.sendLoginError(loginresp.OpInvalidUsernameOrPassword.Opcode)
		}

		if len(req.Password) < 1 || len(req.Password) > 20 {
			return c.sendLoginError(loginresp.OpInvalidUsernameOrPassword.Opcode)
		}

		// TODO: check num of total players on world

		// TODO: check if user logging out

		safeName := util.ToSafeName(req.Username)

		reconnecting := opcode[0] == loginreq.OpReqGameReconnect.Opcode

		var reply byte
		if c.server != nil && c.server.loginClient != nil {
			loginReq := &loginpb.PlayerLoginRequest{
				NodeId:        int32(c.server.cfg.NodeID),
				Profile:       c.server.cfg.NodeProfile,
				NodeMembers:   c.server.cfg.NodeMembers,
				Username:      safeName,
				Password:      req.Password,
				Uid:           int32(req.UID),
				Socket:        c.conn.RemoteAddr().String(),
				RemoteAddress: c.conn.RemoteAddr().String(),
				Reconnecting:  reconnecting,
				HasSave:       false,
			}

			resp, err := c.server.loginClient.PlayerLogin(context.TODO(), loginReq)
			if err != nil {
				c.log.Warn("PlayerLogin RPC failed", "error", err)
				return c.sendLoginError(loginresp.OpLoginServerOffline.Opcode)
			}

			c.log.Info("PlayerLogin RPC response", "result", resp.GetResult())

			result := resp.GetResult()
			reply = loginResultToRS2(result)

			// Only cache session details if the login was accepted.
			if result == loginpb.LoginResult_LOGIN_RESULT_OK ||
				result == loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER ||
				result == loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK {
				c.staffModLevel = resp.GetStaffModLevel()
				c.members = resp.GetMembers()
			}
		} else {
			// login server not configured — reject with try again
			reply = loginresp.OpTryAgain.Opcode
		}

		// Non-accepting replies: send the byte and close the connection.
		switch reply {
		case loginresp.OpOK.Opcode, loginresp.OpReconnectOK.Opcode, loginresp.OpLoginOKWithRights.Opcode:
			// accepted — fall through to post-login handling below
		default:
			return c.sendLoginError(reply)
		}

		// TODO: save var from msg

		// TODO: save + reconnecting check

		c.log.Info("login accepted", "safename", safeName, "reply", reply, "reconnecting", reconnecting)
		return c.sendLoginOK()

	}
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
	default:
		// UNSPECIFIED / unknown future values
		return loginresp.OpIPLimit.Opcode
	}
}

const expectedRevision = 225

// TODO: move this somewhere else
type LoginResponse struct {
	Type          string
	Username      string
	Socket        string
	Save          []uint8
	StaffModLevel int
	MutedUntil    int
	Reply         int
	AccountID     int
	MessageCount  int
	Remaining     int
	Reconnecting  bool
	LowMemory     bool
	Members       bool
}
