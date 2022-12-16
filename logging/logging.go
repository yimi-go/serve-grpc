package logging

import (
	"context"
	"fmt"
	"time"

	"github.com/yimi-go/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	Now   = time.Now
	Since = time.Since
)

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that do logging.
func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		start := Now()
		logger := slog.Ctx(ctx)
		if logger.Enabled(slog.InfoLevel) {
			logger.Info("receive request", slog.Any("req", req))
		}
		resp, err = handler(ctx, req)
		if err == nil {
			if logger.Enabled(slog.InfoLevel) {
				logger.Info("finish handling request",
					slog.String("code", codes.OK.String()),
					slog.Duration("cost", Since(start)),
					slog.Any("resp", resp),
				)
			}
		} else {
			se := errors.FromError(err)
			if logger.Enabled(slog.InfoLevel) {
				logger.Info("fail handling request",
					slog.String("code", se.GRPCStatus().Code().String()),
					slog.String("reason", se.Reason),
					slog.Duration("cost", Since(start)),
					slog.String("err", fmt.Sprintf("%+v", err)),
				)
			}
		}
		return
	}
}

type wrappedStream struct {
	grpc.ServerStream
	logger *slog.Logger
	start  time.Time
}

func (w *wrappedStream) SendMsg(m any) error {
	err := w.ServerStream.SendMsg(m)
	if err == nil {
		if w.logger.Enabled(slog.InfoLevel) {
			w.logger.Info("send msg", slog.Any("resp", m))
		}
	} else {
		if w.logger.Enabled(slog.InfoLevel) {
			w.logger.Info("send msg fail",
				slog.Any("resp", m),
				slog.String("err", fmt.Sprintf("%+v", err)),
			)
		}
	}
	return err
}

func (w *wrappedStream) RecvMsg(m any) error {
	err := w.ServerStream.RecvMsg(m)
	if err == nil {
		param := map[string]any{
			"req": slog.AnyValue(m).Resolve(),
		}
		if w.logger.Enabled(slog.InfoLevel) {
			w.logger.Info("recv msg", slog.Any("param", param))
		}
	} else {
		if w.logger.Enabled(slog.InfoLevel) {
			w.logger.Info("recv msg fail",
				slog.String("err", fmt.Sprintf("%+v", err)))
		}
	}
	return err
}

// StreamInterceptor returns a grpc.StreamServerInterceptor that do logging.
func StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := Now()
		logger := slog.Ctx(ss.Context())
		logger.Info("start streaming")
		err := handler(srv, &wrappedStream{
			ServerStream: ss,
			logger:       logger,
			start:        start,
		})
		if err == nil {
			if logger.Enabled(slog.InfoLevel) {
				logger.Info("finish streaming",
					slog.String("code", codes.OK.String()),
					slog.Duration("cost", Since(start)),
				)
			}
		} else {
			se := errors.FromError(err)
			if logger.Enabled(slog.InfoLevel) {
				logger.Info("error streaming",
					slog.String("code", se.GRPCStatus().Code().String()),
					slog.String("reason", se.Reason),
					slog.Duration("cost", Since(start)),
					slog.String("err", fmt.Sprintf("%+v", err)),
				)
			}
		}
		return err
	}
}
