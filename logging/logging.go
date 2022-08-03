package logging

import (
	"context"
	"time"

	"github.com/yimi-go/errors"
	"github.com/yimi-go/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	Now   = time.Now
	Since = time.Since
)

// Option is logging interceptor option func.
type Option func(o *options)

type options struct {
	logName string
}

// WithLogName sets the log name.
// Default log name is "grpc.server.logging".
func WithLogName(name string) Option {
	return func(o *options) {
		o.logName = name
	}
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that do logging.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	opt := &options{
		logName: "grpc.server.logging",
	}
	for _, o := range opts {
		o(opt)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		start := Now()
		logger := logging.GetFactory().Logger(opt.logName)
		logger = logging.WithContextField(ctx, logger)
		logger.Infow("receive request")
		resp, err = handler(ctx, req)
		if err == nil {
			logger.Infow("finish handling request",
				logging.String("code", codes.OK.String()),
				logging.Duration("cost", Since(start)),
				logging.Any("resp", resp),
			)
		} else {
			se := errors.FromError(err)
			logger.Infow("fail handling request",
				logging.String("code", se.GRPCStatus().Code().String()),
				logging.String("reason", se.Reason),
				logging.Duration("cost", Since(start)),
				logging.Error(err),
			)
		}
		return
	}
}

type wrappedStream struct {
	grpc.ServerStream
	logger logging.Logger
	start  time.Time
}

func (w *wrappedStream) SendMsg(m any) error {
	err := w.ServerStream.SendMsg(m)
	if err == nil {
		w.logger.Infow("send msg", logging.Any("resp", m))
	} else {
		w.logger.Infow("send msg fail",
			logging.Any("resp", m),
			logging.Error(err),
		)
	}
	return err
}

func (w *wrappedStream) RecvMsg(m any) error {
	err := w.ServerStream.RecvMsg(m)
	if err == nil {
		param := map[string]any{
			"req": m,
		}
		w.logger.Infow("recv msg", logging.Any("param", param))
	} else {
		w.logger.Infow("recv msg fail", logging.Error(err))
	}
	return err
}

// StreamInterceptor returns a grpc.StreamServerInterceptor that do logging.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	opt := &options{
		logName: "grpc.server.logging",
	}
	for _, o := range opts {
		o(opt)
	}
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {
		start := Now()
		logger := logging.GetFactory().Logger(opt.logName)
		logger = logging.WithContextField(ss.Context(), logger)
		logger.Infow("start streaming")
		err := handler(srv, &wrappedStream{
			ServerStream: ss,
			logger:       logger,
			start:        start,
		})
		if err == nil {
			logger.Infow("finish streaming",
				logging.String("code", codes.OK.String()),
				logging.Duration("cost", Since(start)),
			)
		} else {
			se := errors.FromError(err)
			logger.Infow("error streaming",
				logging.String("code", se.GRPCStatus().Code().String()),
				logging.String("reason", se.Reason),
				logging.Duration("cost", Since(start)),
				logging.Error(err),
			)
		}
		return err
	}
}
