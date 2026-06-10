package world

import (
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
		log:     parent.With("component", "logger_bridge"),
		nodeID:  nodeID,
		profile: profile,
	}
}

// NotifyPlayerReport emits a 'report' record. Mirrors TS
// World.notifyPlayerReport's loggerThread.postMessage call. rev-244
// re-shape: keyed by username + world + profile + timestamp instead of
// the 225 session uuid (LoggerClient.ts:48-67). Proto message shapes
// stay with the private sibling; this is the dev/debug slog seam only.
func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
	b.log.Info("player_report",
		"type", "report",
		"world", b.nodeID,
		"profile", b.profile,
		"username", p.username,
		"timestamp_ms", time.Now().UnixMilli(),
		"coord", coordgrid.PackCoord(p.level, p.x, p.z),
		"offender", offender,
		"reason", reason,
	)
}

// SubmitInputTracking emits an 'input_track' record. Mirrors TS
// World.submitInputTracking's loggerThread.postMessage call
// (World.ts:2346-2354 @43e02957): username + session_uuid + blobs. At
// 254 each InputTracking.Flush submits exactly ONE blob; the slice
// shape mirrors the TS signature.
//
// Each blob carries Seq, Data (base64), and Coord — matching the TS
// InputTrackingBlob shape (InputTrackingBlob.ts:1-11). The blobs are
// emitted as a JSON-serialisable []any slice for slog structured output.
// Proto message shapes are owned by B5/private-sibling; this slog seam
// is adapted for 244 without touching .proto files.
//
// rev-244 B5: world/profile stamped per the 244 inputTrack envelope
// (LoggerClient.ts:76-86). The TS `timestamp` param is not modeled —
// goscape's seam has no caller-supplied timestamp (the slog record
// carries its own time); recorded with the B5 logger rows in PORTING.md.
func (b *slogLoggerBridge) SubmitInputTracking(username, sessionUUID string, blobs []InputTrackingBlob) {
	blobsAny := make([]any, len(blobs))
	for i, bl := range blobs {
		blobsAny[i] = map[string]any{
			"seq":   bl.Seq,
			"data":  bl.Data,
			"coord": bl.Coord,
		}
	}
	b.log.Info("input_track",
		"type", "input_track",
		"world", b.nodeID,
		"profile", b.profile,
		"username", username,
		"session_uuid", sessionUUID,
		"blob_count", len(blobs),
		"blobs", blobsAny,
	)
}

// SubmitSessionLogs emits one structured slog record per entry. The
// per-tick batch shape is preserved by the call cadence (one
// SubmitSessionLogs call per tick); per-entry record emission is
// chosen for grep/filter friendliness — this is a dev/debug sink, not
// the production LoggerClient WS transport which would JSON-batch.
//
// rev-244 B3: account_id emitted alongside session (SessionLog.ts:2,
// World.ts:2252). Proto shape is unchanged (B5/private-sibling owns
// message shapes); this is the dev/debug slog seam adaptation only.
func (b *slogLoggerBridge) SubmitSessionLogs(logs []SessionLog) {
	for _, lg := range logs {
		b.log.Info("session_log",
			"type", "session_log",
			"account_id", lg.AccountID,
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
