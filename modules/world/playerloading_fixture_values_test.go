package world

// Field values that match the tsx fixture generator (see
// testdata/playerloading/README.md). Used by player_save_test.go for
// per-version decode assertions.
//
// Keep in lock-step with testdata/playerloading/README.md and the tsx
// fixture-generation script.

var fixturePlayerValues = struct {
	X, Z, Level     int
	Body            [7]int
	Colors          [5]int
	Gender          int
	Runenergy       int
	PlaytimeV1      int // u16-fitting
	PlaytimeV2Plus  int // 4-byte
	Stats           [21]int32
	AfkZones        [2]int32
	LastAfkZone     int
	PublicChat      int
	PrivateChat     int
	TradeDuel       int
	PackedChatModes uint8 // (1<<4)|(2<<2)|0
	LastLoginTime   int64
}{
	X: 3094, Z: 3106, Level: 0,
	Body:   [7]int{0, 10, 18, 26, 33, 36, 42},
	Colors: [5]int{3, 7, 11, 13, 17},
	Gender:         0,
	Runenergy:      10000,
	PlaytimeV1:     12345,
	PlaytimeV2Plus: 1234567,
	Stats: func() (s [21]int32) {
		for i := range 21 {
			s[i] = int32(i) * 1000
		}
		return
	}(),
	AfkZones:        [2]int32{200, 300},
	LastAfkZone:     42,
	PublicChat:      1,
	PrivateChat:     2,
	TradeDuel:       0,
	PackedChatModes: 0x18,
	LastLoginTime:   1715200000000,
}
