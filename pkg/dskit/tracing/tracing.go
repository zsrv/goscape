// Provenance-includes-location: https://github.com/weaveworks/common/blob/main/tracing/tracing.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Weaveworks Ltd.

package tracing

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/trace"
)

var (
	// ErrBlankTraceConfiguration is an error to notify client to provide valid trace report agent or config server
	ErrBlankTraceConfiguration = errors.New("no trace report agent, config server, or collector endpoint specified")
)

// ExtractTraceID extracts the trace id, if any from the context.
func ExtractTraceID(ctx context.Context) (string, bool) {
	if tid, _, ok := extractOTelContext(ctx); ok {
		return tid.String(), true
	}
	return "", false
}

// ExtractTraceSpanID extracts the trace id, span id if any from the context.
func ExtractTraceSpanID(ctx context.Context) (string, string, bool) {
	if tid, sid, ok := extractOTelContext(ctx); ok {
		return tid.String(), sid.String(), true
	}
	return "", "", false
}

func extractOTelContext(ctx context.Context) (tid trace.TraceID, sid trace.SpanID, success bool) {
	sp := trace.SpanFromContext(ctx)
	sc := sp.SpanContext()
	if !sc.IsValid() {
		return
	}
	return sc.TraceID(), sc.SpanID(), true
}

// ExtractSampledTraceID works like ExtractTraceID but the returned bool is only
// true if the returned trace id is sampled.
func ExtractSampledTraceID(ctx context.Context) (string, bool) {
	if tid, ok := extractSampledOTelTraceID(ctx); ok {
		return tid.String(), true
	}
	return "", false
}

func extractSampledOTelTraceID(ctx context.Context) (traceID trace.TraceID, sampled bool) {
	sp := trace.SpanFromContext(ctx)
	sc := sp.SpanContext()
	return sc.TraceID(), sc.IsValid() && sc.IsSampled()
}
