package tracing

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"golang.org/x/exp/slog"
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
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		interceptor := UnaryInterceptor()
		var handleCtx context.Context
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCtx = ctx
			logger := slog.Ctx(ctx)
			logger.Info("foo")
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		mp := mps[0]
		assert.Equal(t, slog.InfoLevel.String(), mp[slog.LevelKey])
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
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		ew := &bytes.Buffer{}
		tpDefer := initTp(ew)
		defer tpDefer()
		interceptor := UnaryInterceptor()
		var handleCtx context.Context
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCtx = ctx
			logger := slog.Ctx(ctx)
			logger.Info("foo")
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		mp := mps[0]
		assert.Equal(t, slog.InfoLevel.String(), mp[slog.LevelKey])
		traceId, ok := mp["trace_id"]
		assert.True(t, ok)
		assert.NotEmpty(t, traceId)
		assert.Equal(t, traceId, TraceID(handleCtx))
		spanId, ok := mp["span_id"]
		assert.True(t, ok)
		assert.NotEmpty(t, spanId)
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
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		ew := &bytes.Buffer{}
		tpDefer := initTp(ew)
		defer tpDefer()
		interceptor := StreamInterceptor()
		var handleCtx context.Context
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    ctx,
			sendFn: func(m any) error { return nil },
			recvFn: func(m any) error { return nil },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			handleCtx = stream.Context()
			logger := slog.Ctx(stream.Context())
			logger.Info("foo")
			return nil
		})
		assert.Nil(t, err)
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		mp := mps[0]
		assert.Equal(t, slog.InfoLevel.String(), mp[slog.LevelKey])
		traceId, ok := mp["trace_id"]
		assert.True(t, ok)
		assert.Equal(t, traceId, TraceID(handleCtx))
		spanId, ok := mp["span_id"]
		assert.True(t, ok)
		assert.Equal(t, spanId, SpanID(handleCtx))
	})
}

func initTp(w io.Writer) func() {
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
		otel.SetTracerProvider(otp)
		if err := tp.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}
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
