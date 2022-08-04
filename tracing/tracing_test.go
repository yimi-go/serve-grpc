package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
)

func TestWithPropagator(t *testing.T) {
	op := &options{}
	p := otel.GetTextMapPropagator()
	WithPropagator(p)(op)
	assert.Equal(t, p, op.propagator)
}

func Test_options_otelgrpcOptions(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		op := &options{}
		assert.Empty(t, op.otelgrpcOptions())
	})
	t.Run("p", func(t *testing.T) {
		p := otel.GetTextMapPropagator()
		op := &options{propagator: p}
		otelOptions := op.otelgrpcOptions()
		assert.Equal(t, 1, len(otelOptions))
		assert.Equal(t, otelgrpc.WithPropagators(p), otelOptions[0])
	})
}

func TestUnaryInterceptor(t *testing.T) {
	t.Run("opts", func(t *testing.T) {
		count := 0
		UnaryInterceptor(func(o *options) {
			count++
		})
		assert.Equal(t, 1, count)
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("opts", func(t *testing.T) {
		count := 0
		StreamInterceptor(func(o *options) {
			count++
		})
		assert.Equal(t, 1, count)
	})
}
