package internal

import "github.com/zsrv/goscape/pkg/pathfinder/collision"

// Only used in tests

func BuildCollisionMap(x1, z1, x2, z2 int) collision.FlagMap {
	flags := collision.NewFlagMap()
	for level := range 4 {
		for z := min(z1, z2); z <= max(z1, z2); z++ {
			for x := min(x1, x2); x <= max(x1, x2); x++ {
				flags.AllocateIfAbsent(x, z, level)
			}
		}
	}
	return flags
}

func Flag(flags collision.FlagMap, baseX, baseZ, width, length, mask int) {
	for level := range 4 {
		for z := range length {
			for x := range width {
				flags.Set(baseX+x, baseZ+z, level, mask)
			}
		}
	}
}
