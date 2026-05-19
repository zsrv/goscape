// Package friends hosts the friends-server gRPC module. The in-memory
// repository here is slice 1's persistence stand-in; slice 3 swaps it
// for a SQLite-backed equivalent without changing this method surface.
//
// NAI-S1-D-INMEMORY-REPO — state is lost on restart. Retired by slice 3.
package friends

import (
	"context"
	"database/sql"
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

// AddFriend is wired in Task 6.
func (r *Repository) AddFriend(ctx context.Context, owner, target uint64) error {
	return nil
}

// DeleteFriend is wired in Task 6.
func (r *Repository) DeleteFriend(ctx context.Context, owner, target uint64) error {
	return nil
}

// GetFriends is wired in Task 6.
func (r *Repository) GetFriends(ctx context.Context, owner uint64) ([]uint64, error) {
	return nil, nil
}

// AddIgnore is wired in Task 7.
func (r *Repository) AddIgnore(ctx context.Context, owner, target uint64) error {
	return nil
}

// DeleteIgnore is wired in Task 7.
func (r *Repository) DeleteIgnore(ctx context.Context, owner, target uint64) error {
	return nil
}

// GetIgnores is wired in Task 7.
func (r *Repository) GetIgnores(ctx context.Context, owner uint64) ([]uint64, error) {
	return nil, nil
}

// GetFollowers is wired in Task 8.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — handlers don't call this in slice 1.
// Retired by slice 4.
func (r *Repository) GetFollowers(ctx context.Context, target uint64) ([]uint64, error) {
	return nil, nil
}

// IsVisibleTo is wired in Task 8.
func (r *Repository) IsVisibleTo(ctx context.Context, viewer, other uint64) (bool, error) {
	return false, nil
}
