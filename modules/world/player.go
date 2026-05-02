package world

import (
	"math/rand/v2"
	"sort"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/zone"
)

// InventoryListener associates a player-visible UI component with an inventory.
type InventoryListener struct {
	Type      int  // InvType id
	Com       int  // UI component id
	Source    int  // -1 = world-shared inventory, else owning player's slot
	FirstSeen bool // true until the first UpdateInvFull; then false
}

const (
	userEventLimit       = 5
	clientEventLimit     = 20
	restrictedEventLimit = 2
	afkEventRate         = 500

	modalStateNone = 0x0
	modalStateMain = 0x1
	modalStateChat = 0x2
	modalStateSide = 0x4
)

// playerTimer is a per-player repeating script registration.
// S5i: identified by target scriptID (TS semantics: setTimer at same
// id overwrites).
//
// As of NAI-27 Bundle 1, the single IntArg int field is widened to
// parallel IntArgs []int + StringArgs []string slices to match the TS
// PlayerTimer.args ScriptArgument[] shape (TS
// Engine-TS/src/engine/entity/Player.ts:910 args field). The widening
// is required for SETTIMER/SOFTTIMER's variadic popScriptArgs body
// (PlayerOps.ts:826,834), which Bundle 2 activates.
type playerTimer struct {
	ScriptID   uint32
	Type       script.PlayerTimerType
	Interval   int
	Clock      int
	IntArgs    []int
	StringArgs []string
}

// Player is the game-side representation of a connected player.
// All fields except client and slot are owned exclusively by the tick goroutine.
type Player struct {
	// === network (from sub-spec 1) ===
	slot   int
	client *client

	// === identity ===
	username      string
	username37    uint64
	hash64        uint64
	displayName   string
	uid           int
	members       bool
	staffModLevel int32

	// === coordinates & level (Entity) ===
	x, z, level                     int
	originX, originZ                int
	lastTickX, lastTickZ, lastLevel int
	lastStepX, lastStepZ            int

	// zoneListElement is the player's intrusive subscription element in
	// pkg/zone.Zone.players. Set by Zone.EnterPlayer; nilled after
	// Zone.LeavePlayer. Used to support O(1) Unlink on cross-zone movement.
	// Per NAI-28 Bundle 2.
	zoneListElement *zone.Element[zone.PlayerLike]

	// === movement (PathingEntity) ===
	moveSpeed              MoveSpeed
	moveRestrict           MoveRestrict
	moveStrategy           MoveStrategy
	blockWalk              BlockWalk
	walkDir, runDir        int
	waypointIndex          int
	waypoints              [25]int
	tele, jump             bool
	stepsTaken             int
	followX, followZ       int
	targetX, targetZ       int
	faceAngleX, faceAngleZ int

	// === interaction target ===
	target entity
	// nextTarget queues a script-set interaction target for next-tick
	// application. Written by the OP/AP fire helpers (interaction_trigger.go,
	// player_interaction_trigger.go) capturing whatever a trigger script
	// stored via SetInteraction; popped at processInteraction tail
	// (interaction.go) per TS Player.ts:1255-1258. Nil between ticks.
	nextTarget entity
	targetOp   int
	// targetSubject snapshots the identity of the interaction target at
	// click time. Components:
	//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
	//     lifecycle gate (set by OpLoc handlers after SetInteraction).
	//   com — payload ID:
	//     - OpLocT/OpNpcT/OpObjT/OpPlayerT: spellCom (UI component ID).
	//     - OpPlayerU: useObj (item ID; NAI-62 producer fix).
	//     - OpLoc1..5 / OpNpc1..5 / OpObj1..5 / OpLocU / OpNpcU / OpObjU: -1.
	//     Canonicalised by SetInteraction: com=0 → -1 (NAI-62, matching TS
	//     PathingEntity.ts:520 truthy). Consumed at trigger lookup via
	//     resolveTriggerTypeId (NAI-62, mirrors TS Player.getOpTrigger:993-995
	//     / getApTrigger:1027-1029) and by scripts via
	//     ActivePlayer.TargetSubjectCom().
	targetSubject    struct{ typ, x, z, level, com int }
	interactionKind  InteractionKind
	apRange          int
	apRangeCalled    bool
	interacted       bool
	repathed         bool
	interactionFired bool
	delayed          bool
	delayedUntil     int
	activeScript     *script.ScriptState
	queue            []playerQueueRequest

	// timers is a per-player repeating-script map keyed by script lookup
	// key. Allocated lazily on first SetTimer call.
	timers map[uint32]*playerTimer

	// varps holds the per-player int values for every registered VarPlayerType.
	// Allocated in processLogins after VarpTypeConfigs is available.
	varps []int32

	// === masks ===
	masks      int
	entitymask int

	// === appearance ===
	body           [7]int
	colors         [5]int
	gender         int
	combatLevel    int
	headicons      int
	appearanceInv  int
	appearanceBuf  []byte
	lastAppearance int

	// === stats & vars ===
	stats      [21]int32
	levels     [21]uint8
	baseLevels [21]uint8
	lastStats  [21]int32
	lastLevels [21]uint8
	vars       []int32
	varsString []string

	// === run energy ===
	run, tempRun             int
	runenergy, lastRunEnergy int
	runweight                int

	// === chat state ===
	publicChat, privateChat, tradeDuel int
	chatMessage                        []byte
	chatColour, chatEffect, chatRights int
	mutedUntil                         time.Time
	messageCount                       int

	// === anim-protect (S7b) ===
	// animProtect gates in-engine animation requests when nonzero.
	// Set by the P_ANIMPROTECT script opcode; read by (*Player).PlayAnim
	// per TS Player.ts:1842 (NAI-56).
	animProtect int

	// walktrigger queues a deferred script id to fire from
	// processWalktrigger on the next interaction tick (-1 = unset).
	// Written by P_WALKTRIGGER (opcode 2128); read by GETWALKTRIGGER
	// (opcode 2023) and (*Player).processWalktrigger. Mirrors TS
	// Player.walktrigger at Player.ts:1057-1070.
	walktrigger int

	// === character-design gate (S7e) ===
	// allowDesign permits IdkSaveDesign inbound packets (character-design
	// recustomise) when true. Set by the ALLOWDESIGN script opcode. Reader
	// path unported (S7e-D1).
	allowDesign bool

	// === session flags ===
	playtime                                     int
	lastResponse, lastConnected                  int
	requestLogout, requestIdleLogout, loggingOut bool
	preventLogoutMessage                         string
	preventLogoutUntil                           int
	reconnecting, lowMemory, webClient           bool
	afkEventReady, moveClickRequest              bool
	opcalled                                     bool

	// === AFK zones (sub-spec 4a) ===
	afkZones    [2]int32
	lastAfkZone int

	// === modal (from sub-spec 1) ===
	modalMain, modalChat, modalSide                    int
	lastModalMain, lastModalChat, lastModalSide        int
	modalState                                         int
	modalTutorial                                      int
	tabs                                               [14]int
	refreshModal, refreshModalClose, requestModalClose bool

	// === resume buttons (sub-spec 5f) ===
	// Stored by IF_SETRESUMEBUTTONS; consumed by P_PAUSEBUTTON (future sub-spec).
	resumeButtons [5]int

	// === per-tick rate limits (from sub-spec 1) ===
	userLimit, clientLimit, restrictedLimit int

	// === last* fields — for echo suppression ===
	lastItem, lastSlot, lastUseItem, lastUseSlot, lastTargetSlot, lastCom int

	// === inventory (sub-spec 3a) ===
	invs map[int]*inventory.Inventory
	// invListeners maps UI component ID (Com) to an InventoryListener.
	// Registered via invListenOnCom (S6p); unregistered via
	// invStopListenOnCom or cleared on modal close. Keyed structure
	// enables O(1) lookup in handleOpLocU / handleOpNpcU's item-match
	// validation (S6p closure of S6m-D3 / S6o-D3). Nil until first
	// listener registers; safe to read, range, len-check while nil.
	invListeners map[int]InventoryListener

	// === scenery-window state (sub-spec 3a; flattened from pkg/buildarea
	// at NAI-30 Bundle 4) ===
	// Tracks which mapsquares the client has loaded for LOC/scenery rebuild
	// purposes. Per-player; mutated by rebuildScenery() at zone-window exit.
	//
	// rebuiltOnce gates shouldRebuild's first-build trigger. Legacy
	// pkg/buildarea encoded this via OriginX = -1, but Player.originX is
	// already set to a real coord in tick.go's processLogins loop (anchor
	// for PlayerInfo zone-relative encoding, which runs in updatePlayers
	// BEFORE updateMap each tick). Reusing originX as the sentinel would
	// be silently consumed at login. A separate bool keeps the two roles
	// independent.
	lastBuild   int
	loadedZones map[int]bool
	activeZones map[int]bool
	mapsquares  map[uint16]bool
	rebuiltOnce bool

	// === BAS (basic animation set) — sub-spec 3a ===
	readyanim, turnanim                          int
	walkanim, walkanim_b, walkanim_l, walkanim_r int
	runanim                                      int

	// === visibility + active flag (sub-spec 3b) ===
	visibility rsbuf.Visibility
	active     bool

	// === mask state (sub-spec 3b) ===
	animID, animDelay int
	seqTypes          *objtype.SeqTypeConfigs // seeded conditionally in newPlayer; gates PlayAnim

	sayText []byte

	chatBytes []byte

	damageAmt, damageType int

	spotanimID, spotanimHeight, spotanimDelay int

	exactStartX, exactStartZ, exactEndX, exactEndZ int
	exactBegin, exactFinish, exactDir              int

	faceEntity               int
	faceSquareX, faceSquareZ int

	// OrientationX, OrientationZ are persistent face-direction defaults
	// used by the encoder when faceSquareX/Z are unset. Default -1 = "no
	// value" per upstream player.rs:23-24. NAI-30-D1: producer (set_orient
	// script command + initial orientation from npc-config) deferred to
	// engine-port series; field stays at -1 in NAI-30, encoder fallback to
	// player coord matches upstream behavior at info.rs:328-340.
	OrientationX, OrientationZ int
}

// encodeOut mirrors TS NetworkPlayer.encodeOut(). It sends modal open/close
// packets for any state changes since the last tick. All modal fields default
// to zero on a new Player, so this is a no-op until sub-spec 2 populates them.
func (p *Player) encodeOut() {
	modalChanged := p.modalMain != p.lastModalMain ||
		p.modalChat != p.lastModalChat ||
		p.modalSide != p.lastModalSide ||
		p.refreshModalClose

	if modalChanged {
		if p.refreshModalClose {
			// IF_CLOSE wire event. Per-listener UpdateInvStopTransmit
			// packets were already written at CloseModal time via
			// clearComListeners → invStopListenOnCom (NAI-64; TS
			// Player.ts:728-739, 767, 778, 789).
			p.writeOut(gameserver.OpIfClose, nil)
		}
		p.refreshModalClose = false
		p.lastModalMain = p.modalMain
		p.lastModalChat = p.modalChat
		p.lastModalSide = p.modalSide
	}

	if p.refreshModal {
		switch {
		case p.modalState&modalStateMain != modalStateNone && p.modalState&modalStateSide != modalStateNone:
			payload := []byte{byte(p.modalMain >> 8), byte(p.modalMain), byte(p.modalSide >> 8), byte(p.modalSide)}
			p.writeOut(gameserver.OpIfOpenMainSide, payload)
		case p.modalState&modalStateMain != modalStateNone:
			payload := []byte{byte(p.modalMain >> 8), byte(p.modalMain)}
			p.writeOut(gameserver.OpIfOpenMain, payload)
		case p.modalState&modalStateChat != modalStateNone:
			payload := []byte{byte(p.modalChat >> 8), byte(p.modalChat)}
			p.writeOut(gameserver.OpIfOpenChat, payload)
		case p.modalState&modalStateSide != modalStateNone:
			payload := []byte{byte(p.modalSide >> 8), byte(p.modalSide)}
			p.writeOut(gameserver.OpIfOpenSide, payload)
		}
		p.refreshModal = false
	}
}

// writeOut ISAAC-encrypts op.Opcode, writes any length prefix, then writes
// payload to c.bufw. Does NOT flush — processOut calls flushWrite() once per tick.
func (p *Player) writeOut(op gameserver.Op, payload []byte) {
	c := p.client
	encrypted := byte((int(op.Opcode) + int(c.encryptor.GetNext())) & 0xff)
	c.bufw.WriteByte(encrypted)

	switch op.PayloadSize {
	case -1:
		c.bufw.WriteByte(byte(len(payload)))
	case -2:
		n := len(payload)
		c.bufw.WriteByte(byte(n >> 8))
		c.bufw.WriteByte(byte(n))
	}

	c.bufw.Write(payload)
}

func newPlayer(c *client) *Player {
	p := &Player{
		client:         c,
		reconnecting:   c.reconnecting,
		lowMemory:      c.lowMemory,
		username:       c.username,
		username37:     util.ToBase37(c.username),
		staffModLevel:  c.staffModLevel,
		slot:           -1,
		uid:            -1,
		x:              3094,
		z:              3106,
		level:          0,
		originX:        -1,
		originZ:        -1,
		lastTickX:      -1,
		lastTickZ:      -1,
		lastLevel:      -1,
		lastStepX:      -1,
		lastStepZ:      -1,
		walkDir:        -1,
		runDir:         -1,
		waypointIndex:  -1,
		runenergy:      10000,
		lastRunEnergy:  -1,
		moveSpeed:      MoveSpeedInstant,
		moveStrategy:   MoveStrategySmart,
		moveRestrict:   MoveRestrictNormal,
		blockWalk:      BlockWalkNpc,
		combatLevel:    3,
		colors:         [5]int{0, 0, 0, 0, 0},
		body:           [7]int{0, 10, 18, 26, 33, 36, 42},
		modalTutorial:  -1,
		tabs:           [14]int{-1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1},
		appearanceInv:  -1, // test-only sentinel; production binds via SetAppearanceInv from client.go login wiring (NAI-22 Bundle 3).
		lastAppearance: -1,
		targetOp:       -1,
		walktrigger:    -1,
		apRange:        10,
		followX:        -1,
		followZ:        -1,
		targetX:        -1,
		targetZ:        -1,
		faceAngleX:     -1,
		faceAngleZ:     -1,
		lastItem:       -1,
		lastSlot:       -1,
		lastUseItem:    -1,
		lastUseSlot:    -1,
		lastTargetSlot: -1,
		lastCom:        -1,
		lastConnected:  -1,
		lastResponse:   -1,
		readyanim:      -1,
		turnanim:       -1,
		walkanim:       -1,
		walkanim_b:     -1,
		walkanim_l:     -1,
		walkanim_r:     -1,
		runanim:        -1,
		visibility:     rsbuf.VisibilityDefault,
		active:         false,
		animID:         -1,
		animDelay:      -1,
		chatColour:     -1,
		chatEffect:     -1,
		chatRights:     -1,
		damageAmt:      -1,
		damageType:     -1,
		spotanimID:     -1,
		spotanimHeight: -1,
		spotanimDelay:  -1,
		exactStartX:    -1,
		exactStartZ:    -1,
		exactEndX:      -1,
		exactEndZ:      -1,
		exactBegin:     -1,
		exactFinish:    -1,
		exactDir:       -1,
		faceEntity:     -1,
		entitymask:     rsbuf.MaskFaceEntity,
		faceSquareX:    -1,
		faceSquareZ:    -1,
		OrientationX:   -1,
		OrientationZ:   -1,
		lastBuild:      0,
		loadedZones:    map[int]bool{},
		activeZones:    map[int]bool{},
		mapsquares:     map[uint16]bool{},
	}
	if c.server != nil {
		p.seqTypes = c.server.seqTypes
	}
	// Sentinel values so the first tick of updateStats emits all 21 UpdateStat
	// packets. stats[i] is int32 (always >= 0 in gameplay); levels[i] is uint8
	// (max real value 99). -1 and 255 are unreachable legitimate values.
	for i := 0; i < 21; i++ {
		p.lastStats[i] = -1
		p.lastLevels[i] = 255
	}
	// Initialize AFK zones at player spawn position
	p.afkZones[0] = packAfkCoord(0, p.x-10, p.z-10)
	return p
}

// Slot returns the RS2 slot of this player.
func (p *Player) Slot() int { return p.slot }

// Coords returns the player's current absolute coordinates.
func (p *Player) Coords() (x, z, level int) { return p.x, p.z, p.level }

// X is the script-VM ActivePlayer.X accessor. NAI-35.
func (p *Player) X() int { return p.x }

// Z is the script-VM ActivePlayer.Z accessor. NAI-35.
func (p *Player) Z() int { return p.z }

// Busy returns true when the player cannot accept new interactions —
// either delayed (suspended by script delay) or has a main/chat modal
// open. Mirrors TS Player.busy() at Engine-TS/.../Player.ts:801-803
// (which composes containsModalInterface at Player.ts:796-799 — the
// SIDE bit is intentionally excluded).
func (p *Player) Busy() bool {
	return p.delayed || p.modalState&(modalStateMain|modalStateChat) != 0
}

// IsInWilderness returns true when the player is inside one of the two
// hardcoded wilderness rectangles. Mirrors TS Player.isInWilderness()
// at Engine-TS/.../Player.ts:2082-2090.
//
// South wilderness: x in [2944, 3392), z in [3520, 6400).
// North wilderness: x in [2944, 3392), z in [9920, 12800).
//
// Bounds are inclusive on the lower edge and exclusive on the upper —
// preserve verbatim: `<=` would shift the boundary by one tile vs TS.
func (p *Player) IsInWilderness() bool {
	if p.x >= 2944 && p.x < 3392 && p.z >= 3520 && p.z < 6400 {
		return true
	}
	if p.x >= 2944 && p.x < 3392 && p.z >= 9920 && p.z < 12800 {
		return true
	}
	return false
}

// shouldRebuild reports whether the player has crossed the 13x13 zone
// window centered on (originX, originZ), or whether reconnect is true.
// Mirrors pkg/buildarea.BuildArea.ShouldRebuild (NAI-30 Bundle 4 flatten).
func (p *Player) shouldRebuild() bool {
	if !p.rebuiltOnce {
		return true
	}
	if p.reconnecting {
		return true
	}
	originZoneX := p.originX >> 3
	originZoneZ := p.originZ >> 3
	reloadLeftX := (originZoneX - 4) << 3
	reloadRightX := (originZoneX + 5) << 3
	reloadTopZ := (originZoneZ + 5) << 3
	reloadBottomZ := (originZoneZ - 4) << 3
	if p.x < reloadLeftX || p.z < reloadBottomZ ||
		p.x > reloadRightX-1 || p.z > reloadTopZ-1 {
		return true
	}
	return false
}

// rebuildScenery resets the player's scenery-window state, recomputes
// the 13x13 zone window mapsquares centered on (p.x, p.z), and commits
// the new origin. Returns mapsquare list packed as (mapX<<8)|mapZ.
// Mirrors pkg/buildarea.BuildArea.Rebuild (NAI-30 Bundle 4 flatten).
func (p *Player) rebuildScenery(currentTick int) []uint16 {
	p.loadedZones = map[int]bool{}
	p.activeZones = map[int]bool{}
	p.mapsquares = map[uint16]bool{}

	zoneX := p.x >> 3
	zoneZ := p.z >> 3
	for dx := -6; dx <= 6; dx++ {
		for dz := -6; dz <= 6; dz++ {
			zx := zoneX + dx
			zz := zoneZ + dz
			if zx < 0 || zz < 0 {
				continue
			}
			mapX := zx >> 3
			mapZ := zz >> 3
			if mapX > 0xff || mapZ > 0xff {
				continue
			}
			p.mapsquares[uint16((mapX<<8)|mapZ)] = true
			p.activeZones[coordgrid.ZoneIndex(zx<<3, zz<<3, 0)] = true
		}
	}

	p.originX = p.x
	p.originZ = p.z
	p.lastBuild = currentTick
	p.rebuiltOnce = true

	out := make([]uint16, 0, len(p.mapsquares))
	for m := range p.mapsquares {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (p *Player) updateMap() {
	if p.client == nil || p.client.server == nil {
		return
	}
	if !p.shouldRebuild() {
		return
	}
	// rebuildScenery anchors p.originX/Z to the new rebuild position so the
	// next PlayerInfo teleport block produces local coords in range [0, 104].
	// Staleness would overflow the 7-bit PBit(7, localX) encoding.
	ms := p.rebuildScenery(p.client.server.currentTick)
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
func (p *Player) updateZones() {
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server

	// Unload zones no longer active.
	for idx := range p.loadedZones {
		if !p.activeZones[idx] {
			delete(p.loadedZones, idx)
		}
	}

	// Deliver each active zone.
	for idx := range p.activeZones {
		z := s.zoneMap.GetByIndex(idx)

		if !p.loadedZones[idx] {
			p.writeFullFollows(z, s.currentTick)
		}

		if shared := z.Shared(); len(shared) > 0 {
			buf := packet.NewPacket(nil)
			rsbuf.EncodeZonePartialEnclosed(buf, z.X, z.Z, p.originX, p.originZ, shared)
			p.writeOut(gameserver.OpUpdateZonePartialEnclosed, buf.Bytes())
		}

		p.writePartialFollows(z)
		p.loadedZones[idx] = true
	}
}
func (p *Player) updateInvs() {
	if p.client == nil || p.client.server == nil {
		return
	}
	// Collect all observed invs so we can clear Update after all listeners fire.
	observed := make([]*inventory.Inventory, 0, len(p.invListeners))
	for com, l := range p.invListeners {
		var inv *inventory.Inventory
		if l.Source == -1 {
			inv = p.client.server.invs[l.Type]
		} else {
			other := p.client.server.players[l.Source]
			if other == nil {
				continue
			}
			inv = other.invs[l.Type]
		}
		if inv == nil {
			continue
		}

		if inv.Update || l.FirstSeen {
			sendUpdateInvFullCom(p, l.Com, inv)
			if l.FirstSeen {
				// Flip via read-modify-write — map values are not addressable.
				l.FirstSeen = false
				p.invListeners[com] = l
			}
		}
		observed = append(observed, inv)
	}
	// Clear inv.Update AFTER all listeners (multiple listeners can share an inv).
	for _, inv := range observed {
		inv.Update = false
	}
}
func (p *Player) updateStats() {
	for i := 0; i < 21; i++ {
		if p.stats[i] != p.lastStats[i] || p.levels[i] != p.lastLevels[i] {
			sendUpdateStat(p, i, int(p.stats[i]), int(p.levels[i]))
			p.lastStats[i] = p.stats[i]
			p.lastLevels[i] = p.levels[i]
		}
	}
	if p.runenergy/100 != p.lastRunEnergy/100 {
		sendUpdateRunEnergy(p, p.runenergy)
		p.lastRunEnergy = p.runenergy
	}
}

func (p *Player) processOut() {
	p.updateMap()
	p.updatePlayers()
	p.updateNpcs()
	p.updateZones()
	p.updateInvs()
	p.updateStats()
	p.updateAfkZones()
	p.encodeOut()
	p.client.flushWrite()
}

func (p *Player) processIn(currentTick int) {
	p.playtime++

	if currentTick%afkEventRate == 0 {
		p.afkEventReady = rand.Float64() < 0.0167 // AFK_CHANCE1 from TS
	}

	c := p.client
	if c.state != ClientStateGame {
		return
	}

	p.lastConnected = currentTick // mirrors TS decodeIn() line 63

	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0
	p.opcalled = false

	c.inMu.Lock()
	defer c.inMu.Unlock()

	readAny := false
	for p.userLimit < userEventLimit &&
		p.clientLimit < clientEventLimit &&
		p.restrictedLimit < restrictedEventLimit {

		opcode, ok, err := p.readPacket()
		if err != nil {
			return
		}
		if !ok {
			break
		}
		readAny = true
		switch gameclient.Ops[opcode].Category {
		case gameclient.CategoryUserEvent:
			p.userLimit++
		case gameclient.CategoryRestrictedEvent:
			p.restrictedLimit++
		default:
			p.clientLimit++
		}
	}
	if readAny {
		p.lastResponse = currentTick // mirrors TS decodeIn() line 80
	}
}

// readPacket reads, ISAAC-decrypts, and dispatches one complete packet from c.in.
// Returns (opcode, true, nil) on success, (-1, false, nil) if the buffer is empty
// or the payload is incomplete, and (-1, false, errCloseConn) on a fatal error.
// Must be called with c.inMu held.
func (p *Player) readPacket() (int, bool, error) {
	c := p.client

	if c.opcode == -1 {
		raw, err := c.in.Peek(1)
		if err != nil {
			return -1, false, nil
		}
		decrypted := (int(raw[0]) - int(c.decryptor.GetNext())) & 0xff
		op := gameclient.Ops[decrypted]
		if op.Name == "" {
			c.log.Warn("unknown game opcode", "opcode", decrypted)
			c.conn.Close()
			return -1, false, errCloseConn
		}
		c.in.Next(1)
		c.opcode = decrypted
		c.waiting = op.PayloadSize
	}

	if c.waiting == -1 {
		if c.in.Len() < 1 {
			return -1, false, nil
		}
		c.waiting = int(c.in.Next(1)[0])
	} else if c.waiting == -2 {
		if c.in.Len() < 2 {
			return -1, false, nil
		}
		b := c.in.Next(2)
		c.waiting = int(uint16(b[0])<<8 | uint16(b[1]))
		if c.waiting > 1600 {
			c.log.Warn("oversized game packet, closing", "opcode", c.opcode, "size", c.waiting)
			c.conn.Close()
			return -1, false, errCloseConn
		}
	}

	if c.in.Len() < c.waiting {
		return -1, false, nil
	}

	payload := c.in.Next(c.waiting)
	opcode := c.opcode
	c.opcode = -1

	c.log.Debug("game packet", "opcode", opcode, "name", gameclient.Ops[opcode].Name, "len", len(payload))

	if handler := gameHandlers[opcode]; handler != nil {
		if err := handler(p, payload); err != nil {
			return -1, false, err
		}
	}

	return opcode, true, nil
}

// invListenOnCom registers an inventory listener at the given interface
// component ID, matching TS Player.ts:1441-1462 line-by-line.
//
// Behavior:
//   - invType == -1 → no-op (early-out matches TS).
//   - existing listener at com with same Type → no-op (preserves
//     FirstSeen state across redundant inv_transmit calls).
//   - SCOPE_SHARED inv-type → source rewritten to -1 (world-shared
//     dispatch); requires p.client.server.invTypes wired. Graceful
//     no-op when wiring is absent (test-direct-call paths).
//   - Otherwise → store {Type, Com, Source, FirstSeen=true}; the map
//     overwrite naturally implements TS's same-com-different-type
//     splice.
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
func (p *Player) invListenOnCom(invType, com, source int) {
	if invType == -1 {
		return
	}
	if existing, ok := p.invListeners[com]; ok && existing.Type == invType {
		return
	}
	if p.client != nil && p.client.server != nil && p.client.server.invTypes != nil {
		configs := p.client.server.invTypes.Configs
		if invType < len(configs) {
			if cfg := configs[invType]; cfg != nil && cfg.Scope == objtype.InvTypeScopeShared {
				source = -1
			}
		}
	}
	if p.invListeners == nil {
		p.invListeners = make(map[int]InventoryListener)
	}
	p.invListeners[com] = InventoryListener{
		Type:      invType,
		Com:       com,
		Source:    source,
		FirstSeen: true,
	}
}

// invStopListenOnCom unregisters the listener at the given component
// ID and writes UpdateInvStopTransmit(com) to the client. No-op if no
// listener exists there (mirrors TS L1466-1468 early-return; Go's
// delete-on-nil semantics make nil maps a strict subset of "no listener
// registered"). Mirrors TS Player.ts:1464-1471.
//
// Callers must ensure p.client is non-nil; sendUpdateInvStopTransmit
// (and writeOut underneath) dereferences p.client without a guard.
// Production callers (handleInvStopTransmit, CloseModal via
// clearComListeners) are all reached only with a connected client.
func (p *Player) invStopListenOnCom(com int) {
	if _, ok := p.invListeners[com]; !ok {
		return
	}
	delete(p.invListeners, com)
	sendUpdateInvStopTransmit(p, com)
}

// clearComListeners removes every inv-listener whose Component.RootLayer
// equals rootCom and writes UpdateInvStopTransmit per removal. No-op
// when rootCom is -1 (slot was unset; mirrors TS L729-731). No-op when
// the player has no Server bound (goscape defensive; TS skips this
// check since TS Components are a global singleton — Component.get is
// always reachable).
//
// Mirrors TS Player.ts:728-739. Closes NAI-53-D-CLEARCOMLISTENERS-PER-SLOT.
//
// Iteration safety: Go's spec guarantees `delete` during `range` over a
// map is well-defined — deleted keys are not re-yielded. Calling
// invStopListenOnCom (which deletes) inside the range loop is safe.
func (p *Player) clearComListeners(rootCom int) {
	if rootCom == -1 {
		return
	}
	if p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server
	for com := range p.invListeners {
		c := s.lookupComponent(com)
		if c == nil {
			// goscape defensive; TS assumes Component.get(com) is non-nil.
			continue
		}
		if c.RootLayer == rootCom {
			p.invStopListenOnCom(com)
		}
	}
}

// IsValid returns whether the player's session is live per TS semantics:
// not logging out, default visibility, and the active flag set. Mirrors
// TS Player.isValid (loggingOut → visibility → super.isValid() which
// returns isActive).
func (p *Player) IsValid() bool {
	if p.loggingOut {
		return false
	}
	if p.visibility != rsbuf.VisibilityDefault {
		return false
	}
	return p.active
}
