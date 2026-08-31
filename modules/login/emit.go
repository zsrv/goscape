package login

import (
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// emitLogin publishes a RAW login auth event through the telemetry seam. The
// seam is dormant (no-op) unless the telemetry telemetry shipper has
// installed an emitter. Geo enrichment and the device fingerprint are derived
// downstream by the telemetry geoenrich module from the ip + uid carried
// here; this module performs no external lookups and no hashing.
func emitLogin(accountID int64, worldID int32, ip string, uid int32) {
	telemetry.Get().EmitAuth(&eventspb.AuthEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.New().String(),
		Ts:            timestamppb.Now(),
		AccountId:     accountID,
		WorldId:       worldID,
		Payload: &eventspb.AuthEnvelope_Login{
			Login: &eventspb.LoginEvent{Ip: ip, Uid: uid},
		},
	})
}
