package world

import "testing"

func TestComposeUID(t *testing.T) {
	tests := []struct {
		name       string
		username37 uint64
		slot       int
		want       int
	}{
		{
			name:       "zero username37 + slot returns slot only",
			username37: 0,
			slot:       2,
			want:       2,
		},
		{
			name:       "zero username37 + slot 1",
			username37: 0,
			slot:       1,
			want:       1,
		},
		{
			name:       "zero username37 + max-11-bit slot",
			username37: 0,
			slot:       2047, // 0x7FF
			want:       2047,
		},
		{
			name:       "username37=1 + slot=0 shifts up 11 bits",
			username37: 1,
			slot:       0,
			want:       1 << 11, // 2048
		},
		{
			name:       "username37=1 + slot=2 ORs slot in",
			username37: 1,
			slot:       2,
			want:       (1 << 11) | 2, // 2050
		},
		{
			name:       "max-21-bit username37 + max-11-bit slot",
			username37: 0x1FFFFF,
			slot:       0x7FF,
			want:       (0x1FFFFF << 11) | 0x7FF,
		},
		{
			name:       "username37 above 21 bits is masked",
			username37: 0x1FFFFF | (1 << 21), // bit 21 should be discarded
			slot:       5,
			want:       (0x1FFFFF << 11) | 5,
		},
		{
			name:       "slot above 11 bits is masked",
			username37: 1,
			slot:       0x7FF | (1 << 11), // bit 11 should be discarded
			want:       (1 << 11) | 0x7FF,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := composeUID(tc.username37, tc.slot)
			if got != tc.want {
				t.Errorf("composeUID(%#x, %d) = %d, want %d", tc.username37, tc.slot, got, tc.want)
			}
		})
	}
}
