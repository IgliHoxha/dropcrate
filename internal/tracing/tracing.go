// Package tracing configures optional OpenTelemetry distributed tracing. It is
// disabled unless an OTLP endpoint is configured through the standard
// OTEL_EXPORTER_OTLP_ENDPOINT (or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT)
// environment variable, in which case spans are exported over OTLP/gRPC.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Setup installs a global tracer provider that exports spans over OTLP/gRPC
// when an endpoint is configured, and returns a shutdown function that flushes
// pending spans. When no endpoint is set it is a no-op: the global tracer stays
// the default no-op provider and the returned shutdown does nothing.
func Setup(ctx context.Context) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	name := os.Getenv("OTEL_SERVICE_NAME")
	if name == "" {
		name = "dropcrate"
	}
	res := resource.NewSchemaless(attribute.String("service.name", name))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return tp.Shutdown, nil
}
