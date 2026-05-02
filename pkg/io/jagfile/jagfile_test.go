package jagfile

import (
	"slices"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func MakeTestJagfile() (*Jagfile, error) {
	p := packet.NewPacket(make([]byte, 0, 19))
	p.P3(1)                        // UnpackedSize
	p.P3(1)                        // PackedSize
	p.P2(1)                        // FileCount
	p.P4(-1502153170 & 0xFFFFFFFF) // hitmarks.dat
	p.P3(1)                        // FileUnpackedSize[0]
	p.P3(1)                        // FilePackedSize[0]
	p.P1(255)                      // hitmarks.data file data
	p.Pos = 0

	jf, err := NewJagfile(p)
	if err != nil {
		return nil, err
	}
	return jf, nil
}

func Test_genHash(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name string
		args args
		want uint32
	}{
		{
			name: "valid gnomeball_buttons.dat",
			args: args{
				name: "gnomeball_buttons.dat",
			},
			want: 22834782,
		},
		{
			name: "valid headicons.dat",
			args: args{
				name: "headicons.dat",
			},
			want: -288954319 & 0xFFFFFFFF,
		},
		{
			name: "valid hitmarks.dat",
			args: args{
				name: "hitmarks.dat",
			},
			want: -1502153170 & 0xFFFFFFFF,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := genHash(tt.args.name); got != tt.want {
				t.Errorf("genHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewJagfile_BZip2Path exercises the unpackedSize != packedSize branch
// (whole-jagfile bzip2). Mirrors TS Jagfile constructor's
// `BZip2.decompress(src.data.subarray(6), unpackedSize, true)` — must skip
// the 6-byte size header before handing bytes to the bzip2 decoder.
func TestNewJagfile_BZip2Path(t *testing.T) {
	body := packet.NewPacket(make([]byte, 0, 19))
	body.P2(1)                        // FileCount
	body.P4(-1502153170 & 0xFFFFFFFF) // hitmarks.dat hash
	body.P3(1)                        // FileUnpackedSize[0]
	body.P3(1)                        // FilePackedSize[0]
	body.P1(255)                      // file payload

	compressed, err := BZip2Compress(body.Data, false, true, 1, 0)
	if err != nil {
		t.Fatalf("BZip2Compress: %v", err)
	}

	src := packet.NewPacket(make([]byte, 0, 6+len(compressed)))
	src.P3(uint32(len(body.Data)))
	src.P3(uint32(len(compressed)))
	src.Data = append(src.Data, compressed...)
	src.Pos = 0

	jf, err := NewJagfile(src)
	if err != nil {
		t.Fatalf("NewJagfile (bzip2 path): %v", err)
	}
	if jf.FileCount != 1 || jf.FileName[0] != "hitmarks.dat" {
		t.Fatalf("decoded jagfile mismatch: count=%d name=%q", jf.FileCount, jf.FileName[0])
	}
}

func TestJagfileCreation(t *testing.T) {
	jf, err := MakeTestJagfile()
	if err != nil {
		t.Fatal(err)
	}

	if len(jf.Data) != 19 {
		t.Fatalf("len(jf.Data) = %v, want %v", len(jf.Data), 19)
	}
	if jf.FileCount != 1 {
		t.Fatalf("jf.FileCount = %v, want %v", jf.FileCount, 1)
	}
	if jf.FileHash[0] != -1502153170&0xFFFFFFFF {
		t.Fatalf("jf.FileHash[0] = %v, want %v", jf.FileHash[0], -1502153170&0xFFFFFFFF)
	}
	if jf.FileName[0] != "hitmarks.dat" {
		t.Fatalf("jf.FileName[0] = %v, want %v", jf.FileName[0], "hitmarks.dat")
	}
	if jf.FileUnpackedSize[0] != 1 {
		t.Fatalf("jf.FileUnpackedSize = %v, want %v", jf.FileUnpackedSize[0], 1)
	}
	if jf.FilePackedSize[0] != 1 {
		t.Fatalf("jf.FilePackedSize = %v, want %v", jf.FilePackedSize[0], 1)
	}
	if jf.FilePos[0] != 18 {
		t.Fatalf("jf.FilePos[0] = %v, want %v", jf.FilePos[0], 18)
	}

	// force it unpacked bcos bzip cba
	jf.Unpacked = true

	if _, err := jf.Read("kekw"); err == nil {
		t.Fatal("jf.Read('kekw') should fail")
	}
	jfp, err := jf.Read("hitmarks.dat")
	if err != nil {
		t.Fatal("jf.Read('hitmarks.dat') should not fail")
	}
	if jfp == nil {
		t.Fatal("jf.Read('hitmarks.dat') should not be nil")
	}
	if !slices.Equal(jfp.Data, []byte{255}) {
		t.Fatalf("jfp.Data = %v, want %v", jfp.Data, []byte{255})
	}
}

func TestJagfileDeletion(t *testing.T) {
	jf, err := MakeTestJagfile()
	if err != nil {
		t.Fatal(err)
	}

	jf.Delete("hitmarks.dat")

	if len(jf.FileQueue) != 1 {
		t.Fatalf("len(jf.FileQueue) = %v, want %v", len(jf.FileQueue), 1)
	}
	if jf.FileQueue[0].Delete != true {
		t.Fatalf("jf.FileQueue[0].Delete = %v, want %v", jf.FileQueue[0].Delete, true)
	}
	if jf.FileQueue[0].Write != false {
		t.Fatalf("jf.FileQueue[0].Write = %v, want %v", jf.FileQueue[0].Write, false)
	}
	if jf.FileQueue[0].Rename != false {
		t.Fatalf("jf.FileQueue[0].Rename = %v, want %v", jf.FileQueue[0].Rename, false)
	}
	if jf.FileQueue[0].Hash != -1502153170&0xFFFFFFFF {
		t.Fatalf("jf.FileQueue[0].Hash = %v, want %v", jf.FileQueue[0].Hash, -1502153170&0xFFFFFFFF)
	}
	if jf.FileQueue[0].Name != "hitmarks.dat" {
		t.Fatalf("jf.FileQueue[0].Name = %v, want %v", jf.FileQueue[0].Name, "hitmarks.dat")
	}
}

func TestJagfileWrite(t *testing.T) {
	jf, err := MakeTestJagfile()
	if err != nil {
		t.Fatal(err)
	}

	jf.Write("gnomeball_buttons.dat", packet.NewPacket(make([]byte, 0)))
	if len(jf.FileQueue) != 1 {
		t.Fatalf("len(jf.FileQueue) = %v, want %v", len(jf.FileQueue), 1)
	}
	if jf.FileQueue[0].Write != true {
		t.Fatalf("jf.FileQueue[0].Write = %v, want %v", jf.FileQueue[0].Write, true)
	}
	if jf.FileQueue[0].Delete != false {
		t.Fatalf("jf.FileQueue[0].Delete = %v, want %v", jf.FileQueue[0].Delete, false)
	}
	if jf.FileQueue[0].Rename != false {
		t.Fatalf("jf.FileQueue[0].Rename = %v, want %v", jf.FileQueue[0].Rename, false)
	}
	if jf.FileQueue[0].Hash != 22834782 {
		t.Fatalf("jf.FileQueue[0].Hash = %v, want %v", jf.FileQueue[0].Hash, 28834782)
	}
	if jf.FileQueue[0].Name != "gnomeball_buttons.dat" {
		t.Fatalf("jf.FileQueue[0].Name = %v, want %v", jf.FileQueue[0].Name, "gnomeball_buttons.dat")
	}
}

func TestJagfileRename(t *testing.T) {
	jf, err := MakeTestJagfile()
	if err != nil {
		t.Fatal(err)
	}

	jf.Rename("hitmarks.dat", "gnomeball_buttons.dat")
	if len(jf.FileQueue) != 1 {
		t.Fatalf("len(jf.FileQueue) = %v, want %v", len(jf.FileQueue), 1)
	}
	if jf.FileQueue[0].Rename != true {
		t.Fatalf("jf.FileQueue[0].Rename = %v, want %v", jf.FileQueue[0].Rename, true)
	}
	if jf.FileQueue[0].Write != false {
		t.Fatalf("jf.FileQueue[0].Write = %v, want %v", jf.FileQueue[0].Write, false)
	}
	if jf.FileQueue[0].Delete != false {
		t.Fatalf("jf.FileQueue[0].Delete = %v, want %v", jf.FileQueue[0].Delete, false)
	}
	if jf.FileQueue[0].Hash != -1502153170&0xFFFFFFFF {
		t.Fatalf("jf.FileQueue[0].Hash = %v, want %v", jf.FileQueue[0].Hash, -1502153170&0xFFFFFFFF)
	}
	if jf.FileQueue[0].Name != "hitmarks.dat" {
		t.Fatalf("jf.FileQueue[0].Name = %v, want %v", jf.FileQueue[0].Name, "hitmarks.dat")
	}
	if jf.FileQueue[0].NewHash != 22834782 {
		t.Fatalf("jf.FileQueue[0].NewHash = %v, want %v", jf.FileQueue[0].NewHash, 28834782)
	}
	if jf.FileQueue[0].NewName != "gnomeball_buttons.dat" {
		t.Fatalf("jf.FileQueue[0].NewName = %v, want %v", jf.FileQueue[0].NewName, "gnomeball_buttons.dat")
	}
}
