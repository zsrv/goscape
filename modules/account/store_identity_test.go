package account

import (
	"errors"
	"testing"
)

func TestNormalizeCharacterName(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Zezima", "zezima", true},
		{" My Name ", "my_name", true},
		{"a", "a", true},
		{"exactly12chr", "exactly12chr", true},
		{"thirteenchars", "", false},
		{"", "", false},
		{"bad!char", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizeCharacterName(tc.in)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("NormalizeCharacterName(%q) = %q, %v; want %q ok=%v", tc.in, got, err, tc.want, tc.ok)
		}
	}
}

func TestStore_LinkIdentityRules(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	a, _ := s.CreateAccount(ctx, "a@example.com", "x")
	b, _ := s.CreateAccount(ctx, "b@example.com", "x")

	if err := s.LinkIdentity(ctx, a, "discord", "D1", "alice"); err != nil {
		t.Fatalf("link: %v", err)
	}
	// Same Discord identity on a second account: taken.
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); !errors.Is(err, ErrIdentityTaken) {
		t.Fatalf("cross-account relink: got %v, want ErrIdentityTaken", err)
	}
	// Second Discord on the same account: already linked.
	if err := s.LinkIdentity(ctx, a, "discord", "D2", "alice2"); !errors.Is(err, ErrAlreadyLinked) {
		t.Fatalf("second provider link: got %v, want ErrAlreadyLinked", err)
	}

	// Burn: revoked identity STILL blocks reuse by another account.
	if err := s.RevokeIdentity(ctx, a, "discord"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ids, err := s.IdentitiesByAccount(ctx, a)
	if err != nil || len(ids) != 1 || !ids[0].RevokedAt.Valid {
		t.Fatalf("revoked identity row: %+v err=%v", ids, err)
	}
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); !errors.Is(err, ErrIdentityTaken) {
		t.Fatalf("burned identity must stay taken: got %v", err)
	}

	// Release: hard delete frees it.
	if err := s.ReleaseIdentity(ctx, "discord", "D1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := s.LinkIdentity(ctx, b, "discord", "D1", "mallory"); err != nil {
		t.Fatalf("post-release link: %v", err)
	}
}

func TestStore_GateEligible(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	id, _ := s.CreateAccount(ctx, "a@example.com", "x")
	providers := []string{"discord"}

	if ok, _ := s.GateEligible(ctx, id, providers); ok {
		t.Fatal("fresh account must not be eligible")
	}
	// Linked identity satisfies the gate.
	if err := s.LinkIdentity(ctx, id, "discord", "D1", "alice"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); !ok {
		t.Fatal("linked account must be eligible")
	}
	// Revoked link no longer satisfies it.
	if err := s.RevokeIdentity(ctx, id, "discord"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); ok {
		t.Fatal("revoked link must not satisfy the gate")
	}
	// manually_approved overrides.
	if err := s.AddGroupMember(ctx, GroupManuallyApproved, id, 0); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, providers); !ok {
		t.Fatal("manually_approved must satisfy the gate")
	}
	// Empty provider list: only the group counts.
	if err := s.RemoveGroupMember(ctx, GroupManuallyApproved, id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.GateEligible(ctx, id, nil); ok {
		t.Fatal("empty providers + no group must not be eligible")
	}
}

func TestStore_CreateCharacter(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	a, _ := s.CreateAccount(ctx, "a@example.com", "x")
	b, _ := s.CreateAccount(ctx, "b@example.com", "x")

	ch, err := s.CreateCharacter(ctx, a, "zezima", 2)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ch.AccountID != a || ch.Username != "zezima" || ch.GameAccountID == 0 {
		t.Fatalf("bad character: %+v", ch)
	}

	// The game account row exists with the sentinel password.
	var pw string
	err = s.db.QueryRowContext(ctx, s.db.Rebind(
		`SELECT password FROM account WHERE id = ?`), ch.GameAccountID).Scan(&pw)
	if err != nil || pw != SentinelGamePassword {
		t.Fatalf("game account row: pw=%q err=%v", pw, err)
	}

	// Name uniqueness spans accounts (game account.username UNIQUE).
	if _, err := s.CreateCharacter(ctx, b, "zezima", 2); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("dup name: got %v, want ErrNameTaken", err)
	}

	// Limit.
	if _, err := s.CreateCharacter(ctx, a, "alt1", 2); err != nil {
		t.Fatalf("second char: %v", err)
	}
	if _, err := s.CreateCharacter(ctx, a, "alt2", 2); !errors.Is(err, ErrCharacterLimit) {
		t.Fatalf("over limit: got %v, want ErrCharacterLimit", err)
	}

	chars, err := s.CharactersByAccount(ctx, a)
	if err != nil || len(chars) != 2 {
		t.Fatalf("list: n=%d err=%v", len(chars), err)
	}

	gotCh, gotAcct, err := s.CharacterWithAccount(ctx, "zezima")
	if err != nil || gotCh.ID != ch.ID || gotAcct.ID != a {
		t.Fatalf("with account: %+v %+v %v", gotCh, gotAcct, err)
	}
	if _, _, err := s.CharacterWithAccount(ctx, "ghost"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing char: got %v, want ErrNotFound", err)
	}

	if _, err := s.CreateCharacter(ctx, 99999, "ghostowner", 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nonexistent owner: got %v, want ErrNotFound", err)
	}
}
