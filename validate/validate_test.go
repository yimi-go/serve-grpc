package validate

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/yimi-go/serve-grpc/internal/hello"
)

func TestUnaryInterceptor(t *testing.T) {
	t.Run("not_validator", func(t *testing.T) {
		handleCount := 0
		interceptor := UnaryInterceptor()
		resp, err := interceptor(context.Background(), &msgNotValidator{}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCount++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, resp)
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
	t.Run("validate_fail", func(t *testing.T) {
		handleCount := 0
		interceptor := UnaryInterceptor()
		resp, err := interceptor(context.Background(), &msgValidator{
			vErr: fmt.Errorf("abc"),
		}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCount++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, resp)
		assert.NotNil(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Equal(t, 0, handleCount)
	})
	t.Run("validate_ok", func(t *testing.T) {
		handleCount := 0
		interceptor := UnaryInterceptor()
		resp, err := interceptor(context.Background(), &msgValidator{
			vErr: nil,
		}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCount++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, resp)
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
	t.Run("server_validate_fail", func(t *testing.T) {
		handleCount := 0
		interceptor := UnaryInterceptor()
		resp, err := interceptor(context.Background(), &msgServerValidator{
			vErr:  nil,
			svErr: fmt.Errorf("abc"),
		}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCount++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.Nil(t, resp)
		assert.NotNil(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Equal(t, 0, handleCount)
	})
	t.Run("biz_validate_ok", func(t *testing.T) {
		handleCount := 0
		interceptor := UnaryInterceptor()
		resp, err := interceptor(context.Background(), &msgServerValidator{
			vErr:  nil,
			svErr: nil,
		}, &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			handleCount++
			return &hello.SayHelloResponse{Message: "xyz"}, nil
		})
		assert.NotNil(t, resp)
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("recv_fail", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
		err := interceptor(nil, &mockStream{
			header: metadata.Pairs(),
			ctx:    context.Background(),
			sendFn: func(m any) error { return nil },
			recvFn: func(m any) error { return fmt.Errorf("abc") },
		}, &grpc.StreamServerInfo{
			FullMethod:     "testMethod",
			IsClientStream: false,
			IsServerStream: true,
		}, func(srv any, stream grpc.ServerStream) error {
			m := &msgNotValidator{}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.NotNil(t, err)
		assert.Equal(t, 0, handleCount)
	})
	t.Run("not_validator", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
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
			m := &msgNotValidator{}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
	t.Run("validate_fail", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
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
			m := &msgValidator{
				vErr: fmt.Errorf("abc"),
			}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.NotNil(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Equal(t, 0, handleCount)
	})
	t.Run("validate_ok", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
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
			m := &msgValidator{
				vErr: nil,
			}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
	t.Run("biz_validate_fail", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
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
			m := &msgServerValidator{
				vErr:  nil,
				svErr: fmt.Errorf("abc"),
			}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.NotNil(t, err)
		assert.True(t, errors.IsInvalidArgument(err))
		assert.Equal(t, 0, handleCount)
	})
	t.Run("biz_validate_ok", func(t *testing.T) {
		handleCount := 0
		interceptor := StreamInterceptor()
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
			m := &msgServerValidator{
				vErr:  nil,
				svErr: nil,
			}
			if err := stream.RecvMsg(m); err != nil {
				return err
			}
			handleCount++
			return nil
		})
		assert.Nil(t, err)
		assert.Equal(t, 1, handleCount)
	})
}

type msgNotValidator struct {
}

type msgValidator struct {
	vErr error
}

func (m *msgValidator) Validate() error {
	return m.vErr
}

type msgServerValidator struct {
	vErr  error
	svErr error
}

func (m *msgServerValidator) ServerValidate(server any) error {
	return m.svErr
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
