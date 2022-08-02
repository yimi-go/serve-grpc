package recover

import (
	"context"
	"fmt"

	"github.com/yimi-go/errors"
	"github.com/yimi-go/logging"
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
	logName string
}

func WithUnaryHandler(fn UnaryRecoverHandlerFunc) UnaryOption {
	return func(o *unaryOptions) {
		o.handler = fn
	}
}

func WithUnaryLogName(name string) UnaryOption {
	return func(o *unaryOptions) {
		o.logName = name
	}
}

func UnaryRecover(opts ...UnaryOption) grpc.UnaryServerInterceptor {
	op := unaryOptions{
		logName: "grpc.server.recovery",
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
				logger := logging.GetFactory().Logger(op.logName)
				logger = logging.WithContextField(ctx, logger)
				logger.Errorw(fmt.Sprintf("recovered from panic: %+v", rerr),
					logging.Stack("recover_at"),
				)

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
	logName string
}

func WithStreamHandler(fn StreamRecoverHandlerFunc) StreamOption {
	return func(o *streamOptions) {
		o.handler = fn
	}
}

func WithStreamLogName(name string) StreamOption {
	return func(o *streamOptions) {
		o.logName = name
	}
}

func StreamRecover(opts ...StreamOption) grpc.StreamServerInterceptor {
	op := streamOptions{
		logName: "grpc.server.recovery",
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
				logger := logging.GetFactory().Logger(op.logName)
				logger = logging.WithContextField(ss.Context(), logger)
				logger.Errorw(fmt.Sprintf("recovered from panic: %+v", rerr),
					logging.Stack("recover_at"),
				)

				err = op.handler(ss.Context(), info, rerr)
			}
		}()
		return handler(srv, ss)
	}
}
