package world

import (
	"testing"

	jutil "github.com/zsrv/goscape/pkg/util/jstring"
)

func TestComposeUID(t *testing.T) {
	tests := []struct {
		name       string
		username37 uint64
		pid        int
		want       int
	}{
		{
			name:       "zero username37 + pid returns pid only",
			username37: 0,
			pid:        2,
			want:       2,
		},
		{
			name:       "zero username37 + pid 1",
			username37: 0,
			pid:        1,
			want:       1,
		},
		{
			name:       "zero username37 + max-11-bit pid",
			username37: 0,
			pid:        2047, // 0x7FF
			want:       2047,
		},
		{
			name:       "username37=1 + pid=0 shifts up 11 bits",
			username37: 1,
			pid:        0,
			want:       1 << 11, // 2048
		},
		{
			name:       "username37=1 + pid=2 ORs pid in",
			username37: 1,
			pid:        2,
			want:       (1 << 11) | 2, // 2050
		},
		{
			name:       "max-21-bit username37 + max-11-bit pid",
			username37: 0x1FFFFF,
			pid:        0x7FF,
			want:       (0x1FFFFF << 11) | 0x7FF,
		},
		{
			name:       "username37 above 21 bits is masked",
			username37: 0x1FFFFF | (1 << 21), // bit 21 should be discarded
			pid:        5,
			want:       (0x1FFFFF << 11) | 5,
		},
		{
			name:       "pid above 11 bits is masked",
			username37: 1,
			pid:        0x7FF | (1 << 11), // bit 11 should be discarded
			want:       (1 << 11) | 0x7FF,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := composeUID(tc.username37, tc.pid)
			if got != tc.want {
				t.Errorf("composeUID(%#x, %d) = %d, want %d", tc.username37, tc.pid, got, tc.want)
			}
		})
	}
}

func TestAddPlayerComposesUID(t *testing.T) {
	s := &Server{
		quit:    make(chan struct{}),
		log:     discardLogger(),
		players: newPlayerList(2048),
	}

	p, _ := newTestPlayer(t)
	p.username37 = jutil.ToBase37("alice")

	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer() failed: %v", err)
	}

	want := composeUID(p.username37, p.pid)
	if p.uid != want {
		t.Errorf("p.uid = %d (pid=%d, username37=%#x), want %d", p.uid, p.pid, p.username37, want)
	}
}

func TestAddPlayerEmptyUsernameComposesSlotOnlyUID(t *testing.T) {
	s := &Server{
		quit:    make(chan struct{}),
		log:     discardLogger(),
		players: newPlayerList(2048),
	}

	p, _ := newTestPlayer(t)
	// p.username37 defaults to 0

	if err := s.addPlayer(p); err != nil {
		t.Fatalf("addPlayer() failed: %v", err)
	}

	// With username37=0, uid should equal pid only
	want := composeUID(0, p.pid)
	if p.uid != want {
		t.Errorf("p.uid = %d (pid=%d, username37=0), want %d (pid only)", p.uid, p.pid, want)
	}
}
