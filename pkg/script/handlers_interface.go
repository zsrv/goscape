package script

import "strings"

// S5f interface / modal opcodes. Pop orders are reverse-engineered from
// LostCityRS/Engine-TS src/engine/script/handlers/PlayerOps.ts — the TS
// helper popInts(n) fills the destructured slice top-down, so in
// `const [a, b, c] = state.popInts(3)` the stack top is `c`. We mirror
// that pop order by popping from right to left.

// -- Modal management ---------------------------------------------------

// handleIfClose implements IF_CLOSE.
// TS PlayerOps.ts:245 — no pops; just delegates to closeModal().
func handleIfClose(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_CLOSE"); err != nil {
		return err
	}
	s.activePlayer().CloseModal(true)
	return nil
}

// handleIfOpenMain implements IF_OPENMAIN.
// TS PlayerOps.ts:719-721 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenMain(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENMAIN"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENMAIN"); err != nil {
		return err
	}
	s.activePlayer().OpenMain(com)
	return nil
}

// handleIfOpenOverlay implements IF_OPENOVERLAY (opcode 2041).
// TS PlayerOps.ts:709-712 — raw popInt (NO NumberNotNull wrap). -1 must
// reach openOverlay to clear the overlay. Dispatches to the B3 overlay
// state (Player.OpenOverlay, player_script.go; flushed by encodeOut).
// Closes the B2 (0ef495fb wire row) → B3 (ebce9706 entity state+flush)
// → B4 chain.
func handleIfOpenOverlay(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENOVERLAY"); err != nil {
		return err
	}
	s.activePlayer().OpenOverlay(s.PopInt())
	return nil
}

// handleIfOpenChat implements IF_OPENCHAT.
// TS PlayerOps.ts:641-643 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenChat(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENCHAT"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENCHAT"); err != nil {
		return err
	}
	s.activePlayer().OpenChat(com)
	return nil
}

// handleIfOpenSide implements IF_OPENSIDE.
// TS PlayerOps.ts:727-729 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenSide(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENSIDE"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_OPENSIDE"); err != nil {
		return err
	}
	s.activePlayer().OpenSide(com)
	return nil
}

// handleIfOpenMainSide implements IF_OPENMAIN_SIDE.
// TS PlayerOps.ts:645-652 — popInts(2) → [main, side], so side is on
// stack top. We pop side first, then main. Both wrapped with
// check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfOpenMainSide(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_OPENMAIN_SIDE"); err != nil {
		return err
	}
	side := s.PopInt()
	main := s.PopInt()
	if err := checkNotNull(side, "IF_OPENMAIN_SIDE"); err != nil {
		return err
	}
	if err := checkNotNull(main, "IF_OPENMAIN_SIDE"); err != nil {
		return err
	}
	s.activePlayer().OpenMainModalSide(main, side)
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
	if err := requireActivePlayer(s, "TUT_OPEN"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "TUT_OPEN"); err != nil {
		return err
	}
	s.activePlayer().OpenTutorial(com)
	return nil
}

// handleTutClose implements TUT_CLOSE.
// TS PlayerOps.ts:877-879 — no pops; just delegates to closeTutorial().
//
// s.Self==nil guard is goscape defensive (TS skips this check; pointer
// bit and entity reference are always coupled in TS ScriptState).
func handleTutClose(s *ScriptState) error {
	if err := requireActivePlayer(s, "TUT_CLOSE"); err != nil {
		return err
	}
	s.activePlayer().CloseTutorial()
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
	if err := requireActivePlayer(s, "TUT_FLASH"); err != nil {
		return err
	}
	tab := s.PopInt()
	if err := checkNotNull(tab, "TUT_FLASH"); err != nil {
		return err
	}
	s.activePlayer().FlashTutorial(tab)
	return nil
}

// -- Per-component setters ----------------------------------------------

// handleIfSetText implements IF_SETTEXT.
// TS PlayerOps.ts:731-775 @dee467c8 — `let text = state.popString(); const
// com = check(state.popInt(), NumberNotNull);` — text is popped from the
// string stack first, then com from the int stack. The two stacks are
// independent, so order relative to each other only matters for script
// generation.
//
// rev-274: TS added a multi-line colour-persistence transform — when the
// text contains both the literal two-char delimiter `\n` AND an `@`, the
// saved colour code from each line carries onto continuation lines that
// lack their own leading code (with an `@str@`→`@bla@` special case). See
// ifSetTextColourPersist for the char-for-char port of the TS loop.
func handleIfSetText(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETTEXT"); err != nil {
		return err
	}
	text := s.PopString()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETTEXT"); err != nil {
		return err
	}
	text = ifSetTextColourPersist(text)
	s.activePlayer().IfSetText(com, text)
	return nil
}

// ifSetTextColourPersist is a char-for-char port of the IF_SETTEXT
// colour-persistence loop added in TS PlayerOps.ts:734-771 @dee467c8.
//
// The delimiter is the literal TWO-character sequence `\n` (backslash + n),
// not a newline — the TS source literal `'\\n'` is a two-char string. The
// loop walks each `\n`-separated line, tracking the most recent `@xxx@`
// colour code (savedCol). Continuation lines (i > 0) that lack their own
// leading code get savedCol prepended, with the special case that a line
// containing `@str@` inserts `@bla@` after it (unless already present) and
// clears savedCol. All indexing is byte-oriented to match JS UTF-16
// semantics for the ASCII `@`-delimited colour codes (and the wire writes
// the string as raw bytes via PJStrLF).
func ifSetTextColourPersist(text string) string {
	if !strings.Contains(text, "\\n") || !strings.Contains(text, "@") {
		return text
	}

	lines := strings.Split(text, "\\n")
	var savedCol string
	haveSaved := false

	for i := range lines {
		line := lines[i]
		if i > 0 && haveSaved && len(line) > 0 {
			strIndex := strings.Index(line, "@str@")
			if strIndex != -1 {
				if jsSubstring(line, strIndex+5, strIndex+10) != "@bla@" {
					line = line[:strIndex+5] + "@bla@" + line[strIndex+5:]
				}
				savedCol = ""
				haveSaved = false
			} else {
				line = savedCol + line
			}
			lines[i] = line
		}

		for j := 0; j+4 < len(line); j++ {
			if line[j] == '@' && line[j+4] == '@' {
				col := line[j+1 : j+4]
				if col == "str" {
					savedCol = ""
					haveSaved = false
					if jsSubstring(line, j+5, j+10) == "@bla@" {
						j += 9
						continue
					}
				} else {
					savedCol = line[j : j+5]
					haveSaved = true
				}
				j += 4
			}
		}
	}

	return strings.Join(lines, "\\n")
}

// jsSubstring mirrors JavaScript String.prototype.substring(start, end):
// indices are clamped to [0, len] and swapped if start > end. Used by the
// IF_SETTEXT colour-persistence loop where the TS code reads fixed-length
// windows that may run past the end of a short line.
func jsSubstring(s string, start, end int) string {
	n := len(s)
	if start < 0 {
		start = 0
	} else if start > n {
		start = n
	}
	if end < 0 {
		end = 0
	} else if end > n {
		end = n
	}
	if start > end {
		start, end = end, start
	}
	return s[start:end]
}

// handleIfSetModel implements IF_SETMODEL.
// TS PlayerOps.ts:677-684 — popInts(2) → [com, model], model on top.
// Both com and model wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetModel(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETMODEL"); err != nil {
		return err
	}
	model := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETMODEL"); err != nil {
		return err
	}
	if err := checkNotNull(model, "IF_SETMODEL"); err != nil {
		return err
	}
	s.activePlayer().IfSetModel(com, model)
	return nil
}

// handleIfSetNpcHead implements IF_SETNPCHEAD.
// TS PlayerOps.ts:742-749 — popInts(2) → [com, npc], npc on top.
// com wrapped with check(com, NumberNotNull); npc wrapped with
// check(npc, NpcTypeValid) (NAI-23 Bundle 4c).
func handleIfSetNpcHead(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETNPCHEAD"); err != nil {
		return err
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
	s.activePlayer().IfSetNpcHead(com, npc)
	return nil
}

// handleIfSetPlayerHead implements IF_SETPLAYERHEAD.
// TS PlayerOps.ts:731-733 — pops a single int (com); check(com, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetPlayerHead(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETPLAYERHEAD"); err != nil {
		return err
	}
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETPLAYERHEAD"); err != nil {
		return err
	}
	s.activePlayer().IfSetPlayerHead(com)
	return nil
}

// handleIfSetAnim implements IF_SETANIM.
// TS PlayerOps.ts:696-699 @dee467c8 — popInts(2) → [com, seq], seq on top.
//
// rev-274: TS removed the old `if (seq === -1) return;` early-return — −1
// now transmits to the client (clearing the anim) instead of suppressing
// the wire op. com wrapped with check(com, NumberNotNull); seq is NOT
// null-checked (−1 is a valid value that the wire encodes as 0xFFFF).
func handleIfSetAnim(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETANIM"); err != nil {
		return err
	}
	seq := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETANIM"); err != nil {
		return err
	}
	// seq=-1 now passes through (rev-274): the wire encodes it as 0xFFFF.
	s.activePlayer().IfSetAnim(com, seq)
	return nil
}

// handleIfSetHide implements IF_SETHIDE.
// TS PlayerOps.ts:654-661 — popInts(2) → [com, hide], hide on top. The
// hide int is treated as 0/1 boolean (TS uses `hide === 1`). Both com
// and hide wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetHide(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETHIDE"); err != nil {
		return err
	}
	hide := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETHIDE"); err != nil {
		return err
	}
	if err := checkNotNull(hide, "IF_SETHIDE"); err != nil {
		return err
	}
	s.activePlayer().IfSetHide(com, hide == 1)
	return nil
}

// handleIfSetTab implements IF_SETTAB.
// TS PlayerOps.ts:711-717 — popInts(2) → [com, tab], tab on top.
// tab wrapped with check(tab, NumberNotNull); com is NOT wrapped (NAI-23 Bundle 4c).
func handleIfSetTab(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETTAB"); err != nil {
		return err
	}
	tab := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(tab, "IF_SETTAB"); err != nil {
		return err
	}
	// com is NOT wrapped with NumberNotNull in TS (PlayerOps.ts:711-717) (NAI-23 Bundle 4c).
	s.activePlayer().IfSetTab(com, tab)
	return nil
}

// handleIfSetObject implements IF_SETOBJECT.
// TS PlayerOps.ts:663-671 — popInts(3) → [com, obj, scale], scale on top.
// com and scale wrapped with check(_, NumberNotNull); obj wrapped with
// check(obj, ObjTypeValid) (NAI-23 Bundle 4c).
func handleIfSetObject(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETOBJECT"); err != nil {
		return err
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
	s.activePlayer().IfSetObject(com, obj, scale)
	return nil
}

// handleIfSetColour implements IF_SETCOLOUR.
// TS PlayerOps.ts:632-639 — popInts(2) → [com, colour], colour on top.
// The TS handler converts rgb24→rgb15 before writing the wire op; that
// conversion is the Player impl's responsibility in this codebase.
// Both com and colour wrapped with check(_, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetColour(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETCOLOUR"); err != nil {
		return err
	}
	colour := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETCOLOUR"); err != nil {
		return err
	}
	if err := checkNotNull(colour, "IF_SETCOLOUR"); err != nil {
		return err
	}
	s.activePlayer().IfSetColour(com, colour)
	return nil
}

// handleIfSetPosition implements IF_SETPOSITION.
// TS PlayerOps.ts:751-757 — popInts(3) → [com, x, y], y on top.
// com wrapped with check(com, NumberNotNull); x and y NOT wrapped (NAI-23 Bundle 4c).
func handleIfSetPosition(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETPOSITION"); err != nil {
		return err
	}
	y := s.PopInt()
	x := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETPOSITION"); err != nil {
		return err
	}
	// x and y are NOT wrapped with NumberNotNull in TS (PlayerOps.ts:751-757) (NAI-23 Bundle 4c).
	s.activePlayer().IfSetPosition(com, x, y)
	return nil
}

// handleIfSetScrollPos implements IF_SETSCROLLPOS.
// TS PlayerOps.ts:751-757 @3c16994c — popInts(2) → [com, y], y on top.
// com wrapped with check(com, NumberNotNull); y NOT wrapped.
func handleIfSetScrollPos(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETSCROLLPOS"); err != nil {
		return err
	}
	y := s.PopInt()
	com := s.PopInt()
	if err := checkNotNull(com, "IF_SETSCROLLPOS"); err != nil {
		return err
	}
	s.activePlayer().IfSetScrollPos(com, y)
	return nil
}

// IF_SETRECOL deleted in 244 (ScriptOpcode.ts); handleIfSetRecol removed.
// The seam method ActivePlayer.IfSetRecol + wire row are removed in Task 2.

// -- Misc ---------------------------------------------------------------

// handleIfSetTabActive implements IF_SETTABACTIVE.
// TS PlayerOps.ts:673-675 — pops a single int (tab); check(tab, NumberNotNull) (NAI-23 Bundle 4c).
func handleIfSetTabActive(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_SETTABACTIVE"); err != nil {
		return err
	}
	tab := s.PopInt()
	if err := checkNotNull(tab, "IF_SETTABACTIVE"); err != nil {
		return err
	}
	s.activePlayer().IfSetTabActive(tab)
	return nil
}

// handleIfAddResumeButton implements IF_ADDRESUMEBUTTON (opcode 2047;
// replaced the 43e02957-era IF_SETRESUMEBUTTONS popInts(5) form at the
// 254 pin-advance). Mirrors TS PlayerOps.ts @2e3bcf43:
//
//	[ScriptOpcode.IF_ADDRESUMEBUTTON]: checkedHandler(ActivePlayer, state => {
//	    const comId = state.popInt();
//	    state.activePlayer.resumeButtons.push(comId);
//	});
//
// No wire op is emitted; the Player appends the id for later consumption
// by the resume-button gate. No NumberNotNull check (TS pops bare).
//
// A9 LANDED the full lifecycle: resumeButtons is cleared in player
// cleanup (removePlayerInternal), in CloseModal when the active script is
// COUNTDIALOG/PAUSEBUTTON-suspended, in every modal-open method
// (clearSuspendedDialogScript), and in the executeScript Finished/Aborted
// tail (OnScriptFinishedOrAborted) — all in modules/world, mirroring TS
// Player.ts @2e3bcf43 (93ef2d7f + 2dc4a811). No stale-entry hazard remains.
func handleIfAddResumeButton(s *ScriptState) error {
	if err := requireActivePlayer(s, "IF_ADDRESUMEBUTTON"); err != nil {
		return err
	}
	comId := s.PopInt()
	s.activePlayer().AddResumeButton(comId)
	return nil
}
