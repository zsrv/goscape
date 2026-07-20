package account

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/accountpb"
)

// grpcHandler implements accountpb.AccountServiceServer. Admin RPCs are
// guarded by adminAuthInterceptor, not per-method checks.
type grpcHandler struct {
	accountpb.UnimplementedAccountServiceServer

	cfg   Config
	store *Store
	log   *slog.Logger
}

func newGRPCServer(cfg Config, store *Store, log *slog.Logger) *grpc.Server {
	// Same keepalive posture as the login module (arch-29.2): permit
	// the world's/login's 30s probes.
	s := grpc.NewServer(
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             15 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.UnaryInterceptor(adminAuthInterceptor(cfg.AdminToken)),
	)
	accountpb.RegisterAccountServiceServer(s, &grpcHandler{cfg: cfg, store: store, log: log})
	reflection.Register(s)
	return s
}

// adminAuthInterceptor gates every RPC except VerifyGameLogin behind
// `authorization: Bearer <token>` metadata. Empty configured token =
// admin surface disabled (PermissionDenied), distinct from a bad
// credential (Unauthenticated).
func adminAuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == accountpb.AccountService_VerifyGameLogin_FullMethodName {
			return handler(ctx, req)
		}
		if token == "" {
			return nil, status.Error(codes.PermissionDenied, "admin RPCs disabled: account.admin_token not configured")
		}
		md, _ := metadata.FromIncomingContext(ctx)
		vals := md.Get("authorization")
		if len(vals) != 1 || subtle.ConstantTimeCompare([]byte(vals[0]), []byte("Bearer "+token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid admin token")
		}
		return handler(ctx, req)
	}
}

// VerifyGameLogin resolves character name → portal account and checks
// the account password. Password is verified BEFORE account-state
// checks so a caller without valid credentials cannot probe account
// status. Returns statuses, never gRPC errors, for auth outcomes;
// gRPC errors mean infrastructure failure (login maps them to
// login-server-offline).
func (h *grpcHandler) VerifyGameLogin(ctx context.Context, req *accountpb.VerifyGameLoginRequest) (*accountpb.VerifyGameLoginResponse, error) {
	fail := func(r accountpb.VerifyResult) *accountpb.VerifyGameLoginResponse {
		return &accountpb.VerifyGameLoginResponse{Result: r}
	}
	ch, acct, err := h.store.CharacterWithAccount(ctx, req.CharacterName)
	if errors.Is(err, ErrNotFound) {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS), nil
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "character lookup: %v", err)
	}
	ok, err := VerifyPassword(req.Password, acct.PasswordHash)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "password verify: %v", err)
	}
	if !ok {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_INVALID_CREDENTIALS), nil
	}
	if acct.Status != StatusActive {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_ACCOUNT_DISABLED), nil
	}
	if !acct.EmailVerified {
		return fail(accountpb.VerifyResult_VERIFY_RESULT_EMAIL_UNVERIFIED), nil
	}
	return &accountpb.VerifyGameLoginResponse{
		Result:          accountpb.VerifyResult_VERIFY_RESULT_OK,
		GameAccountId:   ch.GameAccountID,
		PortalAccountId: acct.ID,
	}, nil
}
