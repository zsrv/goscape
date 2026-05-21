package script

import "errors"

// S5f interface / modal opcodes. Pop orders are reverse-engineered from
// LostCityRS/Engine-TS src/engine/script/handlers/PlayerOps.ts — the TS
// helper popInts(n) fills the destructured slice top-down, so in
// `const [a, b, c] = state.popInts(3)` the stack top is `c`. We mirror
// that pop order by popping from right to left.

// -- Modal management ---------------------------------------------------

// handleIfClose implements IF_CLOSE.
// TS PlayerOps.ts:245 — no pops; just delegates to closeModal().
func handleIfClose(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_CLOSE: no active player")
	}
	s.Self.CloseModal(true)
	return nil
}

// handleIfOpenMain implements IF_OPENMAIN.
// TS PlayerOps.ts:719-721 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenMain(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_OPENMAIN: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENMAIN"); err != nil {
		return err
	}
	s.Self.OpenMain(com)
	return nil
}

// handleIfOpenChat implements IF_OPENCHAT.
// TS PlayerOps.ts:641-643 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenChat(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_OPENCHAT: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENCHAT"); err != nil {
		return err
	}
	s.Self.OpenChat(com)
	return nil
}

// handleIfOpenSide implements IF_OPENSIDE.
// TS PlayerOps.ts:727-729 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenSide(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_OPENSIDE: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENSIDE"); err != nil {
		return err
	}
	s.Self.OpenSide(com)
	return nil
}

// handleIfOpenMainSide implements IF_OPENMAIN_SIDE.
// TS PlayerOps.ts:645-652 — popInts(2) → [main, side], so side is on
// stack top. We pop side first, then main. Both wrapped with
// check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenMainSide(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_OPENMAIN_SIDE: no active player")
	}
	side := s.PopInt()
	main := s.PopInt()
	if err := checkNotNull(side, "IF_OPENMAIN_SIDE"); err != nil {
		return err
	}
	if err := checkNotNull(main, "IF_OPENMAIN_SIDE"); err != nil {
		return err
	}
	s.Self.OpenMainSide(main, side)
	return nil
}

// handleTutOpen implements TUT_OPEN.
// TS PlayerOps.ts:723-725 — pops a single int (com); check(com,
// NumberNotNull). TS reserves com=-1 for the closeTutorial path
// (Player.ts:716-726 writes TutOpen(-1) directly via Player.write,
// not through this opcode); see handleTutClose / (*Player).CloseTutorial
// (NAI-102 port).
//
// s.Self==nil guard is goscape defensive (TS skips this check; pointer
// bit and entity reference are always coupled in TS ScriptState).
func handleTutOpen(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TUT_OPEN: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "TUT_OPEN"); err != nil {
		return err
	}
	s.Self.OpenTutorial(com)
	return nil
}

// handleTutClose implements TUT_CLOSE.
// TS PlayerOps.ts:877-879 — no pops; just delegates to closeTutorial().
//
// s.Self==nil guard is goscape defensive (TS skips this check; pointer
// bit and entity reference are always coupled in TS ScriptState).
func handleTutClose(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TUT_CLOSE: no active player")
	}
	s.Self.CloseTutorial()
	return nil
}

// handleTutFlash implements TUT_FLASH.
// TS PlayerOps.ts:694-696 — pops a single int (tab); check(tab,
// NumberNotNull). No protect gate (TS uses checkedHandler(ActivePlayer,
// ...), not ProtectedActivePlayer). Fire-and-forget — writes a
// TUT_FLASH server packet to draw the player's attention to the
// named tab.
//
// Tab argument is not range-checked: TS encoder uses p1() which
// silently truncates >255 to a single byte. Goscape's ^tab_* runescript
// constants are non-negative single-byte tab indices, so this is
// behaviorally equivalent to TS for in-range inputs.
//
// Pointer check s.Pointers&PtrActivePlayer==0 mirrors TS checkedHandler.
// s.Self==nil guard is goscape defensive (TS skips this check; pointer
// bit and entity reference are always coupled in TS ScriptState).
func handleTutFlash(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("TUT_FLASH: no active player")
	}
	tab := s.PopInt()
	if err := checkNotNull(tab, "TUT_FLASH"); err != nil {
		return err
	}
	s.Self.FlashTutorial(tab)
	return nil
}

// -- Per-component setters ----------------------------------------------

// handleIfSetText implements IF_SETTEXT.
// TS PlayerOps.ts:735-740 — `const text = state.popString(); const com =
// state.popInt();` — text is popped from the string stack first, then
// com from the int stack. The two stacks are independent, so order
// relative to each other only matters for script generation.
// com is wrapped with check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetText(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETTEXT: no active player")
	}
	text := s.PopString()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETTEXT"); err != nil {
		return err
	}
	s.Self.IfSetText(com, text)
	return nil
}

// handleIfSetModel implements IF_SETMODEL.
// TS PlayerOps.ts:677-684 — popInts(2) → [com, model], model on top.
// Both com and model wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetModel(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETMODEL: no active player")
	}
	model := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETMODEL"); err != nil {
		return err
	}
	if err := checkNotNull(model, "IF_SETMODEL"); err != nil {
		return err
	}
	s.Self.IfSetModel(com, model)
	return nil
}

// handleIfSetNpcHead implements IF_SETNPCHEAD.
// TS PlayerOps.ts:742-749 — popInts(2) → [com, npc], npc on top.
// com wrapped with check(com, NumberNotNull); npc wrapped with
// check(npc, NpcTypeValid) (NAI-23 Bundle 4c).
func handleIfSetNpcHead(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETNPCHEAD: no active player")
	}
	npc := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	if err := requireConfigs(s, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	if err := checkNpcType(s, npc, "IF_SETNPCHEAD"); err != nil {
		return err
	}
	s.Self.IfSetNpcHead(com, npc)
	return nil
}

// handleIfSetPlayerHead implements IF_SETPLAYERHEAD.
// TS PlayerOps.ts:731-733 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetPlayerHead(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETPLAYERHEAD: no active player")
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETPLAYERHEAD"); err != nil {
		return err
	}
	s.Self.IfSetPlayerHead(com)
	return nil
}

// handleIfSetAnim implements IF_SETANIM.
// TS PlayerOps.ts:698-709 — popInts(2) → [com, seq], seq on top. TS
// short-circuits when seq == -1 ("client crashes"); we preserve that
// guard so the wire op is suppressed. com wrapped with
// check(com, NumberNotNull); seq uses -1 sentinel (NAI-23 Bundle 4c).
func handleIfSetAnim(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETANIM: no active player")
	}
	seq := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETANIM"); err != nil {
		return err
	}
	// seq=-1 is a valid sentinel in TS (client crashes on -1); no checkNotNull (NAI-23 Bundle 4c).
	if seq == -1 {
		return nil
	}
	s.Self.IfSetAnim(com, seq)
	return nil
}

// handleIfSetHide implements IF_SETHIDE.
// TS PlayerOps.ts:654-661 — popInts(2) → [com, hide], hide on top. The
// hide int is treated as 0/1 boolean (TS uses `hide === 1`). Both com
// and hide wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetHide(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETHIDE: no active player")
	}
	hide := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETHIDE"); err != nil {
		return err
	}
	if err := checkNotNull(hide, "IF_SETHIDE"); err != nil {
		return err
	}
	s.Self.IfSetHide(com, hide == 1)
	return nil
}

// handleIfSetTab implements IF_SETTAB.
// TS PlayerOps.ts:711-717 — popInts(2) → [com, tab], tab on top.
// tab wrapped with check(tab, NumberNotNull); com is NOT wrapped (NAI-23 Bundle 4c).
func handleIfSetTab(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETTAB: no active player")
	}
	tab := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(tab, "IF_SETTAB"); err != nil {
		return err
	}
	// com is NOT wrapped with NumberNotNull in TS (PlayerOps.ts:711-717) (NAI-23 Bundle 4c).
	s.Self.IfSetTab(com, tab)
	return nil
}

// handleIfSetObject implements IF_SETOBJECT.
// TS PlayerOps.ts:663-671 — popInts(3) → [com, obj, scale], scale on top.
// com and scale wrapped with check(_, NumberNotNull); obj wrapped with
// check(obj, ObjTypeValid) (NAI-23 Bundle 4c).
func handleIfSetObject(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETOBJECT: no active player")
	}
	scale := s.PopInt()
	obj := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETOBJECT"); err != nil {
		return err
	}
	if err := requireConfigs(s, "IF_SETOBJECT"); err != nil {
		return err
	}
	if err := checkObjType(s, obj, "IF_SETOBJECT"); err != nil {
		return err
	}
	if err := checkNotNull(scale, "IF_SETOBJECT"); err != nil {
		return err
	}
	s.Self.IfSetObject(com, obj, scale)
	return nil
}

// handleIfSetColour implements IF_SETCOLOUR.
// TS PlayerOps.ts:632-639 — popInts(2) → [com, colour], colour on top.
// The TS handler converts rgb24→rgb15 before writing the wire op; that
// conversion is the Player impl's responsibility in this codebase.
// Both com and colour wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetColour(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETCOLOUR: no active player")
	}
	colour := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETCOLOUR"); err != nil {
		return err
	}
	if err := checkNotNull(colour, "IF_SETCOLOUR"); err != nil {
		return err
	}
	s.Self.IfSetColour(com, colour)
	return nil
}

// handleIfSetPosition implements IF_SETPOSITION.
// TS PlayerOps.ts:751-757 — popInts(3) → [com, x, y], y on top.
// com wrapped with check(com, NumberNotNull); x and y NOT wrapped (NAI-23 Bundle 4c).
func handleIfSetPosition(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETPOSITION: no active player")
	}
	y := s.PopInt()
	x := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETPOSITION"); err != nil {
		return err
	}
	// x and y are NOT wrapped with NumberNotNull in TS (PlayerOps.ts:751-757) (NAI-23 Bundle 4c).
	s.Self.IfSetPosition(com, x, y)
	return nil
}

// handleIfSetRecol implements IF_SETRECOL.
// TS PlayerOps.ts:686-692 — popInts(3) → [com, src, dest], dest on top.
// com wrapped with check(com, NumberNotNull); src and dest NOT wrapped (NAI-23 Bundle 4c).
func handleIfSetRecol(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETRECOL: no active player")
	}
	dest := s.PopInt()
	src := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETRECOL"); err != nil {
		return err
	}
	// src and dest are NOT wrapped with NumberNotNull in TS (PlayerOps.ts:686-692) (NAI-23 Bundle 4c).
	s.Self.IfSetRecol(com, src, dest)
	return nil
}

// -- Misc ---------------------------------------------------------------

// handleIfSetTabActive implements IF_SETTABACTIVE.
// TS PlayerOps.ts:673-675 — pops a single int (tab); check(tab, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetTabActive(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETTABACTIVE: no active player")
	}
	tab := s.PopInt()
	if err := checkNotNull(tab, "IF_SETTABACTIVE"); err != nil {
		return err
	}
	s.Self.IfSetTabActive(tab)
	return nil
}

// handleIfSetResumeButtons implements IF_SETRESUMEBUTTONS.
// TS PlayerOps.ts:781-785 — popInts(5) → [b1, b2, b3, b4, b5], b5 on top.
// No wire op is emitted; the Player stores the 5 ids for later
// consumption by P_PAUSEBUTTON.
func handleIfSetResumeButtons(s *ScriptState) error {
	if s.Pointers&PtrActivePlayer == 0 || s.Self == nil {
		return errors.New("IF_SETRESUMEBUTTONS: no active player")
	}
	b5 := s.PopInt()
	b4 := s.PopInt()
	b3 := s.PopInt()
	b2 := s.PopInt()
	b1 := s.PopInt()
	s.Self.SetResumeButtons(b1, b2, b3, b4, b5)
	return nil
}
