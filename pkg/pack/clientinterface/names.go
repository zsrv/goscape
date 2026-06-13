// pkg/pack/clientinterface/names.go
package clientinterface

// nameToType ports PackShared.ts:7-26.
func nameToType(name string) int {
	switch name {
	case "layer", "overlay":
		return 0
	case "inv":
		return 2
	case "rect":
		return 3
	case "text":
		return 4
	case "graphic":
		return 5
	case "model":
		return 6
	case "invtext":
		return 7
	}
	return -1
}

// nameToButtonType ports PackShared.ts:29-46.
func nameToButtonType(name string) int {
	switch name {
	case "normal":
		return 1
	case "target":
		return 2
	case "close":
		return 3
	case "toggle":
		return 4
	case "select":
		return 5
	case "pause":
		return 6
	}
	return 0
}

// nameToComparator ports PackShared.ts:48-61.
func nameToComparator(name string) int {
	switch name {
	case "eq":
		return 1
	case "lt":
		return 2
	case "gt":
		return 3
	case "neq":
		return 4
	}
	return 0
}

// nameToScript ports PackShared.ts:63-108 @ 2e3bcf43 (ops 14-20 —
// push_varbit, subtract, divide, multiply, coordx, coordz,
// push_constant — joined at rev-254).
func nameToScript(name string) int {
	switch name {
	case "stat_level":
		return 1
	case "stat_base_level":
		return 2
	case "stat_xp":
		return 3
	case "inv_count":
		return 4
	case "pushvar":
		return 5
	case "stat_xp_remaining":
		return 6
	case "op7":
		return 7
	case "op8":
		return 8
	case "op9":
		return 9
	case "inv_contains":
		return 10
	case "runenergy":
		return 11
	case "runweight":
		return 12
	case "testbit":
		return 13
	case "push_varbit":
		return 14
	case "subtract":
		return 15
	case "divide":
		return 16
	case "multiply":
		return 17
	case "coordx":
		return 18
	case "coordz":
		return 19
	case "push_constant":
		return 20
	}
	return 0
}

// nameToStat ports PackShared.ts:96-139.
func nameToStat(name string) int {
	switch name {
	case "attack":
		return 0
	case "defence":
		return 1
	case "strength":
		return 2
	case "hitpoints":
		return 3
	case "ranged":
		return 4
	case "prayer":
		return 5
	case "magic":
		return 6
	case "cooking":
		return 7
	case "woodcutting":
		return 8
	case "fletching":
		return 9
	case "fishing":
		return 10
	case "firemaking":
		return 11
	case "crafting":
		return 12
	case "smithing":
		return 13
	case "mining":
		return 14
	case "herblore":
		return 15
	case "agility":
		return 16
	case "thieving":
		return 17
	case "runecraft":
		return 20
	}
	return -1
}

// nameToFont ports interface/PackShared.ts:154-167 @ dee467c8 (rev-274
// renamed the four font asset names to *_full).
func nameToFont(name string) int {
	switch name {
	case "p11_full":
		return 0
	case "p12_full":
		return 1
	case "b12_full":
		return 2
	case "q8_full":
		return 3
	}
	return -1
}
