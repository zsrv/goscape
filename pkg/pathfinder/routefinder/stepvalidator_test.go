package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
)

func TestValidateClearPath(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ

		// make sure to allocate every zone in between the source
		// and destination coordinates
		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		sv := NewStepValidator(m)
		for level := range 4 {
			if !sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, collision.TypeNormal) {
				t.Fatalf("[dir %d, level %d] should be able to travel", dirEnum, level)
			}
		}
	}
}

func TestFailWhenPathBlocked(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ

		// make sure to allocate every zone in between the source
		// and destination coordinates
		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		for level := range 4 {
			m.Set(destX, destZ, level, collision.FlagLoc)
		}

		sv := NewStepValidator(m)
		for level := range 4 {
			if sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, collision.TypeNormal) {
				t.Fatalf("[dir %d, level %d] should not be able to travel", dirEnum, level)
			}
		}
	}
}

func TestWhenPathBlockedByDynamicExtraFlagParameter(t *testing.T) {
	extraFlags := []int{
		collision.FlagBlockPlayers,
		collision.FlagBlockNPCs,
		collision.FlagBlockPlayers | collision.FlagBlockNPCs,
	}

	for _, f := range extraFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + dir.OffX
			destZ := srcZ + dir.OffZ

			// make sure to allocate every zone in between the source
			// and destination coordinates
			for level := range 4 {
				for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
					for x := min(srcX, destX); x <= max(srcX, destX); x++ {
						m.AllocateIfAbsent(x, z, level)
					}
				}
			}

			for level := range 4 {
				m.Set(destX, destZ, level, f)
			}

			sv := NewStepValidator(m)
			for level := range 4 {
				if sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, f, collision.TypeNormal) {
					t.Fatalf("[dir %d, flag %d, level %d] should not be able to travel", dirEnum, f, level)
				}
			}
		}
	}
}

func TestValidateBlockedStrategyPath(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ

		// need to make sure every tile in between the two coordinates
		// is marked properly, otherwise the collision strategy will not
		// allow diagonal movement
		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.Set(x, z, level, collision.FlagBlockWalk)
				}
			}
		}

		sv := NewStepValidator(m)
		for level := range 4 {
			if !sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, collision.TypeBlocked) {
				t.Fatalf("[dir %d, level %d] should be able to travel", dirEnum, level)
			}
		}
	}
}

func TestValidateIndoorsStrategyPath(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ
		outdoorsX := destX + dir.OffX
		outdoorsZ := destZ + dir.OffZ

		// need to make sure every tile in between the two coordinates
		// is marked properly, otherwise the collision strategy will not
		// allow diagonal movement
		for level := range 4 {
			for z := min(srcZ, min(destZ, outdoorsZ)); z <= max(srcZ, max(destZ, outdoorsZ)); z++ {
				for x := min(srcX, min(destX, outdoorsX)); x <= max(srcX, max(destX, outdoorsX)); x++ {
					m.Set(x, z, level, collision.FlagRoof)
				}
			}
		}

		// overwrite the outdoors tiles to remove indoor flag
		for level := range 4 {
			m.Set(outdoorsX, outdoorsZ, level, collision.FlagOpen)
		}

		sv := NewStepValidator(m)
		strategy := collision.TypeIndoors
		// test step is valid if destination is also flagged as indoors
		for level := range 4 {
			if !sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should be able to travel", dirEnum, level)
			}
		}
		// test step is invalid if destination is not flagged as indoors
		for level := range 4 {
			if sv.CanTravel(level, destX, destZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should not be able to travel", dirEnum, level)
			}
		}
	}
}

func TestValidateOutdoorsStrategyPath(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ
		indoorsX := destX + dir.OffX
		indoorsZ := destZ + dir.OffZ

		// make sure to allocate every zone in between the source
		// and destination coordinates
		for level := range 4 {
			for z := min(srcZ, min(destZ, indoorsZ)); z <= max(srcZ, max(destZ, indoorsZ)); z++ {
				for x := min(srcX, min(destX, indoorsX)); x <= max(srcX, max(destX, indoorsX)); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		// set the indoor tile flags
		for level := range 4 {
			m.Set(indoorsX, indoorsZ, level, collision.FlagRoof)
		}

		sv := NewStepValidator(m)
		strategy := collision.TypeOutdoors
		// test step is valid if destination is not flagged as indoors
		for level := range 4 {
			if !sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should be able to travel", dirEnum, level)
			}
		}
		// test step is invalid if destination is flagged as indoors
		for level := range 4 {
			if sv.CanTravel(level, destX, destZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should not be able to travel", dirEnum, level)
			}
		}
	}
}

func TestValidateLineOfSightStrategyPath(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + dir.OffX
		destZ := srcZ + dir.OffZ
		blockedX := destX + dir.OffX
		blockedZ := destZ + dir.OffZ

		// make sure to allocate every zone in between the source
		// and destination coordinates
		for level := range 4 {
			for z := min(srcZ, min(destZ, blockedZ)); z <= max(srcZ, max(destZ, blockedZ)); z++ {
				for x := min(srcX, min(destX, blockedX)); x <= max(srcX, max(destX, blockedX)); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		// set the indoor tile flags
		for level := range 4 {
			m.Set(blockedX, blockedZ, level, collision.FlagLocProjBlocker)
		}

		sv := NewStepValidator(m)
		strategy := collision.TypeLineOfSight
		// test step is valid if destination is not flagged with projectile block flag
		for level := range 4 {
			if !sv.CanTravel(level, srcX, srcZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should be able to travel", dirEnum, level)
			}
		}
		// test step is invalid if destination is flagged with projectile block flag
		for level := range 4 {
			if sv.CanTravel(level, destX, destZ, dir.OffX, dir.OffZ, 1, 0, strategy) {
				t.Fatalf("[dir %d, level %d] should not be able to travel", dirEnum, level)
			}
		}
	}
}
