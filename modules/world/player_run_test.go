package world

import "testing"

func TestDefaultMoveSpeed_RunZero(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 0
	if got := p.defaultMoveSpeed(); got != MoveSpeedWalk {
		t.Errorf("defaultMoveSpeed(run=0): got %v, want MoveSpeedWalk", got)
	}
}

func TestDefaultMoveSpeed_RunOne(t *testing.T) {
	p, _ := newTestPlayer(t)
	p.run = 1
	if got := p.defaultMoveSpeed(); got != MoveSpeedRun {
		t.Errorf("defaultMoveSpeed(run=1): got %v, want MoveSpeedRun", got)
	}
}
