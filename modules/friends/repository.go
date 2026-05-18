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

// AddFriend adds target to username37's friend set. Idempotent.
func (r *Repository) AddFriend(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.friends[username37]
	if !ok {
		set = make(map[uint64]struct{})
		r.friends[username37] = set
	}
	set[target] = struct{}{}
}

// DeleteFriend removes target from username37's friend set. No-op if absent.
func (r *Repository) DeleteFriend(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set, ok := r.friends[username37]; ok {
		delete(set, target)
	}
}

// AddIgnore adds target to username37's ignore set. Idempotent.
func (r *Repository) AddIgnore(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.ignores[username37]
	if !ok {
		set = make(map[uint64]struct{})
		r.ignores[username37] = set
	}
	set[target] = struct{}{}
}

// DeleteIgnore removes target from username37's ignore set. No-op if absent.
func (r *Repository) DeleteIgnore(username37, target uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if set, ok := r.ignores[username37]; ok {
		delete(set, target)
	}
}

// GetFriends returns a copy of the player's friend set in unspecified order.
// Returns nil if the player has no friends.
func (r *Repository) GetFriends(username37 uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.friends[username37]
	if !ok || len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	return out
}

// GetIgnores returns a copy of the player's ignore set in unspecified order.
// Returns nil if the player has no ignores.
func (r *Repository) GetIgnores(username37 uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.ignores[username37]
	if !ok || len(set) == 0 {
		return nil
	}
	out := make([]uint64, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	return out
}

// GetFollowers returns every username that has target in their friend set.
// O(n) scan over the friends map; acceptable for slice 1 since slice 4 is
// the only consumer (broadcastWorldToFollowers fan-out) and it doesn't
// ship yet.
//
// NAI-S1-D-NO-FOLLOWER-BROADCAST — handlers don't call this in slice 1.
// Retired by slice 4.
func (r *Repository) GetFollowers(target uint64) []uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []uint64
	for follower, set := range r.friends {
		if _, ok := set[target]; ok {
			out = append(out, follower)
		}
	}
	return out
}

// IsVisibleTo applies TS visibility rules
// (FriendServerRepository.ts isVisibleTo):
//
//	other.privateChat 0 (ON)      -> always visible
//	other.privateChat 1 (FRIENDS) -> visible only if viewer is in other's friend set
//	other.privateChat 2 (OFF)     -> never visible
//
// If other is not registered, returns false.
func (r *Repository) IsVisibleTo(viewer, other uint64) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ps, ok := r.players[other]
	if !ok {
		return false
	}
	switch ps.privateChat {
	case 0: // ON
		return true
	case 1: // FRIENDS
		if set, ok := r.friends[other]; ok {
			_, isFriend := set[viewer]
			return isFriend
		}
		return false
	default: // OFF or unknown
		return false
	}
}
