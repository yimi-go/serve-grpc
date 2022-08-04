package tracing

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
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
	return otelgrpc.UnaryServerInterceptor(op.otelgrpcOptions()...)
}

// StreamInterceptor returns a grpc.StreamServerInterceptor that trace request.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	op := &options{}
	for _, opt := range opts {
		opt(op)
	}
	return otelgrpc.StreamServerInterceptor(op.otelgrpcOptions()...)
}
