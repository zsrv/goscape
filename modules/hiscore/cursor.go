package hiscore

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrBadCursor marks a cursor that is absent, malformed, or not
// self-consistent. It always maps to 400, never to a panic or a 500.
var ErrBadCursor = errors.New("hiscore: malformed cursor")

// Cursor is a keyset position on a board: the sort key of the last row
// returned, plus the rank the next row will carry.
//
// Cursors are deliberately unsigned. No privilege attaches to one, and
// the query clamps to a single (profile, type) board, so a forged cursor
// can only produce a wrong page for whoever sent it.
type Cursor struct {
	ValueX10  int64     `json:"v"`
	UpdatedAt time.Time `json:"d"`
	AccountID int64     `json:"a"`
	// Rank is the rank of the NEXT row to return. Because the ordering
	// is total, carrying it forward yields true absolute ranks rather
	// than position-within-page.
	Rank int64 `json:"r"`
}

// IsStart reports whether this is the implicit start-of-board position.
// Ranks are 1-based, so a zero Rank can only mean "no cursor supplied".
func (c Cursor) IsStart() bool { return c.Rank == 0 }

// Encode renders the cursor as an opaque base64url token.
func (c Cursor) Encode() string {
	b, err := json.Marshal(c)
	if err != nil {
		// Cursor holds only fixed scalar types; Marshal cannot fail.
		panic(fmt.Sprintf("hiscore: encoding cursor: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses a token produced by Encode.
func DecodeCursor(s string) (Cursor, error) {
	if s == "" {
		return Cursor{}, ErrBadCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, ErrBadCursor
	}
	var c Cursor
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Cursor{}, ErrBadCursor
	}
	// A decoded cursor must name a real position: ranks are 1-based, so
	// rank 0 here means the token was not produced by Encode.
	if c.Rank < 1 || c.AccountID < 1 {
		return Cursor{}, ErrBadCursor
	}
	return c, nil
}
