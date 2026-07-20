package account

import (
	"errors"
	"testing"
	"time"
)

func TestTokenHelpers(t *testing.T) {
	a, err := NewRawToken()
	if err != nil || len(a) < 40 {
		t.Fatalf("NewRawToken: %q err=%v", a, err)
	}
	b, _ := NewRawToken()
	if a == b {
		t.Fatal("tokens must be unique")
	}
	if HashToken(a) == HashToken(b) || len(HashToken(a)) != 64 {
		t.Fatalf("HashToken: %q", HashToken(a))
	}
}

func TestStore_SessionLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")
	cfg := SessionConfig{IdleTTL: time.Hour, AbsoluteTTL: 24 * time.Hour}

	raw, _ := NewRawToken()
	if err := s.CreateSession(ctx, id, HashToken(raw), "1.2.3.4", "ua", cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	acct, err := s.SessionAccount(ctx, HashToken(raw), cfg)
	if err != nil || acct.ID != id {
		t.Fatalf("load: %+v err=%v", acct, err)
	}
	if _, err := s.SessionAccount(ctx, HashToken("wrong"), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bogus token: got %v", err)
	}

	if err := s.DeleteSession(ctx, HashToken(raw)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session must be gone: got %v", err)
	}

	// Expired sessions are rejected.
	raw2, _ := NewRawToken()
	expired := SessionConfig{IdleTTL: -time.Hour, AbsoluteTTL: 24 * time.Hour}
	if err := s.CreateSession(ctx, id, HashToken(raw2), "", "", expired); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw2), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: got %v", err)
	}

	// DeleteAccountSessions clears everything ("log out everywhere").
	raw3, _ := NewRawToken()
	_ = s.CreateSession(ctx, id, HashToken(raw3), "", "", cfg)
	if err := s.DeleteAccountSessions(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionAccount(ctx, HashToken(raw3), cfg); !errors.Is(err, ErrNotFound) {
		t.Fatalf("account sessions must be gone: got %v", err)
	}
}

func TestStore_TokenSingleUse(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")

	raw, _ := NewRawToken()
	if err := s.CreateToken(ctx, id, TokenPurposeVerifyEmail, HashToken(raw), time.Hour); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw))
	if err != nil || got != id {
		t.Fatalf("consume: got=%d err=%v", got, err)
	}
	// Second consume fails (single-use).
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("re-consume: got %v", err)
	}
	// Wrong purpose fails.
	raw2, _ := NewRawToken()
	_ = s.CreateToken(ctx, id, TokenPurposeResetPassword, HashToken(raw2), time.Hour)
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw2)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-purpose consume: got %v", err)
	}
	// Expired fails.
	raw3, _ := NewRawToken()
	_ = s.CreateToken(ctx, id, TokenPurposeVerifyEmail, HashToken(raw3), -time.Hour)
	if _, err := s.ConsumeToken(ctx, TokenPurposeVerifyEmail, HashToken(raw3)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired consume: got %v", err)
	}
}
