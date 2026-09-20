package packetcapture

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/tapper"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PacketRecord is the input to EncodePacket.
type PacketRecord struct {
	WorldID   int32
	Revision  uint16
	Profile   string
	AccountID int64
	SessionID string // UUID 36-char string
	Direction tapper.Direction
	Opcode    uint8
	Payload   []byte
	TS        time.Time
}

// SessionRecord is the input to EncodeSessionStarted / EncodeSessionEnded.
type SessionRecord struct {
	WorldID        int32
	Revision       uint16
	Profile        string
	AccountID      int64
	SessionID      string
	StartedAt      time.Time
	EndedAt        time.Time // zero for STARTED
	CloseReason    string    // empty for STARTED
	PacketCountIn  uint64
	PacketCountOut uint64
	ByteCountIn    uint64
	ByteCountOut   uint64
	TS             time.Time // envelope ts
}

// KafkaTopic is the destination topic for all replay envelopes.
const KafkaTopic = "events.replay_packets"

// schemaVersion is bumped whenever the envelope shape changes.
const schemaVersion = 1

// EncodePacket builds a kgo.Record carrying a PacketEvent envelope.
func EncodePacket(rec PacketRecord) (*kgo.Record, error) {
	if rec.SessionID == "" {
		return nil, fmt.Errorf("EncodePacket: SessionID empty")
	}
	env := &eventspb.ReplayEnvelope{
		SchemaVersion: schemaVersion,
		EventId:       uuid.NewString(),
		Ts:            timestamppb.New(rec.TS),
		WorldId:       rec.WorldID,
		Revision:      uint32(rec.Revision),
		Profile:       rec.Profile,
		Payload: &eventspb.ReplayEnvelope_Packet{
			Packet: &eventspb.PacketEvent{
				AccountId: rec.AccountID,
				SessionId: rec.SessionID,
				Dir:       uint32(rec.Direction),
				Opcode:    uint32(rec.Opcode),
				Payload:   rec.Payload,
			},
		},
	}
	return marshal(env, rec.AccountID)
}

// EncodeSessionStarted builds a kgo.Record for a STARTED session lifecycle.
func EncodeSessionStarted(rec SessionRecord) (*kgo.Record, error) {
	return encodeSession(rec, eventspb.SessionEvent_KIND_STARTED)
}

// EncodeSessionEnded builds a kgo.Record for an ENDED session lifecycle.
func EncodeSessionEnded(rec SessionRecord) (*kgo.Record, error) {
	return encodeSession(rec, eventspb.SessionEvent_KIND_ENDED)
}

func encodeSession(rec SessionRecord, kind eventspb.SessionEvent_Kind) (*kgo.Record, error) {
	if rec.SessionID == "" {
		return nil, fmt.Errorf("encodeSession: SessionID empty")
	}
	se := &eventspb.SessionEvent{
		AccountId:      rec.AccountID,
		SessionId:      rec.SessionID,
		Kind:           kind,
		CloseReason:    rec.CloseReason,
		PacketCountIn:  rec.PacketCountIn,
		PacketCountOut: rec.PacketCountOut,
		ByteCountIn:    rec.ByteCountIn,
		ByteCountOut:   rec.ByteCountOut,
		StartedAt:      timestamppb.New(rec.StartedAt),
	}
	if kind == eventspb.SessionEvent_KIND_ENDED {
		se.EndedAt = timestamppb.New(rec.EndedAt)
	}
	env := &eventspb.ReplayEnvelope{
		SchemaVersion: schemaVersion,
		EventId:       uuid.NewString(),
		Ts:            timestamppb.New(rec.TS),
		WorldId:       rec.WorldID,
		Revision:      uint32(rec.Revision),
		Profile:       rec.Profile,
		Payload:       &eventspb.ReplayEnvelope_Session{Session: se},
	}
	return marshal(env, rec.AccountID)
}

// marshal protobuf-encodes the envelope into a kgo.Record with account_id as
// the partition key, so every record of one account lands on one partition and
// stays in order.
func marshal(env *eventspb.ReplayEnvelope, accountID int64) (*kgo.Record, error) {
	b, err := proto.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal: %w", err)
	}
	return &kgo.Record{
		Topic: KafkaTopic,
		Key:   []byte(fmt.Sprintf("%d", accountID)),
		Value: b,
	}, nil
}
