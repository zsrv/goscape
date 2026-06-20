package req

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	"testing"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
)

func newTestRSAKey(t *testing.T, bits int) *protocol.RSAKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return &protocol.RSAKey{
		Modulus:         k.N,
		PublicExponent:  big.NewInt(int64(k.E)),
		PrivateExponent: k.D,
	}
}

// TestUnmarshalRSA_CustomKey proves the key argument is honored: a freshly
// generated key cannot match DefaultRSAKey, so if UnmarshalRSA ignored the
// param the magic-byte check would fail.
func TestUnmarshalRSA_CustomKey(t *testing.T) {
	rk := newTestRSAKey(t, 1024)

	pt := packet.NewPacket(make([]byte, 0, 256))
	pt.P1(10) // RSA magic number
	for _, s := range []uint32{1, 2, 3, 4} {
		pt.P4(s)
	}
	pt.P4(0xDEADBEEF) // uid
	pt.PJStrLF("alice")
	pt.PJStrLF("hunter2")
	pt.RSAEnc(rk.Modulus, rk.PublicExponent)

	var q GameLogin
	if err := q.UnmarshalRSA(packet.NewPacket(pt.Bytes()), rk); err != nil {
		t.Fatalf("UnmarshalRSA with custom key: %v", err)
	}
	if q.Username != "alice" || q.Password != "hunter2" {
		t.Errorf("round-trip mismatch: user=%q pass=%q", q.Username, q.Password)
	}
	if q.UID != 0xDEADBEEF {
		t.Errorf("uid mismatch: %#x", q.UID)
	}
	if q.ISAACSeed != [4]uint32{1, 2, 3, 4} {
		t.Errorf("seed mismatch: %v", q.ISAACSeed)
	}
}
