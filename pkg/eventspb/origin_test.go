package eventspb_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/eventspb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// envelopeField pins one top-level (non-payload) envelope field.
type envelopeField struct {
	number protoreflect.FieldNumber
	name   protoreflect.Name
	kind   protoreflect.Kind
}

// TestEnvelopeTopLevelFields pins the number, name and kind of every
// envelope field below the payload oneof (tags 100+). These numbers are
// wire contract: renumbering or retyping one breaks every consumer, and
// dropping one silently turns into an unknown field.
func TestEnvelopeTopLevelFields(t *testing.T) {
	tests := []struct {
		name   string
		msg    proto.Message
		fields []envelopeField
	}{
		{
			name: "AuthEnvelope",
			msg:  &eventspb.AuthEnvelope{},
			fields: []envelopeField{
				{1, "schema_version", protoreflect.Uint32Kind},
				{2, "event_id", protoreflect.StringKind},
				{3, "ts", protoreflect.MessageKind},
				{4, "account_id", protoreflect.Int64Kind},
				{5, "world_id", protoreflect.Int32Kind},
				{6, "revision", protoreflect.Uint32Kind},
				{7, "profile", protoreflect.StringKind},
			},
		},
		{
			name: "PlayerInputEnvelope",
			msg:  &eventspb.PlayerInputEnvelope{},
			fields: []envelopeField{
				{1, "schema_version", protoreflect.Uint32Kind},
				{2, "event_id", protoreflect.StringKind},
				{3, "ts", protoreflect.MessageKind},
				{4, "account_id", protoreflect.Int64Kind},
				{5, "world_id", protoreflect.Int32Kind},
				{6, "revision", protoreflect.Uint32Kind},
				{7, "profile", protoreflect.StringKind},
			},
		},
		{
			name: "WealthEnvelope",
			msg:  &eventspb.WealthEnvelope{},
			fields: []envelopeField{
				{1, "schema_version", protoreflect.Uint32Kind},
				{2, "event_id", protoreflect.StringKind},
				{3, "ts", protoreflect.MessageKind},
				{4, "account_id", protoreflect.Int64Kind},
				{5, "world_id", protoreflect.Int32Kind},
				{6, "revision", protoreflect.Uint32Kind},
				{7, "profile", protoreflect.StringKind},
			},
		},
		{
			name: "WorldEnvelope",
			msg:  &eventspb.WorldEnvelope{},
			fields: []envelopeField{
				{1, "schema_version", protoreflect.Uint32Kind},
				{2, "event_id", protoreflect.StringKind},
				{3, "ts", protoreflect.MessageKind},
				{4, "world_id", protoreflect.Int32Kind},
				{5, "account_id", protoreflect.Int64Kind},
				{6, "revision", protoreflect.Uint32Kind},
				{7, "profile", protoreflect.StringKind},
			},
		},
		{
			// ReplayEnvelope already carried revision on tag 5, so it
			// gains only profile, on tag 6. Tag 7 stays unused here.
			name: "ReplayEnvelope",
			msg:  &eventspb.ReplayEnvelope{},
			fields: []envelopeField{
				{1, "schema_version", protoreflect.Uint32Kind},
				{2, "event_id", protoreflect.StringKind},
				{3, "ts", protoreflect.MessageKind},
				{4, "world_id", protoreflect.Int32Kind},
				{5, "revision", protoreflect.Uint32Kind},
				{6, "profile", protoreflect.StringKind},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc := tt.msg.ProtoReflect().Descriptor()
			fields := desc.Fields()

			var got []envelopeField
			for i := range fields.Len() {
				f := fields.Get(i)
				if f.Number() >= 100 {
					continue // payload oneof variant
				}
				got = append(got, envelopeField{f.Number(), f.Name(), f.Kind()})
			}

			if len(got) != len(tt.fields) {
				t.Fatalf("top-level field count = %d (%v), want %d (%v)", len(got), got, len(tt.fields), tt.fields)
			}
			for i, want := range tt.fields {
				if got[i] != want {
					t.Errorf("field[%d] = %+v, want %+v", i, got[i], want)
				}
			}
		})
	}
}

// TestReplayEnvelopeHasNoField7 guards the one asymmetry: ReplayEnvelope
// stamps its revision on tag 5, so tag 7 must stay free for future use
// rather than being filled in to match the other four envelopes.
func TestReplayEnvelopeHasNoField7(t *testing.T) {
	fields := (&eventspb.ReplayEnvelope{}).ProtoReflect().Descriptor().Fields()
	if f := fields.ByNumber(7); f != nil {
		t.Fatalf("ReplayEnvelope field 7 = %q, want unused", f.Name())
	}
}

// goldenPreOrigin holds the wire bytes each envelope produced before the
// origin fields existed, captured from rev-274. Adding fields must not
// change the encoding of a message that leaves them unset, which is what
// makes the change safe for in-flight records and older consumers.
var goldenPreOrigin = map[string]string{
	"auth":         "08011206652d617574681a060880e2cfaa06202a2807a2060e0a093132372e302e302e31d80163",
	"player_input": "08011204652d70691a060880e2cfaa06202a2807a2060408031004",
	"wealth":       "08011203652d771a060880e2cfaa06202a2807aa060508e3071001",
	"world":        "08011207652d776f726c641a060880e2cfaa062007282aa20606088019108019",
	"replay":       "08011208652d7265706c61791a060880e2cfaa062007289202a2060b082a1203732d3118012005",
}

func TestOriginFieldsAreWireCompatible(t *testing.T) {
	ts := timestamppb.New(time.Unix(1700000000, 0).UTC())
	msgs := map[string]proto.Message{
		"auth": &eventspb.AuthEnvelope{
			SchemaVersion: 1, EventId: "e-auth", Ts: ts, AccountId: 42, WorldId: 7,
			Payload: &eventspb.AuthEnvelope_Login{Login: &eventspb.LoginEvent{Ip: "127.0.0.1", Uid: 99}},
		},
		"player_input": &eventspb.PlayerInputEnvelope{
			SchemaVersion: 1, EventId: "e-pi", Ts: ts, AccountId: 42, WorldId: 7,
			Payload: &eventspb.PlayerInputEnvelope_MouseMove{MouseMove: &eventspb.MouseMoveEvent{X: 3, Y: 4}},
		},
		"wealth": &eventspb.WealthEnvelope{
			SchemaVersion: 1, EventId: "e-w", Ts: ts, AccountId: 42, WorldId: 7,
			Payload: &eventspb.WealthEnvelope_ItemDropped{ItemDropped: &eventspb.ItemDroppedEvent{ItemId: 995, Qty: 1}},
		},
		"world": &eventspb.WorldEnvelope{
			SchemaVersion: 1, EventId: "e-world", Ts: ts, WorldId: 7, AccountId: 42,
			Payload: &eventspb.WorldEnvelope_TilePosition{TilePosition: &eventspb.TilePositionEvent{X: 3200, Y: 3200}},
		},
		"replay": &eventspb.ReplayEnvelope{
			SchemaVersion: 1, EventId: "e-replay", Ts: ts, WorldId: 7, Revision: 274,
			Payload: &eventspb.ReplayEnvelope_Packet{Packet: &eventspb.PacketEvent{AccountId: 42, SessionId: "s-1", Dir: 1, Opcode: 5}},
		},
	}

	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := hex.EncodeToString(b); got != goldenPreOrigin[name] {
				t.Errorf("wire bytes with origin fields unset changed\n got: %s\nwant: %s", got, goldenPreOrigin[name])
			}
		})
	}
}
