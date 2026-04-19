package world

const (
	userEventLimit       = 5
	clientEventLimit     = 20
	restrictedEventLimit = 2
	afkEventRate         = 500

	modalStateNone = 0x0
	modalStateMain = 0x1
	modalStateChat = 0x2
	modalStateSide = 0x4
)

// Player is the game-side representation of a connected player.
// All fields except client and slot are owned exclusively by the tick goroutine.
type Player struct {
	slot   int     // RS2 player slot 1–2047; assigned by addPlayer
	client *client // network handle; never nil while the player is registered

	// per-tick tracking
	playtime      int
	afkEventReady bool
	lastConnected int
	lastResponse  int

	// per-tick rate-limit counters (reset at start of each processIn call)
	userLimit       int
	clientLimit     int
	restrictedLimit int

	// modal state — drives encodeOut
	modalMain         int
	modalChat         int
	modalSide         int
	lastModalMain     int
	lastModalChat     int
	lastModalSide     int
	modalState        int
	refreshModal      bool
	refreshModalClose bool
}

func newPlayer(c *client) *Player {
	return &Player{client: c}
}
