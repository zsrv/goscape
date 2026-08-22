package req

import (
	"errors"
	"log/slog"

	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/io/protocol"
)

var (
	OpReqInitGameConnection = protocol.Operation{
		Opcode:      16,
		PayloadSize: -1,
		Name:        "OpReqInitGameConnection",
	}
	OpReqGameReconnect = protocol.Operation{
		Opcode:      18,
		PayloadSize: -1,
		Name:        "OpReqGameReconnect",
	}
)

type GameLogin struct {
	Username         string
	Password         string
	ArchiveChecksums [9]uint32
	ISAACSeed        [4]uint32
	UID              uint32
	Revision         uint8
	LowMemory        bool
}

// LogValue implements slog.LogValuer so that logging a GameLogin (by value
// or pointer) never emits the cleartext password, the ISAAC seed or the
// CRC table. SEC1 M-7: server_login.go logs the whole request at Debug;
// without this a `log_level: debug` world wrote every player's password
// to its log. Value receiver so `*GameLogin` redacts too (Go promotes the
// method to the pointer type).
func (q GameLogin) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("username", q.Username),
		slog.String("password", "[redacted]"),
		slog.Uint64("uid", uint64(q.UID)),
		slog.Int("revision", int(q.Revision)),
		slog.Bool("low_memory", q.LowMemory),
	)
}

func (q *GameLogin) MarshalBinary() ([]byte, error) {
	// opcode (1) + payload size (1) + revision (1) + low memory (1) +
	// archive checksums (36) +
	bCap := 105

	b := packet.NewPacket(make([]byte, 0, bCap))

	b.P1(OpReqInitGameConnection.Opcode)
	b.P1(0) // length placeholder

	start := b.Len()

	b.P1(q.Revision)
	b.PBool(q.LowMemory)
	for i := range q.ArchiveChecksums {
		b.P4(q.ArchiveChecksums[i])
	}

	plaintext := packet.NewPacket(make([]byte, 0, bCap))
	plaintext.P1(10) // RSA magic number
	for i := range q.ISAACSeed {
		plaintext.P4(q.ISAACSeed[i])
	}
	plaintext.P4(q.UID)
	plaintext.PJStrLF(q.Username)
	plaintext.PJStrLF(q.Password)
	plaintext.RSAEnc(protocol.Modulus, protocol.PublicExponent)
	b.PData(plaintext.Bytes())

	b.PSize1(b.Len() - start)

	return b.Bytes(), nil
}

// UnmarshalHeader decodes the cleartext portion of the login packet
// (opcode, payload size, revision, low-memory flag, archive checksums) and
// leaves r positioned at the RSA-encrypted block. TS World.ts gates the
// revision (2119) and CRC (2131) checks on this cleartext before calling
// rsadec (2139), so callers should validate Revision/ArchiveChecksums and
// only then call [GameLogin.UnmarshalRSA] — RSA CPU is never spent on a
// stale-revision or bad-CRC client. L37.
func (q *GameLogin) UnmarshalHeader(r *packet.Packet) error {
	if r.Len() < 1+1 {
		return protocol.ErrIncorrectDataLength
	}

	code := r.G1()
	if code != OpReqInitGameConnection.Opcode && code != OpReqGameReconnect.Opcode {
		return protocol.ErrIncorrectOpcode
	}

	payloadSize := r.G1()
	if r.Len() < int(payloadSize) {
		return protocol.ErrPayloadTooSmall
	}
	if r.Len() > int(payloadSize) {
		return protocol.ErrPayloadTooLarge
	}

	q.Revision = r.G1()
	// TS World.ts:2126-2127 reads an info byte and takes lowMemory from its
	// low bit: `(info & 0x1) !== 0`. GBool() (`== 1`) diverges for any odd
	// value > 1 (e.g. 3 → TS true, GBool false). Mask the low bit to match. L35.
	q.LowMemory = r.G1()&0x1 != 0

	for i := range q.ArchiveChecksums {
		q.ArchiveChecksums[i] = r.G4()
	}

	return nil
}

// UnmarshalRSA decodes the RSA-encrypted tail of the login packet (magic
// number, ISAAC seeds, UID, username, password) from r's current position,
// which must be just past the cleartext header read by
// [GameLogin.UnmarshalHeader].
func (q *GameLogin) UnmarshalRSA(r *packet.Packet, key *protocol.RSAKey) error {
	decrypted, err := r.RSADec(key.Modulus, key.PrivateExponent)
	if err != nil {
		return err
	}
	if decrypted.G1() != 10 {
		return errors.New("invalid RSA magic number")
	}
	for i := range q.ISAACSeed {
		q.ISAACSeed[i] = decrypted.G4()
	}
	q.UID = decrypted.G4()
	q.Username = decrypted.GJStrLF()
	q.Password = decrypted.GJStrLF()

	return nil
}

func (q *GameLogin) UnmarshalBinary(data []byte) error {
	r := packet.NewPacket(data)
	if err := q.UnmarshalHeader(r); err != nil {
		return err
	}
	return q.UnmarshalRSA(r, protocol.DefaultRSAKey)
}
