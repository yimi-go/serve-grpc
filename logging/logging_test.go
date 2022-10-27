package logging

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/errors"
	"github.com/yimi-go/logging"
	zapLogging "github.com/yimi-go/zap-logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type jsonLine struct {
	Resp         any            `json:"resp,omitempty"`
	Param        map[string]any `json:"param,omitempty"`
	Level        string         `json:"level"`
	Logger       string         `json:"logger"`
	Ts           string         `json:"ts"`
	Caller       string         `json:"caller"`
	Msg          string         `json:"msg,omitempty"`
	Code         string         `json:"code,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Error        string         `json:"error,omitempty"`
	ErrorVerbose string         `json:"errorVerbose,omitempty"`
	Cost         int64          `json:"cost,omitempty"`
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

func TestWithLogName(t *testing.T) {
	o := &options{}
	name := "test"
	WithLogName(name)(o)
	assert.Equal(t, name, o.logName)
}

func TestUnaryInterceptor(t *testing.T) {
	t.Run("default_ok", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))
		originNowFn := Now
		originSinceFn := Since
		defer func() {
			Now = originNowFn
			Since = originSinceFn
		}()
		now := Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }

		interceptor := UnaryInterceptor()
		_, err = interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			now = now.Add(time.Second)
			return "mockResp", nil
		})
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("can not scan")
		}
		line := scanner.Bytes()
		t.Log(string(line))
		jl := &jsonLine{}
		err = json.Unmarshal(line, jl)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "receive request", jl.Msg)
		if !scanner.Scan() {
			t.Fatalf("can not scan")
		}
		line = scanner.Bytes()
		t.Log(string(line))
		jl = &jsonLine{}
		assert.Nil(t, json.Unmarshal(line, jl))
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "finish handling request", jl.Msg)
		assert.Equal(t, codes.OK.String(), jl.Code)
		assert.NotNil(t, jl.Resp)
		assert.Equal(t, int64(1000), jl.Cost)
	})
	t.Run("default_err", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))

		interceptor := UnaryInterceptor()
		_, err = interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			return nil, pkgerrors.New("error")
		})
		if err == nil {
			t.Errorf("expect err, got nil")
		}
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("can not scan")
		}
		t.Log(scanner.Text())
		if !scanner.Scan() {
			t.Fatalf("can not scan")
		}
		jl := &jsonLine{}
		t.Log(scanner.Text())
		err = json.Unmarshal(scanner.Bytes(), jl)
		if err != nil {
			t.Errorf("unexpected err: %v", err)
		}
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "fail handling request", jl.Msg)
		assert.Equal(t, codes.Unknown.String(), jl.Code)
		assert.Equal(t, errors.UnknownReason, jl.Reason)
		assert.Nil(t, jl.Resp)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.Error)
		t.Log(jl.ErrorVerbose)
	})
	t.Run("with_option", func(t *testing.T) {
		count := 0
		UnaryInterceptor(func(o *options) {
			count++
		})
		if count != 1 {
			t.Errorf("want 1, got %v", count)
		}
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("default_ok", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))
		originNowFn := Now
		originSinceFn := Since
		defer func() {
			Now = originNowFn
			Since = originSinceFn
		}()
		now := Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }

		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: context.Background(),
			recvFn: func(m any) error {
				sp, ok := m.(*string)
				if !ok {
					return errors.Internal("resv", "msg type mismatch")
				}
				*sp = "hello"
				return nil
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err = interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				msg := ""
				if err := stream.RecvMsg(&msg); err != nil {
					return err
				}
				for i := 0; i < 2; i++ {
					now = now.Add(time.Second)
					if err := stream.SendMsg("goodbye"); err != nil {
						return err
					}
				}
				return nil
			})
		if err != nil {
			t.Fatalf("unexpect err: %v", err)
		}
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes := scanner.Bytes()
		t.Log(string(bytes))
		jl := &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		if err != nil {
			t.Errorf("unexpect unmarshal err: %v", err)
		}
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "start streaming", jl.Msg)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes = scanner.Bytes()
		t.Log(string(bytes))
		jl = &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		if err != nil {
			t.Errorf("unexpect unmarshal err: %v", err)
		}
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "recv msg", jl.Msg)
		assert.NotNil(t, jl.Param["req"])
		for i := 0; i < 2; i++ {
			if !scanner.Scan() {
				t.Fatalf("want scan ok, but can not scan")
			}
			bytes = scanner.Bytes()
			t.Log(string(bytes))
			jl = &jsonLine{}
			err = json.Unmarshal(bytes, jl)
			if err != nil {
				t.Errorf("unexpect unmarshal err: %v", err)
			}
			assert.NotEmpty(t, jl.Logger)
			assert.Equal(t, "send msg", jl.Msg)
			assert.NotEmpty(t, jl.Resp)
		}
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes = scanner.Bytes()
		t.Log(string(bytes))
		jl = &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		if err != nil {
			t.Errorf("unexpect unmarshal err: %v", err)
		}
		assert.NotEmpty(t, jl.Logger)
		assert.Equal(t, "finish streaming", jl.Msg)
		assert.Equal(t, int64(2000), jl.Cost)
		assert.Equal(t, "OK", jl.Code)
	})
	t.Run("default_err", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))

		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: context.Background(),
			recvFn: func(m any) error {
				sp, ok := m.(*string)
				if !ok {
					return errors.Internal("resv", "msg type mismatch")
				}
				*sp = "hello"
				return nil
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err = interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return errors.Unimplemented("test", "test").
					WithCause(pkgerrors.New("test"))
			},
		)
		assert.NotNil(t, err)
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		t.Log(scanner.Text())
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes := scanner.Bytes()
		t.Log(string(bytes))
		jl := &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		assert.Nil(t, err)
		assert.Equal(t, "error streaming", jl.Msg)
		assert.Equal(t, codes.Unimplemented.String(), jl.Code)
		assert.Equal(t, "test", jl.Reason)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.ErrorVerbose)
	})
	t.Run("default_recv_err", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))

		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: context.Background(),
			recvFn: func(m any) error {
				return pkgerrors.New("test")
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err = interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return stream.RecvMsg(nil)
			},
		)
		assert.NotNil(t, err)
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		t.Log(scanner.Text())
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes := scanner.Bytes()
		t.Log(string(bytes))
		jl := &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		assert.Nil(t, err)
		assert.Equal(t, "recv msg fail", jl.Msg)
		assert.Empty(t, jl.Code)
		assert.Empty(t, jl.Reason)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.ErrorVerbose)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes = scanner.Bytes()
		t.Log(string(bytes))
		jl = &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		assert.Nil(t, err)
		assert.Equal(t, "error streaming", jl.Msg)
		assert.NotEmpty(t, jl.Code)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.ErrorVerbose)
	})
	t.Run("default_send_err", func(t *testing.T) {
		origin := os.Stdout
		defer func() {
			os.Stdout = origin
		}()
		r, w, err := os.Pipe()
		if err != nil {
			panic(err)
		}
		os.Stdout = w
		logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
			o.Levels = map[string]logging.Level{"": logging.InfoLevel}
		})))

		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: context.Background(),
			recvFn: func(m any) error {
				return nil
			},
			sendFn: func(m any) error {
				return pkgerrors.New("test")
			},
		}
		err = interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				for i := 0; i < 2; i++ {
					if err := stream.SendMsg("goodbye"); err != nil {
						return err
					}
				}
				return nil
			},
		)
		assert.NotNil(t, err)
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		t.Log(scanner.Text())
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes := scanner.Bytes()
		t.Log(string(bytes))
		jl := &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		assert.Nil(t, err)
		assert.Equal(t, "send msg fail", jl.Msg)
		assert.Empty(t, jl.Code)
		assert.Empty(t, jl.Reason)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.ErrorVerbose)
		if !scanner.Scan() {
			t.Fatalf("want scan ok, but can not scan")
		}
		bytes = scanner.Bytes()
		t.Log(string(bytes))
		jl = &jsonLine{}
		err = json.Unmarshal(bytes, jl)
		assert.Nil(t, err)
		assert.Equal(t, "error streaming", jl.Msg)
		assert.NotEmpty(t, jl.Code)
		assert.NotEmpty(t, jl.Error)
		assert.NotEmpty(t, jl.ErrorVerbose)
		t.Log(jl.ErrorVerbose)
	})
	tests := []struct {
		method       string
		serverStream bool
		clientStream bool
	}{
		{
			method:       "stream",
			serverStream: false,
			clientStream: false,
		},
		{
			method:       "serverStream",
			serverStream: true,
			clientStream: false,
		},
		{
			method:       "clientStream",
			serverStream: false,
			clientStream: true,
		},
		{
			method:       "bidirectionalStream",
			serverStream: true,
			clientStream: true,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("default_%s", test.method), func(t *testing.T) {
			origin := os.Stdout
			defer func() {
				os.Stdout = origin
			}()
			r, w, err := os.Pipe()
			if err != nil {
				panic(err)
			}
			os.Stdout = w
			logging.SwapFactory(zapLogging.NewFactory(zapLogging.NewOptions(func(o *zapLogging.Options) {
				o.Levels = map[string]logging.Level{"": logging.InfoLevel}
			})))

			interceptor := StreamInterceptor()
			ss := &mockStream{
				ctx: context.Background(),
				recvFn: func(m any) error {
					return nil
				},
				sendFn: func(m any) error {
					return nil
				},
			}
			info := &grpc.StreamServerInfo{
				FullMethod:     "testMethod",
				IsClientStream: test.clientStream,
				IsServerStream: test.serverStream,
			}
			err = interceptor(
				nil,
				ss,
				info,
				func(srv any, stream grpc.ServerStream) error {
					return nil
				},
			)
			assert.Nil(t, err)
			scanner := bufio.NewScanner(r)
			if !scanner.Scan() {
				t.Fatalf("want scan ok, but can not scan")
			}
			t.Log(scanner.Text())
			bytes := scanner.Bytes()
			t.Log(string(bytes))
			jl := &jsonLine{}
			err = json.Unmarshal(bytes, jl)
			assert.Nil(t, err)
		})
	}
	t.Run("with_opts", func(t *testing.T) {
		count := 0
		interceptor := StreamInterceptor(func(o *options) {
			count++
		})
		ss := &mockStream{
			ctx: context.Background(),
			recvFn: func(m any) error {
				return nil
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		_ = interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return nil
			},
		)
		assert.Equal(t, 1, count)
	})
}

type wolf struct{}

func (wolf) String() string {
	return "I'm a wolf."
}

func (wolf) LogReplace() any {
	return sheep{}
}

type sheep struct{}

func (sheep) String() string {
	return "I'm a sheep."
}

func Test_msgLogging(t *testing.T) {
	type args struct {
		m any
	}
	tests := []struct {
		name string
		args args
		want any
	}{
		{
			name: "sheep",
			args: args{m: sheep{}},
			want: sheep{},
		},
		{
			name: "wolf",
			args: args{m: wolf{}},
			want: sheep{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, msgLogging(tt.args.m), "msgLogging(%v)", tt.args.m)
		})
	}
}
