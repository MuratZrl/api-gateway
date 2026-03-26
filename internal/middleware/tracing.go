package middleware

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

// InitTracer initializes the OpenTelemetry tracer
func InitTracer(ctx context.Context, serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = tp.Tracer(serviceName)
	return tp, nil
}

func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tracer == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Extract trace context from incoming request
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		spanName := r.Method + " " + normalizePath(r.URL.Path)
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.URLPath(r.URL.Path),
				semconv.URLQuery(r.URL.RawQuery),
				semconv.UserAgentOriginal(r.UserAgent()),
				semconv.ClientAddress(r.RemoteAddr),
			),
		)
		defer span.End()

		// Add trace ID to response headers
		traceID := span.SpanContext().TraceID().String()
		w.Header().Set("X-Trace-ID", traceID)

		// Add trace ID to request for downstream use
		r.Header.Set("X-Trace-ID", traceID)

		recorder := &tracingResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(recorder, r.WithContext(ctx))

		span.SetAttributes(
			semconv.HTTPResponseStatusCode(recorder.statusCode),
		)

		if recorder.statusCode >= 400 {
			span.SetStatus(codes.Error, http.StatusText(recorder.statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}
	})
}

// StartSpan creates a child span for internal operations
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}
	return tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *tracingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}
