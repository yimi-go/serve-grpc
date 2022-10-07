package tracing

import (
	"context"

	"github.com/yimi-go/logging"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type options struct {
	propagator propagation.TextMapPropagator
}

// Option is tracing option func.
type Option func(o *options)

// WithPropagator returns an Option that configure otel tracing propagator.
func WithPropagator(propagator propagation.TextMapPropagator) Option {
	return func(o *options) {
		o.propagator = propagator
	}
}

func (o *options) otelgrpcOptions() []otelgrpc.Option {
	var otelOptions []otelgrpc.Option
	if o.propagator != nil {
		otelOptions = append(otelOptions, otelgrpc.WithPropagators(o.propagator))
	}
	return otelOptions
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that trace request.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	op := &options{}
	for _, opt := range opts {
		opt(op)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		handler1 := func(ctx context.Context, req any) (any, error) {
			ctx = logging.NewContext(ctx,
				logging.String("trace_id", TraceID(ctx)),
				logging.String("span_id", SpanID(ctx)))
			return handler(ctx, req)
		}
		return otelgrpc.UnaryServerInterceptor(op.otelgrpcOptions()...)(ctx, req, info, handler1)
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *wrappedStream) Context() context.Context {
	return s.ctx
}

// StreamInterceptor returns a grpc.StreamServerInterceptor that trace request.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	op := &options{}
	for _, opt := range opts {
		opt(op)
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		handler1 := func(srv any, stream grpc.ServerStream) error {
			ctx := stream.Context()
			ctx = logging.NewContext(ctx,
				logging.String("trace_id", TraceID(ctx)),
				logging.String("span_id", SpanID(ctx)))
			stream = &wrappedStream{stream, ctx}
			return handler(srv, stream)
		}
		return otelgrpc.StreamServerInterceptor(op.otelgrpcOptions()...)(srv, ss, info, handler1)
	}
}

// TraceID returns the context span's trace id or blank if
// the context span does not exist or the context span has no trace id.
func TraceID(ctx context.Context) string {
	if span := trace.SpanContextFromContext(ctx); span.HasTraceID() {
		return span.TraceID().String()
	}
	return ""
}

// SpanID returns the context span's span id or blank if
// the context span does not exist or the context span has no span id.
func SpanID(ctx context.Context) string {
	if span := trace.SpanContextFromContext(ctx); span.HasSpanID() {
		return span.SpanID().String()
	}
	return ""
}
