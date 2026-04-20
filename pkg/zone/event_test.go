package zone

import "testing"

func TestZoneEventTypeValues(t *testing.T) {
	if ZoneEventEnclosed != 0 {
		t.Errorf("ZoneEventEnclosed: got %d, want 0", ZoneEventEnclosed)
	}
	if ZoneEventFollows != 1 {
		t.Errorf("ZoneEventFollows: got %d, want 1", ZoneEventFollows)
	}
}

func TestPublicReceiverSentinel(t *testing.T) {
	if PublicReceiver != -1 {
		t.Errorf("PublicReceiver: got %d, want -1", PublicReceiver)
	}
}
