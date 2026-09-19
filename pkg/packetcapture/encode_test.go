package packetcapture

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/tapper"
	"google.golang.org/protobuf/proto"
)

func TestEncodePacketRoundtrips(t *testing.T) {
	ts := time.Date(2026, 5, 31, 12, 0, 0, 123_000_000, time.UTC)
	payload := []byte{0xAA, 0xBB, 0xCC}

	rec, err := EncodePacket(PacketRecord{
		WorldID:   1,
		Revision:  225,
		AccountID: 42,
		SessionID: "11111111-2222-3333-4444-555555555555",
		Direction: tapper.DirOut,
		Opcode:    181,
		Payload:   payload,
		TS:        ts,
	})
	if err != nil {
		t.Fatalf("EncodePacket: %v", err)
	}
	if rec.Topic == "" {
		t.Fatal("rec.Topic empty")
	}

	var env eventspb.ReplayEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.WorldId != 1 || env.Revision != 225 {
		t.Fatalf("envelope: world=%d rev=%d", env.WorldId, env.Revision)
	}
	pkt := env.GetPacket()
	if pkt == nil {
		t.Fatal("payload not packet")
	}
	if pkt.AccountId != 42 || pkt.Opcode != 181 || pkt.Dir != uint32(tapper.DirOut) ||
		string(pkt.Payload) != string(payload) ||
		pkt.SessionId != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("packet fields drifted: %+v", pkt)
	}
}

func TestEncodeSessionStartedRoundtrips(t *testing.T) {
	startedAt := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)

	rec, err := EncodeSessionStarted(SessionRecord{
		WorldID:   1,
		Revision:  225,
		AccountID: 99,
		SessionID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		StartedAt: startedAt,
		TS:        startedAt,
	})
	if err != nil {
		t.Fatalf("EncodeSessionStarted: %v", err)
	}

	var env eventspb.ReplayEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	s := env.GetSession()
	if s == nil {
		t.Fatal("payload not session")
	}
	if s.Kind != eventspb.SessionEvent_KIND_STARTED || s.AccountId != 99 {
		t.Fatalf("session.started drifted: %+v", s)
	}
}

func TestEncodeSessionEndedCarriesCounters(t *testing.T) {
	ts := time.Date(2026, 5, 31, 13, 0, 0, 0, time.UTC)
	rec, err := EncodeSessionEnded(SessionRecord{
		WorldID:        1,
		Revision:       225,
		AccountID:      99,
		SessionID:      "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		StartedAt:      ts.Add(-time.Hour),
		EndedAt:        ts,
		CloseReason:    tapper.CloseReasonLogout,
		PacketCountIn:  10,
		PacketCountOut: 20,
		ByteCountIn:    100,
		ByteCountOut:   200,
		TS:             ts,
	})
	if err != nil {
		t.Fatalf("EncodeSessionEnded: %v", err)
	}

	var env eventspb.ReplayEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	s := env.GetSession()
	if s.Kind != eventspb.SessionEvent_KIND_ENDED {
		t.Fatalf("Kind=%v want ENDED", s.Kind)
	}
	if s.CloseReason != tapper.CloseReasonLogout ||
		s.PacketCountIn != 10 || s.PacketCountOut != 20 ||
		s.ByteCountIn != 100 || s.ByteCountOut != 200 {
		t.Fatalf("ended session drift: %+v", s)
	}
}
