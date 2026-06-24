package world

import (
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// slogLoggerBridge is the production default LoggerBridge impl. Emits
// one structured slog record per call under a child logger keyed
// component=logger_bridge. NAI-73 closes NAI-72-D-LOGGER-BRIDGE by
// shipping this default; tests still bind recordingBridges via
// installRecordingBridges(s).
type slogLoggerBridge struct {
	log     *slog.Logger
	nodeID  int
	profile string
}

// NewSlogLoggerBridge wraps parent in a child logger keyed
// component=logger_bridge. nodeID/profile are stamped on records whose
// TS message shapes carry world/profile (LoggerClient.ts:48-87).
func NewSlogLoggerBridge(parent *slog.Logger, nodeID int, profile string) LoggerBridge {
	return &slogLoggerBridge{
		log:     parent.With("component", compReport),
		nodeID:  nodeID,
		profile: profile,
	}
}

// NotifyPlayerReport emits a 'report' record. Mirrors TS
// World.notifyPlayerReport's loggerThread.postMessage call. rev-254 A3
// re-key: the 'report' message carries session_uuid: player.session
// instead of the 244-era username (World.ts:2309-2324 @2e3bcf43,
// LoggerThread.ts:45-51 @2e3bcf43). Proto message shapes stay with the
// private sibling; this is the dev/debug slog seam only.
func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
	b.log.Info("player_report",
		"type", "report",
		"world", b.nodeID,
		"profile", b.profile,
		"session_uuid", p.sessionOrHeadless(),
		"timestamp_ms", time.Now().UnixMilli(),
		"coord", coordgrid.PackCoord(p.level, p.x, p.z),
		"offender", offender,
		"reason", reason,
	)
}

// SubmitInputTracking emits an 'input_track' record. rev-254 A5
// re-shape: mirrors TS World.submitInputTracking's
// loggerThread.postMessage call (World.ts:2326-2333 @2e3bcf43):
//
//	{ type: 'input_track', session_uuid: player.session,
//	  timestamp: Date.now(), buf: Buffer.from(buf).toString('base64') }
//
// The 43e02957-era username/seq/coord blob wrapper is gone
// (InputTrackingBlob.ts deleted upstream); blob assembly (base64 +
// session + timestamp) is receiver-side, here. NOTE world/profile are
// NOT stamped — unlike report/session_log/wealth_event, the
// LoggerClient.inputTrack envelope omits them (LoggerClient.ts:64-79
// @2e3bcf43). Proto message shapes are owned by B5/private-sibling;
// this is the dev/debug slog seam only.
func (b *slogLoggerBridge) SubmitInputTracking(p *Player, buf []byte) {
	b.log.Info("input_track",
		"type", "input_track",
		"session_uuid", p.sessionOrHeadless(),
		"timestamp_ms", time.Now().UnixMilli(),
		"buf", base64.StdEncoding.EncodeToString(buf),
	)
}

// SubmitSessionLogs emits one structured slog record per entry. The
// per-tick batch shape is preserved by the call cadence (one
// SubmitSessionLogs call per tick); per-entry record emission is
// chosen for grep/filter friendliness — this is a dev/debug sink, not
// the production LoggerClient WS transport which would JSON-batch.
//
// rev-254 A3: account_id dropped — the row is keyed by session_uuid
// only (World.addSessionLog @2e3bcf43 World.ts:2234-2243). Proto shape
// is unchanged (B5/private-sibling owns message shapes); this is the
// dev/debug slog seam adaptation only.
func (b *slogLoggerBridge) SubmitSessionLogs(logs []SessionLog) {
	for _, lg := range logs {
		b.log.Info("session_log",
			"type", "session_log",
			"session", lg.SessionUUID,
			"timestamp_ms", lg.Timestamp,
			"coord", lg.Coord,
			"event_type", lg.EventType,
			"event", lg.Event,
		)
	}
}

// Compile-time interface satisfaction.
var _ LoggerBridge = (*slogLoggerBridge)(nil)
