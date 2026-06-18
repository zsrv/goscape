package eventspb_test

import (
	"testing"
	"time"

	"github.com/zsrv/goscape/pkg/eventspb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAuthEnvelopeRoundTrip(t *testing.T) {
	original := &eventspb.AuthEnvelope{
		SchemaVersion: 1,
		EventId:       "test-id",
		Ts:            timestamppb.New(time.Unix(1700000000, 0)),
		AccountId:     42,
		WorldId:       1,
		Payload: &eventspb.AuthEnvelope_Login{
			Login: &eventspb.LoginEvent{
				Ip:  "127.0.0.1",
				Uid: 99,
			},
		},
	}
	buf, err := proto.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &eventspb.AuthEnvelope{}
	if err := proto.Unmarshal(buf, decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.AccountId != 42 {
		t.Errorf("AccountId = %d, want 42", decoded.AccountId)
	}
	if got := decoded.GetLogin().GetIp(); got != "127.0.0.1" {
		t.Errorf("Ip = %q, want %q", got, "127.0.0.1")
	}
	if got := decoded.GetLogin().GetUid(); got != 99 {
		t.Errorf("Uid = %d, want 99", got)
	}
}
