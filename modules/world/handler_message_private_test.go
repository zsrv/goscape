package world

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/io/packet"
	util "github.com/zsrv/goscape/pkg/util/jstring"
	"github.com/zsrv/goscape/pkg/wordenc/wordpack"
)

// commonMessagePrivateSetup wires a player against a server with
// recording bridges and a known username. Mirrors commonSocialListSetup
// in handler_social_list_test.go.
func commonMessagePrivateSetup(t *testing.T) (*Player, *recordingBridges) {
	t.Helper()
	s := newTestServer(t)
	p, _ := newTestPlayer(t)
	p.client.server = s
	p.username = "alice"
	rec := installRecordingBridges(s)
	return p, rec
}

// packMessagePayload returns an opcode-148 payload: 8 bytes target
// (big-endian, matching the encoding read by Packet.G8 — see
// payloadG8 in handler_social_list_test.go) followed by word-packed
// message bytes.
func packMessagePayload(target uint64, message string) []byte {
	out := payloadG8(target)
	pk := packet.NewPacket(nil)
	wordpack.Pack(pk, message)
	return append(out, pk.Data...)
}

// TestHandleMessagePrivateHappyPath: bridge receives the message,
// socialProtect flips true, pmCount advances.
func TestHandleMessagePrivateHappyPath(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 1 {
		t.Fatalf("privateMsgs: got %d, want 1", len(rec.privateMsgs))
	}
	got := rec.privateMsgs[0]
	if got.playerUsername != "alice" {
		t.Errorf("playerUsername: got %q, want alice", got.playerUsername)
	}
	if got.target != target {
		t.Errorf("target: got %d, want %d", got.target, target)
	}
	if got.message != "Hi" { // Unpack applies sentence-case to "hi"
		t.Errorf("message: got %q, want %q", got.message, "Hi")
	}
	if got.staffLvl != p.staffModLevel {
		t.Errorf("staffLvl: got %d, want %d", got.staffLvl, p.staffModLevel)
	}
	// pmCount starts at 1 (newTestServer inits pmCount=1 per TS World.ts:167).
	// nextPmId bakes counter=1 into pmId bits 0-15, then increments pmCount
	// to 2. cfg.NodeID is 0 (default from newTestServer). Random byte
	// (bits 16-23) masked out. Expected: NodeID=0, counter=1 → 0x00000001.
	if got.pmId&0xff00ffff != 1 {
		t.Errorf("pmId structure: got %08x masked %08x, want 0x00000001 (NodeID=0, counter=1 pre-increment)",
			got.pmId, got.pmId&0xff00ffff)
	}
	if !p.socialProtect {
		t.Error("socialProtect: must be true after successful PrivateMessage")
	}
	if got := p.client.server.pmCount.Load(); got != 2 {
		t.Errorf("pmCount: got %d, want 2 (started at 1, advanced to 2)", got)
	}
}

// TestHandleMessagePrivateGatedBySocialProtect: early-return when
// p.socialProtect is already set; no bridge call, no protect-set.
func TestHandleMessagePrivateGatedBySocialProtect(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	p.socialProtect = true
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (gated)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (no ban expected)", len(rec.loginMod))
	}
	// pmCount unchanged: newTestServer inits to 1, gate fires before nextPmId.
	if got := p.client.server.pmCount.Load(); got != 1 {
		t.Errorf("pmCount: got %d, want 1 (gate fires before nextPmId)", got)
	}
}

// TestHandleMessagePrivateGatedByLengthCap: payload with > 100 bytes of
// word-packed input is dropped.
func TestHandleMessagePrivateGatedByLengthCap(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	target := util.ToBase37("bob")

	// 101 bytes of word-packed tail after the 8-byte target.
	payload := payloadG8(target)
	tail := make([]byte, 101)
	payload = append(payload, tail...)

	if err := handleMessagePrivate(p, payload); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (length>100 gated)", len(rec.privateMsgs))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on length-gated branch")
	}
	if got := p.client.server.pmCount.Load(); got != 1 {
		t.Errorf("pmCount: got %d, want 1 (init=1, gate before nextPmId)", got)
	}
}

// TestHandleMessagePrivateGatedByMutedUntil: mute window active → drop.
func TestHandleMessagePrivateGatedByMutedUntil(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	p.mutedUntil = time.Now().Add(time.Hour)
	target := util.ToBase37("bob")

	if err := handleMessagePrivate(p, packMessagePayload(target, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (muted)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 0 {
		t.Errorf("loginMod: got %d, want 0 (no ban on mute branch)", len(rec.loginMod))
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on mute branch")
	}
}

// TestHandleMessagePrivateInvalidNameTriggersBan: invalid_name base37
// decode → 48h automated ban; no friends bridge call; no protect-set.
// The sentinel value comes from pkg/util/jstring/jstring_test.go line 7.
func TestHandleMessagePrivateInvalidNameTriggersBan(t *testing.T) {
	p, rec := commonMessagePrivateSetup(t)
	const invalidNameSentinel uint64 = 6582952005840035281

	before := time.Now()
	if err := handleMessagePrivate(p, packMessagePayload(invalidNameSentinel, "hi")); err != nil {
		t.Fatalf("handleMessagePrivate: %v", err)
	}
	after := time.Now()

	if len(rec.privateMsgs) != 0 {
		t.Errorf("privateMsgs: got %d, want 0 (banned)", len(rec.privateMsgs))
	}
	if len(rec.loginMod) != 1 {
		t.Fatalf("loginMod: got %d, want 1", len(rec.loginMod))
	}
	mod := rec.loginMod[0]
	if mod.method != "NotifyPlayerBan" {
		t.Errorf("method: got %q, want NotifyPlayerBan", mod.method)
	}
	if mod.staff != "automated" {
		t.Errorf("staff: got %q, want automated", mod.staff)
	}
	if mod.username != "alice" {
		t.Errorf("username: got %q, want alice", mod.username)
	}
	// Until must be ~48h from now (allow ±5s window for test latency).
	lo := before.Add(48*time.Hour - 5*time.Second)
	hi := after.Add(48*time.Hour + 5*time.Second)
	if mod.until.Before(lo) || mod.until.After(hi) {
		t.Errorf("until: got %v, want within %v..%v", mod.until, lo, hi)
	}
	if p.socialProtect {
		t.Error("socialProtect: must remain false on ban branch")
	}
	if got := p.client.server.pmCount.Load(); got != 1 {
		t.Errorf("pmCount: got %d, want 1 (init=1, gate before nextPmId)", got)
	}
}
