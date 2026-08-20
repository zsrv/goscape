package hiscore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCursor_RoundTrip(t *testing.T) {
	want := Cursor{
		ValueX10:  130_344_310,
		UpdatedAt: time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC),
		AccountID: 4242,
		Rank:      101,
	}
	got, err := DecodeCursor(want.Encode())
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if got.ValueX10 != want.ValueX10 || got.AccountID != want.AccountID || got.Rank != want.Rank {
		t.Errorf("round trip: got %+v, want %+v", got, want)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func TestDecodeCursor_Rejects(t *testing.T) {
	tests := []string{
		"",                 // empty
		"!!!not-base64!!!", // bad encoding
		"YWJjZGVm",         // valid base64, not a cursor
	}
	for _, in := range tests {
		if _, err := DecodeCursor(in); !errors.Is(err, ErrBadCursor) {
			t.Errorf("DecodeCursor(%q): err = %v, want ErrBadCursor", in, err)
		}
	}
}

// TestDecodeCursor_RejectsUnknownField proves DisallowUnknownFields is
// actually wired up: an otherwise well-formed cursor carrying one extra
// field must be rejected, not silently accepted with the extra field
// dropped.
func TestDecodeCursor_RejectsUnknownField(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"v": 130_344_310,
		"d": time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC),
		"a": 4242,
		"r": 101,
		"x": "unexpected field",
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	if _, err := DecodeCursor(encoded); !errors.Is(err, ErrBadCursor) {
		t.Errorf("DecodeCursor with unknown field: err = %v, want ErrBadCursor", err)
	}
}

func TestCursor_IsStart(t *testing.T) {
	if !(Cursor{}).IsStart() {
		t.Error("zero Cursor must report IsStart")
	}
	if (Cursor{Rank: 1}).IsStart() {
		t.Error("Cursor with a real rank must not report IsStart")
	}
}
