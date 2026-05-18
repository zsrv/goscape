package world

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/zsrv/goscape/internal/dskit/signals"
	"github.com/zsrv/goscape/pkg/cache"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	loginreq "github.com/zsrv/goscape/pkg/io/protocol/login/req"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/zone"
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

	players     [2048]*Player
	playerLoop  []*Player
	newPlayers  []*Player // guarded by playersMu; drained by processLogins
	playersMu   sync.RWMutex
	currentTick int

	// shutdownTick is the tick on which the world will halt. -1 means
	// no shutdown scheduled. Set by Server.rebootTimer; consumed by
	// Server.processShutdown (called at top of tick body when
	// s.currentTick >= s.shutdownTick && s.shutdownTick != -1).
	// Mirrors TS World.shutdownTick (World.ts:166). NAI-182.
	shutdownTick int

	// shutdownGraceful is set by Server.processShutdown when zero
	// players remain after a reboot. The tick loop returns when set,
	// and world.go runFn distinguishes this from an "unexpected" stop
	// by checking the flag before returning fmt.Errorf. NAI-182.
	shutdownGraceful bool

	// tickRate is the per-tick sleep interval. Initialised to
	// defaultTickRate by New(...); mutated at runtime by the
	// ::speed dev-block cheat (NAI-188; mirrors TS World.tickRate).
	// Read/written exclusively on the tick goroutine.
	tickRate time.Duration

	// gracefulExit is closed by Server.processShutdown to unblock
	// Server.Run()'s errChan select. Distinct from s.quit (which is
	// closed by Server.Shutdown() via the dskit stoppingFn) to avoid
	// double-close panic. NAI-182.
	gracefulExit chan struct{}

	gamemap *gamemap.GameMap

	// invs is world-shared inventories (banks, shops) keyed by InvType id.
	// Empty until populated by non-4a code. Listeners with Source==-1 read from here.
	invs map[int]*inventory.Inventory

	paramTypes   *objtype.ParamTypeConfigs
	objTypes     *objtype.ObjTypeConfigs
	invTypes     *objtype.InvTypeConfigs
	dbTableTypes *objtype.DbTableTypeConfigs
	dbRowTypes   *objtype.DbRowTypeConfigs
	dbTableIndex *objtype.DbTableIndex
	varpTypes    *objtype.VarpTypeConfigs
	varsTypes    *objtype.VarsTypeConfigs
	varnTypes    *objtype.VarnTypeConfigs
	enumTypes    *objtype.EnumTypeConfigs
	structTypes  *objtype.StructTypeConfigs
	locTypes     *objtype.LocTypeConfigs
	configsView  serverConfigsView
	invLookup    invLookupView
	npcLookup    serverNpcLookup

	// world-scoped var state for PUSH_VARS / POP_VARS.
	vars        []int32
	varsStrings []string
	worldVars   worldVarsView

	npcTypes      *objtype.NPCTypeConfigs
	huntTypes     *objtype.HuntTypeConfigs
	idkTypes       *objtype.IdkTypeConfigs
	mesanimTypes   *objtype.MesanimTypeConfigs
	fontTypes      []*fonttype.FontType
	seqTypes       *objtype.SeqTypeConfigs
	spotanimTypes  *objtype.SpotanimTypeConfigs
	componentTypes *objtype.ComponentTypeConfigs
	npcs          [8192]*Npc
	npcLoop       []*Npc
	npcEventQueue []NpcEventRequest
	nextNpcSlot   int

	// worldScriptQueue holds scripts suspended to the world tick. Each
	// entry awaits its delay countdown, then re-enters ScriptRunner
	// at the start of the next tick where it reaches delay==0. Drained
	// by processWorldQueue (T9). Producer call sites: resumeOrFinish
	// (player path, T10), resumeOrFinishNpc (npc path, T11),
	// processWorldQueue itself (world self-loop via resumeOrFinishWorld,
	// T12). Mirrors TS World.queue at World.ts:534-559.
	//
	// Single-tick goroutine ownership; no mutex required.
	worldScriptQueue []worldScriptQueueEntry
	objDelayedQueue []objDelayedRequest // NAI-134

	renderer *rsbuf.Renderer
	// rsbuf is the per-tick stateful encoder core (NAI-29). Tick-goroutine-
	// owned: connection goroutines never touch it. Hooks for AddPlayer/
	// AddNpc/ComputePlayer/Cleanup wired in NAI-29 Bundle 4 Tasks 4.2-4.6.
	// Parallel-write window: existing encoder does not yet read from this
	// state (canonical at NAI-30+).
	rsbuf *rsbuf.Buf

	zoneMap       *zone.ZoneMap
	zonesTracking map[*zone.Zone]struct{}
	locObjTracker entitypkg.LifecycleTracker // concrete type *locObjTracker (modules/world/loc_tracker.go); initialized in NewServer
	locOps        *serverLocOps

	scriptProvider *script.Provider

	// Social/moderation bridges (NAI-72). Default to noopBridges{} in
	// NewServer; tests inject recordingBridges via installRecordingBridges.
	// Real impls deferred per NAI-72-D-{FRIENDS-SERVER-BRIDGE,
	// LOGIN-SERVER-BRIDGE-MOD, LOGGER-BRIDGE}.
	friendsBridge  FriendsBridge
	loginBridgeMod LoginBridgeMod
	loggerBridge   LoggerBridge

	// sessionLogs is the per-tick session-log accumulator. NAI-74. Pushed by
	// Player.AddSessionLog; flushed via processSessionLogs in the tick loop.
	sessionLogs    []SessionLog

	testPathfinder pathfinderForTarget // injected by tests; nil in production

	// broadcastMesFunc is the broadcast sink for Server.BroadcastMes-style
	// fanouts. Production wiring (nil) routes to BroadcastMes; tests
	// override to capture without exercising the player connection layer.
	// NAI-190.
	broadcastMesFunc func(msg string)

	// pmCount is the monotonic counter feeding the low 16 bits of the
	// pmId stamped on each FriendThread private_message payload.
	// Mirrors TS World.pmCount. Used only by nextPmId (NAI-158).
	pmCount uint32
}

// appendNewPlayer queues a player for registration on the next tick.
func (s *Server) appendNewPlayer(p *Player) {
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()
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

		log:           logger,
		invs:          make(map[int]*inventory.Inventory),
		zoneMap:       zone.NewZoneMap(),
		zonesTracking: map[*zone.Zone]struct{}{},
		locObjTracker: newLocObjTracker(),
		rsbuf:         rsbuf.New(),
		pmCount:       1,
		shutdownTick:  -1,
		tickRate:      defaultTickRate,
		gracefulExit:  make(chan struct{}),
	}
	s.friendsBridge = noopBridges{}
	s.loginBridgeMod = noopBridges{}
	s.loggerBridge = NewSlogLoggerBridge(s.log)
	s.locOps = &serverLocOps{s: s}
	s.tcpWg.Add(1)

	locTypes, err := objtype.LoadLocTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load loc types: %w", err)
	}
	s.locTypes = locTypes

	params, err := objtype.LoadParams(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load params: %w", err)
	}
	objTypes, err := objtype.LoadObjTypes(cfg.CachePath, params)
	if err != nil {
		return nil, fmt.Errorf("load obj types: %w", err)
	}

	gm := gamemap.New(logger)
	gm.SetLocTypes(locTypes)
	gm.SetMembers(cfg.NodeMembers)
	gm.SetObjTypes(objTypes)
	if err := gm.Init(cfg.CachePath); err != nil {
		return nil, fmt.Errorf("failed to load game map: %w", err)
	}
	s.gamemap = gm

	invTypes, err := objtype.LoadInvTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load inv types: %w", err)
	}

	dbTableTypes, err := objtype.LoadDbTableTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load dbtable types: %w", err)
	}

	dbRowTypes, err := objtype.LoadDbRowTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load dbrow types: %w", err)
	}
	s.paramTypes = params
	s.objTypes = objTypes
	s.invTypes = invTypes
	s.dbTableTypes = dbTableTypes
	s.dbRowTypes = dbRowTypes
	s.dbTableIndex = objtype.BuildDbTableIndex(dbTableTypes, dbRowTypes)

	varpTypes, err := objtype.LoadVarpTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load varp types: %w", err)
	}
	varsTypes, err := objtype.LoadVarsTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load vars types: %w", err)
	}
	varnTypes, err := objtype.LoadVarnTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load varn types: %w", err)
	}
	s.varpTypes = varpTypes
	s.varsTypes = varsTypes
	s.varnTypes = varnTypes
	s.vars = make([]int32, len(varsTypes.Configs))
	s.varsStrings = make([]string, len(varsTypes.Configs))
	s.worldVars = worldVarsView{s: s}

	enumTypes, err := objtype.LoadEnumTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load enum types: %w", err)
	}
	structTypes, err := objtype.LoadStructTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load struct types: %w", err)
	}
	s.enumTypes = enumTypes
	s.structTypes = structTypes
	s.configsView = serverConfigsView{s: s}
	s.invLookup = invLookupView{s: s}
	s.npcLookup = serverNpcLookup{s: s}

	s.renderer = rsbuf.NewRenderer()

	npcTypes, err := objtype.LoadNPCTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load npc types: %w", err)
	}
	s.npcTypes = npcTypes

	huntTypes, err := objtype.LoadHuntTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load hunt types: %w", err)
	}
	s.huntTypes = huntTypes

	idkTypes, err := objtype.LoadIdkTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load idk types: %w", err)
	}
	s.idkTypes = idkTypes

	mesanimTypes, err := objtype.LoadMesanimTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load mesanim types: %w", err)
	}
	s.mesanimTypes = mesanimTypes

	fontTypes, err := fonttype.Load(cfg.CachePath)
	if err != nil {
		// Title file is optional in test fixtures; treat NotFound as
		// empty registry but propagate any other error.
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load font types: %w", err)
		}
		s.log.Warn("client/title font cache unavailable; font split disabled", "err", err)
		fontTypes = nil
	}
	s.fontTypes = fontTypes

	seqFrames, err := objtype.LoadSeqFrames(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load seq frames: %w", err)
	}

	seqTypes, err := objtype.LoadSeqTypes(cfg.CachePath, seqFrames)
	if err != nil {
		return nil, fmt.Errorf("load seq types: %w", err)
	}
	s.seqTypes = seqTypes

	spotanimTypes, err := objtype.LoadSpotanimTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load spotanim types: %w", err)
	}
	s.spotanimTypes = spotanimTypes

	componentTypes, err := objtype.LoadComponentTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load component types: %w", err)
	}
	s.componentTypes = componentTypes

	s.scriptProvider = script.NewProvider()
	if count, err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
		s.log.Warn("script provider load failed; scripts will not run", "err", err)
		s.scriptProvider = nil
	} else {
		s.log.Info("script provider loaded", "count", count)
	}

	for _, spawn := range s.gamemap.NpcSpawns() {
		if spawn.TypeID < 0 || spawn.TypeID >= len(npcTypes.Configs) {
			continue
		}
		typ := npcTypes.Configs[spawn.TypeID]
		if typ == nil {
			continue
		}
		n := NewNpc(0, spawn.TypeID, spawn.X, spawn.Z, spawn.Level, typ)
		if err := s.addNpc(n, -1, true); err != nil {
			s.log.Warn("npc registry full; dropping remaining spawns", "err", err)
			break
		}
	}

	s.populateStaticLocsIntoZones()
	s.populateStaticObjsIntoZones()

	return s, nil
}

// populateStaticLocsIntoZones pushes each parsed static loc from the gamemap
// into its owning Zone via Zone.AddStaticLoc and writes the loc's collision
// into the FlagMap when its LocType has BlockWalk=true. Called once at
// server startup, adjacent to the NPC-spawn pass. Mirrors the runtime
// AddLoc collision-write path at world_zone.go:17-22; the boot-time path
// previously omitted the collision write, leaving zones whose only blockers
// are static locs (e.g., Lumbridge castle interior) unallocated and
// producing FlagNull tile reads in pathfinder BFS expansion (Hans waypoint_idx=-1
// in NAI-92 smoke).
func (s *Server) populateStaticLocsIntoZones() {
	for _, loc := range s.gamemap.StaticLocs() {
		if s.locTypes != nil {
			if lt := s.locTypeOrNil(loc.Type()); lt != nil && lt.BlockWalk {
				s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
					loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, true)
			}
		}
		z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
		z.AddStaticLoc(loc)
	}
}

// populateStaticObjsIntoZones constructs an *entity.Obj per parsed
// ObjSpawn and routes it to its owning Zone via Zone.AddStaticObj.
// Called once at server startup, adjacent to populateStaticLocsIntoZones.
// Mirrors TS GameMap.loadObjs's inline getZone().addStaticObj() call;
// goscape splits the parse (gamemap.loadObjs) from the zone-routing
// (here) because the zone registry lives on Server, not GameMap.
// NAI-151.
func (s *Server) populateStaticObjsIntoZones() {
	for _, spawn := range s.gamemap.ObjSpawns() {
		obj := entitypkg.NewObj(spawn.Level, spawn.X, spawn.Z,
			entitypkg.LifecycleRespawn, spawn.TypeID, spawn.Count)
		z := s.zoneMap.Get(spawn.Level, spawn.X, spawn.Z)
		z.AddStaticObj(obj)
	}
	s.log.Info("static objs loaded", "count", len(s.gamemap.ObjSpawns()))
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

	go s.runTickLoop()

	select {
	case err := <-errChan:
		return err
	case <-s.gracefulExit:
		// processShutdown initiated graceful exit. Return nil; world.go
		// runFn checks s.shutdownGraceful to distinguish from
		// "unexpected" stop. NAI-182.
		return nil
	}
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
		if c.player != nil {
			s.removePlayer(c.player)
			c.player = nil
		}
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

		switch c.state {
		case ClientStateLogin:
			if !c.bufferData(msg) {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			err = c.handleData()
			if err != nil {
				if errors.Is(err, protocol.ErrPayloadTooSmall) {
					continue
				}
				if errors.Is(err, errCloseConn) {
					return
				}
				c.log.Error("handleData error, closing connection", "error", err)
				return
			}
		case ClientStateGame:
			c.inMu.Lock()
			ok := c.bufferData(msg)
			c.inMu.Unlock()
			if !ok {
				c.log.Warn("incoming buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
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
		c.lowMemory = req.LowMemory

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
				c.username = safeName
				c.savePayload = resp.GetSave()
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

var errWorldFull = errors.New("world full")

func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	for i := 1; i < len(s.players); i++ {
		if s.players[i] == nil {
			p.slot = i
			p.uid = composeUID(p.username37, p.slot) // NAI-113: TS World.ts:937
			s.players[i] = p
			s.playerLoop = append(s.playerLoop, p)
			p.active = true
			if s.zoneMap != nil {
				z := s.zoneMap.Get(p.level, p.x, p.z)
				p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
			}
			if s.rsbuf != nil {
				s.rsbuf.AddPlayer(int32(p.slot))
			}
			return nil
		}
	}
	return errWorldFull
}

// getTotalPlayers returns the count of live (non-nil) players in the
// server's player slot table. Lock-free read — matches existing read
// patterns at npc_hunt.go:116, handler_opnpc.go:17 (playersMu guards
// writes only).
func (s *Server) getTotalPlayers() int {
	n := 0
	for _, p := range s.players {
		if p != nil {
			n++
		}
	}
	return n
}

// scaleByPlayerCount scales a tick rate (typically a respawn duration)
// by the current live-player count. Mirrors TS
// World.scaleByPlayerCount at World.ts:1715-1719.
//
// Formula: playerCount = min(getTotalPlayers(), 2000)
//
//	return ((4000 - playerCount) * rate) / 4000  // int truncation
//
// Empty world returns rate unchanged; 2000+ players halves it.
func (s *Server) scaleByPlayerCount(rate int) int {
	playerCount := min(s.getTotalPlayers(), 2000)
	return ((4000 - playerCount) * rate) / 4000
}

func (s *Server) removePlayer(p *Player) {
	p.active = false
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players) || s.players[p.slot] != p {
		return
	}
	if s.zoneMap != nil && p.zoneListElement != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		z.LeavePlayer(p, p.zoneListElement, s.zoneMap.Grid(p.level))
		p.zoneListElement = nil
	}
	if s.rsbuf != nil {
		// RemovePlayer follows upstream lib.rs:186-203 ordering: iterate
		// player.Build.Npcs to decrement observer counts, then run
		// Build.Cleanup. Order matters; running cleanup first would
		// clear Npcs before the iteration and silently skip the
		// observer decrement.
		s.rsbuf.RemovePlayer(int32(p.slot))
	}
	s.players[p.slot] = nil

	for i, lp := range s.playerLoop {
		if lp == p {
			s.playerLoop = append(s.playerLoop[:i], s.playerLoop[i+1:]...)
			break
		}
	}
}

const expectedRevision = 225

// TrackZone marks a zone as modified this tick. Idempotent (map semantics).
// processZones will call ComputeShared on each tracked zone; processCleanup
// will Reset them and clear the set.
//
// Must only be called from the tick goroutine — the zonesTracking map is
// unguarded.
func (s *Server) TrackZone(z *zone.Zone) { s.zonesTracking[z] = struct{}{} }

// LookupPlayerByUID returns the logged-in player whose uid field matches
// the argument, or nil if no such player is active. Intended to be
// called from the tick goroutine (playerLoop is unguarded there).
// Implements the script.PlayerLookup interface consumed by
// FINDUID / P_FINDUID (S7a).
//
// Does NOT filter on CanAccess — callers that need the protected
// variant consult the returned player's CanAccess() separately. Mirrors
// TS World.getPlayerByUid which is a pure lookup.
func (s *Server) LookupPlayerByUID(uid int) script.ActivePlayer {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if p.uid == uid {
			return p
		}
	}
	return nil
}

// LookupPlayerByUsername returns the logged-in player whose username
// field matches the argument exactly, or nil if none is active.
// Mirrors TS World.getPlayerByUsername (World.ts:1675-1689). Intended
// to be called from the tick goroutine (playerLoop is unguarded there).
//
// Match is case-sensitive on the goscape username field (which is set
// at login from the client-supplied display name). TS keys on
// username37 (base37-encoded) but the inputs to this lookup are
// already strings in goscape's call sites.
func (s *Server) LookupPlayerByUsername(name string) *Player {
	for _, p := range s.playerLoop {
		if p == nil || !p.active {
			continue
		}
		if p.username == name {
			return p
		}
	}
	return nil
}

// LookupPlayerBySlot returns the logged-in player at the given slot
// index, or nil if slot is out of range or unoccupied. Mirrors TS
// World.getPlayer(slot). Used by OpPlayer handlers to resolve a
// message's PlayerSlot to a target Player.
func (s *Server) LookupPlayerBySlot(slot int) *Player {
	if slot < 0 || slot >= len(s.players) {
		return nil
	}
	return s.players[slot]
}

// ZonePlayers returns all valid players in the zone at (level, zoneX, zoneZ).
// Mirrors the NpcLookup.ZoneNpcs shape and serverNpcLookup.ZoneNpcs impl
// at modules/world/npc_script_lookup.go:115. Zone resolution via
// pkg/zone.ZoneMap.Get which masks coords to zone bounds internally.
// nil zoneMap (defense) and nil zone (off-grid) both return nil.
// PlayersSafe filters non-IsValid entries (zone.go:424). NAI-35.
func (s *Server) ZonePlayers(level, zoneX, zoneZ int) []script.ActivePlayer {
	if s.zoneMap == nil {
		return nil
	}
	z := s.zoneMap.Get(level, zoneX, zoneZ)
	if z == nil {
		return nil
	}
	out := make([]script.ActivePlayer, 0, z.PlayersCount())
	for p := range z.PlayersSafe(true) {
		// Production EnterPlayer only ever receives *Player, which compile-time
		// satisfies script.ActivePlayer (assertion at message_game.go:11). The
		// ok-form is forward-compatible safety: if a future PlayerLike impl
		// doesn't satisfy ActivePlayer, this skips it instead of panicking.
		pp, ok := p.(script.ActivePlayer)
		if !ok {
			continue
		}
		out = append(out, pp)
	}
	return out
}

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
