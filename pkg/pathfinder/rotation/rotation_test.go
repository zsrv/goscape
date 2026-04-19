package rotation

import (
	"testing"

	"github.com/zsrv/goscape/pkg/pathfinder/flag"
)

func TestRotate(t *testing.T) {
	width := 3
	length := 2

	type args struct {
		angle      int
		dimensionA int
		dimensionB int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "rotate loc width angle 0",
			args: args{
				angle:      0,
				dimensionA: width,
				dimensionB: length,
			},
			want: width,
		},
		{
			name: "rotate loc width angle 1",
			args: args{
				angle:      1,
				dimensionA: width,
				dimensionB: length,
			},
			want: length,
		},
		{
			name: "rotate loc width angle 2",
			args: args{
				angle:      2,
				dimensionA: width,
				dimensionB: length,
			},
			want: width,
		},
		{
			name: "rotate loc width angle 3",
			args: args{
				angle:      3,
				dimensionA: width,
				dimensionB: length,
			},
			want: length,
		},
		{
			name: "rotate loc length angle 0",
			args: args{
				angle:      0,
				dimensionA: length,
				dimensionB: width,
			},
			want: length,
		},
		{
			name: "rotate loc length angle 1",
			args: args{
				angle:      1,
				dimensionA: length,
				dimensionB: width,
			},
			want: width,
		},
		{
			name: "rotate loc length angle 2",
			args: args{
				angle:      2,
				dimensionA: length,
				dimensionB: width,
			},
			want: length,
		},
		{
			name: "rotate loc length angle 3",
			args: args{
				angle:      3,
				dimensionA: length,
				dimensionB: width,
			},
			want: width,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Rotate(tt.args.angle, tt.args.dimensionA, tt.args.dimensionB); got != tt.want {
				t.Errorf("Rotate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRotateFlags(t *testing.T) {
	north := int(flag.BlockAccessNorth)
	east := int(flag.BlockAccessEast)
	south := int(flag.BlockAccessSouth)
	west := int(flag.BlockAccessWest)

	type args struct {
		angle            int
		blockAccessFlags int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "rotate north block access flag angle 0",
			args: args{
				angle:            0,
				blockAccessFlags: north,
			},
			want: north,
		},
		{
			name: "rotate north block access flag angle 1",
			args: args{
				angle:            1,
				blockAccessFlags: north,
			},
			want: east,
		},
		{
			name: "rotate north block access flag angle 2",
			args: args{
				angle:            2,
				blockAccessFlags: north,
			},
			want: south,
		},
		{
			name: "rotate north block access flag angle 3",
			args: args{
				angle:            3,
				blockAccessFlags: north,
			},
			want: west,
		},
		{
			name: "rotate east block access flag angle 0",
			args: args{
				angle:            0,
				blockAccessFlags: east,
			},
			want: east,
		},
		{
			name: "rotate east block access flag angle 1",
			args: args{
				angle:            1,
				blockAccessFlags: east,
			},
			want: south,
		},
		{
			name: "rotate east block access flag angle 2",
			args: args{
				angle:            2,
				blockAccessFlags: east,
			},
			want: west,
		},
		{
			name: "rotate east block access flag angle 3",
			args: args{
				angle:            3,
				blockAccessFlags: east,
			},
			want: north,
		},
		{
			name: "rotate south block access flag angle 0",
			args: args{
				angle:            0,
				blockAccessFlags: south,
			},
			want: south,
		},
		{
			name: "rotate south block access flag angle 1",
			args: args{
				angle:            1,
				blockAccessFlags: south,
			},
			want: west,
		},
		{
			name: "rotate south block access flag angle 2",
			args: args{
				angle:            2,
				blockAccessFlags: south,
			},
			want: north,
		},
		{
			name: "rotate south block access flag angle 3",
			args: args{
				angle:            3,
				blockAccessFlags: south,
			},
			want: east,
		},
		{
			name: "rotate west block access flag angle 0",
			args: args{
				angle:            0,
				blockAccessFlags: west,
			},
			want: west,
		},
		{
			name: "rotate west block access flag angle 1",
			args: args{
				angle:            1,
				blockAccessFlags: west,
			},
			want: north,
		},
		{
			name: "rotate west block access flag angle 2",
			args: args{
				angle:            2,
				blockAccessFlags: west,
			},
			want: east,
		},
		{
			name: "rotate west block access flag angle 3",
			args: args{
				angle:            3,
				blockAccessFlags: west,
			},
			want: south,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RotateFlags(tt.args.angle, tt.args.blockAccessFlags); got != tt.want {
				t.Errorf("RotateFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}
