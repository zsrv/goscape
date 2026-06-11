package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/objtype"
	"github.com/zsrv/goscape/pkg/script"
)

// handleOpHeld is the shared implementation for OPHELD1..OPHELD5.
// op is 1..5. Wire format: obj:G2 | slot:G2 | component:G2 (6 bytes).
//
// Gates per TS OpHeldHandler.ts (244 pin, 9aadcec4):
//  1. payload < 6 → drop
//  2. nil component or !Interactable or !IsComponentVisible → clearPendingAction + drop
//  3. for op≠5: nil IOp or IOp[op-1]=="" → clearPendingAction + drop
//     (op=5 skips this gate entirely; wealth-logged in content)
//  4. comId not in invListeners → clearPendingAction + drop
//  5. inv unresolved or !validSlot(slot) or !HasAt(slot, item) → clearPendingAction + drop
//  6. p.delayed → drop (no clearPendingAction)
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction iff
// com.RootLayer != p.modalMain → moveClickRequest=false →
// explicit per-op trigger dispatch (OPHELD1..OPHELD5) →
// sessionlog (MODERATOR, "<iop> <debugname>") for ops 1-4 only →
// fire [opheld<op>,<objId>] via GetByTrigger keyed on
// (objType.id, objType.Category) and runScript with protect=true.
//
// TS OpHeldHandler.ts:62-65 (244): op != 5 emits a MODERATOR session
// log "<iop> <debugname>". (op == 5 is wealth-logged in content
// scripts, not here.) NAI-74 activates this; the prior
// NAI-71-D-OPHELD-NO-SESSION-LOG deviation is closed.
func handleOpHeld(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if len(payload) < 6 {
		return nil
	}

	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	// Gate 2: component must exist, be visible, and be interactable.
	// TS OpHeldHandler.ts:17-20 (244).
	com := s.lookupComponent(comId)
	if com == nil || !com.Interactable || !p.IsComponentVisible(com) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 3: iop validation for op≠5. TS OpHeldHandler.ts:22-25 (244).
	// Condition: (type.iop && !type.iop[op-1]) || !type.iop
	// op=5 bypasses entirely ("wealth logged in content").
	if op != 5 {
		var objType *objtype.ObjType
		if s.objTypes != nil && item >= 0 && item < len(s.objTypes.Configs) {
			objType = s.objTypes.Configs[item]
		}
		if objType == nil || len(objType.IOp) < op || objType.IOp[op-1] == "" {
			p.ClearPendingAction()
			return nil
		}
	}

	// Gate 4: invListeners must contain comId. TS OpHeldHandler.ts:27-30 (244).
	listener, ok := p.invListeners[comId]
	if !ok {
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: inv resolved + HasAt (encompasses slot bounds check).
	// TS OpHeldHandler.ts:32-35 (244); HasAt returns false for out-of-bounds slots.
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: delayed check AFTER all validation. TS OpHeldHandler.ts:37-39 (244).
	// Rejected without calling clearPendingAction.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	// ObjType resolution for session-log and trigger dispatch.
	// goscape defensive: TS throws on missing type; we skip session-log but continue.
	var objType *objtype.ObjType
	if s.objTypes != nil && item >= 0 && item < len(s.objTypes.Configs) {
		objType = s.objTypes.Configs[item]
	}

	p.lastItem = item
	p.lastSlot = slot

	// Conditional clearPendingAction: only when rootLayer differs from modalMain.
	// TS OpHeldHandler.ts:41-43 (244).
	if com.RootLayer != p.modalMain {
		p.ClearPendingAction()
	}

	// ee28c1aa @2e3bcf43 removed the `faceEntity = -1; masks |= entitymask`
	// pair that followed (TS OpHeldHandler.ts diff) — facing is derived
	// per-tick by setFaceEntity() now.
	p.moveClickRequest = false

	// Explicit per-op trigger dispatch (TS OpHeldHandler.ts:46-67, 244).
	// Sessionlog emitted for ops 1-4; op=5 wealth-logged in content.
	var trigger script.ServerTriggerType
	switch op {
	case 1:
		if objType != nil && len(objType.IOp) >= 1 {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("%s %s", objType.IOp[0], objType.DebugName))
		}
		trigger = script.TriggerOpHeld1
	case 2:
		if objType != nil && len(objType.IOp) >= 2 {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("%s %s", objType.IOp[1], objType.DebugName))
		}
		trigger = script.TriggerOpHeld2
	case 3:
		if objType != nil && len(objType.IOp) >= 3 {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("%s %s", objType.IOp[2], objType.DebugName))
		}
		trigger = script.TriggerOpHeld3
	case 4:
		if objType != nil && len(objType.IOp) >= 4 {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("%s %s", objType.IOp[3], objType.DebugName))
		}
		trigger = script.TriggerOpHeld4
	default: // op == 5
		// wealth logged in content (it may not execute!)
		trigger = script.TriggerOpHeld5
	}

	var typeID, typeCat int
	if objType != nil {
		typeID = objType.ConfigType.ID
		typeCat = objType.Category
	} else {
		typeID = item
		typeCat = -1
	}
	sf := s.scriptProvider.GetByTrigger(trigger, typeID, typeCat)
	s.runScript(sf, p, nil, trigger, true, nil, nil)
	return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }

// handleOpHeldT is the handler for OPHELDT (opcode 143, 8-byte payload).
// Spell-on-held-item: player drags a spell from the magic-book interface
// onto an inventory item.
// Wire format: obj:G2 | slot:G2 | component:G2 | spellComponent:G2.
//
// Gates per TS OpHeldTHandler.ts (244 pin, 9aadcec4):
//  1. payload < 8 → drop
//  2. com: nil or !Interactable or !IsComponentVisible → clearPendingAction + drop
//  3. spellCom: nil or !IsComponentVisible or (ActionTarget&HELD)==0 → clearPendingAction + drop
//  4. comId not in invListeners → clearPendingAction + drop
//  5. inv unresolved or !validSlot or !HasAt(slot, item) → clearPendingAction + drop
//  6. p.delayed → drop (no clearPendingAction)
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction
// (unconditional, contrast OPHELD1-5 conditional) →
// fire [opheldt,<spellComId>] via
// GetByTrigger(typeID=spellComId, cat=-1). On no-script: emit
// "Nothing interesting happens.".
//
// Per TS OpHeldTHandler.ts:61 (244): emits a MODERATOR session log
// "Cast <comName> on <debugname>" before script dispatch. NAI-74
// activates this; the prior NAI-71-D-OPHELD-NO-SESSION-LOG
// deviation is closed.
func handleOpHeldT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if len(payload) < 8 {
		return nil
	}

	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	spellComId := int(r.G2())

	// Gate 2: com must exist, be visible, and be interactable.
	// TS OpHeldTHandler.ts:15-18 (244); com checked FIRST in 244.
	com := s.lookupComponent(comId)
	if com == nil || !com.Interactable || !p.IsComponentVisible(com) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 3: spellCom must exist, be visible, and have ActionTarget&HELD set.
	// TS OpHeldTHandler.ts:20-23 (244); merged from separate 225 checks.
	spellCom := s.lookupComponent(spellComId)
	if spellCom == nil || !p.IsComponentVisible(spellCom) || (spellCom.ActionTarget&objtype.ComActionTargetHeld) == 0 {
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: invListeners must contain comId. TS OpHeldTHandler.ts:25-28 (244).
	listener, ok := p.invListeners[comId]
	if !ok {
		p.ClearPendingAction()
		return nil
	}

	// Gate 5: inv resolved + HasAt (encompasses slot bounds check).
	// TS OpHeldTHandler.ts:30-33 (244).
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 6: delayed check AFTER all validation. TS OpHeldTHandler.ts:35-37 (244).
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	p.lastItem = item
	p.lastSlot = slot

	// Unconditional ClearPendingAction (contrast OPHELD1-5 conditional).
	// TS OpHeldTHandler.ts:39 (244).
	// ee28c1aa @2e3bcf43 removed the `faceEntity = -1; masks |= entitymask`
	// pair that followed (TS OpHeldTHandler.ts diff).
	p.ClearPendingAction()

	// NAI-74: NAI-71-D close. TS OpHeldTHandler.ts:61 (244) — unconditional.
	// Inline ObjType lookup is goscape-only (TS uses ObjType.get(obj).debugname which
	// would throw on missing config; goscape skips the session-log on missing —
	// defensive, goscape behaviour-preserving since TS would have thrown).
	if s.objTypes != nil && item >= 0 && item < len(s.objTypes.Configs) {
		if objType := s.objTypes.Configs[item]; objType != nil {
			p.AddSessionLog(LoggerEventTypeModerator,
				fmt.Sprintf("Cast %s on %s", spellCom.ComName, objType.DebugName))
		}
	}

	sf := s.scriptProvider.GetByTrigger(script.TriggerOpHeldT, spellComId, -1)
	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}
	s.runScript(sf, p, nil, script.TriggerOpHeldT, true, nil, nil)
	return nil
}

// handleOpHeldU is the handler for OPHELDU (opcode 58, 12-byte payload).
// Item-on-held-item: player drags one inventory item onto another.
// Wire format: obj:G2 | slot:G2 | component:G2 | useObj:G2 | useSlot:G2 | useComponent:G2.
//
// Gates per TS OpHeldUHandler.ts (244 pin, 9aadcec4):
//  1. payload < 12 → drop
//  2. p.delayed → drop (NO clearPendingAction; first check in 244)
//  3. com: nil or !Interactable or !IsComponentVisible → clearPendingAction + drop
//  4. useCom: nil or !Interactable or !IsComponentVisible → clearPendingAction + drop
//  5. comId not in invListeners → clearPendingAction + drop
//  6. inv unresolved or !validSlot(slot) or !HasAt(slot, item) → moveClickRequest=false +
//     clearPendingAction + drop (TS OpHeldUHandler.ts — "removed early osrs")
//  7. useComId not in invListeners → clearPendingAction + drop
//  8. useInv unresolved or !validSlot(useSlot) or !HasAt(useSlot, useItem) →
//     moveClickRequest=false + clearPendingAction + drop
//
// Note: 244 REMOVES the 225 comId==useComId gate entirely.
//
// On pass: lastItem/lastSlot/lastUseItem/lastUseSlot snapshot →
// ClearPendingAction (unconditional) →
// members-only gate: free world + (objType.Members || useObjType.Members) ⇒
// MessageGame "To use this item please login..." + drop.
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
		return nil
	}
	s := p.client.server

	if len(payload) < 12 {
		return nil
	}

	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	useItem := int(r.G2())
	useSlot := int(r.G2())
	useComId := int(r.G2())

	// Gate 2: delayed is checked FIRST in 244, before component validation.
	// TS OpHeldUHandler.ts:14-16 (244). No clearPendingAction.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	// Note: the 225 comId==useComId gate is REMOVED in 244.
	// TS OpHeldUHandler.ts (244) has no such check.

	// Gate 3: com must exist, be visible, and be interactable.
	// TS OpHeldUHandler.ts:18-21 (244); Interactable replaces Usable.
	com := s.lookupComponent(comId)
	if com == nil || !com.Interactable || !p.IsComponentVisible(com) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 4: useCom must exist, be visible, and be interactable.
	// TS OpHeldUHandler.ts:23-26 (244).
	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Interactable || !p.IsComponentVisible(useCom) {
		p.ClearPendingAction()
		return nil
	}

	// Gate 5+6: comId listener + inv validation.
	// TS OpHeldUHandler.ts:28-36 (244); scoped to a block.
	{
		listener, ok := p.invListeners[comId]
		if !ok {
			p.ClearPendingAction()
			return nil
		}
		inv := resolveListenerInv(s, listener)
		if inv == nil || !inv.HasAt(slot, item) {
			// TS OpHeldUHandler.ts:33-35 — extra cleanup on this specific reject.
			p.moveClickRequest = false
			p.ClearPendingAction()
			return nil
		}
	}

	// Gate 7+8: useComId listener + useInv validation.
	// TS OpHeldUHandler.ts:38-46 (244); scoped to a block.
	{
		useListener, ok := p.invListeners[useComId]
		if !ok {
			p.ClearPendingAction()
			return nil
		}
		useInv := resolveListenerInv(s, useListener)
		if useInv == nil || !useInv.HasAt(useSlot, useItem) {
			// TS OpHeldUHandler.ts:44-46.
			p.moveClickRequest = false
			p.ClearPendingAction()
			return nil
		}
	}

	// State snapshot BEFORE members gate (matches TS OpHeldUHandler.ts:61-64 ordering).
	p.lastItem = item
	p.lastSlot = slot
	p.lastUseItem = useItem
	p.lastUseSlot = useSlot

	// ObjType resolution for both objects (goscape defensive; TS throws here).
	if s.objTypes == nil || item < 0 || item >= len(s.objTypes.Configs) || s.objTypes.Configs[item] == nil {
		return nil
	}
	if useItem < 0 || useItem >= len(s.objTypes.Configs) || s.objTypes.Configs[useItem] == nil {
		return nil
	}
	objType := s.objTypes.Configs[item]
	useObjType := s.objTypes.Configs[useItem]

	// Unconditional ClearPendingAction (TS OpHeldUHandler.ts:69).
	// ee28c1aa @2e3bcf43 removed the `faceEntity = -1; masks |= entitymask`
	// pair that followed (TS OpHeldUHandler.ts diff).
	p.ClearPendingAction()

	// Members-only gate (TS OpHeldUHandler.ts:73-76).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}

	// 4-arm trigger fallback (TS OpHeldUHandler.ts:79-100); first hit wins.
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)

	if sf == nil {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
		// Arm (b): UNCONDITIONAL swap whenever (a) misses, regardless of
		// whether (b)'s lookup succeeded (TS OpHeldUHandler.ts:84-85).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}

	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
	}

	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
		// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:98-99).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}

	if sf == nil {
		p.MessageGame("Nothing interesting happens.")
		return nil
	}

	s.runScript(sf, p, nil, script.TriggerOpHeldU, true, nil, nil)
	return nil
}
