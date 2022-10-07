package serve_grpc

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/logging"
	"github.com/yimi-go/logging/hook"
	runserver "github.com/yimi-go/runner/server"
	zapLogging "github.com/yimi-go/zap-logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

type record struct {
	meth  string
	param []any
}

type jsonLine struct {
	Param     map[string]any `json:"param,omitempty"`
	Other     map[string]any `json:"-"`
	Transport string         `json:"transport"`
	Actor     string         `json:"actor"`
	Address   string         `json:"address"`
	Method    string         `json:"method"`
	Api       string         `json:"api"`
	Msg       string         `json:"msg"`
}

type mockServerStream struct {
	md  metadata.MD
	ctx context.Context
}

func (m *mockServerStream) SetHeader(md metadata.MD) error {
	m.md = metadata.Join(m.md, md)
	return nil
}
func (m *mockServerStream) SendHeader(_ metadata.MD) error { return nil }
func (m *mockServerStream) SetTrailer(_ metadata.MD)       {}
func (m *mockServerStream) Context() context.Context       { return m.ctx }
func (m *mockServerStream) SendMsg(_ any) error            { return nil }
func (m *mockServerStream) RecvMsg(_ any) error            { return nil }

var plainLisFn = func() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

func plainLisPort(port uint16) ListenFunc {
	return func() (net.Listener, error) {
		return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	}
}

func TestWithLogName(t *testing.T) {
	srv := &Server{}
	v := "test"
	WithLogName(v)(srv)
	if v != srv.logName {
		t.Errorf("expect %s, got %s", v, srv.logName)
	}
}

func TestWithTimeout(t *testing.T) {
	srv := &Server{}
	v := time.Second
	WithTimeout(v)(srv)
	if v != srv.timeout {
		t.Errorf("want %v, got %v", v, srv.timeout)
	}
}

func TestWithTLSConfig(t *testing.T) {
	t.Run("no_tls", func(t *testing.T) {
		srv := &Server{}
		WithTLSConfig(nil)(srv)
		if srv.explicitTls {
			t.Errorf("expect false, got true")
		}
		if len(srv.grpcOpts) > 0 {
			t.Errorf("expect no grpcOpts, got some")
		}
	})
	t.Run("tls", func(t *testing.T) {
		srv := &Server{}
		WithTLSConfig(&tls.Config{})(srv)
		if !srv.explicitTls {
			t.Errorf("expect true, got false")
		}
		if len(srv.grpcOpts) == 0 {
			t.Errorf("expect some grpcOpts, got nothing")
		}
	})
}

func TestWithUnaryInterceptor(t *testing.T) {
	srv := &Server{}
	v := []grpc.UnaryServerInterceptor{
		func(
			ctx context.Context,
			req any,
			info *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (resp any, err error) {
			return nil, nil
		},
	}
	WithUnaryInterceptor(v...)(srv)
	if !reflect.DeepEqual(v, srv.unaryInts) {
		t.Errorf("want %v, got %v", v, srv.unaryInts)
	}
}

func TestWithStreamInterceptor(t *testing.T) {
	srv := &Server{}
	v := []grpc.StreamServerInterceptor{
		func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return nil
		},
	}
	WithStreamInterceptor(v...)(srv)
	if !reflect.DeepEqual(v, srv.streamInts) {
		t.Errorf("want %v, got %v", v, srv.streamInts)
	}
}

func TestWithOption(t *testing.T) {
	srv := &Server{}
	v := []grpc.ServerOption{
		grpc.EmptyServerOption{},
	}
	WithOption(v...)(srv)
	if !reflect.DeepEqual(v, srv.grpcOpts) {
		t.Errorf("expect %v, got %v", v, srv.grpcOpts)
	}
}

func TestNewServer(t *testing.T) {
	t.Run("no_option", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		if srv.Server == nil {
			t.Errorf("want srv.gServer not nil, got nil")
		}
		wantLogName := "grpc.server"
		if !reflect.DeepEqual(srv.logName, wantLogName) {
			t.Errorf("got srv.logName: %v, want: %v", srv.logName, wantLogName)
		}
		if len(srv.grpcOpts) != 0 {
			t.Errorf("expect empty srv.grpcOpts, got %v", srv.grpcOpts)
		}
		if _, ok := srv.Server.GetServiceInfo()["grpc.health.v1.Health"]; !ok {
			t.Errorf("expect Health service registed, but not")
		}
		if _, ok := srv.Server.GetServiceInfo()["grpc.reflection.v1alpha.ServerReflection"]; !ok {
			t.Errorf("expect Reflection service registed, but not")
		}
	})
	t.Run("logName", func(t *testing.T) {
		srv := NewServer(plainLisFn, WithLogName("my.server"))
		if !reflect.DeepEqual(srv.logName, "my.server") {
			t.Errorf("want srv.logName: %v, got %v", "my.server", srv.logName)
		}
	})
	t.Run("grpcOption", func(t *testing.T) {
		srv := NewServer(plainLisFn, WithOption(grpc.EmptyServerOption{}))
		if len(srv.grpcOpts) == 0 {
			t.Errorf("expect grpc options not empty, but is")
		}
	})
	t.Run("unaryInts", func(t *testing.T) {
		srv := NewServer(
			plainLisFn,
			WithUnaryInterceptor(
				func(
					ctx context.Context,
					req any,
					info *grpc.UnaryServerInfo,
					handler grpc.UnaryHandler,
				) (resp any, err error) {
					return nil, nil
				},
			),
		)
		if len(srv.unaryInts) == 0 {
			t.Errorf("expect unary interceptors, got empty")
		}
	})
	t.Run("streamInts", func(t *testing.T) {
		srv := NewServer(
			plainLisFn,
			WithStreamInterceptor(
				func(
					srv any,
					ss grpc.ServerStream,
					info *grpc.StreamServerInfo,
					handler grpc.StreamHandler,
				) error {
					return nil
				},
			),
		)
		if len(srv.streamInts) == 0 {
			t.Errorf("expect stream interceptors, got empty")
		}
	})
}

func TestServer_logger(t *testing.T) {
	srv := NewServer(plainLisFn)
	logger := srv.logger(context.Background())
	if logger == nil {
		t.Errorf("expect a logger, got nil")
	}
}

func TestServer_Prepare(t *testing.T) {
	srv := NewServer(plainLisFn)
	if err := srv.Prepare(context.Background()); err != nil {
		t.Errorf("unexpect err: %v", err)
	}
}

func TestServer_Run(t *testing.T) {
	t.Run("listen_err", func(t *testing.T) {
		srv := NewServer(func() (net.Listener, error) {
			return nil, errors.New("some error")
		})
		if err := srv.Run(context.Background()); err == nil {
			t.Errorf("want err, got nil")
		}
	})
	t.Run("listen_with_grpc_creds", func(t *testing.T) {
		ch := make(chan *record, 10)
		originFactory := logging.SwapFactory(
			hook.Hooked(logging.NewNopLoggerFactory(), func(meth string, param ...any) {
				ch <- &record{meth, param}
			}),
		)
		defer func() {
			logging.SwapFactory(originFactory)
		}()
		x509KeyPair, err := tls.LoadX509KeyPair("testdata/rsa-cert.pem", "testdata/rsa-key.pem")
		if err != nil {
			t.Fatalf("err loading x509 key pair: %v", err)
		}
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{x509KeyPair}}
		if err != nil {
			t.Fatalf("unexpect err loading tls config: %v", err)
		}
		srv := NewServer(plainLisFn, WithTLSConfig(tlsConfig))
		go func() {
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		select {
		case r := <-ch:
			if r.meth != "Infof" {
				t.Errorf("expect Infof log, got %v", r)
			}
			t.Logf(r.param[0].(string), r.param[1].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
		if srv.lis == nil {
			t.Errorf("expect a listener, got nil")
		}
		resp, err := srv.health.Check(
			context.Background(),
			&grpc_health_v1.HealthCheckRequest{
				Service: "",
			},
		)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("expect health status: %v, got %v",
				grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
		}
	})
	t.Run("listen_plain", func(t *testing.T) {
		ch := make(chan *record, 10)
		originFactory := logging.SwapFactory(
			hook.Hooked(logging.NewNopLoggerFactory(), func(meth string, param ...any) {
				ch <- &record{meth, param}
			}),
		)
		defer func() {
			logging.SwapFactory(originFactory)
		}()
		srv := NewServer(plainLisFn)
		go func() {
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		select {
		case r := <-ch:
			if r.meth != "Infof" {
				t.Errorf("expect Infof log, got %v", r)
			}
			t.Logf(r.param[0].(string), r.param[1].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
		if srv.lis == nil {
			t.Errorf("expect a listener, got nil")
		}
		resp, err := srv.health.Check(
			context.Background(),
			&grpc_health_v1.HealthCheckRequest{
				Service: "",
			},
		)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Errorf("expect health status: %v, got %v",
				grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
		}
	})
}

func TestServer_Stop(t *testing.T) {
	t.Run("common", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		go func() {
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		ch := make(chan *record, 10)
		originFactory := logging.SwapFactory(
			hook.Hooked(logging.NewNopLoggerFactory(), func(meth string, param ...any) {
				ch <- &record{meth, param}
			}),
		)
		defer func() {
			logging.SwapFactory(originFactory)
		}()
		if err := srv.Stop(context.Background()); err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		select {
		case r := <-ch:
			if r.meth != "Info" {
				t.Errorf("expected Info log, got %v", r)
			}
			t.Log(r.param[0].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
		select {
		case r := <-ch:
			if r.meth != "Info" {
				t.Errorf("expected Info log, got %v", r)
			}
			t.Log(r.param[0].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		go func() {
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		ch := make(chan *record, 10)
		originFactory := logging.SwapFactory(
			hook.Hooked(logging.NewNopLoggerFactory(), func(meth string, param ...any) {
				ch <- &record{meth, param}
			}),
		)
		defer func() {
			logging.SwapFactory(originFactory)
		}()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := srv.Stop(ctx)
		if err == nil {
			t.Errorf("want err, got nil")
		}
		select {
		case r := <-ch:
			if r.meth != "Info" {
				t.Errorf("expected Info log, got %v", r)
			}
			t.Log(r.param[0].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
		select {
		case r := <-ch:
			if r.meth != "Info" {
				t.Errorf("expected Info log, got %v", r)
			}
			t.Log(r.param[0].([]any)...)
		case <-time.After(time.Millisecond):
			t.Errorf("timeout")
		}
	})
}

func TestServer_Address(t *testing.T) {
	t.Run("before_run", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		_, err := srv.Address()
		if err == nil || !runserver.IsErrNotServing(err) {
			t.Errorf("expect err not serving, got %v", err)
		}
	})
	t.Run("running", func(t *testing.T) {
		address := "127.0.0.1"
		port := uint16(9989)
		srv := NewServer(plainLisPort(port))
		downSign := make(chan struct{})
		go func() {
			defer func() {
				downSign <- struct{}{}
			}()
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		defer func() {
			_ = srv.Stop(context.Background())
		}()
		addr, err := srv.Address()
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := fmt.Sprintf("%s:%d", address, port)
		if want != addr.String() {
			t.Errorf("want %s, got %s", want, addr.String())
		}
	})
	t.Run("after_stop", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		downSign := make(chan struct{})
		go func() {
			defer func() {
				downSign <- struct{}{}
			}()
			if err := srv.Run(context.Background()); err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		if err := srv.Stop(context.Background()); err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		_, err := srv.Address()
		if err == nil || !runserver.IsErrNotServing(err) {
			t.Errorf("expect err not serving, got %v", err)
		}
	})
}

func TestServer_unaryServerInterceptor(t *testing.T) {
	t.Run("not_start", func(t *testing.T) {
		srv := &Server{}
		_, err := srv.unaryServerInterceptor()(
			context.Background(),
			&struct{}{},
			&grpc.UnaryServerInfo{},
			func(ctx context.Context, req any) (any, error) {
				return &struct{}{}, nil
			},
		)
		if err == nil {
			t.Errorf("want error, got nil")
		}
	})
	t.Run("no_timeout", func(t *testing.T) {
		srv := NewServer(plainLisPort(9989), WithTimeout(0))
		down := make(chan struct{})
		go func() {
			defer func() {
				down <- struct{}{}
			}()
			_ = srv.Run(context.Background())
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		defer func() {
			_ = srv.Stop(context.Background())
		}()
		_, err := srv.unaryServerInterceptor()(
			context.Background(),
			&struct{}{},
			&grpc.UnaryServerInfo{FullMethod: "some method"},
			func(ctx context.Context, req any) (any, error) {
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("unexpected context Done")
				case <-time.After(time.Millisecond):
				}
				return &struct{}{}, nil
			},
		)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		srv := NewServer(plainLisPort(9989), WithTimeout(10*time.Millisecond))
		down := make(chan struct{})
		go func() {
			defer func() {
				down <- struct{}{}
			}()
			_ = srv.Run(context.Background())
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		defer func() {
			_ = srv.Stop(context.Background())
		}()
		_, err := srv.unaryServerInterceptor()(
			context.Background(),
			&struct{}{},
			&grpc.UnaryServerInfo{FullMethod: "some method"},
			func(ctx context.Context, req any) (any, error) {
				time.Sleep(20 * time.Millisecond)
				select {
				case <-ctx.Done():
				case <-time.After(time.Millisecond):
					return nil, fmt.Errorf("unexpected timeout")
				}
				return &struct{}{}, nil
			},
		)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
	})
	t.Run("logging", func(t *testing.T) {
		srv := NewServer(plainLisPort(9989))
		down := make(chan struct{})
		startSignal := make(chan struct{}, 1)
		go func() {
			defer func() {
				close(down)
			}()
			startSignal <- struct{}{}
			_ = srv.Run(context.Background())
		}()
		defer func() {
			<-down
		}()
		<-startSignal
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		defer func() {
			_ = srv.Stop(context.Background())
		}()
		_, _ = srv.unaryServerInterceptor()(
			context.Background(),
			&struct{}{},
			&grpc.UnaryServerInfo{FullMethod: "some method"},
			func(ctx context.Context, req any) (any, error) {
				origin := os.Stdout
				defer func() {
					os.Stdout = origin
				}()
				r, w, err := os.Pipe()
				if err != nil {
					panic(err)
				}
				os.Stdout = w
				factory := zapLogging.NewFactory(zapLogging.NewOptions())
				logger := factory.Logger("test")
				logger = logging.WithContextField(ctx, logger)
				logger.Info("hello")
				scanner := bufio.NewScanner(r)
				assert.True(t, scanner.Scan())
				assert.Nil(t, scanner.Err())
				assert.NotEmpty(t, scanner.Text())
				jl := &jsonLine{}
				assert.Nil(t, json.Unmarshal(scanner.Bytes(), jl))
				assert.Equal(t, "grpc", jl.Transport)
				assert.Equal(t, "server", jl.Actor)
				assert.Equal(t, "127.0.0.1:9989", jl.Address)
				assert.Equal(t, "unary", jl.Method)
				assert.Equal(t, "some method", jl.Api)
				assert.NotNil(t, jl.Param["req"])
				return &struct{}{}, nil
			},
		)
	})
}

func Test_wrappedStream_Context(t *testing.T) {
	ws := &wrappedStream{
		ctx: context.Background(),
	}
	if !reflect.DeepEqual(context.Background(), ws.Context()) {
		t.Errorf("want %v, got %v", context.Background(), ws.Context())
	}
}

func TestServer_streamServerInterceptor(t *testing.T) {
	t.Run("not_start", func(t *testing.T) {
		srv := &Server{}
		err := srv.streamServerInterceptor()(
			&struct{}{},
			&mockServerStream{metadata.MD{}, context.Background()},
			&grpc.StreamServerInfo{},
			func(srv any, stream grpc.ServerStream) error {
				return nil
			},
		)
		if err == nil {
			t.Errorf("want error, got nil")
		}
	})
	tests := []struct {
		api          string
		wantMethod   string
		clientStream bool
		serverStream bool
	}{
		{
			api:          "imposable",
			clientStream: false,
			serverStream: false,
			wantMethod:   "stream",
		},
		{
			api:          "receive",
			clientStream: true,
			serverStream: false,
			wantMethod:   "clientStream",
		},
		{
			api:          "send",
			clientStream: false,
			serverStream: true,
			wantMethod:   "serverStream",
		},
		{
			api:          "two-way",
			clientStream: true,
			serverStream: true,
			wantMethod:   "bidirectionalStream",
		},
	}
	for _, test := range tests {
		t.Run(test.api, func(t *testing.T) {
			srv := NewServer(plainLisPort(9989))
			down := make(chan struct{})
			startSignal := make(chan struct{}, 1)
			go func() {
				defer func() {
					close(down)
				}()
				startSignal <- struct{}{}
				_ = srv.Run(context.Background())
			}()
			defer func() {
				<-down
			}()
			<-startSignal
			time.Sleep(time.Millisecond) // let Run goroutine do the job.
			defer func() {
				_ = srv.Stop(context.Background())
			}()
			err := srv.streamServerInterceptor()(
				&struct{}{},
				&mockServerStream{metadata.MD{}, context.Background()},
				&grpc.StreamServerInfo{
					FullMethod:     test.api,
					IsClientStream: test.clientStream,
					IsServerStream: test.serverStream,
				},
				func(srv any, stream grpc.ServerStream) error {
					origin := os.Stdout
					defer func() {
						os.Stdout = origin
					}()
					r, w, err := os.Pipe()
					if err != nil {
						panic(err)
					}
					os.Stdout = w
					factory := zapLogging.NewFactory(zapLogging.NewOptions())
					logger := factory.Logger("test")
					logger = logging.WithContextField(stream.Context(), logger)
					logger.Info("hello")
					scanner := bufio.NewScanner(r)
					assert.True(t, scanner.Scan())
					assert.Nil(t, scanner.Err())
					assert.NotEmpty(t, scanner.Text())
					jl := &jsonLine{}
					assert.Nil(t, json.Unmarshal(scanner.Bytes(), jl))
					assert.Equal(t, "grpc", jl.Transport)
					assert.Equal(t, "server", jl.Actor)
					assert.Equal(t, "127.0.0.1:9989", jl.Address)
					assert.Equal(t, test.wantMethod, jl.Method)
					assert.Equal(t, test.api, jl.Api)
					return nil
				},
			)
			assert.Nil(t, err)
		})
	}
}
