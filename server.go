package serve_grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	runserver "github.com/yimi-go/runner/server"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

type ListenFunc func() (net.Listener, error)

// ServerOption is gRPC server option.
type ServerOption func(s *Server)

func WithName(name string) ServerOption {
	return func(s *Server) {
		s.name = name
	}
}

// WithTimeout returns a ServerOption that config timeout.
func WithTimeout(timeout time.Duration) ServerOption {
	return func(s *Server) {
		s.timeout = timeout
	}
}

// WithTLSConfig returns a ServerOption that append grpc.Creds option with the tls.Config.
// If the tls.Config is nil, the returned ServerOption would do nothing.
func WithTLSConfig(c *tls.Config) ServerOption {
	return func(s *Server) {
		if c == nil {
			return
		}
		s.explicitTls = true
		s.grpcOpts = append(s.grpcOpts, grpc.Creds(credentials.NewTLS(c)))
	}
}

// WithUnaryInterceptor returns a ServerOption that sets the UnaryServerInterceptor for the server.
func WithUnaryInterceptor(in ...grpc.UnaryServerInterceptor) ServerOption {
	return func(s *Server) {
		s.unaryInts = in
	}
}

// WithStreamInterceptor returns a ServerOption that sets the StreamServerInterceptor for the server.
func WithStreamInterceptor(in ...grpc.StreamServerInterceptor) ServerOption {
	return func(s *Server) {
		s.streamInts = in
	}
}

// WithOption returns a ServerOption that sets grpc.ServerOption for the server.
func WithOption(opts ...grpc.ServerOption) ServerOption {
	return func(s *Server) {
		s.grpcOpts = opts
	}
}

// Server is a gRPC server runner.
type Server struct {
	lis net.Listener
	*grpc.Server
	lisFn       ListenFunc
	health      *health.Server
	name        string
	unaryInts   []grpc.UnaryServerInterceptor
	streamInts  []grpc.StreamServerInterceptor
	grpcOpts    []grpc.ServerOption
	timeout     time.Duration
	mux         sync.Mutex
	explicitTls bool
}

// NewServer creates a gRPC server by options.
// Note the required listen func should not produce a TSL listener.
func NewServer(lisFn ListenFunc, opts ...ServerOption) *Server {
	srv := &Server{
		lisFn:   lisFn,
		timeout: 1 * time.Second,
		health:  health.NewServer(),
	}
	for _, o := range opts {
		o(srv)
	}
	unaryInts := []grpc.UnaryServerInterceptor{
		srv.unaryServerInterceptor(),
	}
	streamInts := []grpc.StreamServerInterceptor{
		srv.streamServerInterceptor(),
	}
	if len(srv.unaryInts) > 0 {
		unaryInts = append(unaryInts, srv.unaryInts...)
	}
	if len(srv.streamInts) > 0 {
		streamInts = append(streamInts, srv.streamInts...)
	}
	grpcOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInts...),
		grpc.ChainStreamInterceptor(streamInts...),
	}
	if len(srv.grpcOpts) > 0 {
		grpcOpts = append(grpcOpts, srv.grpcOpts...)
	}
	srv.Server = grpc.NewServer(grpcOpts...)
	grpc_health_v1.RegisterHealthServer(srv.Server, srv.health)
	reflection.Register(srv.Server)
	return srv
}

func (s *Server) Name() string {
	if len(s.name) > 0 {
		return s.name
	}
	return "gRPC server"
}

func (s *Server) Run(ctx context.Context) error {
	lis, err := s.lisFn()
	if err != nil {
		return err
	}
	defer func(lis net.Listener) {
		_ = lis.Close()
	}(lis)
	func() {
		s.mux.Lock()
		defer s.mux.Unlock()
		s.lis = lis
	}()
	logger := slog.Ctx(ctx)
	if logger.Enabled(slog.InfoLevel) {
		logger.Info(fmt.Sprintf("%s listening on: %s", s.Name(), lis.Addr().String()))
	}
	s.health.Resume()
	return s.Server.Serve(lis)
}

func (s *Server) Stop(ctx context.Context) error {
	logger := slog.Ctx(ctx)
	if logger.Enabled(slog.InfoLevel) {
		logger.Info(fmt.Sprintf("%s stopping", s.Name()))
	}
	s.health.Shutdown()
	ch := make(chan struct{})
	go func() {
		defer func() {
			close(ch)
		}()
		s.Server.GracefulStop()
		s.mux.Lock()
		defer s.mux.Unlock()
		s.lis = nil
		if logger.Enabled(slog.InfoLevel) {
			logger.Info("[gRPC] server stopped")
		}
	}()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) Address() (net.Addr, error) {
	s.mux.Lock()
	defer s.mux.Unlock()
	lis := s.lis
	if lis == nil {
		return nil, runserver.NotServing()
	}
	return lis.Addr(), nil
}

func (s *Server) unaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		address, err := s.Address()
		if err != nil {
			return
		}
		param := map[string]any{
			"req": req,
		}
		ctx = slog.NewContext(ctx, slog.Ctx(ctx).With(
			slog.String("transport", "grpc"),
			slog.String("actor", "server"),
			slog.String("address", address.String()),
			slog.String("method", "unary"),
			slog.String("api", info.FullMethod),
			slog.Any("param", param),
		))
		if s.timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, s.timeout)
			defer cancel()
		}
		return handler(ctx, req)
	}
}

type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context {
	return w.ctx
}

func (s *Server) streamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		address, err := s.Address()
		if err != nil {
			return err
		}
		method := "stream"
		if info.IsServerStream && info.IsClientStream {
			method = "bidirectionalStream"
		} else if info.IsServerStream {
			method = "serverStream"
		} else if info.IsClientStream {
			method = "clientStream"
		}
		ctx := ss.Context()
		ctx = slog.NewContext(ctx, slog.Ctx(ctx).With(
			slog.String("transport", "grpc"),
			slog.String("actor", "server"),
			slog.String("address", address.String()),
			slog.String("method", method),
			slog.String("api", info.FullMethod),
		))
		return handler(srv, &wrappedStream{ss, ctx})
	}
}
