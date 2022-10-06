package metrics

import (
	"context"
	"time"

	"github.com/yimi-go/errors"
	"github.com/yimi-go/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	Now   = time.Now
	Since = time.Since
)

type metricsWrapper struct {
	// Suggested labels:
	//   app_id: domain.biz.module
	//   env: prd/uat/fat/dev
	//   region: regionId. optional
	//   zone: zoneId. optional
	//   cluster: clusterId. optional
	//   host: hostname
	//   addr: ip:port
	//   transport: grpc
	//   actor: server
	// All these labels can be set before configure into this wrapper with WithXXX functions.

	// Count requests.
	// With labels:
	//   method: unary/stream
	//   api: fullMethod
	//   code: grpc status code
	//   reason: reason
	requestCounter metrics.Counter
	// Observe request handling duration.
	// With labels:
	//   method: unary/stream
	//   api: fullMethod
	//   code: grpc status code
	//   reason: reason
	requestDuration metrics.Observer
	// Gauge current inflight request count.
	// With labels:
	//   method: unary/stream
	//   api: fullMethod
	inflightRequests metrics.Gauge
}

// Option is metrics option func.
type Option func(mw *metricsWrapper)

// WithRequestCounter returns an Option that configures request counter.
// We suggest these labels are set before passing the counter to this function:
//
//	app_id: domain.biz.module
//	env: prd/uat/fat/dev
//	region: regionId. optional
//	zone: zoneId. optional
//	cluster: clusterId. optional
//	host: hostname
//	addr: ip:port
//	transport: grpc
//	actor: server
func WithRequestCounter(m metrics.Counter) Option {
	return func(mw *metricsWrapper) {
		mw.requestCounter = m
	}
}

// WithDurationObserver returns an Option that configures request handling duration observer.
// We suggest these labels are set before passing the observer to this function:
//
//	app_id: domain.biz.module
//	env: prd/uat/fat/dev
//	region: regionId. optional
//	zone: zoneId. optional
//	cluster: clusterId. optional
//	host: hostname
//	addr: ip:port
//	transport: grpc
//	actor: server
func WithDurationObserver(m metrics.Observer) Option {
	return func(mw *metricsWrapper) {
		mw.requestDuration = m
	}
}

// WithInflightGauge returns an Option that configures inflight request count gauge.
// We suggest these labels are set before passing the gauge to this function:
//
//	app_id: domain.biz.module
//	env: prd/uat/fat/dev
//	region: regionId. optional
//	zone: zoneId. optional
//	cluster: clusterId. optional
//	host: hostname
//	addr: ip:port
//	transport: grpc
//	actor: server
func WithInflightGauge(m metrics.Gauge) Option {
	return func(mw *metricsWrapper) {
		mw.inflightRequests = m
	}
}

// UnaryInterceptor returns a grpc.UnaryServerInterceptor that handle request metrics.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	mw := &metricsWrapper{}
	for _, opt := range opts {
		opt(mw)
	}
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		startTime := Now()
		method, api := "unary", info.FullMethod
		if mw.inflightRequests != nil {
			inflightGauge := mw.inflightRequests.With(map[string]string{
				"method": method, "api": api,
			})
			inflightGauge.Add(1)
			defer inflightGauge.Sub(1)
		}
		resp, err = handler(ctx, req)
		var (
			code   string
			reason string
		)
		se := errors.FromError(err)
		if se != nil {
			code, reason = se.GRPCStatus().Code().String(), se.Reason
		} else {
			code = codes.OK.String()
		}
		if mw.requestCounter != nil {
			mw.requestCounter.With(map[string]string{
				"method": method, "api": api, "code": code, "reason": reason,
			}).Inc()
		}
		if mw.requestDuration != nil {
			mw.requestDuration.With(map[string]string{
				"method": method, "api": api, "code": code, "reason": reason,
			}).Observe(float64(Since(startTime).Milliseconds()))
		}
		return resp, err
	}
}

// StreamInterceptor returns a grpc.StreamServerInterceptor that handle request metrics.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	mw := &metricsWrapper{}
	for _, opt := range opts {
		opt(mw)
	}
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		startTime := Now()
		method, api := "stream", info.FullMethod
		if mw.inflightRequests != nil {
			inflightGauge := mw.inflightRequests.With(map[string]string{
				"method": method, "api": api,
			})
			inflightGauge.Add(1)
			defer inflightGauge.Sub(1)
		}
		err := handler(srv, ss)
		var (
			code   string
			reason string
		)
		se := errors.FromError(err)
		if se != nil {
			code, reason = se.GRPCStatus().Code().String(), se.Reason
		} else {
			code = codes.OK.String()
		}
		if mw.requestCounter != nil {
			mw.requestCounter.With(map[string]string{
				"method": method, "api": api, "code": code, "reason": reason,
			}).Inc()
		}
		if mw.requestDuration != nil {
			mw.requestDuration.With(map[string]string{
				"method": method, "api": api, "code": code, "reason": reason,
			}).Observe(float64(Since(startTime).Milliseconds()))
		}
		return err
	}
}
