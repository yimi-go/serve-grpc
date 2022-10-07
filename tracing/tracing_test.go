package tracing

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/logging"
	zapLogging "github.com/yimi-go/zap-logging"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/yimi-go/serve-grpc/internal/hello"
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
		_ = UnaryInterceptor(func(o *options) {
			count++
		})
		assert.Equal(t, 1, count)

	})
	t.Run("no_tp", func(t *testing.T) {
		loggingDefer, r := initLogging()
		defer loggingDefer()
		defer func() { _ = r.Close() }()

		interceptor := UnaryInterceptor()
		var handleCtx context.Context
		resp, err := interceptor(context.Background(), &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCtx = ctx
			logger := logging.GetFactory().Logger("test")
			logger = logging.WithContextField(ctx, logger)
			logger.Info("foo")
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		scanner := bufio.NewScanner(r)
		assert.True(t, scanner.Scan(), "scan fail")
		mp := map[string]any{}
		assert.Nil(t, json.Unmarshal(scanner.Bytes(), &mp))
		traceId, ok := mp["trace_id"]
		assert.True(t, ok)
		assert.Empty(t, traceId)
		assert.Equal(t, traceId, TraceID(handleCtx))
		spanId, ok := mp["span_id"]
		assert.True(t, ok)
		assert.Empty(t, spanId)
		assert.Equal(t, spanId, SpanID(handleCtx))
	})
	t.Run("stdout_trace", func(t *testing.T) {
		ew := &bytes.Buffer{}
		tpDefer, otp := initTp(ew)
		defer tpDefer()
		defer func() {
			otel.SetTracerProvider(otp)
		}()
		loggingDefer, r := initLogging()
		defer loggingDefer()
		defer func() { _ = r.Close() }()

		interceptor := UnaryInterceptor()
		var handleCtx context.Context
		resp, err := interceptor(context.Background(), &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCtx = ctx
			logger := logging.GetFactory().Logger("test")
			logger = logging.WithContextField(ctx, logger)
			logger.Info("foo")
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		scanner := bufio.NewScanner(r)
		assert.True(t, scanner.Scan(), "scan fail")
		mp := map[string]any{}
		assert.Nil(t, json.Unmarshal(scanner.Bytes(), &mp))
		traceId, ok := mp["trace_id"]
		assert.True(t, ok)
		assert.NotEmpty(t, traceId)
		t.Logf("traceId: %s", traceId)
		assert.Equal(t, traceId, TraceID(handleCtx))
		spanId, ok := mp["span_id"]
		assert.True(t, ok)
		assert.NotEmpty(t, spanId)
		t.Logf("span_id: %s", spanId)
		assert.Equal(t, spanId, SpanID(handleCtx))
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
	t.Run("tp", func(t *testing.T) {
		loggingDefer, r := initLogging()
		defer loggingDefer()
		defer func() { _ = r.Close() }()

		interceptor := StreamInterceptor()
		var handleCtx context.Context
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    context.Background(),
			sendFn: func(m any) error { return nil },
			recvFn: func(m any) error { return nil },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			handleCtx = stream.Context()
			logger := logging.GetFactory().Logger("test")
			logger = logging.WithContextField(stream.Context(), logger)
			logger.Info("foo")
			return nil
		})
		assert.Nil(t, err)
		scanner := bufio.NewScanner(r)
		assert.True(t, scanner.Scan(), "scan fail")
		mp := map[string]any{}
		assert.Nil(t, json.Unmarshal(scanner.Bytes(), &mp))
		traceId, ok := mp["trace_id"]
		assert.True(t, ok)
		t.Logf("traceId: %s", traceId)
		assert.Equal(t, traceId, TraceID(handleCtx))
		spanId, ok := mp["span_id"]
		assert.True(t, ok)
		t.Logf("span_id: %s", spanId)
		assert.Equal(t, spanId, SpanID(handleCtx))
	})
}

func initLogging() (func(), io.ReadCloser) {
	origin := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w
	logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
		o.Levels = map[string]logging.Level{"": logging.InfoLevel}
	})))
	return func() {
		os.Stdout = origin
	}, r
}

func initTp(w io.Writer) (func(), trace.TracerProvider) {
	exp, err := newExporter(w)
	if err != nil {
		panic(err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(newResource()),
	)
	otp := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}, otp
}

func newExporter(w io.Writer) (sdktrace.SpanExporter, error) {
	return stdouttrace.New(
		stdouttrace.WithWriter(w),
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithoutTimestamps(),
	)
}

func newResource() *resource.Resource {
	r, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("test"),
			semconv.ServiceVersionKey.String("v0.0.1"),
		),
	)
	return r
}

type mockStream struct {
	header metadata.MD
	ctx    context.Context
	sendFn func(m any) error
	recvFn func(m any) error
}

func (m *mockStream) SetHeader(md metadata.MD) error {
	m.header = metadata.Join(m.header, md)
	return nil
}

func (m *mockStream) SendHeader(_ metadata.MD) error {
	return nil
}

func (m *mockStream) SetTrailer(_ metadata.MD) {
}

func (m *mockStream) Context() context.Context {
	return m.ctx
}

func (m *mockStream) SendMsg(msg any) error {
	return m.sendFn(msg)
}

func (m *mockStream) RecvMsg(msg interface{}) error {
	return m.recvFn(msg)
}
