package recover

import (
	"context"
	"fmt"

	"github.com/yimi-go/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
)

type UnaryRecoverHandlerFunc func(
	ctx context.Context,
	info *grpc.UnaryServerInfo,
	req any,
	err any,
) error

type UnaryOption func(*unaryOptions)

type unaryOptions struct {
	handler UnaryRecoverHandlerFunc
}

func WithUnaryHandler(fn UnaryRecoverHandlerFunc) UnaryOption {
	return func(o *unaryOptions) {
		o.handler = fn
	}
}

func UnaryRecover(opts ...UnaryOption) grpc.UnaryServerInterceptor {
	op := unaryOptions{
		handler: func(ctx context.Context, info *grpc.UnaryServerInfo, req any, err any) error {
			return errors.Unknown("UNKNOWN", "an unknown error occurred")
		},
	}
	for _, o := range opts {
		o(&op)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if rerr := recover(); rerr != nil {
				logger := slog.Ctx(ctx)
				if logger.Enabled(slog.ErrorLevel) {
					logger.Log(slog.ErrorLevel,
						"recovered from panic",
						slog.String("err", fmt.Sprintf("%+v", rerr)))
				}

				err = op.handler(ctx, info, req, rerr)
			}
		}()
		return handler(ctx, req)
	}
}

type StreamRecoverHandlerFunc func(ctx context.Context, info *grpc.StreamServerInfo, rerr any) error

type StreamOption func(o *streamOptions)

type streamOptions struct {
	handler StreamRecoverHandlerFunc
}

func WithStreamHandler(fn StreamRecoverHandlerFunc) StreamOption {
	return func(o *streamOptions) {
		o.handler = fn
	}
}

func StreamRecover(opts ...StreamOption) grpc.StreamServerInterceptor {
	op := streamOptions{
		handler: func(ctx context.Context, info *grpc.StreamServerInfo, rerr any) error {
			return errors.Unknown("UNKNOWN", "an unknown error occurred")
		},
	}
	for _, o := range opts {
		o(&op)
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if rerr := recover(); rerr != nil {
				logger := slog.Ctx(ss.Context())
				if logger.Enabled(slog.ErrorLevel) {
					logger.Log(slog.ErrorLevel, "recovered from panic",
						slog.String("err", fmt.Sprintf("%+v", rerr)))
				}

				err = op.handler(ss.Context(), info, rerr)
			}
		}()
		return handler(srv, ss)
	}
}
