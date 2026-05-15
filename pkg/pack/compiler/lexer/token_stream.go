package lexer

// TokenStream wraps a Lexer, pre-buffering all tokens (including hidden
// channels) into a slice and exposing antlr-CommonTokenStream-parity
// lookahead: LT(k) returns the k-th default-channel token relative to
// the current position; Consume advances past the current default-
// channel token AND any following hidden tokens.
//
// Mirror of antlr4ng's BufferedTokenStream + CommonTokenStream surface.
// NAI-204's parser consumes via LT(k) / Consume / Mark / Rewind.
type TokenStream struct {
	tokens []Token // pre-buffered (all channels, includes EOF)
	pos    int     // current raw index (0-based)
}

// NewTokenStream drains the Lexer into a token slice and constructs
// a TokenStream positioned at index 0.
func NewTokenStream(l *Lexer) *TokenStream {
	ts := &TokenStream{}
	for {
		t := l.NextToken()
		ts.tokens = append(ts.tokens, t)
		if t.Type == EOF {
			break
		}
	}
	return ts
}

// LT returns the k-th token relative to the current default-channel
// position. k must be non-zero. Positive k looks forward (1 = next
// default-channel token from pos); negative k looks backward over
// already-consumed default-channel tokens. Returns a pointer to the
// EOF token if k overshoots either end.
func (s *TokenStream) LT(k int) *Token {
	if k == 0 {
		return nil
	}
	if k > 0 {
		idx := s.pos
		idx = s.nextOnChannel(idx, ChannelDefault)
		for i := 1; i < k; i++ {
			if idx >= len(s.tokens) {
				return &s.tokens[len(s.tokens)-1]
			}
			idx = s.nextOnChannel(idx+1, ChannelDefault)
		}
		if idx >= len(s.tokens) {
			return &s.tokens[len(s.tokens)-1]
		}
		return &s.tokens[idx]
	}
	idx := s.pos - 1
	if idx < 0 {
		return &s.tokens[len(s.tokens)-1]
	}
	idx = s.prevOnChannel(idx, ChannelDefault)
	for i := -1; i > k; i-- {
		if idx < 0 {
			return &s.tokens[len(s.tokens)-1]
		}
		idx = s.prevOnChannel(idx-1, ChannelDefault)
	}
	if idx < 0 {
		return &s.tokens[len(s.tokens)-1]
	}
	return &s.tokens[idx]
}

// LA returns LT(k).Type as a shortcut.
func (s *TokenStream) LA(k int) TokenType {
	t := s.LT(k)
	if t == nil {
		return EOF
	}
	return t.Type
}

// Consume advances pos past the current default-channel token AND any
// following hidden-channel tokens.
func (s *TokenStream) Consume() {
	idx := s.nextOnChannel(s.pos, ChannelDefault)
	if idx >= len(s.tokens) {
		s.pos = len(s.tokens)
		return
	}
	s.pos = idx + 1
	s.pos = s.nextOnChannel(s.pos, ChannelDefault)
}

// Index returns the current raw index (counts every channel).
func (s *TokenStream) Index() int { return s.pos }

// Mark records the current position for a future Release/Rewind.
func (s *TokenStream) Mark() int { return s.pos }

// Release is a no-op; antlr's TokenStream uses Release to flush
// rewind state, but our buffer is fully retained for the stream's
// lifetime.
func (s *TokenStream) Release(int) {}

// Rewind seeks pos back to the supplied mark.
func (s *TokenStream) Rewind(m int) { s.pos = m }

// nextOnChannel returns the first index >= start whose token is on the
// requested channel. Returns len(s.tokens) if none.
func (s *TokenStream) nextOnChannel(start, channel int) int {
	for i := start; i < len(s.tokens); i++ {
		if s.tokens[i].Channel == channel {
			return i
		}
	}
	return len(s.tokens)
}

// prevOnChannel returns the largest index <= start whose token is on
// the requested channel. Returns -1 if none.
func (s *TokenStream) prevOnChannel(start, channel int) int {
	for i := start; i >= 0; i-- {
		if s.tokens[i].Channel == channel {
			return i
		}
	}
	return -1
}
