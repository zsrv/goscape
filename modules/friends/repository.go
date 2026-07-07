// Package friends hosts the friends-server gRPC module. The Repository
// keeps presence state (worlds, players, privateChat, staffLvl) in
// memory and persists friend / ignore lists to the central database via
// *gamedb.DB. The schema lives at pkg/gamedb/migrations/.
package friends

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zsrv/goscape/pkg/gamedb"
	jstring "github.com/zsrv/goscape/pkg/util/jstring"
)

// errAccountMissing is LogPrivateMessage's sentinel for "an endpoint
// account does not exist". Mirrors TS FriendServer.ts:270-271, where
// executeTakeFirstOrThrow throws and the outer catch drops the PM.
// The handler maps it to a silent drop (no delivery, success RPC).
var errAccountMissing = errors.New("account missing")

// friendListLimit / membersFriendListLimit cap the friend list per
// owner: TS 274 is members-aware — `account.members ? 200 : 100`
// (FriendServerRepository.ts:229). ignoreListLimit stays a flat 100
// (FriendServerRepository.ts:270).
const (
	friendListLimit        = 100
	membersFriendListLimit = 200
	ignoreListLimit        = 100
)

// repositories is the lazily-populated per-profile Repository registry.
// Mirrors TS 244 FriendServer.repositories[profile]
// (FriendServer.ts:64-67 declaration, :439-447 lazy creation in
// initializeWorld). All Repositories share one *gamedb.DB; profile
// scoping happens inside each Repository's SQL (r.profile) and in-memory
// maps.
type repositories struct {
	mu sync.Mutex
	db *gamedb.DB
	by map[string]*Repository
}

func newRepositories(db *gamedb.DB) *repositories {
	return &repositories{db: db, by: make(map[string]*Repository)}
}

// get returns the profile's Repository, creating it on first use
// (TS FriendServer.ts:443-445 `if (!this.repositories[profile])`).
func (rs *repositories) get(profile string) *Repository {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r, ok := rs.by[profile]
	if !ok {
		r = NewRepository(rs.db, profile)
		rs.by[profile] = r
	}
	return r
}

// Repository is the friends/ignores/presence store. Presence (worlds,
// players, privateChat, staffLvl) lives in-memory and is guarded by mu.
// Friends and ignores persist to the central database via db. profile
// scopes every SQL operation, mirroring the TS FriendServerRepository(profile)
// ctor.
type Repository struct {
	mu      sync.RWMutex
	db      *gamedb.DB
	profile string
	worlds  map[int32]*worldState
	players map[uint64]*playerState
}

type worldState struct {
	playerCount int
	limit       int
}

// playerState tracks a logged-in player's world placement and chat-mode
// settings. privateChat is the TS ChatModePrivate enum: 0=ON, 1=FRIENDS,
// 2=OFF (see Engine-TS ChatModes.ts).
type playerState struct {
	worldId     int32
	privateChat int32
	staffLvl    int32
}

func NewRepository(db *gamedb.DB, profile string) *Repository {
	return &Repository{
		db:      db,
		profile: profile,
		worlds:  make(map[int32]*worldState),
		players: make(map[uint64]*playerState),
	}
}

// GetWorld returns the world id the player is logged in to, or 0 if the
// player is not currently registered.
func (r *Repository) GetWorld(username37 uint64) int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ps, ok := r.players[username37]; ok {
		return ps.worldId
	}
	return 0
}

// InitializeWorld creates the per-world player-count slot on first
// connect; a duplicate WORLD_CONNECT is a no-op that preserves the
// existing playerCount. Mirrors TS FriendServerRepository.initializeWorld
// (FriendServerRepository.ts:48-54), which early-returns
// `if (this.playersByWorld[world]) return;` so a re-init never zeroes
// the in-memory player table. (The surrounding wrapper
// FriendServer.initializeWorld at FriendServer.ts:412-418 drops the
// prior socket binding, which goscape models separately at
// worldSubscriptions.register — see NAI-S4A-D-PERPLAYER-NOT-PERWORLD-STREAM.)
func (r *Repository) InitializeWorld(worldId int32, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.worlds[worldId]; ok {
		return
	}
	r.worlds[worldId] = &worldState{playerCount: 0, limit: limit}
}

// initializeWorldIfAbsent is the lazy-init variant of InitializeWorld:
// it only creates the world slot if it does not already exist, leaving
// existing playerCount untouched. Used by ensureWorld in the handler
// for the TS-faithful lazy-init paths (non-WorldConnect first message
// from an unknown world).
func (r *Repository) initializeWorldIfAbsent(worldId int32, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.worlds[worldId]; ok {
		return
	}
	r.worlds[worldId] = &worldState{playerCount: 0, limit: limit}
}

// Register places the player on the given world. Returns false iff the
// world's playerCount has already hit its limit, or the world has not
// been initialized. Callers must Unregister the player first to dedupe
// across worlds (TS does this in PLAYER_LOGIN, FriendServer.ts:125-127).
func (r *Repository) Register(worldId int32, username37 uint64, privateChat, staffLvl int32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	ws, ok := r.worlds[worldId]
	if !ok {
		return false
	}
	if ws.playerCount >= ws.limit {
		return false
	}
	r.players[username37] = &playerState{
		worldId:     worldId,
		privateChat: privateChat,
		staffLvl:    staffLvl,
	}
	ws.playerCount++
	return true
}

// Unregister removes the player from whichever world they're on and
// decrements that world's playerCount. No-op if the player is not
// registered (TS FriendServer unregister is also a no-op on miss).
func (r *Repository) Unregister(username37 uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ps, ok := r.players[username37]
	if !ok {
		return
	}
	if ws, ok := r.worlds[ps.worldId]; ok && ws.playerCount > 0 {
		ws.playerCount--
	}
	delete(r.players, username37)
}

// SetChatMode updates the player's privateChat setting. No-op if the
// player is not registered.
func (r *Repository) SetChatMode(username37 uint64, privateChat int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ps, ok := r.players[username37]; ok {
		ps.privateChat = privateChat
	}
}

// GetChatMode returns the player's privateChat setting, or 0 (ON) if the
// player is not registered.
func (r *Repository) GetChatMode(username37 uint64) int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if ps, ok := r.players[username37]; ok {
		return ps.privateChat
	}
	return 0
}

// accountID resolves username37 to its account row id via the central
// database — the friend server verifying a username IS this query
// (TS FriendServerRepository.ts:216-217/256; FriendServer.ts:270-271).
// ok=false with nil error when the account does not exist.
func (r *Repository) accountID(ctx context.Context, username37 uint64) (int64, bool, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(username37),
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("accountID: %w", err)
	}
	return id, true, nil
}

// AddFriend adds target to owner's friend list, resolving both accounts
// against the central database like TS FriendServerRepository.addFriend
// (FriendServerRepository.ts:204-245): either account missing → silent
// no-op; duplicate → no-op; cap members ? 200 : 100. TS counts the cap
// across ALL profiles (no profile filter on the count query,
// FriendServerRepository.ts:224-228) — quirk mirrored. The whole
// read-modify-write runs in one tx so a concurrent DeleteFriend cannot
// interleave (retained from the DB-2-era fix).
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddFriend: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var ownerID, members int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id, members FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(owner),
	).Scan(&ownerID, &members)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :219-222 — missing owner, drop silently
	}
	if err != nil {
		return fmt.Errorf("AddFriend: resolve owner: %w", err)
	}

	var targetID int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(target),
	).Scan(&targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :219-222 — missing target, drop silently
	}
	if err != nil {
		return fmt.Errorf("AddFriend: resolve target: %w", err)
	}

	var dup int
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE profile = ? AND account_id = ? AND friend_account_id = ?`),
		r.profile, ownerID, targetID,
	).Scan(&dup)
	if err != nil {
		return fmt.Errorf("AddFriend: dup check: %w", err)
	}
	if dup == 0 {
		var total int
		err = tx.QueryRowContext(ctx,
			r.db.Rebind(`SELECT COUNT(*) FROM friendlist WHERE account_id = ?`),
			ownerID,
		).Scan(&total)
		if err != nil {
			return fmt.Errorf("AddFriend: cap check: %w", err)
		}
		limit := friendListLimit
		if members != 0 {
			limit = membersFriendListLimit
		}
		if total >= limit {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("AddFriend: commit: %w", err)
			}
			committed = true
			return nil
		}
		if _, err = tx.ExecContext(ctx,
			r.db.Rebind(`INSERT INTO friendlist (profile, account_id, friend_account_id) VALUES (?, ?, ?)`),
			r.profile, ownerID, targetID,
		); err != nil {
			if gamedb.IsForeignKeyViolation(err) {
				// Account deleted between resolve and insert (possible
				// under Postgres read-committed) — same outcome as the
				// TS missing-account path: drop silently (spec §Error
				// handling). Deferred rollback cleans up.
				return nil
			}
			return fmt.Errorf("AddFriend: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("AddFriend: commit: %w", err)
	}
	committed = true
	return nil
}

// DeleteFriend removes target from owner's friend list via account
// subqueries (TS FriendServerRepository.deleteFriend,
// FriendServerRepository.ts:196-201). No-op when either username has
// no account or the row does not exist.
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.Rebind(`DELETE FROM friendlist
		 WHERE profile = ?
		   AND account_id IN (SELECT id FROM account WHERE username = ?)
		   AND friend_account_id IN (SELECT id FROM account WHERE username = ?)`),
		r.profile, jstring.FromBase37(owner), jstring.FromBase37(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteFriend: %w", err)
	}
	return nil
}

// GetFriends returns owner's friend list as username37s, oldest entry
// first. Mirrors TS loadFriends' double INNER JOIN + orderBy f.created
// asc (FriendServerRepository.ts:356-370).
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT a.username FROM account AS a
		 INNER JOIN friendlist AS f ON a.id = f.friend_account_id
		 INNER JOIN account AS local ON local.id = f.account_id
		 WHERE local.username = ? AND f.profile = ?
		 ORDER BY f.created ASC`),
		jstring.FromBase37(owner), r.profile,
	)
	if err != nil {
		return nil, fmt.Errorf("GetFriends: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("GetFriends scan: %w", err)
		}
		out = append(out, jstring.ToBase37(u))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFriends rows: %w", err)
	}
	return out, nil
}

// AddIgnore mirrors TS addIgnore (FriendServerRepository.ts:247-296):
// resolves the OWNER only (missing → no-op); the target is stored as a
// raw username string with NO existence check — ignoring a player who
// doesn't exist is allowed. Cap 100, counted across ALL profiles (TS
// quirk, :264-268). ON CONFLICT DO NOTHING matches TS's sqlite branch
// (:284-285) and is valid on both goscape backends.
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("AddIgnore: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var ownerID int64
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT id FROM account WHERE username = ? LIMIT 1`),
		jstring.FromBase37(owner),
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // TS :258-260
	}
	if err != nil {
		return fmt.Errorf("AddIgnore: resolve owner: %w", err)
	}

	var total int
	err = tx.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM ignorelist WHERE account_id = ?`),
		ownerID,
	).Scan(&total)
	if err != nil {
		return fmt.Errorf("AddIgnore: cap check: %w", err)
	}
	if total >= ignoreListLimit {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("AddIgnore: commit: %w", err)
		}
		committed = true
		return nil
	}

	if _, err = tx.ExecContext(ctx,
		r.db.Rebind(`INSERT INTO ignorelist (profile, account_id, value) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`),
		r.profile, ownerID, jstring.FromBase37(target),
	); err != nil {
		if gamedb.IsForeignKeyViolation(err) {
			return nil // owner deleted mid-flight — TS missing-owner outcome
		}
		return fmt.Errorf("AddIgnore: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("AddIgnore: commit: %w", err)
	}
	committed = true
	return nil
}

// DeleteIgnore removes value from owner's ignore list
// (TS FriendServerRepository.deleteIgnore, :310-315: where profile,
// value, account-subquery).
func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		r.db.Rebind(`DELETE FROM ignorelist
		 WHERE profile = ? AND value = ?
		   AND account_id IN (SELECT id FROM account WHERE username = ?)`),
		r.profile, jstring.FromBase37(target), jstring.FromBase37(owner),
	)
	if err != nil {
		return fmt.Errorf("DeleteIgnore: %w", err)
	}
	return nil
}

// GetIgnores returns owner's ignore list as username37s, oldest first
// (TS loadIgnores, :372-386: join owner account, select i.value,
// orderBy i.created asc; values round-trip through toBase37).
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT i.value FROM account AS local
		 INNER JOIN ignorelist AS i ON local.id = i.account_id
		 WHERE local.username = ? AND i.profile = ?
		 ORDER BY i.created ASC`),
		jstring.FromBase37(owner), r.profile,
	)
	if err != nil {
		return nil, fmt.Errorf("GetIgnores: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("GetIgnores scan: %w", err)
		}
		out = append(out, jstring.ToBase37(v))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetIgnores rows: %w", err)
	}
	return out, nil
}

// GetFollowers returns the username37s of all players who have target
// in their friend list. TS computes this from its in-memory cache
// (FriendServerRepository.ts:176-180); goscape keeps its established
// SQL mechanism, now id-keyed, backed by idx_friendlist_friend.
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT local.username FROM friendlist AS f
		 INNER JOIN account AS local ON local.id = f.account_id
		 INNER JOIN account AS a ON a.id = f.friend_account_id
		 WHERE f.profile = ? AND a.username = ?`),
		r.profile, jstring.FromBase37(target),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFollowers: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("GetFollowers scan: %w", err)
		}
		out = append(out, jstring.ToBase37(u))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFollowers rows: %w", err)
	}
	return out, nil
}

// IsVisibleTo applies TS visibility rules (FriendServerRepository.isVisibleTo,
// FriendServerRepository.ts:332-355), in order:
//
//  1. viewer is staff (staffLvl > 1)   -> always visible
//  2. other has ignored viewer         -> never visible
//  3. other.privateChat 0 (ON)         -> always visible
//     other.privateChat 1 (FRIENDS)    -> visible only if viewer is in other's friend set
//     other.privateChat 2 (OFF)        -> never visible
//
// If other is not registered (no presence row), returns (false, nil).
//
// Locking discipline: r.mu is released before any SQL call to avoid holding
// the in-memory mutex across I/O.
func (r *Repository) IsVisibleTo(ctx context.Context, viewer, other uint64) (bool, error) {
	r.mu.RLock()
	ps, ok := r.players[other]
	if !ok {
		r.mu.RUnlock()
		return false, nil
	}
	mode := ps.privateChat
	viewerStaff := r.isStaffLocked(viewer)
	r.mu.RUnlock()

	// 1. Staff see everyone (TS playerStaff = registered with staffLvl > 1).
	if viewerStaff {
		return true, nil
	}

	// 2. If other has ignored viewer, other's online status is hidden.
	ignored, err := r.isIgnoredBy(ctx, other, viewer)
	if err != nil {
		return false, err
	}
	if ignored {
		return false, nil
	}

	switch mode {
	case 0: // ON
		return true, nil
	case 1: // FRIENDS
		var count int
		err := r.db.QueryRowContext(ctx,
			r.db.Rebind(`SELECT COUNT(*) FROM friendlist AS f
			 INNER JOIN account AS local ON local.id = f.account_id
			 INNER JOIN account AS a ON a.id = f.friend_account_id
			 WHERE f.profile = ? AND local.username = ? AND a.username = ?`),
			r.profile, jstring.FromBase37(other), jstring.FromBase37(viewer),
		).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("IsVisibleTo: %w", err)
		}
		return count > 0, nil
	default: // OFF or unknown
		return false, nil
	}
}

// isStaffLocked reports whether username37 is registered with staffLvl > 1.
// Mirrors TS playerStaff membership (FriendServerRepository.ts:82-83 @dee467c8).
// Caller must hold r.mu (read or write).
func (r *Repository) isStaffLocked(username37 uint64) bool {
	ps, ok := r.players[username37]
	return ok && ps.staffLvl > 1
}

// isIgnoredBy reports whether owner has target on its ignore list
// (TS playerIgnores[other].includes(viewer), FriendServerRepository.ts:339).
func (r *Repository) isIgnoredBy(ctx context.Context, owner, target uint64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		r.db.Rebind(`SELECT COUNT(*) FROM ignorelist AS i
		 INNER JOIN account AS local ON local.id = i.account_id
		 WHERE i.profile = ? AND local.username = ? AND i.value = ?`),
		r.profile, jstring.FromBase37(owner), jstring.FromBase37(target),
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("isIgnoredBy: %w", err)
	}
	return count > 0, nil
}

// IsVisibleToMany is the batched analogue of IsVisibleTo. Returns a
// map[viewer]bool with one entry per input viewer. The empty result is
// a valid response — callers must check the map, not nil.
//
// Locking discipline: same as IsVisibleTo — r.mu is released before any
// SQL call.
//
// Algorithm:
//
//	other.privateChat 0 (ON)      -> all viewers true
//	other.privateChat 1 (FRIENDS) -> one SQL IN query against friendlist
//	                                 where owner = other and target IN
//	                                 viewers; viewers in result are true
//	other.privateChat 2 (OFF)     -> all viewers false
//	other has no presence row     -> all viewers false
//
// Slice 4a uses this from handler.broadcastWorldToFollowers to avoid
// the N+1 round trips that a scalar-IsVisibleTo loop would incur.
func (r *Repository) IsVisibleToMany(ctx context.Context, viewers []uint64, other uint64) (map[uint64]bool, error) {
	out := make(map[uint64]bool, len(viewers))
	if len(viewers) == 0 {
		return out, nil
	}

	r.mu.RLock()
	ps, ok := r.players[other]
	if !ok {
		r.mu.RUnlock()
		for _, v := range viewers {
			out[v] = false
		}
		return out, nil
	}
	mode := ps.privateChat
	staff := make(map[uint64]bool, len(viewers))
	for _, v := range viewers {
		if r.isStaffLocked(v) {
			staff[v] = true
		}
	}
	r.mu.RUnlock()

	// Viewers that `other` has ignored — hidden regardless of chat mode.
	ignored, err := r.ignoreValuesAmong(ctx, other, viewers)
	if err != nil {
		return nil, fmt.Errorf("IsVisibleToMany: %w", err)
	}

	// For FRIENDS mode, the set of viewers in other's friend list.
	var friends map[uint64]bool
	if mode == 1 {
		friends, err = r.friendTargetsAmong(ctx, other, viewers)
		if err != nil {
			return nil, fmt.Errorf("IsVisibleToMany: %w", err)
		}
	}

	for _, v := range viewers {
		switch {
		case staff[v]: // 1. staff see everyone
			out[v] = true
		case ignored[v]: // 2. other ignores viewer
			out[v] = false
		case mode == 0: // ON
			out[v] = true
		case mode == 1: // FRIENDS
			out[v] = friends[v]
		default: // OFF or unknown
			out[v] = false
		}
	}
	return out, nil
}

// friendTargetsAmong returns the subset of candidates present in
// owner's friend list, via one IN query over usernames (id-keyed
// analogue of the old username37 IN probe; avoids N+1).
func (r *Repository) friendTargetsAmong(ctx context.Context, owner uint64, candidates []uint64) (map[uint64]bool, error) {
	found := make(map[uint64]bool, len(candidates))
	if len(candidates) == 0 {
		return found, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, 0, 2+len(candidates))
	args = append(args, r.profile, jstring.FromBase37(owner))
	for _, c := range candidates {
		args = append(args, jstring.FromBase37(c))
	}
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT a.username FROM friendlist AS f
		 INNER JOIN account AS local ON local.id = f.account_id
		 INNER JOIN account AS a ON a.id = f.friend_account_id
		 WHERE f.profile = ? AND local.username = ? AND a.username IN (`+placeholders+`)`),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("friendTargetsAmong: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return nil, fmt.Errorf("friendTargetsAmong scan: %w", err)
		}
		found[jstring.ToBase37(u)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("friendTargetsAmong rows: %w", err)
	}
	return found, nil
}

// ignoreValuesAmong is friendTargetsAmong against ignorelist.value
// (raw username strings, no target join).
func (r *Repository) ignoreValuesAmong(ctx context.Context, owner uint64, candidates []uint64) (map[uint64]bool, error) {
	found := make(map[uint64]bool, len(candidates))
	if len(candidates) == 0 {
		return found, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(candidates)), ",")
	args := make([]any, 0, 2+len(candidates))
	args = append(args, r.profile, jstring.FromBase37(owner))
	for _, c := range candidates {
		args = append(args, jstring.FromBase37(c))
	}
	rows, err := r.db.QueryContext(ctx,
		r.db.Rebind(`SELECT i.value FROM ignorelist AS i
		 INNER JOIN account AS local ON local.id = i.account_id
		 WHERE i.profile = ? AND local.username = ? AND i.value IN (`+placeholders+`)`),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("ignoreValuesAmong: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("ignoreValuesAmong scan: %w", err)
		}
		found[jstring.ToBase37(v)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ignoreValuesAmong rows: %w", err)
	}
	return found, nil
}

// LogPrivateMessage persists a PM keyed by resolved account ids
// (TS FriendServer.ts:266-284: resolve from + to via
// executeTakeFirstOrThrow, insert {account_id, profile, to_account_id,
// timestamp, coord, message}). Either endpoint missing →
// errAccountMissing: the handler drops the PM silently, matching the
// TS throw-and-catch. timestamp uses the column DEFAULT (equivalent to
// TS's explicit toDbDate(Date.now())).
func (r *Repository) LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error {
	fromID, ok, err := r.accountID(ctx, from)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	if !ok {
		return fmt.Errorf("LogPrivateMessage from %d: %w", from, errAccountMissing)
	}
	toID, ok, err := r.accountID(ctx, to)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	if !ok {
		return fmt.Errorf("LogPrivateMessage to %d: %w", to, errAccountMissing)
	}
	if _, err := r.db.ExecContext(ctx,
		r.db.Rebind(`INSERT INTO private_chat (account_id, profile, coord, to_account_id, message)
		 VALUES (?, ?, ?, ?, ?)`),
		fromID, r.profile, coord, toID, message,
	); err != nil {
		if gamedb.IsForeignKeyViolation(err) {
			// Endpoint deleted between resolve and insert — same
			// outcome as the TS missing-account throw: drop.
			return fmt.Errorf("LogPrivateMessage: %w", errAccountMissing)
		}
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	return nil
}
