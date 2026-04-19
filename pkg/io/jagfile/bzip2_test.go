package jagfile

import (
	"reflect"
	"testing"
)

func TestBZip2Compress(t *testing.T) {
	type args struct {
		decompressed     []byte
		prefixLength     bool
		removeHeader     bool
		blockSize        int
		compressedLength int
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "valid with header, no length prefix, compression level 1",
			args: args{
				decompressed:     []byte("Hello World!"),
				prefixLength:     false,
				removeHeader:     false,
				blockSize:        1,
				compressedLength: 0,
			},
			want:    []byte{66, 90, 104, 49, 49, 65, 89, 38, 83, 89, 107, 26, 124, 174, 0, 0, 1, 23, 128, 96, 0, 0, 64, 0, 128, 6, 4, 144, 0, 32, 0, 34, 42, 55, 250, 169, 250, 167, 237, 8, 6, 11, 2, 197, 57, 112, 187, 146, 41, 194, 132, 131, 88, 211, 229, 112},
			wantErr: false,
		},
		{
			name: "valid without header, compression level 1",
			args: args{
				decompressed:     []byte("Hello World!"),
				prefixLength:     true,
				removeHeader:     true,
				blockSize:        1,
				compressedLength: 0,
			},
			want:    []byte{49, 65, 89, 38, 83, 89, 107, 26, 124, 174, 0, 0, 1, 23, 128, 96, 0, 0, 64, 0, 128, 6, 4, 144, 0, 32, 0, 34, 42, 55, 250, 169, 250, 167, 237, 8, 6, 11, 2, 197, 57, 112, 187, 146, 41, 194, 132, 131, 88, 211, 229, 112},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BZip2Compress(tt.args.decompressed, tt.args.prefixLength, tt.args.removeHeader, tt.args.blockSize, tt.args.compressedLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("BZip2Compress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BZip2Compress() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBZip2Decompress(t *testing.T) {
	type args struct {
		compressed                 []byte
		decompressedLength         int
		prependHeader              bool
		containsDecompressedLength bool
	}
	tests := []struct {
		name    string
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name: "valid with header, no length prefix, compression level 1",
			args: args{
				compressed:                 []byte{66, 90, 104, 49, 49, 65, 89, 38, 83, 89, 107, 26, 124, 174, 0, 0, 1, 23, 128, 96, 0, 0, 64, 0, 128, 6, 4, 144, 0, 32, 0, 34, 42, 55, 250, 169, 250, 167, 237, 8, 6, 11, 2, 197, 57, 112, 187, 146, 41, 194, 132, 131, 88, 211, 229, 112},
				decompressedLength:         0,
				prependHeader:              false,
				containsDecompressedLength: false,
			},
			want:    []byte("Hello World!"),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BZip2Decompress(tt.args.compressed, tt.args.decompressedLength, tt.args.prependHeader, tt.args.containsDecompressedLength)
			if (err != nil) != tt.wantErr {
				t.Errorf("BZip2Decompress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BZip2Decompress() got = %v, want %v", got, tt.want)
			}
		})
	}
}
