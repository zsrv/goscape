package pack

import (
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
)

func TestBuildVerify_OK(t *testing.T) {
	data := []uint8("hello world")
	crc := packet.GetCRC(data, 0, len(data))
	if err := BuildVerify(data, len(data), int32(crc)); err != nil {
		t.Errorf("BuildVerify: %v, want nil", err)
	}
}

func TestBuildVerify_Mismatch(t *testing.T) {
	data := []uint8("hello world")
	wrong := int32(0x7fffffff) // any value extremely unlikely to match CRC32 of "hello world"
	if err := BuildVerify(data, len(data), wrong); err == nil {
		t.Errorf("BuildVerify(wrong CRC): err=nil, want non-nil")
	}
}
