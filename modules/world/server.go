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
	"runtime/debug"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/dskit/signals"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/friendspb"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/filestream"
	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
	loginreq "github.com/zsrv/goscape/pkg/io/protocol/login/req"
	loginresp "github.com/zsrv/goscape/pkg/io/protocol/login/resp"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/loginpb"
	"github.com/zsrv/goscape/pkg/midi"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/packall"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/tapper"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
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
	log        *slog.Logger // component=world.server (server lifecycle)
	logNet     *slog.Logger // component=world.net (per-connection I/O)
	logTick    *slog.Logger // component=world.tick
	logScript  *slog.Logger // component=world.script
	logContent *slog.Logger // component=world.content
	loginClient LoginClient
	// tap is the seam handle owned by the tapper dskit
	// module. Nil in test paths (newTestServer); production always non-nil via
	// NewServer. Threaded onto per-connection client.tap in handleTCPConn.
	tap tapper.Tapper
	// friendsClient is the gRPC seam to the friends server. Nil when
	// FriendsServerEnabled=false; in that case s.friendsBridge resolves
	// to noopBridges{} via defaultFriendsBridge.
	friendsClient FriendsClient
	cfg           Config
	// rsaKey is the RSA key used to decrypt the login block. Resolved in
	// NewServer from cfg.RSAPrivateKeyPath (custom) or protocol.DefaultRSAKey
	// (built-in). May be nil in test-only Server literals; the login decode
	// site falls back to DefaultRSAKey when nil.
	rsaKey *protocol.RSAKey
	tcpWg  sync.WaitGroup
	// tickWg tracks the runTickLoop goroutine spawned in Run(). Shutdown
	// closes s.quit and then waits on tickWg so the tick goroutine has
	// fully exited before cleanup proceeds. Arc 18 R2 — without this,
	// Shutdown could return while runTickLoop was still executing tick
	// phases (processSessionLogs / processCleanup / etc.).
	tickWg sync.WaitGroup

	// onDemand is the 50ms OnDemand cycle that services client cache
	// requests (OnDemand.ts:11-120 at pin 9aadcec4). Initialized in
	// NewServer; run loop started in Run() alongside the tick goroutine
	// and stopped by the same s.quit signal in Shutdown().
	// odWg lets Shutdown() wait for the run goroutine to exit before
	// returning, keeping teardown ordering consistent with tickWg.
	onDemand *onDemand
	odWg     sync.WaitGroup

	// saveWg tracks in-flight player-save RPC goroutines (PlayerLogout /
	// PlayerAutosave, which carry the Save() blob). Shutdown waits on it
	// before cancelling bridgesCtx so a save fired moments before stop —
	// e.g. a player who logged out just before the operator killed the
	// server — actually reaches the login server instead of being aborted
	// by bridgesCancel mid-flight.
	saveWg sync.WaitGroup

	// players bundles the rev-254 World player containers (TS
	// World.ts:145-148 @2e3bcf43, upstream a8186b95): the protocol slot
	// array (`players: (Player|null)[2048]`, lowest-free assignment via
	// getNextPlayerSlot) plus the IP-bucketed playerLoop
	// (HashTable<Player>(8)) that fixes per-tick processing order.
	// players.all() iterates the playerLoop (bucket order then login
	// order), NOT slot order. Keeps PORTING-EXCEPTION
	// (gap-db-datastruct-4) closed, now via the playerLoop port.
	// TS refs: login insert World.ts:875-922, removePlayer
	// World.ts:1576-1599, getNextPlayerSlot World.ts:1634-1642,
	// getTotalPlayers World.ts:1691-1702.
	players    *playerList
	newPlayers []*Player // guarded by playersMu; drained by processLogins
	playersMu  sync.RWMutex
	// playerScratch is the reusable snapshot buffer behind snapshotPlayers
	// (tick.go). Tick-goroutine-only: the per-tick passes that snapshot
	// players run strictly sequentially on the tick goroutine, so one
	// buffer serves them all. Off-tick snapshotters (broadcastRebuildStaff
	// on the rebuild worker goroutine, saveAllOnShutdown) must keep
	// allocating their own copies.
	playerScratch []*Player
	// huntScratch is the reusable candidate buffer shared by the four
	// npc-hunt variants (huntPlayers/huntNpcs/huntObjs/huntLocs). PERF-2:
	// hunts run once per hunting NPC per tick, and pre-fix each scan
	// allocated its candidate slice. Tick-goroutine-only; one hunt scan
	// runs at a time, and huntAll nils the entries after picking so the
	// scratch never pins despawned entities.
	huntScratch []entity
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

	// lineValidatorOverride is a test-only seam: when non-nil,
	// scriptLineValidator() returns it instead of
	// gamemap.Pathfinder.LineValidator. Production New() never sets
	// this; only test fixtures that need to wire a stub LineValidator
	// without a real gamemap (e.g., FindClosestNpcBy* tests in
	// modules/world/npc_script_lookup_test.go) write to it directly.
	// Nil = production path (read from gamemap).
	lineValidatorOverride script.LineValidator

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
	varbitTypes  *objtype.VarBitTypeConfigs
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

	npcTypes       *objtype.NPCTypeConfigs
	huntTypes      *objtype.HuntTypeConfigs
	idkTypes       *objtype.IdkTypeConfigs
	mesanimTypes   *objtype.MesanimTypeConfigs
	fontTypes      []*fonttype.FontType
	seqTypes       *objtype.SeqTypeConfigs
	spotanimTypes  *objtype.SpotanimTypeConfigs
	categoryTypes  *objtype.CategoryTypeConfigs
	componentTypes *objtype.ComponentTypeConfigs

	// configsPtr holds the concurrent-reader snapshot of all type-config
	// registries. Updated atomically by storeConfigsSnapshot after each
	// Reload and at NewServer startup. Per-connection goroutines read
	// through loginConfigs() instead of accessing raw fields directly.
	// DEVIATION-NAI-C-CONFIGS-ATOMIC-SWAP; see configs_snapshot.go.
	configsPtr atomic.Pointer[serverConfigsSnapshot]

	// wordenc filters player-visible chat text through the RS2 word-encoding
	// substitution rules loaded from the wordenc jagfile. Populated at
	// NewServer via encfilter.Load (data/raw/wordenc); test paths inject
	// encfilter.Empty(). TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:35-37.
	wordenc *encfilter.Filter

	// midi caches per-track MIDI lengths parsed from the client cache's
	// archive 3 at world start. Mirrors TS Midi (src/cache/midi/Midi.ts
	// @2e3bcf43, loaded by World.start at World.ts:296). Feeds
	// PlayJingle's delay field (Midi.getLength) and the MIDI_LENGTH op
	// (Midi.getTickLength). A10 replaced the 244-era midiPack name→id
	// registry — name resolution moved to compile time (ScriptVarType
	// MIDI; tools/pack/Compiler.ts:199 loads midi.pack as a symbol table).
	// Nil-safe: a nil *midi.Cache degrades every length to 0.
	midi *midi.Cache

	npcs          [16384]*Npc
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
	objDelayedQueue  []objDelayedRequest // NAI-134

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
	friendsBridge         FriendsBridge
	friendsDispatcher     FriendsDispatcher
	friendsAdminBridge    FriendsAdminBridge
	worldEventsDispatcher WorldEventsDispatcher
	worldEventsCancel     context.CancelFunc
	loginBridgeMod        LoginBridgeMod
	loggerBridge          LoggerBridge

	// bridgesCtx is the parent context for fire-and-forget gRPC calls
	// from grpcFriendsBridge / loginGRPCBridgeMod and from the inline
	// PlayerLogout / PlayerForceLogout / PlayerAutosave / PlayerLogin
	// goroutines spawned in server.go and tick.go. Each call wraps it
	// with a per-call WithTimeout (bridgeCallTimeout). bridgesCancel is
	// invoked from Shutdown so in-flight bridge calls observe cancellation
	// promptly instead of running until their per-call deadline.
	// Arc 18 R3 — concurrency / shutdown-safety.
	bridgesCtx    context.Context
	bridgesCancel context.CancelFunc

	// sessionLogs is the per-tick session-log accumulator. NAI-74. Pushed by
	// Player.AddSessionLog; flushed via processSessionLogs in the tick loop.
	//
	// sessionLogsMu guards append (AddSessionLog, called from any goroutine
	// via packet-handler/script paths and from the tick goroutine via
	// processSessionLogs' coord-log push) and the per-tick swap+clear
	// performed by processSessionLogs. Arc 18 audit (R1 — concurrency).
	sessionLogsMu sync.Mutex
	sessionLogs   []SessionLog

	// Login rate-limit attempt counters (rev-254 A4). Mirror TS
	// World.loginAddressAttempts / loginDeviceAttempts (World.ts:176-177
	// @2e3bcf43): per-remote-address (60s TTL) and per-uid@address (15s
	// TTL) counters consulted by handleLogin when NodeProduction is on.
	// Zero values are ready to use; internally mutex-guarded (handleLogin
	// runs on per-connection goroutines). See login_ratelimit.go.
	loginAddressAttempts ttlAttemptCache
	loginDeviceAttempts  ttlAttemptCache

	testPathfinder pathfinderForTarget // injected by tests; nil in production

	// broadcastMesFunc is the broadcast sink for Server.BroadcastMes-style
	// fanouts. Production wiring (nil) routes to BroadcastMes; tests
	// override to capture without exercising the player connection layer.
	// NAI-190.
	broadcastMesFunc func(msg string)

	// pmCount is the monotonic counter feeding the low 16 bits of the
	// pmId stamped on each FriendThread private_message payload.
	// Mirrors TS World.pmCount. Used only by nextPmId (NAI-158).
	// R4 (Arc 18): atomic.Uint32 to future-proof against cross-goroutine
	// callers; today only the tick goroutine calls nextPmId so the field
	// is safe but the audit flagged it as fragile.
	pmCount atomic.Uint32

	// --- rebuild async worker plane (spec 2026-05-18-rebuild-async-fsnotify) ---

	// rebuildReq carries pack-and-reload requests from the ::rebuild handler
	// and the contentWatcher to runRebuildWorker. Depth 1 with non-blocking
	// send: a queued request coalesces all concurrent senders into one pack.
	rebuildReq chan struct{}

	// rebuildResult carries completion events back to the tick goroutine,
	// drained non-blocking at the top of runTickLoopWithRate's for-body.
	// Depth 1; worker waits for in-flight result to be drained before
	// accepting the next request, so a second concurrent enqueue is
	// impossible.
	rebuildResult chan rebuildResult

	// rebuildMu guards rebuildBusy + rebuildPending + rebuildManualInvoker.
	// Held only across state transitions; never across packFn.
	rebuildMu sync.Mutex

	// rebuildBusy is true while packFn is running on the worker.
	rebuildBusy bool

	// rebuildPending is set by dispatchRebuildRequest. Worker re-queues
	// itself on completion if pending is true (closes the race where a
	// request arrives during the brief window between worker drain and
	// busy=false). Mirrors TS DevThread.processNextQueue.
	rebuildPending bool

	// rebuildManualInvoker holds the *Player that triggered the in-flight
	// rebuild via ::rebuild. nil for fsnotify-triggered. Cleared when the
	// worker posts the result.
	rebuildManualInvoker *Player

	// packFn is the function the worker invokes. Defaults to
	// packall.PackAll; test code overrides to avoid 7s real-content packs.
	packFn func(srcDir, outDir, dataPackDir, rawDir string) error

	// reloadFn is the function the tick-drain invokes on success. Defaults
	// to s.Reload; test code overrides to record invocations / inject errors.
	reloadFn func(clearInvs bool) error

	// watchSessionFn is the function runContentWatcher's supervisor loop
	// invokes for each fsnotify watcher session. Defaults to
	// s.runWatchSession; test code overrides to stub session lifecycle.
	// Mirrors the packFn/reloadFn seam pattern. Returns true to request
	// a supervisor restart, false to exit cleanly.
	watchSessionFn func() bool

	// runScriptFn is the seam tick.go uses to fire a script (the four
	// call sites at tick.go:275, :482, :531, :582 — processLogins,
	// processPlayerQueue, processPlayerInteractions, and the timer
	// dispatcher respectively; line numbers may drift, grep for
	// `s\.runScriptFn\(` in tick.go to re-verify). Defaults to
	// (*Server).runScript in NewServer + newTestServer. Tests override
	// to capture invocation args (e.g., the LONG-strip pin in
	// TestProcessPlayerQueue_LongStripsArgs0).
	runScriptFn func(sf *script.ScriptFile, self script.ActivePlayer, target any, trigger script.ServerTriggerType, protect bool, intArgs []int, stringArgs []string)

	// relayActionQueue carries closures enqueued by WorldStateOps
	// methods (the impl of which lives on *Server, world_state_ops.go).
	// Drained at the top of the tick loop body so all field mutations
	// run on the tick goroutine — preserves single-writer semantics
	// on Player state. Buffer 64; drop-newest on full per
	// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL posture (slice 5b adopts the
	// same posture client-side).
	relayActionQueue chan func()

	// cycleStats/lastCycleStats mirror TS World.cycleStats /
	// lastCycleStats (Uint16Array(12), World.ts — both pins; the surface
	// is new to goscape at rev-244 B4, closing a pre-existing 225-era
	// gap). Tick-goroutine-owned; uint16 wrap is TS-faithful.
	// worldStats.go defines the stat constants and the three helpers
	// (addCycleTime, resetCycleTimes, snapshotCycleStats).
	cycleStats     [numWorldStats]uint16
	lastCycleStats [numWorldStats]uint16
}

// appendNewPlayer queues a player for registration on the next tick.
func (s *Server) appendNewPlayer(p *Player) {
	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()
}

func NewServer(cfg Config, loginClient LoginClient, friendsClient FriendsClient, logger *slog.Logger, tap tapper.Tapper) (*Server, error) {
	rsaKey := protocol.DefaultRSAKey
	if cfg.RSAPrivateKeyPath != "" {
		k, err := protocol.LoadRSAKeyPEM(cfg.RSAPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load RSA private key: %w", err)
		}
		rsaKey = k
	}

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
		cfg:           cfg,
		handler:       handler,
		tcpListener:   tcpListener,
		loginClient:   loginClient,
		friendsClient: friendsClient,
		tap:           tap,
		quit:          make(chan interface{}),

		log:              logger.With("component", compServer),
		invs:             make(map[int]*inventory.Inventory),
		zoneMap:          zone.NewZoneMap(),
		zonesTracking:    map[*zone.Zone]struct{}{},
		locObjTracker:    newLocObjTracker(),
		rsbuf:            rsbuf.New(),
		shutdownTick:     -1,
		tickRate:         defaultTickRate,
		gracefulExit:     make(chan struct{}),
		rebuildReq:       make(chan struct{}, 1),
		rebuildResult:    make(chan rebuildResult, 1),
		relayActionQueue: make(chan func(), 64),
		players:          newPlayerList(2048),
	}
	// pmCount inits to 1 per TS World.ts:167 ("can't be 0 as clients will
	// ignore the pm, their array is filled with 0 as default"). R4: atomic
	// field can't be initialized in a struct literal, so Store post-alloc.
	s.pmCount.Store(1)
	s.logNet = logger.With("component", compNet)
	s.logTick = logger.With("component", compTick)
	s.logScript = logger.With("component", compScript)
	s.logContent = logger.With("component", compContent)
	s.rsaKey = rsaKey
	s.packFn = packall.PackAll
	s.reloadFn = s.Reload
	s.watchSessionFn = s.runWatchSession
	s.runScriptFn = s.runScript
	// Arc 18 R3 — bridges parent context; canceled by Shutdown so
	// in-flight fire-and-forget gRPC calls observe shutdown promptly.
	s.bridgesCtx, s.bridgesCancel = context.WithCancel(context.Background())
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), cfg.NodeProfile, s.bridgesCtx, logger.With("component", compFriends))
	s.friendsDispatcher = newEmitFriendsDispatcher(s, logger.With("component", compFriends))
	s.friendsAdminBridge = defaultFriendsAdminBridge(friendsClient, cfg.NodeProfile, logger.With("component", compFriends))
	// Slice 5b: production dispatcher composes the slice-5a slog
	// dispatcher with WorldStateOps so each RELAY_* event both logs
	// AND applies its world-state effect.
	innerSlog := newSlogWorldEventsDispatcher(logger.With("component", compFriends))
	s.worldEventsDispatcher = newActionWorldEventsDispatcher(innerSlog, s)
	if friendsClient != nil {
		ctx, cancel := context.WithCancel(context.Background())
		s.worldEventsCancel = cancel
		sub := newWorldEventsSubscriber(friendsClient, int32(cfg.NodeID), cfg.NodeProfile, s.worldEventsDispatcher, logger.With("component", compFriends))
		go sub.run(ctx)
	}
	s.loginBridgeMod = defaultLoginBridgeMod(loginClient, s.bridgesCtx, logger.With("component", compLogin))
	s.loggerBridge = NewSlogLoggerBridge(logger, s.cfg.NodeID, s.cfg.NodeProfile)
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
	// TS World load order puts VarBitType right after VarPlayerType
	// (World.ts:236). Empty registry (not an error) when the cache
	// predates 254 and has no server/varbit.dat — see LoadVarBitTypes.
	varbitTypes, err := objtype.LoadVarBitTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load varbit types: %w", err)
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
	s.varbitTypes = varbitTypes
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

	animFrames, err := objtype.LoadAnimFrames(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load anim frames: %w", err)
	}

	seqTypes, err := objtype.LoadSeqTypes(cfg.CachePath, animFrames)
	if err != nil {
		return nil, fmt.Errorf("load seq types: %w", err)
	}
	s.seqTypes = seqTypes

	spotanimTypes, err := objtype.LoadSpotanimTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load spotanim types: %w", err)
	}
	s.spotanimTypes = spotanimTypes

	categoryTypes, err := objtype.LoadCategoryTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load category types: %w", err)
	}
	s.categoryTypes = categoryTypes

	componentTypes, err := objtype.LoadComponentTypes(cfg.CachePath)
	if err != nil {
		return nil, fmt.Errorf("load component types: %w", err)
	}
	s.componentTypes = componentTypes

	// Load word-encoding filter from the raw jagfile. Rev-244: TS dropped the
	// existence check and hardcoded "data/raw/wordenc" — missing file is now a
	// fatal boot error. TS ref: Engine-TS/src/cache/wordenc/WordEnc.ts:35-37.
	s.wordenc, err = encfilter.Load()
	if err != nil {
		return nil, fmt.Errorf("load wordenc: %w", err)
	}

	s.scriptProvider = script.NewProvider()
	if count, err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
		s.log.Warn("script provider load failed; scripts will not run", "err", err)
		s.scriptProvider = nil
	} else {
		s.log.Info("script provider loaded", "count", count)
	}

	// A10 @2e3bcf43: the 244-era midiPack name→id registry load that lived
	// here is gone — TS deleted the runtime PackFile lookup; song/jingle
	// names resolve to ids at COMPILE time (tools/pack/Compiler.ts:199)
	// and playSong/playJingle are id-based. The runtime Midi length cache
	// loads below, next to the OnDemand FileStream it reads from.

	for _, spawn := range s.gamemap.NpcSpawns() {
		// Nil/bounds guard: invalid type IDs are rejected before nid
		// consumption (TS GameMap.ts:126-129 at 9aadcec4 — printFatalError
		// + continue fires before `new Npc`).
		if spawn.TypeID < 0 || spawn.TypeID >= len(npcTypes.Configs) {
			continue
		}
		typ := npcTypes.Configs[spawn.TypeID]
		if typ == nil {
			continue
		}
		// 254 pin: spawnBootNpc gates on members BEFORE allocating a nid
		// (TS GameMap.ts:132-136 @2e3bcf43 — the Npc ctor moved inside the
		// members gate; the 244 nid-burn hoist is gone). [gamemap-2]
		_, err := s.spawnBootNpc(typ, spawn.TypeID, spawn.X, spawn.Z, spawn.Level, cfg.NodeMembers)
		if err != nil {
			s.log.Warn("npc registry full; dropping remaining spawns", "err", err)
			break
		}
	}

	s.populateStaticLocsIntoZones()
	s.populateStaticObjsIntoZones()

	// Build the initial concurrent-reader snapshot now that all type-config
	// fields have been populated. DEVIATION-NAI-C-CONFIGS-ATOMIC-SWAP.
	s.storeConfigsSnapshot()

	// OnDemand cache (OnDemand.ts:12: new FileStream('data/pack')).
	// createNew=false, readOnly=true — we only serve reads; the packer
	// owns writes. New() creates empty cache files if they are absent
	// (MkdirAll + WriteFile on first open), which is acceptable: every
	// request will be rejected with a size=0 frame until a real pack
	// populates the files (B6-deferred-cache posture).
	odFS := filestream.New(cfg.CachePath, false, true)
	s.onDemand = newOnDemand(odFS)

	// A10: load the MIDI length cache from the same client-cache
	// FileStream (TS Midi.load reads OnDemand.cache archive 3; called
	// from World.start at World.ts:296 @2e3bcf43, before reload()).
	// Boot-time single-threaded read — the onDemand cacheMu concurrency
	// guard is not yet needed. Degrades to an all-zero cache when the
	// pack hasn't populated archive 3 ("No MIDI data in cache." warning,
	// TS Midi.ts:266).
	s.midi = midi.Load(odFS, func(format string, args ...any) {
		s.log.Warn(fmt.Sprintf(format, args...))
	})

	return s, nil
}

// shouldSpawnNpc gates a boot-time NPC spawn against the world's members
// flag, mirroring TS GameMap.loadNpcs (GameMap.ts:132 at pin 9aadcec4): a
// members-only NpcType (npcType.members == true) spawns only on a members
// world (this.members == true). The TS expression
// `(npcType.members && this.members) || !npcType.members` reduces to:
// skip iff npcType is members-only AND world is F2P.
//
// At the 254 pin (GameMap.ts:132-136 @2e3bcf43) the members gate wraps
// Npc construction, so getNextNid() is only consumed for NPCs that pass.
// goscape's analog is spawnBootNpc (npc_registry.go [gamemap-2]) which
// calls shouldSpawnNpc BEFORE allocNpcSlot. shouldSpawnNpc itself is the
// gate predicate.
//
// The tile F2P gate (GameMap.ts:122-124) is already enforced upstream in
// pkg/gamemap/load.go's loadNPCs and stays orthogonal to this gate.
// A nil typ is rejected by the spawn loop's bounds check before calling
// spawnBootNpc — see [gamemap-2] in npc_registry.go.
// [gamemap-1]
func shouldSpawnNpc(typ *objtype.NpcType, worldMembers bool) bool {
	if typ == nil {
		return false
	}
	if typ.Members && !worldMembers {
		return false
	}
	return true
}

// populateStaticLocsIntoZones pushes each parsed static loc from the gamemap
// into its owning Zone via Zone.AddStaticLoc and writes the loc's collision
// into the FlagMap when its LocType has BlockWalk=true. Called once at
// server startup, adjacent to the NPC-spawn pass. Mirrors the runtime
// AddLoc collision-write path at world_zone.go:20-24, and faithful to TS
// GameMap.loadLocs (GameMap.ts:259-263: `if (type.blockwalk)
// changeLocCollision(...)` before `addStaticLoc(...)`). The boot-time path
// previously omitted the collision write, leaving zones whose only blockers
// are static locs (e.g., Lumbridge castle interior) unallocated and
// producing FlagNull tile reads in pathfinder BFS expansion (Hans waypoint_idx=-1
// in NAI-92 smoke).
func (s *Server) populateStaticLocsIntoZones() {
	for _, loc := range s.gamemap.StaticLocs() {
		lt := s.locTypeOrNil(loc.Type())
		// Collision is written REGARDLESS of active, gated only on blockwalk
		// (TS GameMap.ts:280-282 @dee467c8: `if (type.blockwalk)
		// changeLocCollision(...)`).
		if lt != nil && lt.BlockWalk {
			s.gamemap.ChangeLocCollision(loc.Shape(), loc.Angle(), lt.BlockRange,
				loc.Length, loc.Width, lt.Active, loc.X, loc.Z, loc.Level, true)
		}
		// rev-274 zone-registration gate (TS GameMap.ts:284-286 @dee467c8:
		// `if (type.active) { ...addStaticLoc(...) }`): a static loc is added
		// to its owning zone ONLY when its LocType is active. Inactive locs
		// still contribute collision (above) but are absent from the zone's
		// loc list, so they cannot be op'd/animated as scenery. When the
		// LocType registry is unloaded (lt == nil — the empty-cache test
		// fixture path; TS would printFatalError on a missing type), preserve
		// the prior permissive add since there is no active flag to consult.
		if lt == nil || lt.Active != 0 {
			z := s.zoneMap.Get(loc.Level, loc.X, loc.Z)
			z.AddStaticLoc(loc)
		}
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

	s.tickWg.Add(1)
	go func() {
		defer s.tickWg.Done()
		s.runTickLoop()
	}()

	// rev-274 Task 22a: the OnDemand pump goroutine is started once when the
	// world is ready, alongside the tick loop. It blocks on a signal channel
	// (woken per enqueue) and drains the per-client round-robin — the 254 50ms
	// ticker is gone (TS OnDemandThread.ts @dee467c8). Stopped by s.quit.
	s.odWg.Add(1)
	go func() {
		defer s.odWg.Done()
		s.onDemand.run(s.quit)
	}()

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
	if s.worldEventsCancel != nil {
		s.worldEventsCancel()
	}
	// NB: bridgesCancel is deliberately NOT called here. The save RPCs fired
	// by the tick's final save-all (and by a just-completed logout) are
	// parented to bridgesCtx; cancelling it now would abort them mid-flight
	// and lose the saves. It is cancelled at the end, after waitForSaveFlush.
	close(s.quit)
	s.log.Debug("closing tcp listener")
	s.tcpListener.Close()
	s.log.Debug("waiting for tcp connections to close")
	s.tcpWg.Wait()
	s.log.Debug("all tcp connections closed")
	// Arc 18 R2 — wait for the tick goroutine to observe s.quit and exit
	// before returning from Shutdown. Without this, tick-phase code could
	// still be running (touching s.sessionLogs / s.players / etc.)
	// after Shutdown returned. The tickWg is no-op for graceful-exit
	// paths (processShutdown returns from runTickLoop without taking the
	// quit branch, but Done still fires via the defer).
	s.tickWg.Wait()
	s.log.Debug("tick goroutine exited")

	// Wait for the OnDemand run goroutine to exit. It observes the same
	// s.quit channel as the tick loop, so it will exit promptly.
	s.odWg.Wait()
	s.log.Debug("ondemand goroutine exited")

	// The tick's final save-all (saveAllOnShutdown) and any just-fired logout
	// save are in-flight save RPCs parented to bridgesCtx. Wait (bounded) for
	// them to flush, THEN cancel bridgesCtx — cancelling earlier would abort
	// the saves and lose recent progress.
	s.log.Debug("waiting for player saves to flush")
	s.waitForSaveFlush()
	if s.bridgesCancel != nil {
		s.bridgesCancel()
	}
	s.log.Debug("player saves flushed")

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
		go s.serveConn(conn)
	}
}

// serveConn runs handleTCPConn for one accepted connection and accounts for it
// against tcpWg (which Shutdown waits on).
//
// gap-login-wire-1: the RS2 packet read methods (G1/G2/G4/GData/GJStrLF) panic
// on under-read rather than returning errors, so an unauthenticated, malformed
// login packet — e.g. a short/truncated RSA block — drives RSADec into a
// slice-out-of-range / io.EOF panic during login decode (see
// req.TestUnmarshalBinary_TruncatedRSABlockPanics). Without per-connection
// isolation that panic crosses the goroutine boundary and crashes the entire
// world process, dropping every connected player. The recover() below contains
// any such panic to the single offending connection: handleTCPConn's own defer
// has already run the connection teardown (player removal, flush, socket close)
// during unwinding, so this is the Go equivalent of TS's per-connection
// try/catch -> client.terminate() (TcpServer.ts:29-41). tcpWg.Done() is
// deferred so Shutdown's tcpWg.Wait() can never hang on a panicked connection.
func (s *Server) serveConn(conn net.Conn) {
	defer s.tcpWg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("recovered panic in connection handler",
				"panic", r,
				"remote_addr", conn.RemoteAddr(),
				"stack", string(debug.Stack()))
		}
	}()
	s.handleTCPConn(conn)
}

func (s *Server) handleTCPConn(conn net.Conn) {
	//c := newClient(conn, s, s.log)
	c := newClient(conn, s.cfg.TCPServerWriteTimeout, s.logNet)
	c.server = s
	c.tap = s.tap

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
		if c.tap != nil && c.sessionID != "" {
			c.tap.SessionEnded(c.accountID, c.sessionID, time.Now(), tapper.CloseReasonDisconnect)
			c.sessionID = ""
		}
		if c.player != nil {
			s.removePlayerOnDisconnect(c.player)
			c.player = nil
		}
		// rev-274 Task 22a: drop this connection's OnDemand queue + round-robin
		// entry (TS OnDemandThread 'client_closed' → deleteClient). Harmless for
		// non-ondemand connections (the connID was never inserted → map miss).
		if s.onDemand != nil {
			s.onDemand.clientClosed(&clientODAdapter{c: c})
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

	// rev-244 B3: connect-time seed send REMOVED.
	// At 225, TcpServer.ts:24-27 sent an 8-byte seed immediately on connect.
	// At 244, TcpServer.ts has no such send (9aadcec4) — the seed is now
	// generated and sent inside the op-14 reply (World.ts:2151-2155). A fresh
	// connection receives NO unsolicited bytes.

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
			// logger-transport-4: TS TcpServer.ts:44-67 emits an ENGINE
			// session log on every disconnect kind (close / error / timeout)
			// while a player is attached. Gating on c.player != nil mirrors
			// TS's `if (client.player)` check — pre-login disconnects have
			// no session_uuid to attach the log to.
			if c.player != nil {
				msg, extra := disconnectSessionLogEvent(err)
				c.player.AddSessionLog(LoggerEventTypeEngine, msg, extra...)
			}
			return
		}

		msg := buf[:n]
		// LOG-1: per-byte payload dump is noise at Info. Keep num_bytes at
		// Info for traffic-volume diagnostics; demote raw bytes to Debug.
		c.log.Info("received data", "num_bytes", len(msg))
		c.log.Debug("received data payload", "data", fmt.Sprintf("%v", msg))

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
		case ClientStateOndemand:
			// rev-244 B3: op-15 transitioned this connection to OnDemand mode.
			// Route received bytes to the onDemand handler via a *clientODAdapter.
			// Per-connection buffering: accumulate msg into c.in so partial frames
			// (<4 bytes) are retained across reads, matching the consumed-contract
			// of onClientData (ondemand.go adaptation note (1)).
			// TS: TcpServer.ts:35-37 → OnDemand.onClientData(client).
			if s.onDemand == nil {
				// Defensive: only reachable on hand-built test servers that
				// never ran NewServer. Don't silently drop frames — close.
				c.log.Warn("ondemand handler nil; closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			if !c.bufferData(msg) {
				c.log.Warn("ondemand buffer overflow, closing connection", "remote_addr", conn.RemoteAddr())
				return
			}
			adapter := &clientODAdapter{c: c}
			pending := c.in.Bytes()
			consumed := s.onDemand.onClientData(adapter, pending)
			if consumed > 0 {
				// Advance Pos by consumed bytes; Next() is the Packet equivalent
				// of discard — it returns the slice and advances Pos.
				c.in.Next(consumed)
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
			c.log.Info("partial packet data received, waiting for more", "opcode", loginreq.OpReqInitGameConnection, "length", pLen)
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

		// TODO: save var from msg

		// TODO: save + reconnecting check

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
	c.log.Info("PlayerLogin RPC response", "result", resp.GetResult())

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
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "TCP socket timeout", nil
	}
	return "TCP socket error", []string{err.Error()}
}

var errWorldFull = errors.New("world full")

func (s *Server) addPlayer(p *Player) error {
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	// Lowest-free slot assignment. TS World.ts:875-883 @2e3bcf43:
	// getNextPlayerSlot() (linear 1..2046); -1 → world full → reject.
	slot := s.players.nextSlot()
	if slot == -1 {
		return errWorldFull
	}
	// playerLoop bucketing by client IP (TS World.ts:902-917): connected
	// clients derive the key from their remote address; headless logins
	// (no client socket) land in the 127.0.0.1 bucket. The bucket fixes
	// this player's per-tick processing position — independent of slot.
	var remoteAddr string
	if p.client != nil && p.client.conn != nil {
		remoteAddr = p.client.conn.RemoteAddr().String()
	}
	s.players.add(slot, playerLoopKey(remoteAddr), p) // sets p.slot (TS World.ts:919-921)
	p.uid = composeUID(p.username37, p.slot)          // NAI-113: TS World.ts:922
	p.active = true
	// Seed the default-south orientation now that p.x/p.z are set, so
	// the always-forced FACE_COORD low-def orients a freshly-logged-in
	// player south rather than the client's north-east default.
	p.unfocus()
	if s.zoneMap != nil {
		z := s.zoneMap.Get(p.level, p.x, p.z)
		p.zoneListElement = z.EnterPlayer(p, s.zoneMap.Grid(p.level))
	}
	if s.rsbuf != nil {
		s.rsbuf.AddPlayer(int32(p.slot))
	}
	return nil
}

// getTotalPlayers returns the count of live players.
// TS World.ts:1691-1702 @2e3bcf43 recounts occupied slots 1..2046 on
// every call (with a "todo: could cache this" note); playerList.count
// caches the identical value.
// Takes the read lock: called from connection goroutines (handleLogin's
// NodeMaxConnected gate) concurrently with tick-goroutine mutations.
func (s *Server) getTotalPlayers() int {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	return int(s.players.count.Load())
}

// isUsernameLoggingOut reports whether a player slot is occupied by an
// entry with this username (already safe-name normalized) whose
// loggingOut flag is set. Mirrors the lookup TS World.logoutRequests.has
// (World.ts:2194) performs against its in-flight-logout set; goscape
// stores the equivalent signal on Player.loggingOut (player.go:310,
// flipped in world_state_ops.go:101 / tick.go:342,350 / reboot.go:56).
// Takes the read lock: called from connection goroutines while the tick
// goroutine's loopUnlink rewrites bucket-slice headers via slices.Delete
// (the pre-A2 fixed array tolerated lock-free pointer reads; the bucket
// slices do NOT).
func (s *Server) isUsernameLoggingOut(safeName string) bool {
	s.playersMu.RLock()
	defer s.playersMu.RUnlock()
	for p := range s.players.all() {
		if p.username == safeName && p.loggingOut {
			return true
		}
	}
	return false
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

// removePlayerInternal performs the slot/zone/players cleanup for p.
// Must only be called from the tick goroutine.
//
// Callers should use removePlayerOnTick or removePlayerOnDisconnect,
// which add the appropriate gRPC-side cleanup before invoking this.
//
// TS Player.cleanup at Engine-TS/src/engine/entity/Player.ts:446 calls
// player.heroPoints.clear() as part of cleanup. goscape omits the
// call: newPlayer (player.go:506) allocates a fresh *Player per login
// with a fresh NewHeroPoints(16) (player.go:544), so clearing the
// about-to-be-GC'd ledger has no observable effect. Informal English
// deferral (no NAI-XXX-D pin); precedent set by combat sub-spec
// framing cleanup (2026-05-20). NAI-120 Bundle 2D follow-up.
func (s *Server) removePlayerInternal(p *Player) {
	p.active = false
	s.playersMu.Lock()
	defer s.playersMu.Unlock()

	if p.slot < 1 || p.slot >= len(s.players.entities) || s.players.get(p.slot) != p {
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
	// TS World.ts:1588-1589 @2e3bcf43: `delete this.players[player.slot]`
	// + `player.unlink()` (drop out of the playerLoop bucket).
	s.players.remove(p)

	// world-ops-2: TS World.removePlayer (World.ts:1642) calls
	// changeNpcCollision(player.width, player.x, player.z, player.level,
	// false) unconditionally after deleting the slot, clearing the
	// FlagBlockNPCs at the player's current tile. The flag is planted by
	// SetVisibility(Default) (player.go:674) and moved on every step and
	// teleport by refreshPlayerZonePresence (zone_refresh.go), so the
	// current tile is where it lives. Width is always 1 per TS
	// PathingEntity init (matching the goscape hardcode in SetVisibility).
	if s.gamemap != nil {
		s.gamemap.ChangeNPCCollision(1, p.x, p.z, p.level, false)
	}

	// 244 delta: TS Player.cleanup (Player.ts:452-454) clears buildArea then
	// resets appearanceInv to -1. heroPoints.clear() (Player.ts:452) is
	// omitted — newPlayer allocates a fresh ledger per login (NAI-120 B2D).
	// buildArea.clear(false) wired here per TS field order; the onReconnect
	// path calls clear(true), which is a TS no-op (BuildArea.ts:24-28).
	p.buildArea.clear(false)
	p.appearanceInv = -1
	// A9 @2e3bcf43: TS Player.cleanup clears resumeButtons — twice, a
	// 2dc4a811 sync quirk (Player.ts:454 `this.resumeButtons = []` AND
	// :456 `this.resumeButtons.length = 0`). One nil-out suffices; mostly
	// hygiene since goscape allocates a fresh *Player per login, but it
	// also guards a late RESUME/IF_BUTTON packet racing the teardown.
	p.resumeButtons = nil
	// 254 delta: TS Player.cleanup (Player.ts:458 @43e02957) ends with
	// this.input.flush() so a tracked player logging out mid-buffer
	// still submits the partial accumulation blob. Nil-guard is
	// goscape-defensive for direct struct-literal Players in tests.
	if p.input != nil {
		p.input.Flush()
	}
}

// removePlayerOnTick handles graceful logout from the tick goroutine.
// Captures p.Save() while still on-tick (thread-safe) and fires a
// best-effort PlayerLogout RPC in a goroutine, then performs internal
// cleanup.
//
// Deviation NAI-PLAYERLOADING-D-LOGOUT-NO-FORCE-FALLBACK: on RPC
// failure, log only — no PlayerForceLogout belt-and-braces (TS parity).
//
// PlayerLogout RPC contents pinned by TestRemovePlayerOnTick_*
// (server_logout_test.go).
func (s *Server) removePlayerOnTick(p *Player) {
	if s.loginClient != nil && p.username != "" {
		save := p.Save(s.invTypes, s.varpTypes)
		username := p.username
		// saveWg: Shutdown waits for this to flush before bridgesCancel so a
		// logout-then-stop doesn't lose the save (the RPC is parented to
		// bridgesCtx, which Shutdown otherwise cancels immediately).
		s.saveWg.Add(1)
		go func() {
			defer s.saveWg.Done()
			// Arc 18 R3 — parent moved from context.Background to
			// Server.bridgesCtx so Shutdown cancels in-flight RPC promptly.
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			defer cancel()
			_, err := s.loginClient.PlayerLogout(ctx, &loginpb.PlayerLogoutRequest{
				NodeId:   int32(s.cfg.NodeID),
				Profile:  s.cfg.NodeProfile,
				Username: username,
				Save:     save,
			})
			if err != nil {
				s.log.Warn("PlayerLogout RPC failed",
					slog.String("username", username), slog.Any("err", err))
			}
		}()
	}
	if s.friendsClient != nil && p.username != "" {
		username37 := p.username37
		worldID := int32(s.cfg.NodeID)
		profile := s.cfg.NodeProfile
		// Arc 18 R3 — per-call timeout + shutdown-derived parent so a hung
		// friends-server cannot pile up goroutines.
		go func() {
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			defer cancel()
			s.friendsClient.PlayerLogout(ctx, &friendspb.PlayerLogoutRequest{
				WorldId:    worldID,
				Profile:    profile,
				Username37: username37,
			})
		}()
	}
	if p.friendsSubCancel != nil {
		p.friendsSubCancel()
		p.friendsSubCancel = nil
	}
	// logger-transport-4: TS World.removePlayer (World.ts:1606) emits a
	// MODERATOR session log immediately before flushPlayer/cleanup. Mirror
	// the order here — fire BEFORE removePlayerInternal so p.session, p.x,
	// p.z are still set when AddSessionLog snapshots them. The graceful and
	// disconnect paths both funnel through this function (the disconnect
	// path enqueues this on the relay queue), so the log emits once per
	// logout regardless of how the player left.
	p.AddSessionLog(LoggerEventTypeModerator, "Logged out")
	s.removePlayerInternal(p)
}

// removePlayerOnDisconnect handles an ungraceful socket close from the
// per-conn goroutine. It cannot call p.Save() here (that reads player state
// the tick goroutine concurrently mutates — a data race), so it defers the
// whole removal to the tick by enqueuing removePlayerOnTick on the
// relayActionQueue (drained at the top of the tick loop). removePlayerOnTick
// runs on-tick, so p.Save() is safe and the player IS saved — matching TS,
// which keeps a dropped player in-world and saves them via the idle-logout
// (the earlier "PlayerForceLogout, no save" path lost all progress since the
// last 15-minute autosave, and its "TS has the same window" note was wrong).
//
// removePlayerInternal is idempotent (slot-identity guard), so this is safe
// even if the tick's own no-connection idle-logout fires for the same player.
func (s *Server) removePlayerOnDisconnect(p *Player) {
	s.enqueueRelayAction(func() {
		s.removePlayerOnTick(p)
	})
}

// playerSaveFlushTimeout bounds how long Shutdown waits for in-flight save
// RPCs to flush before cancelling bridgesCtx — long enough for one bridge
// call (bridgeCallTimeout) plus margin, but bounded so a hung login server
// cannot wedge shutdown indefinitely.
const playerSaveFlushTimeout = bridgeCallTimeout + 2*time.Second

// saveAllOnShutdown saves and removes every online player. Called from the
// tick goroutine when the tick loop is exiting (Shutdown closed s.quit), so
// p.Save() inside removePlayerOnTick is race-free. Each removePlayerOnTick
// fires a saveWg-tracked PlayerLogout RPC; Shutdown then waits on saveWg
// (bounded by playerSaveFlushTimeout) before cancelling bridgesCtx so these
// saves reach the login server. Without this, players still online when the
// operator stops the server lost all progress since the last autosave.
func (s *Server) saveAllOnShutdown() {
	s.playersMu.RLock()
	var players []*Player
	for p := range s.players.all() {
		players = append(players, p)
	}
	s.playersMu.RUnlock()
	for _, p := range players {
		if p.username != "" {
			s.removePlayerOnTick(p)
		}
	}
}

// waitForSaveFlush blocks until all in-flight save RPCs complete or
// playerSaveFlushTimeout elapses, whichever comes first.
func (s *Server) waitForSaveFlush() {
	done := make(chan struct{})
	go func() {
		s.saveWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(playerSaveFlushTimeout):
		s.log.Warn("timed out waiting for player saves to flush on shutdown")
	}
}

// PlayerSaveRate is the autosave cadence in ticks. 1500 ticks at ~600ms
// ≈ 15 minutes. Mirrors TS World.PLAYER_SAVERATE.
const PlayerSaveRate = 1500

// autosavePlayers fires a best-effort PlayerAutosave RPC for each
// active player. Must only be called from the tick goroutine
// (reads s.players and captures per-player Save() bytes on-tick
// for goroutine-safety).
//
// Deviation NAI-PLAYERLOADING-D-AUTOSAVE-FIRE-AND-FORGET: per-call
// failures log only (PlayerAutosave is best-effort by design); no
// automatic remediation.
func (s *Server) autosavePlayers() {
	if s.loginClient == nil {
		return
	}
	for p := range s.players.all() {
		if p.username == "" {
			continue
		}
		save := p.Save(s.invTypes, s.varpTypes)
		req := &loginpb.PlayerAutosaveRequest{
			Profile:  s.cfg.NodeProfile,
			Username: p.username,
			Save:     save,
		}
		// Arc 18 R3 — per-call timeout + shutdown-derived parent.
		s.saveWg.Add(1)
		go func() {
			defer s.saveWg.Done()
			ctx, cancel := context.WithTimeout(s.bridgesCtx, bridgeCallTimeout)
			defer cancel()
			s.loginClient.PlayerAutosave(ctx, req)
		}()
	}
}

const expectedRevision = revision.Expected

// TrackZone marks a zone as modified this tick. Idempotent (map semantics).
// processZones will call ComputeShared on each tracked zone; processCleanup
// will Reset them and clear the set.
//
// Must only be called from the tick goroutine — the zonesTracking map is
// unguarded.
func (s *Server) TrackZone(z *zone.Zone) { s.zonesTracking[z] = struct{}{} }

// LookupPlayerByUID returns the logged-in player whose uid field matches
// the argument, or nil if no such player is active. Intended to be
// called from the tick goroutine (players is unguarded there).
// Implements the script.PlayerLookup interface consumed by
// FINDUID / P_FINDUID (S7a).
//
// Does NOT filter on CanAccess — callers that need the protected
// variant consult the returned player's CanAccess() separately. Mirrors
// TS World.getPlayerByUid which is a pure lookup.
func (s *Server) LookupPlayerByUID(uid int) script.ActivePlayer {
	// Compare via int32-cast: scripts push uids through ScriptState.PushInt
	// which int32-normalises per TS toInt32 parity (Numbers.ts:7), while
	// composeUID stores the uint32 bit-pattern as a positive Go int. Both
	// representations share the bottom 32 bits; sign-extension reconciles
	// them. Without this, ~50% of usernames (those with bit 31 set in
	// composeUID output) failed every p_finduid call after the int32-cast
	// PushInt fix landed.
	target := int32(uid)
	for p := range s.players.all() {
		if !p.active {
			continue
		}
		if int32(p.uid) == target {
			return p
		}
	}
	return nil
}

// LookupPlayerByUsername returns the logged-in player whose username
// field matches the argument exactly, or nil if none is active.
// Mirrors TS World.getPlayerByUsername (World.ts:1675-1689). Intended
// to be called from the tick goroutine (players is unguarded there).
//
// Match is case-sensitive on the goscape username field (which is set
// at login from the client-supplied display name). TS keys on
// username37 (base37-encoded) but the inputs to this lookup are
// already strings in goscape's call sites.
func (s *Server) LookupPlayerByUsername(name string) *Player {
	for p := range s.players.all() {
		if !p.active {
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
	return s.players.get(slot)
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
