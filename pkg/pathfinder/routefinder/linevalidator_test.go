package routefinder

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/collision"
	"github.com/zsrv/goscape/pkg/pathfinder/flag"
)

func TestLineValidatorLineOfSightValidWhenOnTopOfBlockingCollisionFlagIfTargetOnSameCoordinates(t *testing.T) {
	m := collision.NewFlagMap()
	x, z := 3200, 3200
	for level := range 4 {
		m.Set(x, z, level, collision.FlagLoc)
	}

	lv := NewLineValidator(m)
	for level := range 4 {
		if !lv.HasLineOfSight(level, x, z, x, z, 1, 0, 0, 0) {
			t.Fatalf("[level %d] expected to have line of sight", level)
		}
	}
}

func TestLineValidatorLineOfSightValidWhenTargetCoordinateIsMarkedWithExtraFlagCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)
	lv := NewLineValidator(m)
	if !lv.HasLineOfSight(0, 3200, 3202, 3200, 3200, 1, 1, 1, collision.FlagBlockPlayers) {
		t.Fatal("expected to have line of sight")
	}
}

func TestLineValidatorLineOfSightFailWhenBlockedByExtraFlagBeforeReachingTarget(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)
	lv := NewLineValidator(m)
	if lv.HasLineOfSight(0, 3200, 3202, 3200, 3199, 1, 1, 1, collision.FlagBlockPlayers) {
		t.Fatal("expected not to have line of sight")
	}
}

func TestLineValidatorLineOfSightFailWhenOnTopOfBlockingCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagLoc)
	lv := NewLineValidator(m)
	if lv.HasLineOfSight(0, 3200, 3200, 3200, 3201, 1, 0, 0, 0) {
		t.Fatal("expected not to have line of sight")
	}
}

func TestLineValidatorLineOfSightFailWhenOnTopOfExtraFlagCollisionFlag(t *testing.T) {
	m := collision.NewFlagMap()
	m.Add(3200, 3200, 0, collision.FlagBlockPlayers)
	lv := NewLineValidator(m)
	if lv.HasLineOfSight(0, 3200, 3200, 3200, 3201, 1, 0, 0, collision.FlagBlockPlayers) {
		t.Fatal("expected not to have line of sight")
	}
}

func TestLineValidatorLineOfSightValidWhenOnTopOfTarget(t *testing.T) {
	m := collision.NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)
	lv := NewLineValidator(m)
	if !lv.HasLineOfSight(0, 3200, 3200, 3200, 3200, 1, 0, 0, 0) {
		t.Fatal("expected to have line of sight")
	}
}

func TestLineValidatorLineOfSightValidWhenPassingThroughNonBlockingCollisionFlags(t *testing.T) {
	validFlags := []int{
		collision.FlagOpen,
		collision.FlagBlockWalk,
		collision.FlagGroundDecor,
		collision.FlagBlockWalk | collision.FlagGroundDecor,
	}

	for _, f := range validFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
					for x := min(srcX, destX); x <= max(srcX, destX); x++ {
						m.AllocateIfAbsent(x, z, level)
					}
				}
			}

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			lv := NewLineValidator(m)
			for level := range 4 {
				if !lv.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0) {
					t.Fatalf("[flag %d, dir %d, level %d] expected to have line of sight", f, dirEnum, level)
				}
			}
		}
	}
}

func TestLineValidatorLineOfSightFailWhenBlockedByLoc(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, collision.FlagLocProjBlocker)
		}

		lv := NewLineValidator(m)
		for level := range 4 {
			if lv.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0) {
				t.Fatalf("[dir %d, level %d] expected not to have line of sight", dirEnum, level)
			}
		}
	}
}

func TestLineValidatorLineOfSightFailWhenBlockedByExtraFlagCollisionFlag(t *testing.T) {
	extraFlags := []int{
		collision.FlagBlockPlayers,
		collision.FlagBlockNPCs,
		collision.FlagBlockPlayers | collision.FlagBlockNPCs,
	}

	for _, f := range extraFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
					for x := min(srcX, destX); x <= max(srcX, destX); x++ {
						m.AllocateIfAbsent(x, z, level)
					}
				}
			}

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			lv := NewLineValidator(m)
			for level := range 4 {
				if lv.HasLineOfSight(level, srcX, srcZ, destX, destZ, 1, 0, 0, f) {
					t.Fatalf("[flag %d, dir %d, level %d] expected not to have line of sight", f, dirEnum, level)
				}
			}
		}
	}
}

func TestLineValidatorLineOfWalkValidWhenOnTopOfTargetCoordinates(t *testing.T) {
	m := collision.NewFlagMap()
	m.AllocateIfAbsent(3200, 3200, 0)
	lv := NewLineValidator(m)
	if !lv.HasLineOfWalk(0, 3200, 3200, 3200, 3200, 1, 0, 0, 0) {
		t.Fatal("expected to have line of walk")
	}
}

func TestLineValidatorLineOfWalkValidWhenPathClearOfCollisionFlags(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			for z := min(srcZ, destZ); z <= max(srcZ, destZ); z++ {
				for x := min(srcX, destX); x <= max(srcX, destX); x++ {
					m.AllocateIfAbsent(x, z, level)
				}
			}
		}

		lv := NewLineValidator(m)
		for level := range 4 {
			if !lv.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0) {
				t.Fatalf("[dir %d, level %d] expected to have line of walk", dirEnum, level)
			}
		}
	}
}

func TestLineValidatorLineOfWalkFailWhenPathBlockedByLoc(t *testing.T) {
	for dirEnum, dir := range flag.DirectionToOffset {
		m := collision.NewFlagMap()
		srcX, srcZ := 3200, 3200
		destX := srcX + (dir.OffX * 3)
		destZ := srcZ + (dir.OffZ * 3)

		for level := range 4 {
			m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, collision.FlagLoc)
		}

		lv := NewLineValidator(m)
		for level := range 4 {
			if lv.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, 0) {
				t.Fatalf("[dir %d, level %d] expected not to have line of walk", dirEnum, level)
			}
		}
	}
}

func TestLineValidatorLineOfWalkFailWhenPathBlockedByExtraFlagCollisionFlag(t *testing.T) {
	extraFlags := []int{
		collision.FlagBlockPlayers,
		collision.FlagBlockNPCs,
		collision.FlagBlockPlayers | collision.FlagBlockNPCs,
	}

	for _, f := range extraFlags {
		for dirEnum, dir := range flag.DirectionToOffset {
			m := collision.NewFlagMap()
			srcX, srcZ := 3200, 3200
			destX := srcX + (dir.OffX * 3)
			destZ := srcZ + (dir.OffZ * 3)

			for level := range 4 {
				m.Set(srcX+dir.OffX, srcZ+dir.OffZ, level, f)
			}

			lv := NewLineValidator(m)
			for level := range 4 {
				if lv.HasLineOfWalk(level, srcX, srcZ, destX, destZ, 1, 0, 0, f) {
					t.Fatalf("[flag %d, dir %d, level %d] expected not to have line of walk", f, dirEnum, level)
				}
			}
		}
	}
}
