package recover

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcinsecure "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	serve_grpc "github.com/yimi-go/serve-grpc"
	"github.com/yimi-go/serve-grpc/internal/hello"
)

type testHelloSvc struct {
	hello.UnimplementedGreeterServiceServer
	sayHelloFn     func(ctx context.Context, request *hello.SayHelloRequest) (*hello.SayHelloResponse, error)
	serverGreetsFn func(request *hello.ServerGreetsRequest, srv hello.GreeterService_ServerGreetsServer) error
}

func (t *testHelloSvc) SayHello(ctx context.Context, request *hello.SayHelloRequest) (*hello.SayHelloResponse, error) {
	return t.sayHelloFn(ctx, request)
}

func (t *testHelloSvc) ServerGreets(
	request *hello.ServerGreetsRequest,
	srv hello.GreeterService_ServerGreetsServer,
) error {
	return t.serverGreetsFn(request, srv)
}

func lis127(port uint16) serve_grpc.ListenFunc {
	return func() (net.Listener, error) {
		return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	}
}

func TestWithUnaryHandler(t *testing.T) {
	ch := make(chan struct{}, 10)
	testHandler := UnaryRecoverHandlerFunc(
		func(ctx context.Context, info *grpc.UnaryServerInfo, req any, err any) error {
			ch <- struct{}{}
			return errors.Unknown("unknown", "")
		},
	)
	target := &unaryOptions{}
	WithUnaryHandler(testHandler)(target)
	_ = target.handler(context.Background(), &grpc.UnaryServerInfo{}, "", "")
	select {
	case <-ch:
	case <-time.After(time.Millisecond):
		t.Errorf("timeout")
	}
}

func TestUnaryRecover(t *testing.T) {
	t.Run("no_panic", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		srvErr := make(chan error)
		svc := &testHelloSvc{
			sayHelloFn: func(ctx context.Context, request *hello.SayHelloRequest) (*hello.SayHelloResponse, error) {
				return &hello.SayHelloResponse{Message: fmt.Sprintf("Hello, %s", request.Name)}, nil
			},
		}
		srv := serve_grpc.NewServer(
			lis127(0),
			serve_grpc.WithOption(grpc.ChainUnaryInterceptor(UnaryRecover())),
		)
		hello.RegisterGreeterServiceServer(srv, svc)
		go func() {
			if err := srv.Run(ctx); err != nil {
				srvErr <- err
			}
			close(srvErr)
		}()
		defer func(ctx context.Context) {
			_ = srv.Stop(ctx)
			e, ok := <-srvErr
			if ok {
				t.Errorf("unexpect Run err: %v", e)
			}
		}(ctx)
		time.Sleep(time.Millisecond) // let server start running
		address, _ := srv.Address()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(
			ctx,
			address.String(),
			grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("err dialing %v: %v", address.String(), err)
		}
		greeterClient := hello.NewGreeterServiceClient(conn)
		_, err = greeterClient.SayHello(ctx, &hello.SayHelloRequest{Name: "abc"})
		if err != nil {
			t.Fatalf("err calling SayHello: %v", err)
		}
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
	t.Run("panic", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		srvErr := make(chan error)
		svc := &testHelloSvc{
			sayHelloFn: func(ctx context.Context, request *hello.SayHelloRequest) (*hello.SayHelloResponse, error) {
				panic("abc")
			},
		}
		srv := serve_grpc.NewServer(
			lis127(0),
			serve_grpc.WithOption(grpc.ChainUnaryInterceptor(UnaryRecover())),
		)
		hello.RegisterGreeterServiceServer(srv, svc)
		go func(ctx context.Context) {
			if err := srv.Run(ctx); err != nil {
				srvErr <- err
			}
			close(srvErr)
		}(ctx)
		defer func(ctx context.Context) {
			_ = srv.Stop(ctx)
			<-srvErr
		}(ctx)
		time.Sleep(time.Millisecond) // let server start running
		address, _ := srv.Address()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(
			ctx,
			address.String(),
			grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("err dialing %v: %v", address.String(), err)
		}
		greeterClient := hello.NewGreeterServiceClient(conn)
		_, err = greeterClient.SayHello(ctx, &hello.SayHelloRequest{Name: "abc"})
		if err == nil {
			t.Fatalf("expect err, got nil")
		}
		gCode := errors.Code(err)
		if gCode != codes.Unknown {
			t.Fatalf("expect %v, got %v", codes.Unknown, gCode)
		}
		reason := errors.Reason(err)
		t.Logf("reason: %s", reason)
		xErr := errors.FromError(err)
		wantMessage := "an unknown error occurred"
		if xErr.Message != wantMessage {
			t.Errorf("want message: %v, got %v", wantMessage, xErr.Message)
		}
		wantReason := "UNKNOWN"
		if xErr.Reason != wantReason {
			t.Errorf("want reason: %v, got %v", wantReason, xErr.Reason)
		}
		if gs, ok := status.FromError(err); ok {
			t.Logf("status from err: %#v", gs)
		}
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
}

func TestWithStreamHandler(t *testing.T) {
	ch := make(chan struct{}, 10)
	testHandler := StreamRecoverHandlerFunc(func(ctx context.Context, info *grpc.StreamServerInfo, rerr any) error {
		ch <- struct{}{}
		return errors.Unknown("unknown", "")
	})
	target := &streamOptions{}
	WithStreamHandler(testHandler)(target)
	_ = target.handler(context.Background(), &grpc.StreamServerInfo{}, "")
	select {
	case <-ch:
	case <-time.After(time.Millisecond):
		t.Errorf("timeout")
	}
}

func TestStreamRecover(t *testing.T) {
	t.Run("no_panic", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		srvErr := make(chan error)
		svc := &testHelloSvc{
			serverGreetsFn: func(
				request *hello.ServerGreetsRequest,
				srv hello.GreeterService_ServerGreetsServer,
			) error {
				for i := 0; i < 10; i++ {
					time.Sleep(time.Millisecond)
					select {
					case <-srv.Context().Done():
						return srv.Context().Err()
					default:
					}
					respMsg := &hello.ServerGreetsResponse{Message: fmt.Sprintf("hello, %s", request.Name)}
					if err := srv.Send(respMsg); err != nil {
						return err
					}
				}
				return nil
			},
		}
		srv := serve_grpc.NewServer(
			lis127(0),
			serve_grpc.WithOption(grpc.ChainStreamInterceptor(StreamRecover())),
		)
		hello.RegisterGreeterServiceServer(srv, svc)
		go func(ctx context.Context) {
			if err := srv.Run(ctx); err != nil {
				srvErr <- err
			}
			close(srvErr)
		}(ctx)
		defer func(ctx context.Context) {
			_ = srv.Stop(ctx)
			e, ok := <-srvErr
			if ok {
				t.Errorf("unexpect Run err: %v", e)
			}
		}(ctx)
		time.Sleep(5 * time.Millisecond) // let server start running
		address, _ := srv.Address()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(
			ctx,
			address.String(),
			grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("err dialing %v: %v", address.String(), err)
		}
		greeterClient := hello.NewGreeterServiceClient(conn)
		greets, err := greeterClient.ServerGreets(ctx, &hello.ServerGreetsRequest{Name: "abc"})
		if err != nil {
			t.Fatalf("err calling ServerGreets: %v", err)
		}
		for {
			_, err = greets.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Errorf("unexpected err: %v", err)
			}
		}
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
	t.Run("panic", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		srvErr := make(chan error)
		svc := &testHelloSvc{
			serverGreetsFn: func(
				request *hello.ServerGreetsRequest,
				srv hello.GreeterService_ServerGreetsServer,
			) error {
				panic("abc")
			},
		}
		srv := serve_grpc.NewServer(
			lis127(0),
			serve_grpc.WithOption(grpc.ChainStreamInterceptor(StreamRecover())),
		)
		hello.RegisterGreeterServiceServer(srv, svc)
		go func(ctx context.Context) {
			if err := srv.Run(ctx); err != nil {
				srvErr <- err
			}
			close(srvErr)
		}(ctx)
		defer func(ctx context.Context) {
			_ = srv.Stop(ctx)
			if e, ok := <-srvErr; ok {
				t.Errorf("unexpected Run err: %v", e)
			}
		}(ctx)
		time.Sleep(5 * time.Millisecond) // let Run goroutine do the job.
		address, _ := srv.Address()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn, err := grpc.DialContext(
			ctx,
			address.String(),
			grpc.WithTransportCredentials(grpcinsecure.NewCredentials()),
		)
		if err != nil {
			t.Fatalf("err dialing %v: %v", address.String(), err)
		}
		greeterClient := hello.NewGreeterServiceClient(conn)
		greets, err := greeterClient.ServerGreets(ctx, &hello.ServerGreetsRequest{Name: "abc"})
		if err != nil {
			t.Fatalf("err calling ServerGreets: %v", err)
		}
		_, err = greets.Recv()
		if err == nil {
			t.Errorf("expect err, got nil")
		} else if errors.Is(err, io.EOF) {
			t.Errorf("unexpect EOF: %v", err)
		} else {
			code := errors.Code(err)
			if codes.Unknown != code {
				t.Errorf("want %v, got %v", codes.Unknown, code)
			}
		}
		var mps []map[string]any
		scanner := bufio.NewScanner(logBuf)
		for scanner.Scan() {
			if errors.Is(scanner.Err(), io.EOF) {
				continue
			}
			t.Logf("%s", scanner.Text())
			mp := map[string]any{}
			if err := json.Unmarshal(scanner.Bytes(), &mp); err == nil {
				mps = append(mps, mp)
			} else {
				t.Logf("err line: %v: %s", err, scanner.Text())
			}
		}
		logBuf.Reset()
		assert.Len(t, mps, 1)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
	})
}
