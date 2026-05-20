package world

import (
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// TestSendUpdateFriendList_EmitsExactByteSequence pins the wire bytes of
// sendUpdateFriendList. Fixed 9-byte payload: p8(username37) + p1(worldId).
func TestSendUpdateFriendList_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateFriendList(p, 0x0102030405060708, 7)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateFriendList.Opcode) + int(enc.GetNext())) & 0xff),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x07,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendUpdateIgnoreList_EmitsExactByteSequence pins UPDATE_IGNORELIST
// with a 2-entry snapshot. Variable 2-byte-length-prefixed payload.
func TestSendUpdateIgnoreList_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateIgnoreList(p, []uint64{0x0102030405060708, 0xAABBCCDDEEFF0011})
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x10, // 2-byte BE length = 16 bytes (2 entries × 8)
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x00, 0x11,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendUpdateIgnoreList_EmptyListEmitsZeroLengthPayload pins the
// no-entries case: opcode + `00 00` length prefix + zero payload.
// Mirrors TS `player.write(new UpdateIgnoreList([]))`.
func TestSendUpdateIgnoreList_EmptyListEmitsZeroLengthPayload(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendUpdateIgnoreList(p, nil)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpUpdateIgnoreList.Opcode) + int(enc.GetNext())) & 0xff),
		0x00, 0x00,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendChatFilterSettings_EmitsExactByteSequence pins CHAT_FILTER_SETTINGS.
// Fixed 3-byte payload: p1(public) + p1(private) + p1(trade).
func TestSendChatFilterSettings_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendChatFilterSettings(p, 2, 1, 3)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpChatFilterSettings.Opcode) + int(enc.GetNext())) & 0xff),
		0x02, 0x01, 0x03,
	}
	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendMessagePrivate_EmitsExactByteSequence pins MESSAGE_PRIVATE.
// Variable 1-byte-length-prefixed payload:
//
//	p8(from) + p4(pmId) + p1(staffLvl) + WordPack.pack(chat).
//
// staffLvl=0 ⇒ wire byte 00.
func TestSendMessagePrivate_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Compute the wordpacked bytes for "hi" exactly the way the encoder will.
	wpBuf := packet.NewPacket(nil)
	wordpack.Pack(wpBuf, "hi")
	wpBytes := wpBuf.Bytes()

	received := drainConn(t, cc)
	sendMessagePrivate(p, 0x0102030405060708, 0xDEADBEEF, 0, "hi")
	p.client.flushWrite()

	got := <-received
	header := []byte{
		byte((int(gameserver.OpMessagePrivate.Opcode) + int(enc.GetNext())) & 0xff),
		// 1-byte length prefix: 8 (from) + 4 (pmId) + 1 (staffLvl) + len(wpBytes).
		byte(8 + 4 + 1 + len(wpBytes)),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0xDE, 0xAD, 0xBE, 0xEF,
		0x00, // staffLvl=0, no adjustment
	}
	want := append(header, wpBytes...)

	if string(got) != string(want) {
		t.Fatalf("wire bytes: got % x, want % x", got, want)
	}
}

// TestSendMessagePrivate_StaffLvlAdjustmentPositive pins the TS-faithful
// `staffLvl > 0 ⇒ +1` adjustment. staffLvl=2 ⇒ wire byte 03.
func TestSendMessagePrivate_StaffLvlAdjustmentPositive(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, _ = isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendMessagePrivate(p, 1, 2, 2, "x")
	p.client.flushWrite()

	got := <-received
	// got = [encrypted-opcode, length, 8-byte-from, 4-byte-pmId, staffLvlByte, wpBytes...].
	// Offset 1+1+8+4 = 14 is the staffLvl byte.
	if got[14] != 0x03 {
		t.Fatalf("staffLvl byte: got 0x%02x, want 0x03 (2 + 1 adjustment)", got[14])
	}
}

// TestSendMessagePrivate_StaffLvlAdjustmentNegative pins that the
// adjustment ONLY applies when staffLvl > 0. staffLvl=-1 ⇒ wire 0xFF.
func TestSendMessagePrivate_StaffLvlAdjustmentNegative(t *testing.T) {
	p, cc := newTestPlayer(t)
	_, _ = isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendMessagePrivate(p, 1, 2, -1, "x")
	p.client.flushWrite()

	got := <-received
	if got[14] != 0xFF {
		t.Fatalf("staffLvl byte: got 0x%02x, want 0xFF (-1, no adjustment)", got[14])
	}
}
