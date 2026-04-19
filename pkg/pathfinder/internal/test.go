package internal

import "github.com/zsrv/goscape/pkg/pathfinder/collision"

// Only used in tests

func BuildCollisionMap(x1, z1, x2, z2 int) collision.FlagMap {
	flags := collision.NewFlagMap()
	for level := 0; level < 4; level++ {
		for z := min(z1, z2); z <= max(z1, z2); z++ {
			for x := min(x1, x2); x <= max(x1, x2); x++ {
				flags.AllocateIfAbsent(x, z, level)
			}
		}
	}
	return flags
}

func Flag(flags collision.FlagMap, baseX, baseZ, width, length, mask int) {
	for level := 0; level < 4; level++ {
		for z := 0; z < length; z++ {
			for x := 0; x < width; x++ {
				flags.Set(baseX+x, baseZ+z, level, mask)
			}
		}
	}
}
