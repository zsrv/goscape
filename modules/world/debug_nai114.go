package world

import (
	"log/slog"
	"sort"
	"strings"

	"github.com/zsrv/goscape/pkg/script"
)

// logOpHeldUScriptInventory emits a single INFO line listing every
// registered script whose name starts with "[opheldu,". Called once at
// server boot after script provider Load succeeds. NAI-114 Stage 3
// instrumentation; revert at Stage 4 close.
func logOpHeldUScriptInventory(p *script.Provider, log *slog.Logger) {
	if p == nil || log == nil {
		return
	}
	all := p.Names()
	matches := make([]string, 0, 8)
	for _, name := range all {
		if strings.HasPrefix(name, "[opheldu,") {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)
	log.Info("opheldu script registry",
		"count", len(matches),
		"names", strings.Join(matches, ","))
}
