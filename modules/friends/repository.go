// Package friends hosts the friends-server gRPC module. The Repository
// keeps presence state (worlds, players, privateChat, staffLvl) in
// memory and persists friend / ignore lists to SQLite via *sql.DB. The
// schema lives at modules/friends/migrations/000001_init.up.sql.
package friends

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
)

// repositories is the lazily-populated per-profile Repository registry.
// Mirrors TS 244 FriendServer.repositories[profile]
// (FriendServer.ts:64-67 declaration, :439-447 lazy creation in
// initializeWorld). All Repositories share one *sql.DB; profile scoping
// happens inside each Repository's SQL (r.profile) and in-memory maps.
type repositories struct {
	mu sync.Mutex
	db *sql.DB
	by map[string]*Repository
}

func newRepositories(db *sql.DB) *repositories {
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

// friendListLimit caps both the friend list and the ignore list per owner,
// matching the hardcoded 100 in TS FriendServerRepository (addFriend/addIgnore).
const friendListLimit = 100

// Repository is the friends/ignores/presence store. Presence (worlds,
// players, privateChat, staffLvl) lives in-memory and is guarded by mu.
// Friends and ignores persist to SQLite via db. profile scopes every
// SQL operation, mirroring the TS FriendServerRepository(profile) ctor.
type Repository struct {
	mu      sync.RWMutex
	db      *sql.DB
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

func NewRepository(db *sql.DB, profile string) *Repository {
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

// AddFriend adds target to owner's friend list. Idempotent: a duplicate
// insert (same profile+owner+target PK) is silently ignored.
//
// PORTING.md Arc 18 DB-2: the recheck-then-insert is wrapped in a
// per-call BeginTx so the read-modify-write window cannot interleave
// with a concurrent DeleteFriend.
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	return r.atomicUpsertList(ctx, "friendlist", owner, target, "AddFriend")
}

// DeleteFriend removes target from owner's friend list. No-op if the row
// does not exist.
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM friendlist
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteFriend: %w", err)
	}
	return nil
}

// GetFriends returns all target_username37 values in owner's friend list,
// oldest entry first. The `ORDER BY created ASC` matches TS
// FriendServerRepository.loadFriends (orderBy('f.created', 'asc')) so the
// client renders the friend list in insertion order rather than an
// undefined order. L44.
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM friendlist
		 WHERE profile = ? AND owner_username37 = ?
		 ORDER BY created ASC`,
		r.profile, int64(owner),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFriends: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("GetFriends scan: %w", err)
		}
		out = append(out, uint64(t))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetFriends rows: %w", err)
	}
	return out, nil
}

// AddIgnore mirrors AddFriend but against ignorelist. Idempotent; same
// DB-2 atomic-insert posture (see AddFriend).
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	return r.atomicUpsertList(ctx, "ignorelist", owner, target, "AddIgnore")
}

// atomicUpsertList performs an idempotent insert into one of the
// (friendlist | ignorelist) tables under a serializable tx so a
// concurrent delete cannot interleave between the existence check
// and the insert. table is a hardcoded literal at call sites (not
// user input), so direct interpolation is safe.
func (r *Repository) atomicUpsertList(ctx context.Context, table string, owner, target uint64, op string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var count int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM `+table+`
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("%s: existence check: %w", op, err)
	}
	if count == 0 {
		// M29: enforce the 100-entry cap before inserting a NEW entry, matching
		// TS FriendServerRepository.addFriend/addIgnore (count >= 100 → return).
		// Like TS this is a silent no-op at the cap (not an error); the dup case
		// above already short-circuits, so the cap only gates genuinely new rows.
		var total int
		err = tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM `+table+`
			 WHERE profile = ? AND owner_username37 = ?`,
			r.profile, int64(owner),
		).Scan(&total)
		if err != nil {
			return fmt.Errorf("%s: cap check: %w", op, err)
		}
		if total >= friendListLimit {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("%s: commit: %w", op, err)
			}
			committed = true
			return nil
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO `+table+` (profile, owner_username37, target_username37)
			 VALUES (?, ?, ?)`,
			r.profile, int64(owner), int64(target),
		)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	committed = true
	return nil
}

func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("DeleteIgnore: %w", err)
	}
	return nil
}

// GetIgnores returns all target_username37 values in owner's ignore list,
// oldest entry first — matching TS FriendServerRepository.loadIgnores
// (orderBy('i.created', 'asc')). See GetFriends. L44.
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ?
		 ORDER BY created ASC`,
		r.profile, int64(owner),
	)
	if err != nil {
		return nil, fmt.Errorf("GetIgnores: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("GetIgnores scan: %w", err)
		}
		out = append(out, uint64(t))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetIgnores rows: %w", err)
	}
	return out, nil
}

// GetFollowers returns the username37s of all players who have target in their
// friend list. Uses the idx_friendlist_target index for O(log n) lookup.
//
// Broadcast wiring lives in handler.broadcastWorldToFollowers (slice 4a).
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT owner_username37 FROM friendlist
		 WHERE profile = ? AND target_username37 = ?`,
		r.profile, int64(target),
	)
	if err != nil {
		return nil, fmt.Errorf("GetFollowers: %w", err)
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var o int64
		if err := rows.Scan(&o); err != nil {
			return nil, fmt.Errorf("GetFollowers scan: %w", err)
		}
		out = append(out, uint64(o))
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
			`SELECT COUNT(*) FROM friendlist
			 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
			r.profile, int64(other), int64(viewer),
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
// Mirrors TS playerStaff membership (FriendServerRepository.ts:82-84). Caller
// must hold r.mu (read or write).
func (r *Repository) isStaffLocked(username37 uint64) bool {
	ps, ok := r.players[username37]
	return ok && ps.staffLvl > 1
}

// isIgnoredBy reports whether owner has target on its ignorelist. Mirrors TS
// playerIgnores[other].includes(viewer) (FriendServerRepository.ts:340).
func (r *Repository) isIgnoredBy(ctx context.Context, owner, target uint64) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ? AND target_username37 = ?`,
		r.profile, int64(owner), int64(target),
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
	ignored, err := r.targetsAmong(ctx, "ignorelist", other, viewers)
	if err != nil {
		return nil, fmt.Errorf("IsVisibleToMany: %w", err)
	}

	// For FRIENDS mode, the set of viewers in other's friend list.
	var friends map[uint64]bool
	if mode == 1 {
		friends, err = r.targetsAmong(ctx, "friendlist", other, viewers)
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

// targetsAmong returns the subset of candidates present as target_username37
// in the given list table (friendlist | ignorelist) for the given owner under
// r.profile, via a single parameterized IN query. Used by IsVisibleToMany to
// avoid N+1 round trips. table is a trusted internal constant, never user input.
func (r *Repository) targetsAmong(ctx context.Context, table string, owner uint64, candidates []uint64) (map[uint64]bool, error) {
	found := make(map[uint64]bool, len(candidates))
	if len(candidates) == 0 {
		return found, nil
	}
	placeholders := make([]byte, 0, 2*len(candidates))
	args := make([]any, 0, 2+len(candidates))
	args = append(args, r.profile, int64(owner))
	for i, c := range candidates {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, int64(c))
	}
	query := `SELECT target_username37 FROM ` + table + `
	          WHERE profile = ? AND owner_username37 = ?
	            AND target_username37 IN (` + string(placeholders) + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("targetsAmong(%s): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("targetsAmong(%s) scan: %w", table, err)
		}
		found[uint64(t)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("targetsAmong(%s) rows: %w", table, err)
	}
	return found, nil
}

// LogPrivateMessage appends one row to private_chat under r.profile.
// Mirrors TS FriendServer.ts:273-283 — append-only, no dedupe, no
// validation. Insert is the synchronous gate for PrivateMessage
// delivery: a failure here returns an error to the handler which
// surfaces codes.Internal to the caller, matching the TS thrown-
// await pattern.
//
// No account-existence check on from/to — see handler.PrivateMessage's
// NAI-S4A-D-FED-NO-ACCOUNT-EXISTENCE-CHECK block; in TS the throw on a
// missing account would drop the PM without persisting, but the
// federation choice (DB-2, db.go:21-35) makes the equivalent
// cross-service RPC undesirable and the persistence + delivery is
// well-behaved without it (orphan rows are read-side-tolerated;
// undeliverable PMs no-op at h.subs.send).
func (r *Repository) LogPrivateMessage(ctx context.Context, from, to uint64, coord int32, message string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO private_chat (profile, from_username37, to_username37, coord, message)
		 VALUES (?, ?, ?, ?, ?)`,
		r.profile, int64(from), int64(to), coord, message,
	)
	if err != nil {
		return fmt.Errorf("LogPrivateMessage: %w", err)
	}
	return nil
}

// LogPublicMessage appends one row to public_chat under r.profile.
// Mirrors TS FriendServer.ts:286-297 — append-only, no dedupe, no
// validation, no session_uuid existence check. Insert is the
// synchronous gate for the PublicMessage RPC: a failure here returns
// an error to the handler which surfaces codes.Internal to the caller,
// matching the TS thrown-await pattern and slice 6's posture.
func (r *Repository) LogPublicMessage(ctx context.Context, sessionUUID string, coord int32, message string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO public_chat (profile, session_uuid, coord, message)
		 VALUES (?, ?, ?, ?)`,
		r.profile, sessionUUID, coord, message,
	)
	if err != nil {
		return fmt.Errorf("LogPublicMessage: %w", err)
	}
	return nil
}
