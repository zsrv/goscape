package login

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/gamedb"
	"github.com/zsrv/goscape/pkg/loginpb"
)

// handler implements loginpb.LoginServiceServer.
type handler struct {
	loginpb.UnimplementedLoginServiceServer

	db  *gamedb.DB
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
	// PORTING-EXCEPTION (rev244-b5-startup-profile, world_startup keeps
	// profile): TS 244 dropped `profile` from the world_startup message
	// (LoginClient.ts:13-27) but LoginServer.ts:161-170 still filters the
	// account_login reset by the now-undefined profile — the upstream
	// reset matches nothing at the pin. goscape keeps the field and the
	// per-profile reset (correct behavior over broken-line fidelity;
	// rev244-b3-ws-origin precedent). See docs/PORTING.md.
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

	// Per-login UUID. Used for the session-table insert and stamped on
	// every positive response so the world can assign Player.session =
	// <uuid>. Mirrors TS crypto.randomUUID().
	sessionUUID := uuid.NewString()

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
		// Lowercase before hash — mirrors TS LoginServer.ts:213
		// `bcrypt.hashSync(password.toLowerCase(), 10)`. Cross-server
		// account parity requires byte-identical hash input.
		hashed, err := bcrypt.GenerateFromPassword([]byte(strings.ToLower(req.Password)), h.cfg.BCryptCost)
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

	// 3b. Per-attempt rate limit + attempt log. TS LoginServer.ts:234-268:
	// runs only when the account exists (auto-registered accounts
	// included), BEFORE the password compare; 3 rows for (account, ip)
	// inside 5s → response 8; a rate-limited attempt does NOT insert.
	// goscape's per-attempt sessionUUID stands in for TS's socket uuid.
	// (TS's `if (account)` guard covers its no-auto-register fallthrough,
	// which goscape already returned from at step 3 — account is non-nil
	// here.)
	recent, err := countRecentLoginAttempts(ctx, h.db, account.ID, ip, 5*time.Second)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "countRecentLoginAttempts: %v", err)
	}
	if recent >= 3 {
		return &loginpb.PlayerLoginResponse{
			Result: loginpb.LoginResult_LOGIN_RESULT_RATE_LIMITED,
		}, nil
	}
	if err := insertLoginAttempt(ctx, h.db, sessionUUID, account.ID, int(req.NodeId), int(req.Uid), ip); err != nil {
		return nil, status.Errorf(codes.Internal, "insertLoginAttempt: %v", err)
	}

	// 4. Password check.
	// Lowercase before compare — mirrors TS LoginServer.ts:233
	// `bcrypt.compare(password.toLowerCase(), account.password)`.
	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(strings.ToLower(req.Password))); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return &loginpb.PlayerLoginResponse{
				Result:    loginpb.LoginResult_LOGIN_RESULT_INVALID_CREDENTIALS,
				AccountId: int32(account.ID),
			}, nil
		}
		return nil, status.Errorf(codes.Internal, "bcrypt compare: %v", err)
	}

	// 5. Ban check (account-level).
	if account.BannedUntil.Valid && time.Now().Before(account.BannedUntil.Time) {
		return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_ACCOUNT_DISABLED, account, nil, sessionUUID), nil
	}

	// 6. Members check.
	if account.Members == 0 && req.NodeMembers {
		if h.cfg.AutoSubscribeMembers {
			if err := setAccountMembers(ctx, h.db, account.ID); err != nil {
				return nil, status.Errorf(codes.Internal, "setAccountMembers: %v", err)
			}
			account.Members = 1
		} else {
			return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_NOT_A_MEMBER, account, nil, sessionUUID), nil
		}
	}

	// 7. Already-logged-in / reconnect detection. Mirrors TS
	// LoginServer.ts:271,318 — an if/else-if over an account that is already
	// logged in (logged_in !== null && !== 0): the reconnect branch fires ONLY
	// for `reconnecting && logged_in === nodeId`; EVERY other already-logged-in
	// case falls to `else if` → response 3 (ALREADY_LOGGED_IN). That includes a
	// same-node FRESH (non-reconnect) login, which must be rejected rather than
	// admitted into a second full-login session (login-server-1). We do NOT gate
	// the reconnect branch on req.HasSave (M27) — the save is re-served inside it
	// when the client lost it.
	reconnect := false
	if account.HasLoginRow && account.LoggedIn == 1 {
		if account.NodeID == int(req.NodeId) && req.Reconnecting {
			reconnect = true
		} else {
			return buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_ALREADY_LOGGED_IN, account, nil, sessionUUID), nil
		}
	} else if account.StaffModLevel < 2 &&
		account.LoggedOut != 0 &&
		account.LoggedOut != int(req.NodeId) &&
		account.LogoutTime.Valid {
		// 45s hop timer — TS LoginServer.ts:366-379: a non-staff account
		// that gracefully logged out of a DIFFERENT world less than 45s
		// ago is rejected with response 6. logged_out/logout_time are the
		// per-profile columns from migration 000005 (login-server-7
		// closure). TS's `logged_out !== null` collapses into != 0 (the
		// column is NOT NULL DEFAULT 0).
		if !account.LogoutTime.Time.Before(time.Now().UTC().Add(-45 * time.Second)) {
			return &loginpb.PlayerLoginResponse{
				Result: loginpb.LoginResult_LOGIN_RESULT_HOP_TIMER,
			}, nil
		}
	}

	// 9. Reconnect (TS LoginServer.ts:271-317): record a session row, and when
	// the client lost its save (!hasSave) read it back from disk + verify and
	// serve it; otherwise the client keeps its own copy. Always RECONNECT_OK,
	// no login-row upsert (already logged in on this node). M27: previously this
	// only fired when req.HasSave was set — which the world never sends — so
	// reconnects fell through to the full-login path and returned OK, not
	// RECONNECT_OK, and never re-served a lost save.
	if reconnect {
		if err := insertSession(ctx, h.db, sessionUUID, account.ID, req.Profile, int(req.NodeId), int(req.Uid), ip); err != nil {
			return nil, status.Errorf(codes.Internal, "insertSession (reconnect): %v", err)
		}
		var saveBytes []byte
		if !req.HasSave {
			b, err := os.ReadFile(filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav"))
			if err != nil {
				// TS rejectLoginForSafety (LoginServer.ts:288-290): a reconnecting
				// client lost its save and we cannot read it back — reject rather
				// than resume the session with no character data.
				return nil, status.Errorf(codes.DataLoss, "reconnect save read for %q: %v", req.Username, err)
			}
			if !verifySave(b) {
				return nil, status.Errorf(codes.DataLoss, "reconnect save verify failed for %q", req.Username)
			}
			saveBytes = b
		}
		// TS LoginServer.ts:322 — messageCount on the reconnect reply too.
		mc, err := getUnreadMessageCount(ctx, h.db, account.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "getUnreadMessageCount: %v", err)
		}
		resp := buildLoginResponse(loginpb.LoginResult_LOGIN_RESULT_RECONNECT_OK, account, saveBytes, sessionUUID)
		resp.MessageCount = int32(mc)
		return resp, nil
	}

	// 8. Record session + login row atomically (docs/PORTING.md Arc 18 DB-1).
	// The intermediate save-file read sits inside the transaction window
	// but does not itself perform DB I/O; if it fails (non-ErrNotExist),
	// the deferred rollback drops the just-inserted session row so we
	// never leave orphan session rows without a matching login upsert.
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "begin tx: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := insertSessionTx(ctx, h.db, tx, sessionUUID, account.ID, req.Profile, int(req.NodeId), int(req.Uid), ip); err != nil {
		return nil, status.Errorf(codes.Internal, "insertSession: %v", err)
	}

	// 10. Read save file (idempotent file I/O; safe inside the tx window).
	savePath := filepath.Join(h.cfg.SavePath, req.Profile, req.Username+".sav")
	saveBytes, err := os.ReadFile(savePath)
	result := loginpb.LoginResult_LOGIN_RESULT_OK
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// M25 (TS LoginServer.ts:397-403): a missing save is only benign for
			// an account that never logged in on THIS profile. If the per-profile
			// logout_time is set the player logged out before, so a save SHOULD
			// exist — its absence is data loss, not a new player. Reject for
			// safety rather than silently resetting a real character to fresh.
			// The deferred rollback drops the just-inserted session row.
			// (TS rejectLoginForSafety → resp 7.)
			// login-server-7 CLOSED (rev-244 B5): logout_time is now read from
			// account_login.logout_time (per-profile) via the accountByUsername
			// LEFT-JOIN, eliminating spurious rejects on cross-profile first logins.
			if account.LogoutTime.Valid {
				return nil, status.Errorf(codes.DataLoss, "save missing but logout_time set for %q", req.Username)
			}
			result = loginpb.LoginResult_LOGIN_RESULT_NEW_PLAYER
			saveBytes = nil
		} else {
			return nil, status.Errorf(codes.Internal, "read save: %v", err)
		}
	} else if !verifySave(saveBytes) {
		// Safety check (TS LoginServer.ts:364-367, PlayerLoading.verify): an
		// existing save with bad magic/version/CRC must not be served to the
		// world. Reject the login (rollback drops the just-inserted session
		// row) rather than handing back corrupt data. TS "rejectLoginForSafety".
		return nil, status.Errorf(codes.DataLoss, "save verify failed for %q", req.Username)
	}

	// 11. Upsert login row.
	if err := upsertAccountLoginTx(ctx, h.db, tx, account.ID, req.Profile, int(req.NodeId)); err != nil {
		return nil, status.Errorf(codes.Internal, "upsertAccountLogin: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, status.Errorf(codes.Internal, "commit tx: %v", err)
	}
	committed = true

	emitLogin(int64(account.ID), req.NodeId, ip, req.Uid)

	// TS LoginServer.ts:395 — messageCount computed after the session
	// insert, attached to the success reply (:412/:433).
	mc, err := getUnreadMessageCount(ctx, h.db, account.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getUnreadMessageCount: %v", err)
	}
	resp := buildLoginResponse(result, account, saveBytes, sessionUUID)
	resp.MessageCount = int32(mc)
	return resp, nil
}

// persistSaveIfValid writes save to disk only if it passes verifySave AND would
// not roll back the player's progress (wouldResetSaveFile). A failing gate is
// logged and skipped — NOT an error — mirroring TS LoginServer player_logout /
// player_autosave (LoginServer.ts:410-418, 455-463), which log "Invalid save
// file" and simply don't write. Only an actual write/IO failure returns an error.
func (h *handler) persistSaveIfValid(profile, username string, save []byte) error {
	if !verifySave(save) {
		h.log.Warn("rejecting save: verify failed",
			slog.String("profile", profile), slog.String("username", username))
		return nil
	}
	savePath := filepath.Join(h.cfg.SavePath, profile, username+".sav")
	reset, err := wouldResetSaveFile(savePath, save)
	if err != nil {
		return fmt.Errorf("wouldResetSaveFile: %w", err)
	}
	if reset {
		h.log.Warn("rejecting save: would roll back progress",
			slog.String("profile", profile), slog.String("username", username))
		return nil
	}
	return writeSave(h.cfg.SavePath, profile, username, save)
}

// PlayerLogout writes the final save blob to disk and marks the account as logged out.
func (h *handler) PlayerLogout(ctx context.Context, req *loginpb.PlayerLogoutRequest) (*loginpb.PlayerLogoutResponse, error) {
	if err := h.persistSaveIfValid(req.Profile, req.Username, req.Save); err != nil {
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

	// Export the logged-out player's enabled-stat XP/levels to the hiscore
	// leaderboard tables. Mirrors TS LoginServer.ts:450 (the call site, right
	// after setLoggedOut). Best-effort: TS sends the logout response before
	// awaiting updateHiscores, so a hiscore failure must not fail the logout —
	// log it and still report success (login-server-9 / gap-db-datastruct-9).
	if err := updateHiscores(ctx, h.db, account, req.Save, req.Profile); err != nil {
		h.log.Warn("updateHiscores failed; logout unaffected",
			slog.String("username", req.Username),
			slog.String("profile", req.Profile),
			slog.Any("err", err))
	}

	return &loginpb.PlayerLogoutResponse{Success: true}, nil
}

// PlayerAutosave writes the save blob to disk (best-effort; failures log and return success).
func (h *handler) PlayerAutosave(_ context.Context, req *loginpb.PlayerAutosaveRequest) (*emptypb.Empty, error) {
	if err := h.persistSaveIfValid(req.Profile, req.Username, req.Save); err != nil {
		h.log.Warn("autosave write failed",
			slog.String("profile", req.Profile),
			slog.String("username", req.Username),
			slog.Any("err", err),
		)
	}
	return &emptypb.Empty{}, nil
}

// PlayerForceLogout clears the logged-in flag for a given account/profile.
// Mirrors TS LoginServer.ts:477-487, which writes only `logged_in:0,
// login_time:null` — NOT logout_time. Force-logout is meant to release the
// logged-in lock (e.g. when a sibling world detects a dangling session) so
// the player can reconnect cleanly; stamping logout_time would arm the M25
// "save missing but logout_time set" safety reject on the next login.
// [login-server-2]
func (h *handler) PlayerForceLogout(ctx context.Context, req *loginpb.PlayerForceLogoutRequest) (*emptypb.Empty, error) {
	account, err := accountByUsername(ctx, h.db, req.Username, req.Profile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "accountByUsername: %v", err)
	}
	if account == nil {
		return nil, status.Errorf(codes.NotFound, "account %q not found", req.Username)
	}
	if err := clearLoggedInFlag(ctx, h.db, account.ID, req.Profile); err != nil {
		return nil, status.Errorf(codes.Internal, "clearLoggedInFlag: %v", err)
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

// writeSave atomically and durably writes save bytes to
// {basePath}/{profile}/{username}.sav. It writes to a temp file, fsyncs it,
// renames it into place (atomic), then fsyncs the directory so both the file
// contents and the rename survive a power loss / kernel panic — not just a
// graceful process exit.
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
	// fsync the temp file so its contents are on stable storage before the
	// rename publishes it as the live save (otherwise a crash could leave the
	// renamed file pointing at unflushed/zeroed data).
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("fsync temp save: %w", err)
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp save: %w", err)
	}
	if err = os.Rename(tmpName, filepath.Join(dir, username+".sav")); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename save: %w", err)
	}
	// fsync the directory so the rename (the new directory entry) is durable.
	// Best-effort: the save is already written and renamed, so a fsync failure
	// here only weakens power-loss durability — it must not fail the save.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// buildLoginResponse constructs a PlayerLoginResponse with account-derived fields populated.
func buildLoginResponse(result loginpb.LoginResult, account *accountRow, save []byte, sessionUUID string) *loginpb.PlayerLoginResponse {
	resp := &loginpb.PlayerLoginResponse{
		Result:        result,
		AccountId:     int32(account.ID),
		StaffModLevel: int32(account.StaffModLevel),
		Members:       account.Members == 1,
		SessionUuid:   sessionUUID,
	}
	if len(save) > 0 {
		resp.Save = save
	}
	if account.MutedUntil.Valid && time.Now().Before(account.MutedUntil.Time) {
		resp.MutedUntil = timestamppb.New(account.MutedUntil.Time)
	}
	return resp
}
