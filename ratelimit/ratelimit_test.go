package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	limiter "github.com/yimi-go/rate-limiter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/yimi-go/serve-grpc/internal/hello"
)

func Test_defaultOptions(t *testing.T) {
	o := defaultOptions()
	id := o.id(context.Background(), "haha")
	assert.Equal(t, "haha", id)
}

func TestErrLimitExceed(t *testing.T) {
	err := ErrLimitExceed()
	assert.Same(t, errLimitExceed, err)
}

func TestIsErrLimitExceed(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		args args
		name string
		want bool
	}{
		{
			name: "not",
			args: args{err: errors.New("some err")},
			want: false,
		},
		{
			name: "just",
			args: args{err: errLimitExceed},
			want: true,
		},
		{
			name: "wrapped",
			args: args{fmt.Errorf("abc: %w", errLimitExceed)},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsErrLimitExceed(tt.args.err); got != tt.want {
				t.Errorf("IsErrNotAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithLimiter(t *testing.T) {
	o := &options{}
	l := &testLimiter{}
	WithLimiter(l)(o)
	assert.Same(t, l, o.limiter)
}

func TestWithId(t *testing.T) {
	o := &options{}
	f := func(ctx context.Context, api string) string {
		return "haha"
	}
	WithId(f)(o)
	assert.NotNil(t, o.id)
	assert.Equal(t, "haha", o.id(context.Background(), ""))
}

func TestUnaryInterceptor(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		handleCounter := 0
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{err: limiter.ErrLimitExceed()}))
		resp, err := interceptor(context.Background(), &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCounter++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, errLimitExceed)
	})
	t.Run("queueing_timeout", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{
			ready: make(chan struct{}),
		}
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCounter++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, limiter.StateTimeout, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	t.Run("queueing_canceled", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{
			ready: make(chan struct{}),
		}
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer func() { cancel() }()
		go func() {
			<-time.After(time.Millisecond)
			cancel()
		}()
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCounter++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, limiter.StateErr, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	closedCh := make(chan struct{})
	close(closedCh)
	t.Run("handle_ok", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCounter++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 1, handleCounter)
		assert.Equal(t, limiter.StateOk, token.doneState)
		assert.Nil(t, token.doneErr)
	})
	t.Run("handle_timeout", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
				handleCounter++
				return &hello.SayHelloResponse{Message: "xyz"}, nil
			}
		})
		assert.NotNil(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, limiter.StateTimeout, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	t.Run("handle_canceled", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := UnaryInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-time.After(time.Millisecond)
			cancel()
		}()
		defer func() { cancel() }()
		resp, err := interceptor(ctx, &hello.SayHelloRequest{Name: "abc"}, &grpc.UnaryServerInfo{
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
				handleCounter++
				return &hello.SayHelloResponse{Message: "xyz"}, nil
			}
		})
		assert.NotNil(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, limiter.StateErr, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		handleCounter := 0
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{err: limiter.ErrLimitExceed()}))
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
			handleCounter++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, errLimitExceed)
	})
	t.Run("queueing_timeout", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{
			ready: make(chan struct{}),
		}
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
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
			handleCounter++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, limiter.StateTimeout, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	t.Run("queueing_canceled", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{
			ready: make(chan struct{}),
		}
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		defer func() { cancel() }()
		go func() {
			<-time.After(time.Millisecond)
			cancel()
		}()
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
			handleCounter++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, limiter.StateErr, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	closedCh := make(chan struct{})
	close(closedCh)
	t.Run("handle_ok", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
		sendCount := 0
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    ctx,
			sendFn: func(m any) error {
				sendCount++
				return nil
			},
			recvFn: func(m any) error { return nil },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			for i := 0; i < 10; i++ {
				select {
				case <-stream.Context().Done():
					return stream.Context().Err()
				default:
				}
				if err := stream.SendMsg("abc"); err != nil {
					return err
				}
			}
			handleCounter++
			return nil
		})
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCounter)
		assert.Equal(t, 10, sendCount)
		assert.Equal(t, limiter.StateOk, token.doneState)
		assert.Nil(t, token.doneErr)
	})
	t.Run("handle_timeout", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, time.Millisecond)
		defer func() { cancel() }()
		sendCount := 0
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    ctx,
			sendFn: func(m any) error {
				sendCount++
				return nil
			},
			recvFn: func(m any) error { return nil },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			for i := 0; i < 10; i++ {
				select {
				case <-stream.Context().Done():
					return stream.Context().Err()
				case <-time.After(time.Millisecond):
				}
				if err := stream.SendMsg("abc"); err != nil {
					return err
				}
			}
			handleCounter++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, limiter.StateTimeout, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
	t.Run("handle_canceled", func(t *testing.T) {
		handleCounter := 0
		token := &testToken{ready: closedCh}
		interceptor := StreamInterceptor(WithLimiter(&testLimiter{token: token}))
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		go func() {
			<-time.After(time.Millisecond)
			cancel()
		}()
		defer func() { cancel() }()
		sendCount := 0
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    ctx,
			sendFn: func(m any) error {
				sendCount++
				return nil
			},
			recvFn: func(m any) error { return nil },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			for i := 0; i < 10; i++ {
				select {
				case <-stream.Context().Done():
					return stream.Context().Err()
				case <-time.After(time.Millisecond):
				}
				if err := stream.SendMsg("abc"); err != nil {
					return err
				}
			}
			handleCounter++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCounter)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, limiter.StateErr, token.doneState)
		assert.Equal(t, err, token.doneErr)
	})
}

type testLimiter struct {
	token limiter.Token
	err   error
}

func (l *testLimiter) Allow(_ string) (limiter.Token, error) {
	return l.token, l.err
}

type testToken struct {
	doneErr   error
	ready     <-chan struct{}
	doneState limiter.State
}

func (t *testToken) Ready() <-chan struct{} {
	return t.ready
}

func (t *testToken) Done(state limiter.State, err error) {
	t.doneState, t.doneErr = state, err
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
