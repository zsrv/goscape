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

// InitializeWorld (re)creates the per-world player-count slot, resetting
// playerCount to 0 and setting the limit. Mirrors TS FriendServer
// initializeWorld (FriendServer.ts:412-418) where re-init implicitly
// drops any prior socket binding; here it simply resets the counter.
func (r *Repository) InitializeWorld(worldId int32, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
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
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO friendlist (profile, owner_username37, target_username37)
		 VALUES (?, ?, ?)`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("AddFriend: %w", err)
	}
	return nil
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

// GetFriends returns all target_username37 values in owner's friend list.
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM friendlist
		 WHERE profile = ? AND owner_username37 = ?`,
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

func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO ignorelist (profile, owner_username37, target_username37)
		 VALUES (?, ?, ?)`,
		r.profile, int64(owner), int64(target),
	)
	if err != nil {
		return fmt.Errorf("AddIgnore: %w", err)
	}
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

func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT target_username37 FROM ignorelist
		 WHERE profile = ? AND owner_username37 = ?`,
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

// IsVisibleTo applies TS visibility rules:
//
//	other.privateChat 0 (ON)      -> always visible
//	other.privateChat 1 (FRIENDS) -> visible only if viewer is in other's friend set
//	other.privateChat 2 (OFF)     -> never visible
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
	r.mu.RUnlock()

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
	r.mu.RUnlock()

	switch mode {
	case 0: // ON
		for _, v := range viewers {
			out[v] = true
		}
		return out, nil
	case 1: // FRIENDS
		// Build a parameterized IN clause.
		placeholders := make([]byte, 0, 2*len(viewers))
		args := make([]any, 0, 2+len(viewers))
		args = append(args, r.profile, int64(other))
		for i, v := range viewers {
			if i > 0 {
				placeholders = append(placeholders, ',')
			}
			placeholders = append(placeholders, '?')
			args = append(args, int64(v))
		}
		query := `SELECT target_username37 FROM friendlist
		          WHERE profile = ? AND owner_username37 = ?
		            AND target_username37 IN (` + string(placeholders) + `)`
		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("IsVisibleToMany: %w", err)
		}
		defer rows.Close()

		// Default everyone to false; flip the ones returned.
		for _, v := range viewers {
			out[v] = false
		}
		for rows.Next() {
			var t int64
			if err := rows.Scan(&t); err != nil {
				return nil, fmt.Errorf("IsVisibleToMany scan: %w", err)
			}
			out[uint64(t)] = true
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("IsVisibleToMany rows: %w", err)
		}
		return out, nil
	default: // OFF or unknown
		for _, v := range viewers {
			out[v] = false
		}
		return out, nil
	}
}
