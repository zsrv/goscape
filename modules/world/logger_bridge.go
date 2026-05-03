package world

import (
	"encoding/base64"
	"log/slog"

	"github.com/zsrv/goscape/pkg/coordgrid"
)

// slogLoggerBridge is the production default LoggerBridge impl. Emits
// one structured slog record per call under a child logger keyed
// component=logger_bridge. NAI-73 closes NAI-72-D-LOGGER-BRIDGE by
// shipping this default; tests still bind recordingBridges via
// installRecordingBridges(s).
type slogLoggerBridge struct {
	log *slog.Logger
}

// NewSlogLoggerBridge wraps parent in a child logger keyed
// component=logger_bridge.
func NewSlogLoggerBridge(parent *slog.Logger) *slogLoggerBridge {
	return &slogLoggerBridge{log: parent.With("component", "logger_bridge")}
}

// NotifyPlayerReport emits a 'report' record. Mirrors TS
// World.notifyPlayerReport's loggerThread.postMessage call (World.ts:2305).
func (b *slogLoggerBridge) NotifyPlayerReport(p *Player, offender, reason string) {
	b.log.Info("player_report",
		"type", "report",
		"session", p.session,
		"coord", coordgrid.PackCoord(p.level, p.x, p.z),
		"offender", offender,
		"reason", reason,
	)
}

// SubmitInputTracking emits an 'input_track' record. Mirrors TS
// World.submitInputTracking's loggerThread.postMessage call (World.ts:2315).
// blob is base64-encoded for log readability and to match TS
// Buffer.from(buf).toString('base64') (World.ts:2319).
func (b *slogLoggerBridge) SubmitInputTracking(p *Player, blob []byte) {
	b.log.Info("input_track",
		"type", "input_track",
		"session", p.session,
		"blob_len", len(blob),
		"blob_b64", base64.StdEncoding.EncodeToString(blob),
	)
}

// SubmitSessionLogs emits one structured slog record per entry. The
// per-tick batch shape is preserved by the call cadence (one
// SubmitSessionLogs call per tick); per-entry record emission is
// chosen for grep/filter friendliness — this is a dev/debug sink, not
// the production LoggerClient WS transport which would JSON-batch.
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
