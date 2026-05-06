package world

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// handleOpHeld is the shared implementation for OPHELD1..OPHELD5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | com:G2 (6 bytes).
//
// Gates per TS OpHeldHandler.ts:
//  1. p.delayed → drop
//  2. payload < 6 → drop
//  3. nil component or !Operable → drop
//  4. !IsComponentVisible → drop
//  5. comId not in invListeners → drop
//  6. listener's inventory unresolved → drop
//  7. inv.HasAt(slot, obj) false → drop
//  8. ObjType not registered (goscape defensive; TS throws here) → drop
//  9. objType.IOp[op-1] == "" → drop
//
// On pass: p.lastItem/lastSlot snapshot → ClearPendingAction iff
// com.RootLayer != p.modalMain → moveClickRequest=false →
// faceEntity=-1 + emit entitymask (unconditional, matches TS) →
// fire [opheld<op>,<objId>] via GetByTrigger keyed on
// (objType.id, objType.Category) and runScript with protect=true.
//
// Per TS OpHeldHandler.ts:62-65: op != 5 emits a MODERATOR session
// log "<iop> <debugname>". (op == 5 is wealth-logged in content
// scripts, not here.) NAI-74 activates this; the prior
// NAI-71-D-OPHELD-NO-SESSION-LOG deviation is closed.
func handleOpHeld(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 6 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	com := s.lookupComponent(comId)
	if com == nil || !com.Operable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) {
		return nil
	}
	objType := s.objTypes.Configs[obj]
	if objType == nil { // goscape defensive; TS throws here
		return nil
	}
	if len(objType.IOp) < op || objType.IOp[op-1] == "" {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	if com.RootLayer != p.modalMain {
		p.ClearPendingAction()
	}

	p.moveClickRequest = false
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	// NAI-74: NAI-71-D close. TS OpHeldHandler.ts:62-65 — unconditional
	// at this point in the pipeline (before script lookup).
	if op != 5 {
		p.AddSessionLog(LoggerEventTypeModerator,
			fmt.Sprintf("%s %s", objType.IOp[op-1], objType.DebugName))
	}

	trigger := script.TriggerOpHeld1 + script.ServerTriggerType(op-1)
	sf := s.scriptProvider.GetByTrigger(trigger, obj, objType.Category)
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }

// handleOpHeldT is the handler for OPHELDT (opcode 48, 8-byte payload).
// Spell-on-held-item: player drags a spell from the magic-book interface
// onto an inventory item.
// Wire format: obj:G2 | slot:G2 | com:G2 | spellCom:G2.
//
// Gates per TS OpHeldTHandler.ts:
//  1. p.delayed → drop
//  2. payload < 8 → drop
//  3. spellCom: nil or (ActionTarget & HELD) == 0 → drop
//  4. spellCom: !IsComponentVisible → drop
//  5. com: nil or !Usable → drop
//  6. com: !IsComponentVisible → drop
//  7. comId not in invListeners → drop
//  8. listener's inventory unresolved → drop
//  9. inv.HasAt(slot, obj) false → drop
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction
// (unconditional, contrast OPHELD1-5 conditional) → faceEntity=-1 +
// emit entitymask → fire [opheldt,<spellComId>] via
// GetByTrigger(typeID=spellComId, cat=-1). On no-script: emit
// "Nothing interesting happens.".
//
// Per TS OpHeldTHandler.ts:61: emits a MODERATOR session log
// "Cast <comName> on <debugname>" before script dispatch. NAI-74
// activates this; the prior NAI-71-D-OPHELD-NO-SESSION-LOG
// deviation is closed.
func handleOpHeldT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}
	if len(payload) < 8 {
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	spellComId := int(r.G2())

	spellCom := s.lookupComponent(spellComId)
	if spellCom == nil || (spellCom.ActionTarget&objtype.ComActionTargetHeld) == 0 {
		return nil
	}
	if !p.IsComponentVisible(spellCom) {
		return nil
	}

	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		return nil
	}
	if !p.IsComponentVisible(com) {
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		return nil
	}
	if !inv.HasAt(slot, obj) {
		return nil
	}

	p.lastItem = obj
	p.lastSlot = slot

	p.ClearPendingAction()
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	// NAI-74: NAI-71-D close. TS OpHeldTHandler.ts:61 — unconditional at
	// this point in the pipeline. Inline ObjType lookup is goscape-only
	// (TS uses ObjType.get(obj).debugname which would throw on missing
	// config; goscape skips the session-log on missing — defensive,
	// goscape behaviour-preserving since TS would have thrown).
	if s.objTypes != nil && obj >= 0 && obj < len(s.objTypes.Configs) {
		if objType := s.objTypes.Configs[obj]; objType != nil {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("Cast %s on %s", spellCom.ComName, objType.DebugName))
		}
	}

	sf := s.scriptProvider.GetByTrigger(script.TriggerOpHeldT, spellComId, -1)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}
	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

// handleOpHeldU is the handler for OPHELDU (opcode 130, 12-byte payload).
// Item-on-held-item: player drags one inventory item onto another.
// Wire format: obj:G2 | slot:G2 | com:G2 | useObj:G2 | useSlot:G2 | useCom:G2.
//
// Gates per TS OpHeldUHandler.ts:
//  1. p.delayed → drop
//  2. payload < 12 → drop
//  3. comId != useComId → drop
//  4. com: nil or !Usable → drop
//  5. com: !IsComponentVisible → drop
//  6. useCom: nil or !Usable → drop
//  7. useCom: !IsComponentVisible → drop
//  8. comId not in invListeners → drop
//  9. listener's inventory unresolved → drop
//  10. inv.HasAt(slot, obj) false → moveClickRequest=false +
//      ClearPendingAction + drop (TS OpHeldUHandler.ts:54-58)
//  11. useComId not in invListeners → drop
//  12. useInv unresolved → drop
//  13. useInv.HasAt(useSlot, useObj) false → moveClickRequest=false +
//      ClearPendingAction + drop (TS OpHeldUHandler.ts:71-75)
//
// On pass: lastItem/lastSlot/lastUseItem/lastUseSlot snapshot →
// ClearPendingAction (unconditional, contrast OPHELD1-5 conditional) →
// faceEntity=-1 + emit entitymask → members-only gate: free world +
// (objType.Members || useObjType.Members) ⇒ MessageGame "To use this
// item please login..." + drop.
//
// Trigger fallback (4 arms; first hit wins):
//
//	(a) GetByTriggerSpecific(OPHELDU, objType.id, -1)         — no swap
//	(b) GetByTriggerSpecific(OPHELDU, useObjType.id, -1)      — UNCONDITIONAL swap of
//	                                                             (lastItem,lastUseItem) and
//	                                                             (lastSlot,lastUseSlot)
//	                                                             whenever (a) misses,
//	                                                             regardless of whether
//	                                                             (b)'s lookup succeeded.
//	(c) GetByTriggerSpecific(OPHELDU, -1, objType.Category)   — no swap; only if
//	                                                             objType.Category != -1
//	(d) GetByTriggerSpecific(OPHELDU, -1, useObjType.Category) — UNCONDITIONAL swap
//	                                                             of both pairs whenever
//	                                                             (c) misses (or is
//	                                                             skipped), regardless of
//	                                                             (d)'s lookup result;
//	                                                             only if useObjType.Category
//	                                                             != -1.
//
// On miss across all 4: MessageGame "Nothing interesting happens.".
//
// Note on TS labelling: TS calls (a)/(b) "[opheldu,b]/[opheldu,a]"
// where 'b' = the inventory-listed (dragged-from) item and 'a' = the
// dragged-onto target. The (a)/(b)/(c)/(d) labelling here is plan-local
// for clarity.
func handleOpHeldU(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		slog.Default().Debug("opheldu reject", "gate", "client_nil",
			"client_nil", p.client == nil)
		return nil
	}
	s := p.client.server

	if p.delayed && s.currentTick < p.delayedUntil {
		s.log.Debug("opheldu reject", "gate", "delayed",
			"currentTick", s.currentTick, "delayedUntil", p.delayedUntil)
		return nil
	}
	if len(payload) < 12 {
		s.log.Debug("opheldu reject", "gate", "short_payload",
			"payload_len", len(payload))
		return nil
	}

	r := packet.NewPacket(payload)
	obj := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	useObj := int(r.G2())
	useSlot := int(r.G2())
	useComId := int(r.G2())

	s.log.Debug("opheldu entry",
		"tick", s.currentTick,
		"obj", obj, "slot", slot, "comId", comId,
		"useObj", useObj, "useSlot", useSlot, "useComId", useComId,
		"delayed", p.delayed, "delayedUntil", p.delayedUntil)

	if comId != useComId {
		s.log.Debug("opheldu reject", "gate", "comId_mismatch",
			"comId", comId, "useComId", useComId)
		return nil
	}

	com := s.lookupComponent(comId)
	if com == nil || !com.Usable {
		s.log.Debug("opheldu reject", "gate", "com_nil_or_unusable",
			"com_nil", com == nil, "com_usable", com != nil && com.Usable)
		return nil
	}
	if !p.IsComponentVisible(com) {
		s.log.Debug("opheldu reject", "gate", "com_invisible")
		return nil
	}

	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Usable {
		s.log.Debug("opheldu reject", "gate", "useCom_nil_or_unusable",
			"useCom_nil", useCom == nil, "useCom_usable", useCom != nil && useCom.Usable)
		return nil
	}
	if !p.IsComponentVisible(useCom) {
		s.log.Debug("opheldu reject", "gate", "useCom_invisible")
		return nil
	}

	listener, ok := p.invListeners[comId]
	if !ok {
		s.log.Debug("opheldu reject", "gate", "invListener_missing",
			"comId", comId,
			"listener_count", len(p.invListeners),
			"listener_keys", snapshotInvListenerKeys(p))
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil {
		s.log.Debug("opheldu reject", "gate", "inv_unresolved",
			"listener_type", listener.Type, "listener_source", listener.Source)
		return nil
	}
	if !inv.HasAt(slot, obj) {
		s.log.Debug("opheldu reject", "gate", "inv_hasAt_failed",
			"slot", slot, "obj", obj)
		// TS OpHeldUHandler.ts:54-58 — extra cleanup on this specific reject.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}

	useListener, ok := p.invListeners[useComId]
	if !ok {
		s.log.Debug("opheldu reject", "gate", "useInvListener_missing",
			"useComId", useComId,
			"listener_count", len(p.invListeners),
			"listener_keys", snapshotInvListenerKeys(p))
		return nil
	}
	useInv := resolveListenerInv(s, useListener)
	if useInv == nil {
		s.log.Debug("opheldu reject", "gate", "useInv_unresolved",
			"useListener_type", useListener.Type, "useListener_source", useListener.Source)
		return nil
	}
	if !useInv.HasAt(useSlot, useObj) {
		s.log.Debug("opheldu reject", "gate", "useInv_hasAt_failed",
			"useSlot", useSlot, "useObj", useObj)
		// TS OpHeldUHandler.ts:71-75.
		p.moveClickRequest = false
		p.ClearPendingAction()
		return nil
	}

	// State snapshot BEFORE members gate (matches TS OpHeldUHandler.ts:78-81 ordering).
	p.lastItem = obj
	p.lastSlot = slot
	p.lastUseItem = useObj
	p.lastUseSlot = useSlot

	// ObjType resolution for both objects (goscape defensive; TS throws here).
	if s.objTypes == nil || obj < 0 || obj >= len(s.objTypes.Configs) || s.objTypes.Configs[obj] == nil {
		s.log.Debug("opheldu reject", "gate", "objType_unregistered",
			"which", "obj", "id", obj)
		return nil // goscape defensive; TS throws here
	}
	if useObj < 0 || useObj >= len(s.objTypes.Configs) || s.objTypes.Configs[useObj] == nil {
		s.log.Debug("opheldu reject", "gate", "objType_unregistered",
			"which", "useObj", "id", useObj)
		return nil // goscape defensive; TS throws here
	}
	objType := s.objTypes.Configs[obj]
	useObjType := s.objTypes.Configs[useObj]

	p.ClearPendingAction()
	if p.faceEntity != -1 {
		p.faceEntity = -1
	}
	p.masks |= p.entitymask

	// Members-only gate (TS OpHeldUHandler.ts:90-93).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		s.log.Debug("opheldu reject", "gate", "members_only",
			"obj_members", objType.Members,
			"useObj_members", useObjType.Members,
			"node_members", s.cfg.NodeMembers)
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}

	// 4-arm trigger fallback (TS OpHeldUHandler.ts:96-117); first hit wins.
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)

	if sf == nil {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
		// Arm (b): UNCONDITIONAL swap whenever (a) misses, regardless of
		// whether (b)'s lookup succeeded (TS OpHeldUHandler.ts:101-102).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}

	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
	}

	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
		// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:115-116).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}

	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}

	s.runScript(sf, p, nil, true, nil, nil)
	return nil
}

// snapshotInvListenerKeys returns up to 16 sorted comId keys from
// p.invListeners. NAI-114 Stage 4 throwaway instrumentation; reverted
// at Stage 5 close.
func snapshotInvListenerKeys(p *Player) []int {
	keys := make([]int, 0, len(p.invListeners))
	for k := range p.invListeners {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	if len(keys) > 16 {
		keys = keys[:16]
	}
	return keys
}
