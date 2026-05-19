package world

import (
	"bytes"
	"sync"
)

// syncBuffer wraps bytes.Buffer with a mutex so a test's polling
// goroutine and a callback's logging goroutine don't race on the
// underlying buffer state. Extracted from tick_friends_login_test.go
// (slice 4c T3) for re-use across world-package tests.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
