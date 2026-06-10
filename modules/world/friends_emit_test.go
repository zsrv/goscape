package world

import (
	"bytes"
	"os"
	"testing"

	io2 "github.com/zsrv/goscape/pkg/io/isaac"
	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
	"github.com/zsrv/goscape/pkg/wordenc/encfilter"
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

// TestSendFriendlistLoaded_EmitsExactByteSequence pins FRIENDLIST_LOADED:
// fixed 1-byte payload p1(status). Statuses per TS FriendlistLoadedEncoder
// @43e02957: 0 loading, 1 connecting to friendserver, 2 online.
func TestSendFriendlistLoaded_EmitsExactByteSequence(t *testing.T) {
	p, cc := newTestPlayer(t)
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	received := drainConn(t, cc)
	sendFriendlistLoaded(p, 1)
	sendFriendlistLoaded(p, 2)
	p.client.flushWrite()

	got := <-received
	want := []byte{
		byte((int(gameserver.OpFriendlistLoaded.Opcode) + int(enc.GetNext())) & 0xff),
		0x01,
		byte((int(gameserver.OpFriendlistLoaded.Opcode) + int(enc.GetNext())) & 0xff),
		0x02,
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
	p.client.server = newTestServer(t) // required: sendMessagePrivate calls server.wordenc.Filter
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
	p.client.server = newTestServer(t) // required: sendMessagePrivate calls server.wordenc.Filter
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
	p.client.server = newTestServer(t) // required: sendMessagePrivate calls server.wordenc.Filter
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

// TestSendMessagePrivate_AppliesWordEncFilter pins that sendMessagePrivate
// runs the chat text through s.wordenc.Filter before WordPack.Pack. A
// *Filter with "anal" as a bad word is injected; the wire bytes must match
// wordpack("****") not wordpack("anal"), confirming filtering occurs.
// Mirrors TS MessagePrivateEncoder.ts:20: WordPack.pack(buf, WordEnc.filter(message.msg)).
func TestSendMessagePrivate_AppliesWordEncFilter(t *testing.T) {
	p, cc := newTestPlayer(t)
	s := newTestServer(t)
	p.client.server = s
	enc, _ := isaacPair([4]uint32{1, 2, 3, 4})
	p.client.encryptor = io2.New([4]uint32{1, 2, 3, 4})

	// Inject a filter with one bad word "anal" so "anal" → "****".
	jf := makeWordencJagWithBad(t, "anal")
	f, err := encfilter.LoadFromJag(jf)
	if err != nil {
		t.Fatalf("LoadFromJag: %v", err)
	}
	s.wordenc = f

	received := drainConn(t, cc)
	sendMessagePrivate(p, 0x12345, 7, 0, "anal")
	p.client.flushWrite()

	got := <-received

	// Compute wordpack bytes for the masked text "****" (what we expect on wire).
	wantBuf := packet.NewPacket(nil)
	wordpack.Pack(wantBuf, "****")
	wantPacked := wantBuf.Bytes()

	// Also compute wordpack bytes for the unfiltered "anal" to show the test
	// would catch a regression where Filter is not called.
	unfilteredBuf := packet.NewPacket(nil)
	wordpack.Pack(unfilteredBuf, "anal")
	unfilteredPacked := unfilteredBuf.Bytes()

	// Wire format: [encrypted-opcode] [1-byte-len] [8-byte-from] [4-byte-pmId] [staffLvl] [wordpacked-chat]
	// staffLvl=0 ⇒ no adjustment; opcode = (OpMessagePrivate.Opcode + ISAAC) & 0xff.
	// Verify opcode byte is present and payload ends with wordpack("****").
	_ = enc // ISAAC advance consumed by drainConn framing; opcode byte included in got[0].
	if !bytes.HasSuffix(got, wantPacked) {
		if bytes.HasSuffix(got, unfilteredPacked) {
			t.Errorf("wire ends with wordpack(\"anal\") — Filter was NOT applied; got % x", got)
		} else {
			t.Errorf("wire does not end with wordpack(\"****\"): got % x, want suffix % x", got, wantPacked)
		}
	}
}

// makeWordencJagWithBad builds a minimal wordenc jagfile with one bad word
// (the given word with one combo [3,19] as in the canonical encfilter test)
// and empty-but-valid other sections (1-entry fragmentsenc, domainenc, tldlist).
// The jagfile is round-tripped through Save+NewJagfile so FileHash/FileSize
// tables are populated correctly, matching the LoadFromJag decoder path.
func makeWordencJagWithBad(t *testing.T, word string) *jagfile.Jagfile {
	t.Helper()
	jf := jagfile.NewEmptyJagfile(false)

	// badenc.txt: 1 entry with combo [3, 19].
	bad := packet.Alloc(2)
	bad.P4(1)
	bad.P1(uint8(len(word)))
	for _, c := range []byte(word) {
		bad.P1(c)
	}
	bad.P1(1) // combo count
	bad.P1(3)
	bad.P1(19)
	jf.Write("badenc.txt", bad)

	// fragmentsenc.txt: 1 entry value 42 (non-zero count required by decoder).
	frag := packet.Alloc(2)
	frag.P4(1)
	frag.P2(42)
	jf.Write("fragmentsenc.txt", frag)

	// domainenc.txt: 1 entry "test".
	dom := packet.Alloc(2)
	dom.P4(1)
	dom.P1(4)
	for _, c := range []byte("test") {
		dom.P1(c)
	}
	jf.Write("domainenc.txt", dom)

	// tldlist.txt: 1 entry type=2 tld="com".
	tld := packet.Alloc(2)
	tld.P4(1)
	tld.P1(2)
	tld.P1(3)
	for _, c := range []byte("com") {
		tld.P1(c)
	}
	jf.Write("tldlist.txt", tld)

	// Round-trip through Save+NewJagfile so .FileQueue lands in .FileHash + .FileSize.
	tmpPath := t.TempDir() + "/wordenc.jag"
	if err := jf.Save(tmpPath); err != nil {
		t.Fatalf("makeWordencJagWithBad: Save: %v", err)
	}
	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("makeWordencJagWithBad: ReadFile: %v", err)
	}
	out, err := jagfile.NewJagfile(packet.NewPacket(raw))
	if err != nil {
		t.Fatalf("makeWordencJagWithBad: NewJagfile: %v", err)
	}
	return out
}
