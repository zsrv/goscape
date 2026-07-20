package account

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/accountpb"
)

func TestAdminRPCs(t *testing.T) {
	s := openTestStore(t)
	cfg := defaultConfig(t)
	cfg.AdminToken = "sekrit"
	cfg.PublicURL = "http://portal.test"
	client := startBufconnServer(t, cfg, s)
	ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer sekrit")

	portalID := seedVerifiedAccountWithCharacter(t, s, "a@example.com", "zezima")
	if err := s.LinkIdentity(t.Context(), portalID, "discord", "D1", "alice"); err != nil {
		t.Fatal(err)
	}

	// Search: by email substring, character name, provider user id.
	for _, q := range []string{"a@example", "zezima", "D1"} {
		resp, err := client.SearchAccounts(ctx, &accountpb.SearchAccountsRequest{Query: q})
		if err != nil || len(resp.Accounts) != 1 || resp.Accounts[0].Id != portalID {
			t.Fatalf("search %q: %+v err=%v", q, resp, err)
		}
	}

	// GetAccount by id: identities + characters + groups present.
	got, err := client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Account.Email != "a@example.com" || got.Account.CharacterCount != 1 ||
		len(got.Identities) != 1 || got.Identities[0].ProviderUserId != "D1" ||
		len(got.Characters) != 1 || got.Characters[0].Username != "zezima" {
		t.Fatalf("get payload: %+v", got)
	}

	// Group membership round-trip.
	if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
		AccountId: portalID, Group: GroupManuallyApproved, Member: true}); err != nil {
		t.Fatal(err)
	}
	got, _ = client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if len(got.Groups) != 1 || got.Groups[0] != GroupManuallyApproved {
		t.Fatalf("groups after add: %v", got.Groups)
	}
	if _, err := client.SetGroupMembership(ctx, &accountpb.SetGroupMembershipRequest{
		AccountId: portalID, Group: "bogus", Member: true}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bogus group: %v", err)
	}

	// Status.
	if _, err := client.SetAccountStatus(ctx, &accountpb.SetAccountStatusRequest{
		AccountId: portalID, Status: StatusDisabled}); err != nil {
		t.Fatal(err)
	}

	// Unlink (burn) then release.
	if _, err := client.UnlinkIdentity(ctx, &accountpb.UnlinkIdentityRequest{
		AccountId: portalID, Provider: "discord"}); err != nil {
		t.Fatal(err)
	}
	got, _ = client.GetAccount(ctx, &accountpb.GetAccountRequest{Id: portalID})
	if !got.Identities[0].Revoked {
		t.Fatalf("identity not revoked: %+v", got.Identities)
	}
	if _, err := client.ReleaseIdentity(ctx, &accountpb.ReleaseIdentityRequest{
		Provider: "discord", ProviderUserId: "D1"}); err != nil {
		t.Fatal(err)
	}

	// AdminResetPassword returns a reset URL whose token consumes.
	rp, err := client.AdminResetPassword(ctx, &accountpb.AdminResetPasswordRequest{AccountId: portalID})
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "http://portal.test/reset-password?token="
	if len(rp.ResetUrl) <= len(prefix) || rp.ResetUrl[:len(prefix)] != prefix {
		t.Fatalf("reset url: %q", rp.ResetUrl)
	}
	raw := rp.ResetUrl[len(prefix):]
	if _, err := s.ConsumeToken(t.Context(), TokenPurposeResetPassword, HashToken(raw)); err != nil {
		t.Fatalf("reset token must consume: %v", err)
	}

	// BootstrapAdmin by email.
	if _, err := client.BootstrapAdmin(ctx, &accountpb.BootstrapAdminRequest{Email: "a@example.com"}); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.IsGroupMember(t.Context(), GroupAdmin, portalID); !ok {
		t.Fatal("bootstrap-admin must add admin group")
	}

	// Every admin action audited.
	entries, err := s.RecentAudit(t.Context(), 50, "")
	if err != nil || len(entries) < 6 {
		t.Fatalf("audit entries: n=%d err=%v", len(entries), err)
	}
}
