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
// Gates per TS OpHeldHandler.ts @2e3bcf43 (the 254 pin moves the delayed
// check FIRST, drops clearPendingAction from every rejection branch, and
// gates ALL five ops on the iop array — including op 5, whose 'Drop'
// class default passes for ordinary items):
//  1. p.delayed → drop. TS:16-19.
//  2. payload < 6 → drop (goscape defensive).
//  3. com: nil || !operable (Go field Interactable), then !visible →
//     drop. TS:21-28 (two branches; Go combines — same accept set).
//  4. listener/inv unresolved → drop. TS:30-35.
//  5. !validSlot(slot) || !hasAt(slot, item) → drop. TS:37-43.
//  6. obj.iop[op-1] === null → drop (ALL ops; "" encodes TS null;
//     'hidden' is NOT rejected here — contrast the op-loc/npc/obj
//     ground-click handlers). TS:45-49.
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction iff
// com.RootLayer != p.modalMain → moveClickRequest=false →
// sessionlog (MODERATOR, "<iop> <debugname>") for ops 1-4 only →
// fire [opheld<op>,<objId>] via GetByTrigger keyed on
// (objType.id, objType.Category) and runScript with protect=true.
// TS:51-72.
func handleOpHeld(p *Player, payload []byte, op int) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed — FIRST at the 254 pin. TS OpHeldHandler.ts:16-19.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	if len(payload) < 6 {
		return nil
	}

	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())

	// Gate 3: component check. TS OpHeldHandler.ts:21-28 @2e3bcf43
	// (`!com.operable` — goscape's Interactable field is the TS operable).
	com := s.lookupComponent(comId)
	if com == nil || !com.Interactable || !p.IsComponentVisible(com) {
		return nil
	}

	// Gates 4+5: listener → inv → slot → item. TS OpHeldHandler.ts:30-43
	// @2e3bcf43 (getInventoryFromListener takes the possibly-undefined
	// find result; HasAt covers validSlot via OOB-slot false).
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		return nil
	}

	// Gate 6: iop validation for ALL ops (254 moves it after the inv
	// checks and removes the op-5 bypass — op 5 passes via the 'Drop'
	// class default for ordinary items). TS OpHeldHandler.ts:45-49
	// @2e3bcf43: `if (obj.iop[message.op - 1] === null) return false;`.
	var objType *objtype.ObjType
	if s.objTypes != nil && item >= 0 && item < len(s.objTypes.Configs) {
		objType = s.objTypes.Configs[item]
	}
	if objType == nil || len(objType.IOp) < op || objType.IOp[op-1] == "" {
		return nil
	}

	p.lastItem = item
	p.lastSlot = slot

	// Conditional clearPendingAction: only when rootLayer differs from modalMain.
	// TS OpHeldHandler.ts:54-56 @2e3bcf43.
	if com.RootLayer != p.modalMain {
		p.ClearPendingAction()
	}

	// TS OpHeldHandler.ts:58 — "uses the dueling ring op to move whilst
	// busy & queue pending".
	p.moveClickRequest = false

	// Sessionlog for ops 1-4; op=5 wealth-logged in content.
	// TS OpHeldHandler.ts:60-63 @2e3bcf43.
	if op != 5 {
		p.AddSessionLog(LoggerEventTypeModerator,
			fmt.Sprintf("%s %s", objType.IOp[op-1], objType.DebugName))
	}

	var trigger script.ServerTriggerType
	switch op {
	case 1:
		trigger = script.TriggerOpHeld1
	case 2:
		trigger = script.TriggerOpHeld2
	case 3:
		trigger = script.TriggerOpHeld3
	case 4:
		trigger = script.TriggerOpHeld4
	default: // op == 5
		trigger = script.TriggerOpHeld5
	}

	sf := s.scriptProvider.GetByTrigger(trigger, objType.ConfigType.ID, objType.Category)
	s.runScript(sf, p, nil, trigger, true, nil, nil)
	return nil
}

func handleOpHeld1(p *Player, payload []byte) error { return handleOpHeld(p, payload, 1) }
func handleOpHeld2(p *Player, payload []byte) error { return handleOpHeld(p, payload, 2) }
func handleOpHeld3(p *Player, payload []byte) error { return handleOpHeld(p, payload, 3) }
func handleOpHeld4(p *Player, payload []byte) error { return handleOpHeld(p, payload, 4) }
func handleOpHeld5(p *Player, payload []byte) error { return handleOpHeld(p, payload, 5) }

// handleOpHeldT is the handler for OPHELDT (8-byte payload).
// Spell-on-held-item: player drags a spell from the magic-book interface
// onto an inventory item.
// Wire format: obj:G2 | slot:G2 | component:G2 | spellComponent:G2.
//
// Gates per TS OpHeldTHandler.ts @2e3bcf43 (delayed FIRST; no
// clearPendingAction on rejects; the held-item component gate reverts
// to com.usable; spellCom is validated BEFORE com):
//  1. p.delayed → drop. TS:17-20.
//  2. payload < 8 → drop (goscape defensive).
//  3. spellCom: nil || (ActionTarget&HELD)==0, then !visible → drop.
//     TS:22-29 (two branches; Go combines — same accept set).
//  4. com: nil || !usable, then !visible → drop. TS:31-38.
//  5. listener/inv unresolved → drop. TS:40-45.
//  6. !validSlot(slot) || !hasAt(slot, item) → drop. TS:47-53.
//
// On pass: lastItem/lastSlot snapshot → ClearPendingAction
// (unconditional, contrast OPHELD1-5 conditional) → sessionlog
// "Cast <comName> on <debugname>" → fire [opheldt,<spellComId>] via
// GetByTrigger(typeID=spellComId, cat=-1). On no-script: emit
// "Nothing interesting happens.". TS:55-73.
func handleOpHeldT(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	// Gate 1: delayed — FIRST at the 254 pin. TS OpHeldTHandler.ts:17-20.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	if len(payload) < 8 {
		return nil
	}

	r := packet.NewPacket(payload)
	item := int(r.G2())
	slot := int(r.G2())
	comId := int(r.G2())
	spellComId := int(r.G2())

	// Gate 3: spellCom check — validated BEFORE com at the pin.
	// TS OpHeldTHandler.ts:22-29 @2e3bcf43.
	spellCom := s.lookupComponent(spellComId)
	if spellCom == nil || !p.IsComponentVisible(spellCom) || (spellCom.ActionTarget&objtype.ComActionTargetHeld) == 0 {
		return nil
	}

	// Gate 4: com check — 254 reverts to com.usable (TS
	// OpHeldTHandler.ts:31-38 @2e3bcf43 `!com.usable`; 244 had
	// interactable here).
	com := s.lookupComponent(comId)
	if com == nil || !com.Usable || !p.IsComponentVisible(com) {
		return nil
	}

	// Gates 5+6: listener → inv → slot → item. TS OpHeldTHandler.ts:40-53.
	listener, ok := p.invListeners[comId]
	if !ok {
		return nil
	}
	inv := resolveListenerInv(s, listener)
	if inv == nil || !inv.HasAt(slot, item) {
		return nil
	}

	p.lastItem = item
	p.lastSlot = slot

	// Unconditional ClearPendingAction (contrast OPHELD1-5 conditional).
	// TS OpHeldTHandler.ts:58 @2e3bcf43.
	p.ClearPendingAction()

	// TS OpHeldTHandler.ts:60 @2e3bcf43 — unconditional MODERATOR log.
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

// handleOpHeldU is the handler for OPHELDU (12-byte payload).
// Item-on-held-item: player drags one inventory item onto another.
// Wire format: obj:G2 | slot:G2 | component:G2 | useObj:G2 | useSlot:G2 | useComponent:G2.
//
// Gates per TS OpHeldUHandler.ts @2e3bcf43 (delayed FIRST; the
// comId == useComId gate is RESTORED — 244 had dropped it; both
// component gates revert to com.usable; rejects do NOT
// clearPendingAction except the two hasAt failures, which keep their
// "removed early osrs" moveClickRequest+clearPendingAction cleanup):
//  1. p.delayed → drop. TS:16-19.
//  2. payload < 12 → drop (goscape defensive).
//  3. comId !== useComId → drop ("opheldu cannot target different
//     components"). TS:21-24.
//  4. com: nil || !usable, then !visible → drop. TS:26-33.
//  5. useCom: nil || !usable, then !visible → drop. TS:35-42.
//  6. listener/inv unresolved → drop. TS:44-49.
//  7. !validSlot(slot) → drop; !hasAt(slot, item) →
//     moveClickRequest=false + clearPendingAction + drop. TS:51-59.
//  8. useListener/useInv unresolved → drop. TS:61-66.
//  9. !validSlot(useSlot) → drop; !hasAt(useSlot, useObj) →
//     moveClickRequest=false + clearPendingAction + drop. TS:68-76.
//
// On pass: lastItem/lastSlot/lastUseItem/lastUseSlot snapshot →
// ClearPendingAction (unconditional) → members-only gate: free world +
// (objType.Members || useObjType.Members) ⇒ MessageGame "To use this
// item please login..." + drop. TS:78-93.
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

	// Decode stage (TS OpHeldUDecoder runs BEFORE the handler, so the
	// length guard + field reads sitting ahead of gate 1 mirror the TS
	// decoder/handler split; a short payload is unreachable from the
	// framed read loop and drops the same way in both engines).
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

	// Gate 1: delayed FIRST among the handler gates.
	// TS OpHeldUHandler.ts:16-19 @2e3bcf43.
	if p.delayed && s.currentTick < p.delayedUntil {
		return nil
	}

	// Gate 3: 254 RESTORES the comId == useComId gate that 244 dropped.
	// TS OpHeldUHandler.ts:21-24 @2e3bcf43: "bad client: opheldu cannot
	// target different components".
	if comId != useComId {
		return nil
	}

	// Gate 4: com check — 254 reverts to com.usable. TS OpHeldUHandler.ts:26-33.
	com := s.lookupComponent(comId)
	if com == nil || !com.Usable || !p.IsComponentVisible(com) {
		return nil
	}

	// Gate 5: useCom check. TS OpHeldUHandler.ts:35-42.
	useCom := s.lookupComponent(useComId)
	if useCom == nil || !useCom.Usable || !p.IsComponentVisible(useCom) {
		return nil
	}

	// Gates 6+7: comId listener + inv validation. TS OpHeldUHandler.ts:44-59.
	{
		listener, ok := p.invListeners[comId]
		if !ok {
			return nil
		}
		inv := resolveListenerInv(s, listener)
		if inv == nil {
			return nil
		}
		if !inv.HasAt(slot, item) {
			// TS OpHeldUHandler.ts:55-59 — extra cleanup on the hasAt
			// reject only ("removed early osrs"). goscape's HasAt also
			// covers TS's separate validSlot branch (plain drop); an OOB
			// slot takes this cleanup path too — the only divergence is
			// the cleanup pair on a byte-corrupt slot index, harmless.
			p.moveClickRequest = false
			p.ClearPendingAction()
			return nil
		}
	}

	// Gates 8+9: useComId listener + useInv validation. TS OpHeldUHandler.ts:61-76.
	{
		useListener, ok := p.invListeners[useComId]
		if !ok {
			return nil
		}
		useInv := resolveListenerInv(s, useListener)
		if useInv == nil {
			return nil
		}
		if !useInv.HasAt(useSlot, useItem) {
			// TS OpHeldUHandler.ts:72-76.
			p.moveClickRequest = false
			p.ClearPendingAction()
			return nil
		}
	}

	// State snapshot BEFORE members gate (TS OpHeldUHandler.ts:78-81 ordering).
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

	// Unconditional ClearPendingAction (TS OpHeldUHandler.ts:86).
	p.ClearPendingAction()

	// Members-only gate (TS OpHeldUHandler.ts:88-91).
	if (objType.Members || useObjType.Members) && !s.cfg.NodeMembers {
		p.MessageGame("To use this item please login to a members' server.")
		return nil
	}

	// 4-arm trigger fallback (TS OpHeldUHandler.ts:93-117); first hit wins.
	sf := s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, objType.ConfigType.ID, -1)

	if sf == nil {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, useObjType.ConfigType.ID, -1)
		// Arm (b): UNCONDITIONAL swap whenever (a) misses, regardless of
		// whether (b)'s lookup succeeded (TS OpHeldUHandler.ts:98-100).
		p.lastItem, p.lastUseItem = p.lastUseItem, p.lastItem
		p.lastSlot, p.lastUseSlot = p.lastUseSlot, p.lastSlot
	}

	if sf == nil && objType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, objType.Category)
	}

	if sf == nil && useObjType.Category != -1 {
		sf = s.scriptProvider.GetByTriggerSpecific(script.TriggerOpHeldU, -1, useObjType.Category)
		// Arm (d): UNCONDITIONAL swap whenever (c) misses or is skipped,
		// regardless of whether (d)'s lookup succeeded (TS OpHeldUHandler.ts:112-114).
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
