package pixpack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestWriteImage_NoMeta_RowMajor(t *testing.T) {
	bm := &Bitmap{Width: 2, Height: 1, Data: []uint8{1, 2, 3, 0xff, 4, 5, 6, 0xff}}
	colors := []int32{0xff00ff, 0x010203, 0x040506}
	data := packet.Alloc(16)
	index := packet.Alloc(16)
	defer data.Release()
	defer index.Release()

	WriteImage(bm, data, index, colors, nil)

	wantIdx := []byte{0, 0, 0, 2, 0, 1, 1}
	if string(index.Data) != string(wantIdx) {
		t.Errorf("index = %v, want %v", index.Data, wantIdx)
	}
	wantData := []byte{1, 2}
	if string(data.Data) != string(wantData) {
		t.Errorf("data = %v, want %v", data.Data, wantData)
	}
}

func TestWriteImage_UnknownColorTerminatesEarly(t *testing.T) {
	// Pixel (1,0) has RGB(9,9,9) which is NOT in the palette; the
	// row-major write must stop after emitting only the first pixel.
	bm := &Bitmap{Width: 2, Height: 1, Data: []uint8{1, 2, 3, 0xff, 9, 9, 9, 0xff}}
	colors := []int32{0xff00ff, 0x010203}
	data := packet.Alloc(16)
	index := packet.Alloc(16)
	defer data.Release()
	defer index.Release()

	WriteImage(bm, data, index, colors, nil)

	if len(data.Data) != 1 || data.Data[0] != 1 {
		t.Errorf("data = %v, want [1]", data.Data)
	}
}
