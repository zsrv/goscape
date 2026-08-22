package world

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zsrv/goscape/pkg/dskit/signals"
	entitypkg "github.com/zsrv/goscape/pkg/entity"
	"github.com/zsrv/goscape/pkg/fonttype"
	"github.com/zsrv/goscape/pkg/gamemap"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/filestream"
	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/packall"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	"github.com/zsrv/goscape/pkg/tapper"
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
	quit        chan struct{}
	log         *slog.Logger // component=world.server (server lifecycle)
	logNet      *slog.Logger // component=world.net (per-connection I/O)
	logTick     *slog.Logger // component=world.tick
	logScript   *slog.Logger // component=world.script
	logContent  *slog.Logger // component=world.content
	logFriends  *slog.Logger // component=world.friends
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
	// admissionGateMu guards the quit-check/tcpWg.Add/trackConn triple at
	// BOTH admission sites (serveTCP's accept loop and the WS bridge's
	// HandleConn) against Shutdown's close(quit)+closeLiveConns+Wait,
	// making admission atomic: a connection either registers in tcpWg AND
	// the live-conn registry before quit closes (so Shutdown both closes
	// it and observes it in Wait) or is refused. Gating matters even in
	// production: a chatty client re-arms its read deadline forever, so an
	// admitted-but-untracked conn would wedge Shutdown — the gate is what
	// makes closeLiveConns complete. (serveTCP's floor registration in
	// tcpWg only prevents WaitGroup misuse — Add racing a Wait that saw a
	// transient zero — it cannot prevent a missed trackConn.)
	admissionGateMu sync.Mutex
	// liveConns/liveConnsMu track every accepted connection so Shutdown can
	// close them. The read loop in handleTCPConn re-arms its deadline on
	// every successful read, so a chatty client that keeps sending never
	// trips the deadline on its own — without an explicit close, Shutdown's
	// tcpWg.Wait would block on that connection's goroutine forever
	// (arch-28.4c). Populated by trackConn at the two admission sites,
	// under admissionGateMu and synchronously with each tcpWg.Add(1)
	// (serveTCP's accept loop; HandleConn), and cleared by untrackConn
	// (deferred in serveConn); closeLiveConns (called from Shutdown)
	// closes every tracked conn.
	liveConns   map[net.Conn]struct{}
	liveConnsMu sync.Mutex
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

	// players is the 244 PlayerList: pid-keyed registry with round-robin
	// allocation. Replaces the 225-era flat players [2048]*Player array
	// and playerLoop []*Player insertion-order slice.
	// TS World.ts:244 uses a single EntityList/PlayerList for both slot
	// lookup (getPlayer) and ordered iteration (players.all() → pid order).
	// Closes PORTING-EXCEPTION (gap-db-datastruct-4).
	// TS refs: login insert World.ts:940-961, removePlayer World.ts:1643-1648,
	// getNextPid World.ts:1758-1773, getTotalPlayers World.ts:1730-1732.
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

	// lastTickNano/currentTickAtomic/lastCycleMillis mirror tick-goroutine
	// state into atomics so HealthSnapshot (arch-29.6) can be called from
	// any goroutine (the ondemand /healthz + /debug/status handlers) without
	// taking playersMu or racing s.currentTick/s.lastCycleStats, which are
	// tick-goroutine-owned. Stamped together, once per tick, by stampTick
	// (tick.go, immediately after s.currentTick++) — zero locks added to
	// the tick path.
	lastTickNano      atomic.Int64 // UnixNano of the last completed tick; 0 before the first tick
	currentTickAtomic atomic.Int64 // snapshot of s.currentTick
	lastCycleMillis   atomic.Int64 // snapshot of lastCycleStats[statCycle]

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
	// NewServer via encfilter.Load(cfg.WordEncPath) (default
	// "data/raw/wordenc"); test paths inject encfilter.Empty(). TS ref:
	// Engine-TS/src/cache/wordenc/WordEnc.ts:35-37.
	wordenc *encfilter.Filter

	// midiPack is the name→id registry loaded from <ContentPath>/pack/midi.pack
	// at world start. Mirrors TS PackFileBase.ts:50-71 (load) + :129-131
	// (getByName). Nil or empty map degrades every midiIDByName lookup to -1,
	// mirroring TS's unknown-name posture (Player.ts:1921-1929 id!==-1 guard).
	midiPack map[string]int

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
	friendsBridge      FriendsBridge
	friendsDispatcher  FriendsDispatcher
	friendsAdminBridge FriendsAdminBridge
	// friendsMutationDispatcher is the single global ordered FIFO queue
	// for every friends-server mutation RPC (arch-29.13). NOT the same
	// thing as friendsDispatcher above (that's the opposite-direction
	// friends-server -> world push sink) — see friends_dispatcher.go's
	// naming note. Constructed here in NewServer; its worker goroutine is
	// started separately by NewWorldService's startingBody (folded into
	// bridgeWg alongside retryBridgeRegistration, arch-29.3's convention).
	friendsMutationDispatcher *friendsMutationDispatcher
	worldEventsDispatcher     WorldEventsDispatcher
	worldEventsCancel         context.CancelFunc
	loginBridgeMod            LoginBridgeMod
	loggerBridge              LoggerBridge

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

	// logoutSaveRetryDelay is the pause between sendPlayerLogoutWithRetry
	// attempts (arch-28.5). Set to 2*time.Second in NewServer; tests shrink
	// it to keep retry tests fast.
	logoutSaveRetryDelay time.Duration

	// bridgeRetryDelay is the pause between retryBridgeRegistration
	// attempts (arch-29.3). Set to 5*time.Second in NewServer; tests shrink
	// it to keep retry tests fast.
	bridgeRetryDelay time.Duration

	// bridgeWg tracks the retryBridgeRegistration goroutines so Shutdown
	// can join them after bridgesCancel — arch-29.3 fix wave: Shutdown
	// must never return with a registration retrier still live.
	bridgeWg sync.WaitGroup

	// worldStartupDone gates the PlayerLogin dispatch (callPlayerLoginRPC)
	// until the WorldStartup registration RPC has succeeded — arch-29.3
	// fix wave. WorldStartup performs a blanket
	// `UPDATE account_login SET logged_in=0 WHERE node_id=? AND profile=?`,
	// so admitting a login while the background registration retry is
	// still pending would let the eventually-successful retry wipe that
	// LIVE session's logged_in flag and falsify the duplicate-login guard.
	// Because the gate only opens inside the same RPC that performs the
	// wipe (worldStartupCall), any login admitted after it opens strictly
	// postdates the wipe — restoring the TS ordering guarantee (the TS
	// login queue processes the reset before any login). Standalone worlds
	// (no login client) have no registration to wait for; initLoginGate
	// opens the gate immediately for them.
	worldStartupDone atomic.Bool

	// sessionLogs is the per-tick session-log accumulator. NAI-74. Pushed by
	// Player.AddSessionLog; flushed via processSessionLogs in the tick loop.
	//
	// sessionLogsMu guards append (AddSessionLog, called from any goroutine
	// via packet-handler/script paths and from the tick goroutine via
	// processSessionLogs' coord-log push) and the per-tick swap+clear
	// performed by processSessionLogs. Arc 18 audit (R1 — concurrency).
	sessionLogsMu sync.Mutex
	sessionLogs   []SessionLog

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

	// tickBodyFn is the per-tick body seam (SEC1 test hook); defaults to
	// s.tickOnce.
	tickBodyFn func()

	// relayActionQueue carries closures enqueued by WorldStateOps
	// methods (the impl of which lives on *Server, world_state_ops.go).
	// Drained at the top of the tick loop body so all field mutations
	// run on the tick goroutine — preserves single-writer semantics
	// on Player state. Buffer 64; drop-newest on full per
	// NAI-S5A-D-WORLDEVENTS-DROP-ON-FULL posture (slice 5b adopts the
	// same posture client-side).
	relayActionQueue chan func()

	// removalQueue/removalMu back enqueueRemoval/drainRemovals: an
	// unbounded, guaranteed-delivery queue for lifecycle-critical
	// closures (player removal on disconnect). Unlike
	// relayActionQueue this never drops — a dropped removal ghosts a
	// player in-world for the 100-tick no-response timeout while the
	// tick keeps writing into a dead connection's buffers (arch-28.4a).
	removalQueue []func()
	removalMu    sync.Mutex

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
		quit:          make(chan struct{}),

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
	s.initChildLoggers(logger)
	s.rsaKey = rsaKey
	s.packFn = packall.PackAll
	s.reloadFn = s.Reload
	s.watchSessionFn = s.runWatchSession
	s.runScriptFn = s.runScript
	s.tickBodyFn = s.tickOnce
	// Arc 18 R3 — bridges parent context; canceled by Shutdown so
	// in-flight fire-and-forget gRPC calls observe shutdown promptly.
	s.bridgesCtx, s.bridgesCancel = context.WithCancel(context.Background())
	// arch-28.5: default retry pause for sendPlayerLogoutWithRetry; tests
	// override to keep retry loops fast.
	s.logoutSaveRetryDelay = 2 * time.Second
	// arch-29.3: default retry pause for retryBridgeRegistration; tests
	// override to keep retry loops fast.
	s.bridgeRetryDelay = 5 * time.Second
	s.initLoginGate(loginClient)
	// arch-29.13: construct the dispatcher itself here (mirrors every
	// other bridge field); its worker goroutine must NOT start yet — see
	// the field doc comment — so this stays a plain allocation, no
	// goroutine spawn, safe during NewServer/module-init.
	s.friendsMutationDispatcher = newFriendsMutationDispatcher(logger.With("component", compFriends))
	s.friendsBridge = defaultFriendsBridge(friendsClient, int32(cfg.NodeID), cfg.NodeProfile, s.friendsMutationDispatcher, logger.With("component", compFriends))
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
	// cfg.WordEncPath defaults to that same literal, resolved against the
	// process working directory as before; embedders may override it.
	s.wordenc, err = encfilter.Load(cfg.WordEncPath)
	if err != nil {
		return nil, fmt.Errorf("load wordenc: %w", err)
	}

	s.scriptProvider = script.NewProvider(s.log)
	if count, err := s.scriptProvider.Load(filepath.Join(cfg.CachePath, "server")); err != nil {
		s.log.Warn("script provider load failed; scripts will not run", "err", err)
		s.scriptProvider = nil
	} else {
		s.log.Info("script provider loaded", "count", count)
	}

	// Load MidiPack name→id registry from <ContentPath>/pack/midi.pack.
	// TS: MidiPack = new PackFile('midi', …) loaded at server startup via
	// PackFileBase.ts:50-71. Absent file → empty registry → every
	// midiIDByName lookup returns -1 → PlaySong/PlayJingle are silent
	// no-ops (TS unknown-name posture, Player.ts:1921-1929). ContentPath
	// is empty in tests; os.ReadFile fails silently → empty map.
	if cfg.ContentPath != "" {
		midiPackPath := filepath.Join(cfg.ContentPath, "pack", "midi.pack")
		s.midiPack = loadMidiPack(midiPackPath)
		if len(s.midiPack) == 0 {
			s.log.Warn("midi.pack not loaded; PlaySong/PlayJingle will be silent no-ops", "path", midiPackPath)
		} else {
			s.log.Info("midi.pack loaded", "count", len(s.midiPack))
		}
	} else {
		// B6 live-smoke finding: an empty ContentPath used to skip this
		// block silently — in-game music degraded to nothing with no log
		// (the client keeps looping title music). Surface it.
		s.log.Warn("world.content-path unset; midi name registry unavailable — PlaySong/PlayJingle will be silent no-ops")
	}

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
		// 244 hoist: spawnBootNpc consumes a nid BEFORE the members gate
		// (TS GameMap.ts:131-134 at pin 9aadcec4). On F2P worlds, members-
		// only NPCs are gated out but their nid is still consumed, keeping
		// the nid sequence identical to a members world. [gamemap-2]
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

	return s, nil
}

// shouldSpawnNpc gates a boot-time NPC spawn against the world's members
// flag, mirroring TS GameMap.loadNpcs (GameMap.ts:132 at pin 9aadcec4): a
// members-only NpcType (npcType.members == true) spawns only on a members
// world (this.members == true). The TS expression
// `(npcType.members && this.members) || !npcType.members` reduces to:
// skip iff npcType is members-only AND world is F2P.
//
// At 244 (pin 9aadcec4) the members gate (GameMap.ts:132) is applied AFTER
// Npc construction which consumes getNextNid() (GameMap.ts:131). goscape's
// analog is spawnBootNpc (npc_registry.go [gamemap-2]) which calls
// allocNpcSlot() before invoking shouldSpawnNpc, so nids are consumed even
// for gated-out members NPCs. shouldSpawnNpc itself is unchanged by the hoist
// — it remains the gate predicate used at the post-alloc decision point.
//
// The tile F2P gate (GameMap.ts:122-124) is already enforced upstream in
// pkg/gamemap/load.go's loadNPCs and stays orthogonal to this gate.
// A nil typ is rejected by the spawn loop's bounds check before calling
// spawnBootNpc — see the [gamemap-2] hoist comment in NewServer.
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
		// net.ErrClosed from Accept only arises when Shutdown closes the
		// listener (never from a client-side close), so mapping it to nil is
		// the clean-shutdown path; the accept loop's <-s.quit guard corroborates.
		if errors.Is(err, net.ErrClosed) {
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

	// OnDemand.ts:357 (World.ts): OnDemand.cycle() is started once when the
	// world is ready, alongside the tick loop. Go uses a dedicated goroutine
	// running a 50ms ticker (onDemand.run) stopped by the same s.quit signal.
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
	// Under admissionGateMu so both admission sites' quit-check/tcpWg.Add/
	// trackConn sequences (serveTCP's accept loop; HandleConn, see
	// conn_handler.go) are atomic with this close.
	s.admissionGateMu.Lock()
	close(s.quit)
	s.admissionGateMu.Unlock()
	s.log.Debug("closing tcp listener")
	s.tcpListener.Close()
	// Close every accepted connection: read loops re-arm their deadlines
	// per read, so without this a connected client blocks tcpWg.Wait
	// indefinitely. Closing is safe concurrently with in-flight reads and
	// writes; each conn goroutine exits through its normal error path
	// (enqueueing its player's removal for the tick's final save-all).
	s.closeLiveConns()
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
	// arch-29.3 fix wave: join the registration retriers (retryBridgeRegistration)
	// so Shutdown never returns with live goroutines.
	s.bridgeWg.Wait()
	s.log.Debug("player saves flushed")
}

var errWorldFull = errors.New("world full")

// logoutSaveAttempts bounds sendPlayerLogoutWithRetry's retry loop
// (arch-28.5): TS's login "server" is an in-process worker whose message
// queue survives with the process, so a momentary outage never lost a
// save; the gRPC split introduced a loss window (last-autosave rollback,
// up to ~15 min) that a couple of retries close for restart-blip outages.
// Retries abort early once bridgesCtx is cancelled (shutdown's
// waitForSaveFlush stays bounded).
const logoutSaveAttempts = 3

// playerSaveFlushTimeout bounds how long Shutdown waits for in-flight save
// RPCs to flush before cancelling bridgesCtx — long enough for one bridge
// call (bridgeCallTimeout) plus margin, but bounded so a hung login server
// cannot wedge shutdown indefinitely.
const playerSaveFlushTimeout = bridgeCallTimeout + 2*time.Second

// PlayerSaveRate is the autosave cadence in ticks. 1500 ticks at ~600ms
// ≈ 15 minutes. Mirrors TS World.PLAYER_SAVERATE.
const PlayerSaveRate = 1500

const expectedRevision = revision.Expected
