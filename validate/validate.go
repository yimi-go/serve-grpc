package validate

import (
	"context"

	"github.com/yimi-go/errors"
	"google.golang.org/grpc"
)

type validator interface {
	Validate() error
}

type serverValidator interface {
	ServerValidate(server any) error
}

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if v, ok := req.(validator); ok {
			if err := v.Validate(); err != nil {
				return nil, errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
			}
		}
		if v, ok := req.(serverValidator); ok {
			if err := v.ServerValidate(info.Server); err != nil {
				return nil, errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
			}
		}
		return handler(ctx, req)
	}
}

func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &wrappedStream{ss, srv})
	}
}

type wrappedStream struct {
	grpc.ServerStream
	server any
}

func (w *wrappedStream) RecvMsg(m any) error {
	err := w.ServerStream.RecvMsg(m)
	if err != nil {
		return err
	}
	if v, ok := m.(validator); ok {
		if err = v.Validate(); err != nil {
			return errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
		}
	}
	if v, ok := m.(serverValidator); ok {
		if err = v.ServerValidate(w.server); err != nil {
			return errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
		}
	}
	return nil
}
