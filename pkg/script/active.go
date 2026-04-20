package script

// ActivePlayer is the minimal surface RuneScript needs from a Player.
// Sub-spec S2 wires modules/world.Player to this interface.
type ActivePlayer interface {
	MessageGame(msg string)
	Username() string
}

// Stubs for later sub-specs; defined now to avoid interface churn in S6.
type ActiveNpc interface{}
type ActiveLoc interface{}
type ActiveObj interface{}
