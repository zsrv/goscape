package world

import (
	"bytes"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// TestSendUpdatePid_EmitsExactByteSequence pins the 244 wire bytes of
// sendUpdatePid: p2(slot) + pbool(members). NAI-182 B1; size updated in
// rev-244 B2 Task 3. TS UpdatePidEncoder.ts (244): p2(uid) pbool(members).
func TestSendUpdatePid_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// members=false → pbool encodes as 0x00; slot=0x1234.
	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
		0x12, 0x34, // p2(slot)
		0x00, // pbool(members=false)
	}

	received := drainConn(t, cc)
	p.pid = 0x1234
	sendUpdatePid(p, p.pid, false)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendUpdatePid_MembersTrue pins pbool(members=true) encodes as 0x01.
// TS UpdatePidEncoder.ts (244): pbool(message.members).
func TestSendUpdatePid_MembersTrue(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpUpdatePid.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x01, // p2(slot=1)
		0x01, // pbool(members=true)
	}

	received := drainConn(t, cc)
	p.pid = 1
	sendUpdatePid(p, p.pid, true)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetClientVarCache_EmitsOpcodeOnly pins the wire bytes of sendResetClientVarCache. NAI-182 B1.
func TestSendResetClientVarCache_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetClientVarCache.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetClientVarCache(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestSendResetAnims_EmitsOpcodeOnly pins the wire bytes of sendResetAnims. NAI-182 B1.
func TestSendResetAnims_EmitsOpcodeOnly(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	want := []byte{
		byte((int(gameserver.OpResetAnims.Opcode) + int(enc.GetNext())) & 0xff),
	}

	received := drainConn(t, cc)
	sendResetAnims(p)
	p.client.flushWrite()

	got := <-received
	if !bytes.Equal(got, want) {
		t.Errorf("wire: got %v, want %v", got, want)
	}
}

// TestProcessLogins_FreshLogin_EmitsOpcodeOrder pins CHAT_FILTER_SETTINGS →
// FRIENDLIST_LOADED(2) → UPDATE_IGNORELIST([]) → UPDATE_PID →
// RESET_CLIENT_VARCACHE → RESET_ANIMS emit order on fresh login with NO
// friends server (newTestServer leaves s.friendsClient nil). Verifies the
// sequence wired into processLogins at NAI-182 B3 (UPDATE_PID onward),
// NAI-182-D5 (CHAT_FILTER_SETTINGS prepend), and the 254 no-friendserver
// bootstrap branch per TS Player.ts:496-501 @43e02957 (status 2 = online +
// empty ignore-list bootstrap).
func TestProcessLogins_FreshLogin_EmitsOpcodeOrder(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Walk the byte stream: each packet is an encrypted opcode byte followed
	// by its fixed-length payload. Verify opcodes in order.
	// addPlayer assigns slot 1 (first available slot), so UPDATE_PID carries 0x0001.
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
	}
	// CHAT_FILTER_SETTINGS payload: 3 bytes (publicChat, privateChat, tradeDuel all default 0).
	want = append(want, 0x00, 0x00, 0x00)
	want = append(want,
		byte((int(gameserver.OpFriendlistLoaded.Opcode)+int(enc.GetNext()))&0xff),
	)
	// FRIENDLIST_LOADED payload: 1 byte — status 2 (online; no friendserver).
	want = append(want, 0x02)
	want = append(want,
		byte((int(gameserver.OpUpdateIgnoreList.Opcode)+int(enc.GetNext()))&0xff),
	)
	// UPDATE_IGNORELIST payload: 2-byte BE length prefix = 0 (empty bootstrap).
	want = append(want, 0x00, 0x00)
	want = append(want,
		byte((int(gameserver.OpUpdatePid.Opcode)+int(enc.GetNext()))&0xff),
	)
	// UPDATE_PID payload: 3 bytes (244): p2(slot) + pbool(members).
	// newTestServer has cfg.NodeMembers=false (zero-value); p.members=false → members=false → 0x00.
	want = append(want, 0x00, byte(p.pid), 0x00)
	want = append(want,
		byte((int(gameserver.OpResetClientVarCache.Opcode)+int(enc.GetNext()))&0xff),
	)
	// RESET_CLIENT_VARCACHE: 0-byte payload
	want = append(want,
		byte((int(gameserver.OpResetAnims.Opcode)+int(enc.GetNext()))&0xff),
	)
	// RESET_ANIMS: 0-byte payload

	if len(got) < len(want) {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), len(want))
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_FriendsEnabled_EmitsConnectingNoIgnoreBootstrap
// pins the friends-server-enabled login branch (TS Player.ts:496-498
// @43e02957): CHAT_FILTER_SETTINGS is followed by FRIENDLIST_LOADED status 1
// ("connecting to friendserver") and NO UPDATE_IGNORELIST bootstrap — the
// real list arrives later via the UPDATE_IGNORELIST relay. The exact stream
// length pins the absence of the empty ignore-list packet.
func TestProcessLogins_FreshLogin_FriendsEnabled_EmitsConnectingNoIgnoreBootstrap(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	s.friendsClient = newFakeFriendsClient()
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// p.username stays "" so the PlayerLogin RPC + SubscribeUpdates
	// goroutines (gated on username != "") stay quiet; the FRIENDLIST_LOADED
	// branch is gated on s.friendsClient alone (TS Environment.FRIEND_SERVER
	// is a static config flag, not a per-player condition).

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00, 0x00,
	}
	want = append(want,
		byte((int(gameserver.OpFriendlistLoaded.Opcode)+int(enc.GetNext()))&0xff),
		0x01, // status 1 = connecting to friendserver
	)
	want = append(want,
		byte((int(gameserver.OpUpdatePid.Opcode)+int(enc.GetNext()))&0xff),
		0x00, byte(p.pid), 0x00,
	)
	want = append(want,
		byte((int(gameserver.OpResetClientVarCache.Opcode)+int(enc.GetNext()))&0xff),
	)
	want = append(want,
		byte((int(gameserver.OpResetAnims.Opcode)+int(enc.GetNext()))&0xff),
	)

	// Exact-length pin: CFS=4 + FRIENDLIST_LOADED=2 + UPDATE_PID=4 + RCV=1 +
	// RA=1 = 12 bytes. An UPDATE_IGNORELIST bootstrap would add 3 more.
	if len(got) != len(want) {
		t.Errorf("stream length: got %d, want %d (no UPDATE_IGNORELIST bootstrap)", len(got), len(want))
	}
	for i, b := range want {
		if i >= len(got) {
			break
		}
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer verifies
// that UPDATE_REBOOT_TIMER is emitted after the 5 standard fresh-login
// packets when s.shutdownTick != -1. NAI-182 B3.
func TestProcessLogins_FreshLogin_WithShutdownPending_EmitsRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	s.shutdownTick = s.currentTick + 25

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Consume the first 5 packets: CHAT_FILTER_SETTINGS (1+3 bytes),
	// FRIENDLIST_LOADED (1+1 bytes), UPDATE_IGNORELIST (1+2 bytes, empty
	// bootstrap; friendsClient nil), UPDATE_PID (1+3 bytes, 244: p2+pbool),
	// RESET_CLIENT_VARCACHE (1 byte), RESET_ANIMS (1 byte) = 15 bytes total.
	enc.GetNext() // CHAT_FILTER_SETTINGS opcode key
	enc.GetNext() // FRIENDLIST_LOADED opcode key
	enc.GetNext() // UPDATE_IGNORELIST opcode key
	enc.GetNext() // UPDATE_PID opcode key
	enc.GetNext() // RESET_CLIENT_VARCACHE opcode key
	enc.GetNext() // RESET_ANIMS opcode key
	offset := 4 + 2 + 3 + 4 + 1 + 1

	// Next packet: UPDATE_REBOOT_TIMER opcode + 2-byte payload (25 == 0x0019).
	wantOpcode := byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff)
	wantPayload := []byte{0x00, 0x19}

	if len(got) < offset+3 {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), offset+3)
	}
	if got[offset] != wantOpcode {
		t.Errorf("UPDATE_REBOOT_TIMER opcode byte: got 0x%02x, want 0x%02x", got[offset], wantOpcode)
	}
	if got[offset+1] != wantPayload[0] || got[offset+2] != wantPayload[1] {
		t.Errorf("UPDATE_REBOOT_TIMER payload: got [0x%02x 0x%02x], want [0x%02x 0x%02x]",
			got[offset+1], got[offset+2], wantPayload[0], wantPayload[1])
	}
}

// TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer asserts that no
// UPDATE_REBOOT_TIMER opcode appears in the stream when shutdownTick == -1.
// NAI-182 B3.
func TestProcessLogins_FreshLogin_NoShutdown_NoRebootTimer(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	// shutdownTick defaults to -1 in newTestServer; leave it unchanged.

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received

	// Compute the expected UPDATE_REBOOT_TIMER encrypted opcode if it were
	// sent. We consume 6 ISAAC values for the 6 standard packets first.
	enc.GetNext() // CHAT_FILTER_SETTINGS
	enc.GetNext() // FRIENDLIST_LOADED
	enc.GetNext() // UPDATE_IGNORELIST
	enc.GetNext() // UPDATE_PID
	enc.GetNext() // RESET_CLIENT_VARCACHE
	enc.GetNext() // RESET_ANIMS
	forbiddenOpcode := byte((int(gameserver.OpUpdateRebootTimer.Opcode) + int(enc.GetNext())) & 0xff)

	// The stream should be exactly 15 bytes (CFS=4, FRIENDLIST_LOADED=2,
	// UPDATE_IGNORELIST=3 (empty bootstrap), UPDATE_PID=4 (244: p2+pbool),
	// RCV=1, RA=1). Anything beyond that, including a byte matching the
	// reboot opcode, is a bug.
	const wantLen = 15
	if len(got) != wantLen {
		t.Errorf("stream length: got %d, want %d (no reboot timer packet)", len(got), wantLen)
	}
	// Additionally verify no byte in the observed stream matches the forbidden opcode.
	for i, b := range got {
		if b == forbiddenOpcode {
			t.Errorf("byte[%d] matches UPDATE_REBOOT_TIMER encrypted opcode 0x%02x — should not be present",
				i, forbiddenOpcode)
		}
	}
}

// TestProcessLogins_FreshLogin_EmitsChatFilterSettingsFirst pins that
// CHAT_FILTER_SETTINGS is the first packet on a fresh-login wire,
// carrying the player's publicChat/privateChat/tradeDuel triple.
// NAI-182-D5 T6.
func TestProcessLogins_FreshLogin_EmitsChatFilterSettingsFirst(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Direct field writes pre-processLogins are clobbered by fresh-init
	// for skills/invs/varps. publicChat/privateChat/tradeDuel are NOT
	// reset by initPlayerVarps; setting here is safe.
	p.publicChat = 1
	p.privateChat = 2
	p.tradeDuel = 0

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x00,
	}
	if len(got) < len(want) {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), len(want))
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_ChatFilterDefaults pins that
// publicChat/privateChat/tradeDuel default to 0 emit `00 00 00` on the wire.
// NAI-182-D5 T6.
func TestProcessLogins_FreshLogin_ChatFilterDefaults(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00, 0x00,
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}

// TestProcessLogins_FreshLogin_ChatFilterEmitReflectsSAV pins that the
// fresh-login CHAT_FILTER_SETTINGS emit reflects publicChat/privateChat/
// tradeDuel restored from the SAV payload via LoadSave (player_save.go:104
// pack + player_load.go:225-229 unpack, v4+), not the post-construct zero
// values. processLogins must call LoadSave BEFORE sendChatFilterSettings.
//
// Retires DEVIATION-NAI-182-D5-CHAT-FILTER-NO-RESTORE (the marker was
// authored against the wire-emit ordering before the author noticed that
// NAI-PLAYERLOADING had already wired SAV round-trip for these fields).
func TestProcessLogins_FreshLogin_ChatFilterEmitReflectsSAV(t *testing.T) {
	seed, invTypes := newTestPlayerForLoadSave(t)
	seed.publicChat = 2
	seed.privateChat = 1
	seed.tradeDuel = 3
	sav := seed.Save(invTypes, nil)

	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	s.invTypes = invTypes
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})
	p.client.savePayload = sav

	s.playersMu.Lock()
	s.newPlayers = append(s.newPlayers, p)
	s.playersMu.Unlock()

	received := drainConn(t, cc)
	s.processLogins()
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x02, 0x01, 0x03,
	}
	if len(got) < len(want) {
		t.Fatalf("wire too short: got %d bytes, want at least %d", len(got), len(want))
	}
	for i, b := range want {
		if got[i] != b {
			t.Errorf("byte[%d]: got 0x%02x, want 0x%02x", i, got[i], b)
		}
	}
}
