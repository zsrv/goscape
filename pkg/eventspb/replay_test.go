package eventspb

import "testing"

// The capture topic is consumed by external pipelines keyed on the
// fully-qualified proto name; renaming the package or message breaks them.
func TestReplayEnvelopeFullName(t *testing.T) {
	got := string((&ReplayEnvelope{}).ProtoReflect().Descriptor().FullName())
	if got != "events.v1.ReplayEnvelope" {
		t.Fatalf("full name = %q, want events.v1.ReplayEnvelope", got)
	}
}
