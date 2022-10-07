package validate

import (
	"context"

	"github.com/yimi-go/errors"
	"google.golang.org/grpc"
)

type validator interface {
	Validate() error
}

type bizValidator interface {
	validator
	BizValidate() error
}

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		v, ok := req.(validator)
		if !ok {
			return handler(ctx, req)
		}
		if err := v.Validate(); err != nil {
			return nil, errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
		}
		v2, ok := v.(bizValidator)
		if !ok {
			return handler(ctx, req)
		}
		if err := v2.BizValidate(); err != nil {
			return nil, errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
		}
		return handler(ctx, req)
	}
}

func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &wrappedStream{ss})
	}
}

type wrappedStream struct {
	grpc.ServerStream
}

func (w *wrappedStream) RecvMsg(m any) error {
	err := w.ServerStream.RecvMsg(m)
	if err != nil {
		return err
	}
	v, ok := m.(validator)
	if !ok {
		return nil
	}
	if err = v.Validate(); err != nil {
		return errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
	}
	v2, ok := m.(bizValidator)
	if !ok {
		return nil
	}
	if err = v2.BizValidate(); err != nil {
		return errors.InvalidArgument("VALIDATE", err.Error()).WithCause(err)
	}
	return nil
}
