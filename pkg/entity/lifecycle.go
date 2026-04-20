package entity

// Lifecycle describes how a non-pathing entity comes into and goes out of
// existence. Locs and Objs both use this.
type Lifecycle int

const (
	LifecycleForever Lifecycle = iota // statics — never despawn
	LifecycleRespawn                  // engine-added; comes back after a timer
	LifecycleDespawn                  // script-added; goes away after a timer
)
