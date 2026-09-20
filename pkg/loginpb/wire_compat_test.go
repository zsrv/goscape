package loginpb_test

import (
	"encoding/hex"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// revisionTag is the PlayerLoginRequest.revision field number. It is wire
// contract ACROSS revision branches, not just within one: the login service
// and its central DB are revision-agnostic, so a world of one revision may
// call a login server built from another, and the two only agree about this
// field if every goscape revision branch spells it with the SAME tag. Pick a
// different number on a branch and that branch's worlds silently report no
// revision at all (the value lands in unknown fields).
const revisionTag protoreflect.FieldNumber = 11

// TestPlayerLoginRequestRevisionField pins the new field's number, name and
// kind against a per-branch renumbering.
func TestPlayerLoginRequestRevisionField(t *testing.T) {
	fields := (&loginpb.PlayerLoginRequest{}).ProtoReflect().Descriptor().Fields()

	byNumber := fields.ByNumber(revisionTag)
	if byNumber == nil {
		t.Fatalf("PlayerLoginRequest has no field %d; revision must keep this tag on every revision branch", revisionTag)
	}
	if byNumber.Name() != "revision" {
		t.Errorf("field %d = %q, want %q", revisionTag, byNumber.Name(), "revision")
	}
	if byNumber.Kind() != protoreflect.Uint32Kind {
		t.Errorf("field %d kind = %v, want %v", revisionTag, byNumber.Kind(), protoreflect.Uint32Kind)
	}

	byName := fields.ByName("revision")
	if byName == nil {
		t.Fatal("PlayerLoginRequest has no field named revision")
	}
	if byName.Number() != revisionTag {
		t.Errorf("revision tag = %d, want %d", byName.Number(), revisionTag)
	}
}

// goldenPreRevision is the wire encoding a fully-populated
// PlayerLoginRequest produced BEFORE the revision field existed, captured
// from the pristine branch. Adding a field must not change the encoding of a
// message that leaves it unset — that is what lets a world build without the
// field keep talking to an upgraded login server, and vice versa.
const goldenPreRevision = "08071204626574611801220a676f6c64656e757365722a0768756e74657232302a42113139322e3136382e312e313a313233343548015001"

func TestPlayerLoginRequestWireCompatible(t *testing.T) {
	msg := &loginpb.PlayerLoginRequest{
		NodeId:        7,
		Profile:       "beta",
		NodeMembers:   true,
		Username:      "goldenuser",
		Password:      "hunter2",
		Uid:           42,
		RemoteAddress: "192.168.1.1:12345",
		Reconnecting:  true,
		HasSave:       true,
		// Revision deliberately unset — this is the pre-upgrade encoding.
	}

	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := hex.EncodeToString(b); got != goldenPreRevision {
		t.Errorf("wire bytes with revision unset changed\n got: %s\nwant: %s", got, goldenPreRevision)
	}
}
