package req

import (
	"errors"

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

func (q *GameLogin) UnmarshalBinary(data []byte) error {
	r := packet.NewPacket(data)

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
	q.LowMemory = r.GBool()

	for i := range q.ArchiveChecksums {
		q.ArchiveChecksums[i] = r.G4()
	}

	decrypted, err := r.RSADec(protocol.Modulus, protocol.PrivateExponent)
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
