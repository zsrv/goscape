package xtea

// Keys returns the 4-word XTEA key for a mapsquare.
//
// Sub-spec 3a always returns zeros. The map pack files in data/pack/client/maps/
// are already decrypted; zero-key decrypt on the client side returns the same
// bytes unchanged. When encrypted distribution is needed, load real per-mapsquare
// keys from maps/xteas.json or similar.
func Keys(mapX, mapZ int) [4]uint32 {
	return [4]uint32{0, 0, 0, 0}
}
