package tracing

import (
	"go.opentelemetry.io/otel/trace"
)

var _ SpanOption = SpanKindRPCClient{}

type SpanOption interface {
	spanOptions() []trace.SpanStartOption
}

type SpanKindRPCClient struct{}

func (SpanKindRPCClient) spanOptions() []trace.SpanStartOption {
	return []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindClient)}
}
