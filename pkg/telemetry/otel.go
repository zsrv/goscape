package telemetry

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitProviders configures the global OTel metric, tracer, and logger providers.
// If cfg.Enabled is false, providers are left as their no-op defaults and
// shutdown returns nil. shutdown is idempotent.
func InitProviders(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	if !cfg.Enabled {
		var once sync.Once
		return func(context.Context) error { once.Do(func() {}); return nil }, nil
	}

	host, _ := os.Hostname()
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("goscape"),
			semconv.ServiceInstanceID(uuid.NewString()),
			semconv.HostName(host),
		),
	)
	if err != nil {
		return nil, err
	}

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.OTLP.Endpoint),
		insecureMetric(cfg.OTLP.Insecure),
	)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
	)
	otel.SetMeterProvider(mp)

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLP.Endpoint),
		insecureTrace(cfg.OTLP.Insecure),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.OTLP.SampleRatio)),
	)
	otel.SetTracerProvider(tp)

	logExp, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(cfg.OTLP.Endpoint),
		insecureLog(cfg.OTLP.Insecure),
	)
	if err != nil {
		return nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
	)
	otellogglobal.SetLoggerProvider(lp)

	var once sync.Once
	shutdown = func(sctx context.Context) error {
		var combined error
		once.Do(func() {
			if e := mp.Shutdown(sctx); e != nil {
				combined = errors.Join(combined, e)
			}
			if e := tp.Shutdown(sctx); e != nil {
				combined = errors.Join(combined, e)
			}
			if e := lp.Shutdown(sctx); e != nil {
				combined = errors.Join(combined, e)
			}
		})
		return combined
	}
	return shutdown, nil
}

func insecureMetric(b bool) otlpmetricgrpc.Option {
	if b {
		return otlpmetricgrpc.WithInsecure()
	}
	return otlpmetricgrpc.WithCompressor("gzip")
}
func insecureTrace(b bool) otlptracegrpc.Option {
	if b {
		return otlptracegrpc.WithInsecure()
	}
	return otlptracegrpc.WithCompressor("gzip")
}
func insecureLog(b bool) otlploggrpc.Option {
	if b {
		return otlploggrpc.WithInsecure()
	}
	return otlploggrpc.WithCompressor("gzip")
}
