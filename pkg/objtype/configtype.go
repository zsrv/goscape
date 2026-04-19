package objtype

import (
	"github.com/zsrv/goscape/pkg/io/packet"
)

type ConfigTypeDecoder interface {
	Decode(code uint8, buf *packet.Packet) error
}

type ConfigType struct {
	ConfigTypeDecoder
	ID        int
	DebugName string
}

// TODO: turn this into a non-receiver func that takes a ConfigTypeDecoder param - easier this way
//func (ct *ConfigType) DecodeType(buf *packet.Packet) error {
//	for buf.Len() > 0 {
//		code := buf.G1()
//		if code == 0 {
//			break
//		}
//		return ct.Decode(code, buf)
//	}
//	return nil
//}

func DecodeType(buf *packet.Packet, f ConfigTypeDecoder) error {
	for buf.Len() > 0 {
		code := buf.G1()
		if code == 0 {
			break
		}
		if err := f.Decode(code, buf); err != nil {
			return err
		}
	}
	return nil
}
