// Package friends hosts the friends-server gRPC module. The in-memory
// repository here is slice 1's persistence stand-in; slice 3 swaps it
// for a SQLite-backed equivalent without changing this method surface.
//
// NAI-S1-D-INMEMORY-REPO — state is lost on restart. Retired by slice 3.
package friends

import "sync"

// Repository is the in-memory friend/ignore/presence store. All methods
// are safe for concurrent use; a single sync.RWMutex guards every map.
// No I/O happens inside critical sections.
type Repository struct {
	mu      sync.RWMutex
	worlds  map[int32]*worldState
	players map[uint64]*playerState
	friends map[uint64]map[uint64]struct{}
	ignores map[uint64]map[uint64]struct{}
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

func NewRepository() *Repository {
	return &Repository{
		worlds:  make(map[int32]*worldState),
		players: make(map[uint64]*playerState),
		friends: make(map[uint64]map[uint64]struct{}),
		ignores: make(map[uint64]map[uint64]struct{}),
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
