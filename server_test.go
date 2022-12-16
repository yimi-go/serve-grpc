package serve_grpc

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	runserver "github.com/yimi-go/runner/server"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
)

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

func TestWithName(t *testing.T) {
	srv := &Server{}
	v := "testRunner"
	WithName(v)(srv)
	assert.Equal(t, v, srv.name)
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
		wantName := ""
		if !reflect.DeepEqual(srv.name, wantName) {
			t.Errorf("got srv.name: %v, want: %v", srv.name, wantName)
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
	t.Run("name", func(t *testing.T) {
		srv := NewServer(plainLisFn, WithName("my.server"))
		if !reflect.DeepEqual(srv.name, "my.server") {
			t.Errorf("want srv.name: %v, got %v", "my.server", srv.name)
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
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))

		x509KeyPair, err := tls.LoadX509KeyPair("testdata/rsa-cert.pem", "testdata/rsa-key.pem")
		assert.Nilf(t, err, "err loading x509 key pair: %v", err)
		srv := NewServer(plainLisFn, WithTLSConfig(&tls.Config{Certificates: []tls.Certificate{x509KeyPair}}))
		go func() {
			err := srv.Run(ctx)
			assert.Nilf(t, err, "unexpected err: %v", err)
		}()
		time.Sleep(time.Millisecond)
		defer func(srv *Server, ctx context.Context) {
			_ = srv.Stop(ctx)
		}(srv, ctx)

		assert.NotNilf(t, srv.lis, "expect a listener, got nil")
		resp, err := srv.health.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: ""})
		assert.Nilf(t, err, "unexpected err: %v", err)
		assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
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
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
	t.Run("listen_plain", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		srv := NewServer(plainLisFn)
		go func() {
			err := srv.Run(ctx)
			assert.Nilf(t, err, "unexpected err: %v", err)
		}()
		time.Sleep(time.Millisecond)
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
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
}

func TestServer_Stop(t *testing.T) {
	t.Run("common", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		go func() {
			err := srv.Run(context.Background())
			assert.Nilf(t, err, "unexpected err: %v", err)
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		err := srv.Stop(ctx)
		assert.Nilf(t, err, "unexpected err: %v", err)
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
		assert.Len(t, mps, 2)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
	})
	t.Run("cancel", func(t *testing.T) {
		srv := NewServer(plainLisFn)
		go func() {
			err := srv.Run(context.Background())
			assert.Nilf(t, err, "unexpected err: %v", err)
		}()
		time.Sleep(time.Millisecond) // let Run goroutine do the job.
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		err := srv.Stop(ctx)
		assert.NotNilf(t, err, "want err, got nil")
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
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
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
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		_, _ = srv.unaryServerInterceptor()(
			ctx,
			&struct{}{},
			&grpc.UnaryServerInfo{FullMethod: "some method"},
			func(ctx context.Context, req any) (any, error) {
				slog.Ctx(ctx).Info("hello")
				return &struct{}{}, nil
			},
		)
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
		assert.Equal(t, "grpc", mp["transport"])
		assert.Equal(t, "server", mp["actor"])
		assert.Equal(t, "127.0.0.1:9989", mp["address"])
		assert.Equal(t, "unary", mp["method"])
		assert.Equal(t, "some method", mp["api"])
		param, ok := mp["param"].(map[string]any)
		assert.True(t, ok)
		assert.NotNil(t, param["req"])
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
			func(srv any, stream grpc.ServerStream) error { return nil },
		)
		assert.NotNilf(t, err, "want error, got nil")
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
			logBuf := &bytes.Buffer{}
			ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
			err := srv.streamServerInterceptor()(
				&struct{}{},
				&mockServerStream{metadata.MD{}, ctx},
				&grpc.StreamServerInfo{
					FullMethod:     test.api,
					IsClientStream: test.clientStream,
					IsServerStream: test.serverStream,
				},
				func(srv any, stream grpc.ServerStream) error {
					slog.Ctx(stream.Context()).Info("hello")
					return nil
				},
			)
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
			assert.Equal(t, "grpc", mp["transport"])
			assert.Equal(t, "server", mp["actor"])
			assert.Equal(t, "127.0.0.1:9989", mp["address"])
			assert.Equal(t, test.wantMethod, mp["method"])
			assert.Equal(t, test.api, mp["api"])
		})
	}
}
