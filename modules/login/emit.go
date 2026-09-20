package login

import (
	"uuid"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zsrv/goscape/pkg/eventspb"
	"github.com/zsrv/goscape/pkg/io/protocol/revision"
	"github.com/zsrv/goscape/pkg/telemetry"
)

// emitLogin publishes a RAW login auth event through the telemetry seam. The
// seam is dormant (no-op) unless the telemetry telemetry shipper has
// installed an emitter. Geo enrichment and the device fingerprint are derived
// downstream by the telemetry geoenrich module from the ip + uid carried
// here; this module performs no external lookups and no hashing.
//
// BOTH halves of the origin come off the PlayerLoginRequest, because the
// login service is revision- and profile-agnostic: one login server serves
// every profile, and (the central DB being revision-agnostic too) it may
// serve worlds of several revisions at once.
//
// profile is the ORIGINATING world's deployment profile, carried on the
// PlayerLoginRequest — the same value every DB query in this handler scopes
// by. One login server serves every profile (account.id is global, while
// account_login is keyed by (account_id, profile)), so the request's profile,
// not this process's config, is what the event came from.
//
// worldRevision is the CALLING world binary's revision.Expected, likewise off
// the request. A zero value means the world build predates that field, so the
// event falls back to this login binary's own revision.Expected — the value
// stamped before the field existed, which is correct for the deployments that
// have one login server per revision.
func emitLogin(accountID int64, worldID int32, worldRevision uint32, profile, ip string, uid int32) {
	if worldRevision == 0 {
		worldRevision = revision.Expected
	}
	telemetry.Get().EmitAuth(&eventspb.AuthEnvelope{
		SchemaVersion: 1,
		EventId:       uuid.New().String(),
		Ts:            timestamppb.Now(),
		AccountId:     accountID,
		WorldId:       worldID,
		Revision:      worldRevision,
		Profile:       profile,
		Payload: &eventspb.AuthEnvelope_Login{
			Login: &eventspb.LoginEvent{Ip: ip, Uid: uid},
		},
	})
}
