package world

import (
	"math/rand/v2"
	"time"

	"github.com/zsrv/goscape/pkg/buildarea"
	"github.com/zsrv/goscape/pkg/inventory"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameclient "github.com/zsrv/goscape/pkg/io/protocol/game/client"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/rsbuf"
	"github.com/zsrv/goscape/pkg/script"
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
type playerTimer struct {
	ScriptID uint32
	Type     script.PlayerTimerType
	Interval int
	Clock    int
	IntArg   int
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
	target   entity
	targetOp int
	// targetSubject snapshots the identity of the interaction target at
	// click time. Components:
	//   typ, x, z, level — loc identity for tryFireXxxTriggerLoc's
	//     lifecycle gate (set by OpLoc handlers after SetInteraction).
	//   com — spell-component ID for OpLocT; -1 for OpLoc1..5 and OpLocU.
	//     Scripts read via ActivePlayer.TargetSubjectCom() (S6m).
	// S6m: com field resurrected from S6j shrink to carry spellCom.
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
	// Set by the P_ANIMPROTECT script opcode. Reader path unported (S7b-D1).
	animProtect int

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

	// === AFK zones (sub-spec 4a) ===
	afkZones    [2]int32
	lastAfkZone int

	// === modal (from sub-spec 1) ===
	modalMain, modalChat, modalSide             int
	lastModalMain, lastModalChat, lastModalSide int
	modalState                                  int
	refreshModal, refreshModalClose             bool

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

	// === build area (sub-spec 3a) ===
	buildArea *buildarea.BuildArea

	// === BAS (basic animation set) — sub-spec 3a ===
	readyanim, turnanim                          int
	walkanim, walkanim_b, walkanim_l, walkanim_r int
	runanim                                      int

	// === visibility + active flag (sub-spec 3b) ===
	visibility rsbuf.Visibility
	active     bool

	// === mask state (sub-spec 3b) ===
	animID, animDelay int

	sayText []byte

	chatBytes []byte

	damageAmt, damageType int

	spotanimID, spotanimHeight, spotanimDelay int

	exactStartX, exactStartZ, exactEndX, exactEndZ int
	exactBegin, exactFinish, exactDir              int

	faceEntity               int
	faceSquareX, faceSquareZ int
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
			p.writeOut(gameserver.OpIfClose, nil)
			// Stop transmitting every currently-registered inv.
			// Approximation: TS only stops listeners bound to the closing
			// modal's components; we don't yet have a component-to-modal
			// mapping, so clear all. Re-registered on next modal open.
			for _, l := range p.invListeners {
				sendUpdateInvStopTransmit(p, l.Com)
			}
			clear(p.invListeners) // Go 1.21+ map reset; keeps allocated buckets
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
		appearanceInv:  -1,
		targetOp:       -1,
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

func (p *Player) updateMap() {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return
	}
	if !p.buildArea.ShouldRebuild(p.x, p.z, p.reconnecting) {
		return
	}
	ms := p.buildArea.Rebuild(p.x, p.z, p.client.server.currentTick)
	// Anchor the player's scene-base origin to the new rebuild position
	// so the next PlayerInfo teleport block produces local coords in range
	// [0, 104]. Staleness would overflow the 7-bit PBit(7, localX) encoding.
	p.originX = p.x
	p.originZ = p.z
	p.reconnecting = false
	sendRebuildNormal(p, ms)
}
func (p *Player) updateZones() {
	if p.buildArea == nil || p.client == nil || p.client.server == nil {
		return
	}
	s := p.client.server

	// Unload zones no longer active.
	for idx := range p.buildArea.LoadedZones {
		if !p.buildArea.ActiveZones[idx] {
			delete(p.buildArea.LoadedZones, idx)
		}
	}

	// Deliver each active zone.
	for idx := range p.buildArea.ActiveZones {
		z := s.zoneMap.GetByIndex(idx)

		if !p.buildArea.LoadedZones[idx] {
			p.writeFullFollows(z, s.currentTick)
		}

		if shared := z.Shared(); len(shared) > 0 {
			buf := packet.NewPacket(nil)
			rsbuf.EncodeZonePartialEnclosed(buf, z.X, z.Z, p.originX, p.originZ, shared)
			p.writeOut(gameserver.OpUpdateZonePartialEnclosed, buf.Bytes())
		}

		p.writePartialFollows(z)
		p.buildArea.LoadedZones[idx] = true
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

	p.userLimit = 0
	p.clientLimit = 0
	p.restrictedLimit = 0

	c.inMu.Lock()
	defer c.inMu.Unlock()

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
		switch gameclient.Ops[opcode].Category {
		case gameclient.CategoryUserEvent:
			p.userLimit++
		case gameclient.CategoryRestrictedEvent:
			p.restrictedLimit++
		default:
			p.clientLimit++
		}
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
// component ID. If a listener already exists at com, it's replaced and
// FirstSeen resets to true (matches TS Player.ts:1441-1462 add-or-
// replace semantics).
//
// Source = -1 → world-shared inventory (Server.invs[Type]).
// Source >= 0 → another player's slot (Server.players[Source].invs[Type]).
//
// Lazy-initializes the invListeners map on first call.
func (p *Player) invListenOnCom(invType, com, source int) {
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
// ID. No-op if no listener exists there, including when the map itself
// is nil (Go's delete-on-nil is safe). Matches TS Player.ts:1464-1471.
func (p *Player) invStopListenOnCom(com int) {
	delete(p.invListeners, com)
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
