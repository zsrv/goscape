package telemetry

import (
	"encoding/binary"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/zsrv/goscape/pkg/eventspb"
)

// NewEmitter returns an Emitter that marshals events and stages them in the
// supplied ring buffer for the shipper to drain.
func NewEmitter(buf *RingBuffer) Emitter {
	return &bufEmitter{buf: buf}
}

type bufEmitter struct{ buf *RingBuffer }

var _ Emitter = (*bufEmitter)(nil)

func (e *bufEmitter) EmitAuth(env *eventspb.AuthEnvelope) {
	e.publish("events.auth", accountIDKey(env.GetAccountId()), env)
}

func (e *bufEmitter) EmitWorld(env *eventspb.WorldEnvelope) {
	e.publish("events.world", worldIDKey(env.GetWorldId()), env)
}

func (e *bufEmitter) EmitPlayerInput(env *eventspb.PlayerInputEnvelope) {
	e.publish("events.player_input", accountIDKey(env.GetAccountId()), env)
}

func (e *bufEmitter) EmitWealth(env *eventspb.WealthEnvelope) {
	e.publish("events.wealth", accountIDKey(env.GetAccountId()), env)
}

func (e *bufEmitter) publish(topic string, key []byte, env proto.Message) {
	payload, err := proto.Marshal(env)
	if err != nil {
		return
	}
	e.buf.Push(&kgo.Record{Topic: topic, Key: key, Value: payload})
}

func accountIDKey(id int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(id))
	return b[:]
}

func worldIDKey(id int32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(id))
	return b[:]
}
