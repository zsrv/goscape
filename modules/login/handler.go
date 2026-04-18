package login

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/loginpb"
)

// handler implements loginpb.LoginServiceServer.
type handler struct {
	loginpb.UnimplementedLoginServiceServer

	db  *sql.DB
	cfg Config
	log *slog.Logger

	// loginRequests tracks usernames whose login flow is currently in progress,
	// so duplicate attempts (racing login) can be rejected cleanly.
	// Key: username (string), value: struct{}.
	loginRequests sync.Map
}

// extractIP strips the port from a "host:port" remote address.
// If the address cannot be parsed, it is returned unchanged.
func extractIP(remoteAddr string) string {
	if remoteAddr == "" {
		return remoteAddr
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	// SplitHostPort failed — address is already a bare IP with no port.
	return remoteAddr
}

// WorldStartup clears all login sessions for the given node+profile.
func (h *handler) WorldStartup(ctx context.Context, req *loginpb.WorldStartupRequest) (*emptypb.Empty, error) {
	if err := clearWorldSessions(ctx, h.db, int(req.NodeId), req.Profile); err != nil {
		return nil, status.Errorf(codes.Internal, "clearWorldSessions: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// PlayerLogin performs the full auth flow.
func (h *handler) PlayerLogin(ctx context.Context, req *loginpb.PlayerLoginRequest) (*loginpb.PlayerLoginResponse, error) {
	// 1. Duplicate in-flight check.
	if _, loaded := h.loginRequests.LoadOrStore(req.Username, struct{}{}); loaded {
		return &loginpb.PlayerLoginResponse{
			Result: loginpb.LoginResult_LOGIN_RESULT_LOGIN_IN_PROGRESS,
		}, nil
	}
	defer h.loginRequests.Delete(req.Username)

	// 2. IP ban check.
	ip := extractIP(req.RemoteAddress)
	banned, err := ipBanned(ctx, h.db, ip)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ipBanned: %v", err)
	}
	if banned {
		return &loginpb.PlayerLoginResponse{
			Result: loginpb.LoginResult_LOGIN_RESULT_IP_BANNED,
		}, nil
	}

	// 3. Account lookup / auto-registration.
	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "accountByUsername: %v", err)
	}
	if account == nil {
		if !h.cfg.AutoRegister {
			return &loginpb.PlayerLoginResponse{
				Result: loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS,
			}, nil
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), h.cfg.BCryptCost)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "bcrypt hash: %v", err)
		}
		if _, err := insertAccount(ctx, h.db, req.Username, string(hashed), ip); err != nil {
			return nil, status.Errorf(codes.Internal, "insertAccount: %v", err)
		}
		account, err = accountByUsername(ctx, h.db, req.Username, req.Profile)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "accountByUsername (post-insert): %v", err)
		}
		if account == nil {
			return nil, status.Error(codes.Internal, "account not found after insert")
		}
	}

	// 4. Password check.
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(req.Password)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return &loginpb.PlayerLoginResponse{
				Result:    loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS,
				AccountId: int32(account.ID),
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "bcrypt compare: %v", err)
	}

	// 5. Ban check (account-level).
	if account.BannedUntil.Valid {
		if t, err := time.Parse(dbTimeFormat, account.BannedUntil.String); err == nil {
			if time.Now().Before(t) {
				return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED, account, nil), nil
			}
		}
	}

	// 6. Members check.
	if account.Members == 0 && req.NodeMembers {
		if h.cfg.AutoSubscribeMembers {
			if err := setAccountMembers(ctx, h.db, account.ID); err != nil {
				return nil, status.Errorf(codes.Internal, "setAccountMembers: %v", err)
			}
			account.Members = 1
		} else {
			return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER, account, nil), nil
		}
	}

	// 7. Already-logged-in / reconnect detection.
	reconnect := false
	if account.HasLoginRow && account.LoggedIn == 1 {
		if account.NodeID != int(req.NodeId) {
			return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, account, nil), nil
		}
		// Same node: this is a reconnect if the client indicates it.
		if req.Reconnecting && req.HasSave {
			reconnect = true
		}
	}

	// 9. Reconnect short-circuits — no session insert, no save read, no login-row upsert.
	if reconnect {
		return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, account, nil), nil
	}

	// 8. Record session (store the extracted IP for consistency with ipBanned lookups).
	if err := insertSession(ctx, h.db, req.Socket, account.ID, req.Profile, int(req.NodeId), int(req.Uid), ip); err != nil {
		return nil, status.Errorf(codes.Internal, "insertSession: %v", err)
	}

	// 10. Read save file.
	savePath := filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav")
	saveBytes, err := os.ReadFile(savePath)
	result := loginpb.LoginResult_LOGIN_RESULT_OK
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result = loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER
			saveBytes = nil
		} else {
			return nil, status.Errorf(codes.Internal, "read save: %v", err)
		}
	}

	// 11. Upsert login row.
	if err := upsertAccountLogin(ctx, h.db, account.ID, req.Profile, int(req.NodeId)); err != nil {
		return nil, status.Errorf(codes.Internal, "upsertAccountLogin: %v", err)
	}

	return buildLoginResponse(result, account, saveBytes), nil
}

// PlayerLogout writes the final save blob to disk and marks the account as logged out.
func (h *handler) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	if err := writeSave(h.cfg.SavePath, req.Profile, req.Username, req.Save); err != nil {
		return nil, status.Errorf(codes.Internal, "write save: %v", err)
	}

	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "accountByUsername: %v", err)
	}
	if account == nil {
		return nil, status.Errorf(codes.NotFound, "account %q not found", req.Username)
	}
	if err := setLoggedOut(ctx, h.db, account.ID, req.Profile, int(req.NodeId)); err != nil {
		return nil, status.Errorf(codes.Internal, "setLoggedOut: %v", err)
	}

	// TODO: updateHiscores

	return &loginpb.PlayerLogoutResponse{Success: true}, nil
}

// PlayerAutosave writes the save blob to disk (best-effort; failures log and return success).
func (h *handler) PlayerAutosave(_ context.Context, req *loginpb.PlayerAutosaveRequest) (*emptypb.Empty, error) {
	if err := writeSave(h.cfg.SavePath, req.Profile, req.Username, req.Save); err != nil {
		h.log.Warn("autosave write failed",
			slog.String("profile", req.Profile),
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
	return &emptypb.Empty{}, nil
}

// PlayerForceLogout clears the logged-in flag for a given account/node/profile.
func (h *handler) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) (*emptypb.Empty, error) {
	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "accountByUsername: %v", err)
	}
	if account == nil {
		return nil, status.Errorf(codes.NotFound, "account %q not found", req.Username)
	}
	if err := setLoggedOut(ctx, h.db, account.ID, req.Profile, int(req.NodeId)); err != nil {
		return nil, status.Errorf(codes.Internal, "setLoggedOut: %v", err)
	}
	return &emptypb.Empty{}, nil
}

// PlayerBan sets the banned_until timestamp for an account.
func (h *handler) PlayerBan(ctx context.Context, req *loginpb.PlayerBanRequest) (*loginpb.PlayerBanResponse, error) {
	if err := setAccountBanned(ctx, h.db, req.Username, req.Until.AsTime()); err != nil {
		return nil, status.Errorf(codes.Internal, "setAccountBanned: %v", err)
	}
	return &loginpb.PlayerBanResponse{}, nil
}

// PlayerMute sets the muted_until timestamp for an account.
func (h *handler) PlayerMute(ctx context.Context, req *loginpb.PlayerMuteRequest) (*loginpb.PlayerMuteResponse, error) {
	if err := setAccountMuted(ctx, h.db, req.Username, req.Until.AsTime()); err != nil {
		return nil, status.Errorf(codes.Internal, "setAccountMuted: %v", err)
	}
	return &loginpb.PlayerMuteResponse{}, nil
}

// writeSave atomically writes save bytes to {basePath}/{profile}/{username}.sav.
// It writes to a temp file first, then renames so a crash never leaves a partial save.
func writeSave(basePath, profile, username string, save []byte) error {
	dir := filepath.Join(basePath, profile)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "."+username+".sav.tmp*")
	if err != nil {
		return fmt.Errorf("create temp save: %w", err)
	}
	tmpName := tmp.Name()
	if _, err = tmp.Write(save); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp save: %w", err)
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp save: %w", err)
	}
	if err = os.Rename(tmpName, filepath.Join(dir, username+".sav")); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename save: %w", err)
	}
	return nil
}

// buildLoginResponse constructs a PlayerLoginResponse with account-derived fields populated.
func buildLoginResponse(result loginpb.LoginResult, account *accountRow, save []byte) *loginpb.PlayerLoginResponse {
	resp := &loginpb.PlayerLoginResponse{
		Result:        result,
		AccountId:     int32(account.ID),
		StaffModLevel: int32(account.StaffModLevel),
		Members:       account.Members == 1,
	}
	if len(save) > 0 {
		resp.Save = save
	}
	if account.MutedUntil.Valid {
		if t, err := time.Parse(dbTimeFormat, account.MutedUntil.String); err == nil {
			if time.Now().Before(t) {
				resp.MutedUntil = timestamppb.New(t)
			}
		}
	}
	return resp
}
