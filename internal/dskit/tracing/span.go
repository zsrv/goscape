package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span is an OpenTelemetry span.
type Span struct {
	span trace.Span
}

// StartSpanFromContext starts a new OpenTelemetry span.
func StartSpanFromContext(ctx context.Context, operation string, options ...SpanOption) (*Span, context.Context) {
	var spanStartOptions []trace.SpanStartOption
	for _, opt := range options {
		spanStartOptions = append(spanStartOptions, opt.spanOptions()...)
	}
	ctx, span := tracer.Start(ctx, operation, spanStartOptions...)
	s := &Span{span: span}
	return s, ctx
}

func (s *Span) SetTag(name string, value any) {
	if s.span != nil {
		s.span.SetAttributes(KeyValueToOTelAttribute(name, value))
	}
}

func (s *Span) SetError() {
	if s.span != nil {
		s.span.SetStatus(codes.Error, "error")
		return
	}
}

func (s *Span) LogError(err error) {
	if s.span != nil {
		s.span.RecordError(err)
		return
	}
}

func (s *Span) Finish() {
	if s.span != nil {
		s.span.End()
	}
}

func SpanFromContext(ctx context.Context) (span trace.Span, sampled bool) {
	span = trace.SpanFromContext(ctx)
	spanContext := span.SpanContext()
	if spanContext.IsValid() {
		return span, spanContext.IsSampled()
	}

	return nil, false
}

func KeyValueToOTelAttribute(key string, val any) attribute.KeyValue {
	var attr attribute.KeyValue
	switch v := val.(type) {
	case string:
		attr = attribute.String(key, v)
	case int:
		attr = attribute.Int(key, v)
	case int64:
		attr = attribute.Int64(key, v)
	case float64:
		attr = attribute.Float64(key, v)
	case bool:
		attr = attribute.Bool(key, v)
	case []string:
		attr = attribute.StringSlice(key, v)
	case []int:
		attr = attribute.IntSlice(key, v)
	case []int64:
		attr = attribute.Int64Slice(key, v)
	case fmt.Stringer:
		attr = attribute.Stringer(key, v)
	case []byte:
		attr = attribute.String(key, string(v))
	default:
		// Fallback to string representation for unsupported types.
		attr = attribute.String(key, fmt.Sprintf("%v", val))
	}
	return attr
}
