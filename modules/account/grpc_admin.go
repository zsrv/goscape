package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// audit records an admin action; the gRPC surface has no portal actor,
// so entries carry a NULL actor and a "grpc-admin" detail prefix.
func (h *grpcHandler) audit(ctx context.Context, action, target, details string) {
	if err := h.store.AppendAudit(ctx, 0, action, target, "grpc-admin: "+details); err != nil {
		h.log.Warn("audit append failed", slog.String("action", action), slog.Any("err", err))
	}
}

func accountTarget(id int64) string { return fmt.Sprintf("account:%d", id) }

func (h *grpcHandler) summary(ctx context.Context, a *PortalAccount) (*accountpb.AccountSummary, error) {
	chars, err := h.store.CharactersByAccount(ctx, a.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "characters: %v", err)
	}
	return &accountpb.AccountSummary{
		Id:             a.ID,
		Email:          a.Email,
		EmailVerified:  a.EmailVerified,
		Status:         a.Status,
		CharacterCount: int32(len(chars)),
	}, nil
}

func (h *grpcHandler) SearchAccounts(ctx context.Context, req *accountpb.SearchAccountsRequest) (*accountpb.SearchAccountsResponse, error) {
	accts, err := h.store.SearchAccounts(ctx, req.Query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search: %v", err)
	}
	resp := &accountpb.SearchAccountsResponse{}
	for i := range accts {
		sum, err := h.summary(ctx, &accts[i])
		if err != nil {
			return nil, err
		}
		resp.Accounts = append(resp.Accounts, sum)
	}
	return resp, nil
}

func (h *grpcHandler) GetAccount(ctx context.Context, req *accountpb.GetAccountRequest) (*accountpb.GetAccountResponse, error) {
	var (
		acct *PortalAccount
		err  error
	)
	switch {
	case req.Id != 0:
		acct, err = h.store.AccountByID(ctx, req.Id)
	case req.Email != "":
		acct, err = h.store.AccountByEmail(ctx, req.Email)
	default:
		return nil, status.Error(codes.InvalidArgument, "one of id or email is required")
	}
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}

	sum, serr := h.summary(ctx, acct)
	if serr != nil {
		return nil, serr
	}
	resp := &accountpb.GetAccountResponse{Account: sum}

	ids, err := h.store.IdentitiesByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "identities: %v", err)
	}
	for _, id := range ids {
		resp.Identities = append(resp.Identities, &accountpb.IdentityInfo{
			Provider:         id.Provider,
			ProviderUserId:   id.ProviderUserID,
			ProviderUsername: id.ProviderUsername,
			Revoked:          id.RevokedAt.Valid,
		})
	}
	chars, err := h.store.CharactersByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "characters: %v", err)
	}
	for _, ch := range chars {
		resp.Characters = append(resp.Characters, &accountpb.CharacterInfo{
			Id: ch.ID, Username: ch.Username, GameAccountId: ch.GameAccountID,
		})
	}
	groups, err := h.store.GroupsByAccount(ctx, acct.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "groups: %v", err)
	}
	resp.Groups = groups
	return resp, nil
}

func (h *grpcHandler) SetGroupMembership(ctx context.Context, req *accountpb.SetGroupMembershipRequest) (*emptypb.Empty, error) {
	if !slices.Contains([]string{GroupManuallyApproved, GroupAdmin}, req.Group) {
		return nil, status.Errorf(codes.InvalidArgument, "unknown group %q", req.Group)
	}
	var err error
	if req.Member {
		err = h.store.AddGroupMember(ctx, req.Group, req.AccountId, 0)
	} else {
		err = h.store.RemoveGroupMember(ctx, req.Group, req.AccountId)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "group membership: %v", err)
	}
	h.audit(ctx, "group.set", accountTarget(req.AccountId), fmt.Sprintf("%s=%v", req.Group, req.Member))
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) SetAccountStatus(ctx context.Context, req *accountpb.SetAccountStatusRequest) (*emptypb.Empty, error) {
	if err := h.store.SetAccountStatus(ctx, req.AccountId, req.Status); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if req.Status == StatusDisabled {
		// Disabling an account kills its portal sessions too.
		if err := h.store.DeleteAccountSessions(ctx, req.AccountId); err != nil {
			return nil, status.Errorf(codes.Internal, "clear sessions: %v", err)
		}
	}
	h.audit(ctx, "account.status", accountTarget(req.AccountId), req.Status)
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) UnlinkIdentity(ctx context.Context, req *accountpb.UnlinkIdentityRequest) (*emptypb.Empty, error) {
	if err := h.store.RevokeIdentity(ctx, req.AccountId, req.Provider); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "no active link for that provider")
		}
		return nil, status.Errorf(codes.Internal, "revoke: %v", err)
	}
	h.audit(ctx, "identity.unlink", accountTarget(req.AccountId), req.Provider)
	return &emptypb.Empty{}, nil
}

func (h *grpcHandler) ReleaseIdentity(ctx context.Context, req *accountpb.ReleaseIdentityRequest) (*emptypb.Empty, error) {
	if err := h.store.ReleaseIdentity(ctx, req.Provider, req.ProviderUserId); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "identity not found")
		}
		return nil, status.Errorf(codes.Internal, "release: %v", err)
	}
	h.audit(ctx, "identity.release", req.Provider+":"+req.ProviderUserId, "")
	return &emptypb.Empty{}, nil
}

// AdminResetPassword mints a 1h single-use reset token and returns the
// portal URL. The portal's /reset-password page (Task 17) consumes it.
func (h *grpcHandler) AdminResetPassword(ctx context.Context, req *accountpb.AdminResetPasswordRequest) (*accountpb.AdminResetPasswordResponse, error) {
	if _, err := h.store.AccountByID(ctx, req.AccountId); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "account not found")
		}
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}
	raw, err := NewRawToken()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "token: %v", err)
	}
	if err := h.store.CreateToken(ctx, req.AccountId, TokenPurposeResetPassword, HashToken(raw), time.Hour); err != nil {
		return nil, status.Errorf(codes.Internal, "create token: %v", err)
	}
	h.audit(ctx, "account.reset_password", accountTarget(req.AccountId), "reset link minted")
	return &accountpb.AdminResetPasswordResponse{
		ResetUrl: h.cfg.PublicURL + "/reset-password?token=" + raw,
	}, nil
}

func (h *grpcHandler) BootstrapAdmin(ctx context.Context, req *accountpb.BootstrapAdminRequest) (*emptypb.Empty, error) {
	acct, err := h.store.AccountByEmail(ctx, req.Email)
	if errors.Is(err, ErrNotFound) {
		return nil, status.Error(codes.NotFound, "account not found")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "lookup: %v", err)
	}
	if err := h.store.AddGroupMember(ctx, GroupAdmin, acct.ID, 0); err != nil {
		return nil, status.Errorf(codes.Internal, "add admin: %v", err)
	}
	h.audit(ctx, "group.set", accountTarget(acct.ID), GroupAdmin+"=true (bootstrap)")
	return &emptypb.Empty{}, nil
}
