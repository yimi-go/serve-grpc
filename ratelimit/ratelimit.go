package ratelimit

import (
	"context"

	afr "github.com/yimi-go/afr-rate-limiter"
	"github.com/yimi-go/afr-rate-limiter/stat"
	"github.com/yimi-go/errors"
	limiter "github.com/yimi-go/rate-limiter"
	"google.golang.org/grpc"
)

var errLimitExceed = errors.ResourceExhausted("RATE_LIMIT", "rate limitation exceeded")

// ErrLimitExceed returns an error indicates an exceeding of rate limitation.
func ErrLimitExceed() error {
	return errLimitExceed
}

// IsErrLimitExceed checks whether an error is indicating an exceeding of rate limitation.
// This function supports wrapped errors.
func IsErrLimitExceed(err error) bool {
	return errors.Is(err, errLimitExceed)
}

type options struct {
	limiter limiter.Limiter
	id      func(ctx context.Context, api string) string
}

// Option is rate limit option.
type Option func(*options)

// WithLimiter set Limiter implementation.
func WithLimiter(limiter limiter.Limiter) Option {
	return func(o *options) {
		o.limiter = limiter
	}
}

// WithId set id extract function.
// The default function use just the api as request id.
func WithId(f func(ctx context.Context, api string) string) Option {
	return func(o *options) {
		o.id = f
	}
}

func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		allow, err := o.limiter.Allow(o.id(ctx, info.FullMethod))
		if err != nil {
			// 立即被拒绝
			return nil, ErrLimitExceed()
		}
		defer func() {
			state := limiter.StateOk
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					state = limiter.StateTimeout
				} else {
					state = limiter.StateErr
				}
			}
			allow.Done(state, err)
		}()
		// 等待调度
		select {
		case <-ctx.Done():
			// 超时
			err = ctx.Err()
			return
		case <-allow.Ready():
		}
		// 执行
		resp, err = handler(ctx, req)
		return
	}
}

func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		allow, err := o.limiter.Allow(o.id(ctx, info.FullMethod))
		if err != nil {
			// 立即被拒绝
			return ErrLimitExceed()
		}
		defer func() {
			state := limiter.StateOk
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					state = limiter.StateTimeout
				} else {
					state = limiter.StateErr
				}
			}
			allow.Done(state, err)
		}()
		// 等待调度
		select {
		case <-ctx.Done():
			// 超时
			err = ctx.Err()
			return err
		case <-allow.Ready():
		}
		// 执行
		err = handler(srv, ss)
		return err
	}
}

func defaultOptions() *options {
	return &options{
		limiter: afr.New(stat.New(stat.WithRTEmaDecay(0.5))),
		id:      func(ctx context.Context, api string) string { return api },
	}
}
