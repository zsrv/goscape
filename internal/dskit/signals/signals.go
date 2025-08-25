// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/signals/signals.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

package signals

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/zsrv/goscape/pkg/util/log"
)

// SignalReceiver represents a subsystem/server/... that can be stopped or
// queried about the status with a signal
type SignalReceiver interface {
	Stop() error
}

// Handler handles signals, can be interrupted.
// On SIGINT or SIGTERM it will exit, on SIGQUIT it
// will dump goroutine stacks to the Logger.
type Handler struct {
	log       *slog.Logger
	receivers []SignalReceiver
	quit      chan struct{}
}

// NewHandler makes a new Handler.
func NewHandler(log *slog.Logger, receivers ...SignalReceiver) *Handler {
	return &Handler{
		log:       log,
		receivers: receivers,
		quit:      make(chan struct{}),
	}
}

// Stop the handler
func (h *Handler) Stop() {
	close(h.quit)
}

// Loop handles signals.
func (h *Handler) Loop() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	defer signal.Stop(sigs)
	buf := make([]byte, 1<<20)
	for {
		select {
		case <-h.quit:
			h.log.Info("handler stopped")
			return
		case sig := <-sigs:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				h.log.Info("received SIGINT/SIGTERM, exiting")
				for _, subsystem := range h.receivers {
					_ = subsystem.Stop()
				}
				return
			case syscall.SIGQUIT:
				stacklen := runtime.Stack(buf, true)
				h.log.Info("received SIGQUIT, dumping goroutine", "stackTrace", string(buf[:stacklen]))
			}
		}
	}
}

// SignalHandlerLoop blocks until it receives a SIGINT, SIGTERM or SIGQUIT.
// For SIGINT and SIGTERM, it exits; for SIGQUIT is print a goroutine stack
// dump.
func SignalHandlerLoop(log *slog.Logger, ss ...SignalReceiver) {
	NewHandler(log, ss...).Loop()
}
