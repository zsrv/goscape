package io

import (
	"testing"
)

func TestNew(t *testing.T) {
	type args struct {
		seed [4]uint32
	}
	tests := []struct {
		name string
		args args
		want uint32
	}{
		{
			// checks that isaac is shuffling correctly
			name: "seed(0, 0, 0, 0)",
			args: args{
				seed: [4]uint32{0, 0, 0, 0},
			},
			want: 1536048213,
		},
		{
			// checks that rsl was populated and that isaac is shuffling correctly
			name: "seed(1, 2, 3, 4)",
			args: args{
				seed: [4]uint32{1, 2, 3, 4},
			},
			want: -107094133 & 0xFFFFFFFF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := New(tt.args.seed)
			for range 1_000_000 {
				is.GetNext()
			}

			if got := is.GetNext(); got != tt.want {
				t.Errorf("GetNext() = %v, want %v", got, tt.want)
			}
		})
	}
}
