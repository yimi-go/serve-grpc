package logging

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/errors"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type mockStream struct {
	header metadata.MD
	ctx    context.Context
	sendFn func(m any) error
	recvFn func(m any) error
}

func (m *mockStream) SetHeader(md metadata.MD) error {
	m.header = metadata.Join(m.header, md)
	return nil
}

func (m *mockStream) SendHeader(_ metadata.MD) error {
	return nil
}

func (m *mockStream) SetTrailer(_ metadata.MD) {
}

func (m *mockStream) Context() context.Context {
	return m.ctx
}

func (m *mockStream) SendMsg(msg any) error {
	return m.sendFn(msg)
}

func (m *mockStream) RecvMsg(msg interface{}) error {
	return m.recvFn(msg)
}

func TestUnaryInterceptor(t *testing.T) {
	t.Run("default_ok", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		originNowFn := Now
		originSinceFn := Since
		defer func() {
			Now = originNowFn
			Since = originSinceFn
		}()
		now := Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }

		interceptor := UnaryInterceptor()
		_, err := interceptor(ctx, "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			now = now.Add(time.Second)
			return "mockResp", nil
		})
		assert.Nilf(t, err, "unexpected err: %v", err)
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
		assert.Len(t, mps, 2)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "receive request", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "finish handling request", mps[1]["msg"])
		assert.Equal(t, codes.OK.String(), mps[1]["code"])
		assert.NotNil(t, mps[1]["resp"])
		assert.Equal(t, 1000_000_000.0, mps[1]["cost"])
	})
	t.Run("default_err", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		interceptor := UnaryInterceptor()
		_, err := interceptor(ctx, "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			return nil, pkgerrors.New("error")
		})
		assert.NotNilf(t, err, "expect err, got nil")
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
		assert.Len(t, mps, 2)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "receive request", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "fail handling request", mps[1]["msg"])
		assert.Equal(t, codes.Unknown.String(), mps[1]["code"])
		assert.Equal(t, errors.UnknownReason, mps[1]["reason"])
		assert.Nil(t, mps[1]["resp"])
		assert.NotEmpty(t, mps[1]["err"])
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("default_ok", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		originNowFn := Now
		originSinceFn := Since
		defer func() {
			Now = originNowFn
			Since = originSinceFn
		}()
		now := Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }

		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: ctx,
			recvFn: func(m any) error {
				sp, ok := m.(*string)
				if !ok {
					return errors.Internal("resv", "msg type mismatch")
				}
				*sp = "hello"
				return nil
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err := interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				msg := ""
				if err := stream.RecvMsg(&msg); err != nil {
					return err
				}
				for i := 0; i < 2; i++ {
					now = now.Add(time.Second)
					if err := stream.SendMsg("goodbye"); err != nil {
						return err
					}
				}
				return nil
			})
		assert.Nilf(t, err, "unexpect err: %v", err)
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
		assert.Len(t, mps, 5)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "start streaming", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "recv msg", mps[1]["msg"])
		param, ok := mps[1]["param"].(map[string]any)
		assert.True(t, ok)
		assert.NotNil(t, param["req"])
		for i := 0; i < 2; i++ {
			assert.Equal(t, slog.InfoLevel.String(), mps[2+i][slog.LevelKey])
			assert.Equal(t, "send msg", mps[2+i]["msg"])
			assert.NotEmpty(t, mps[2+i]["resp"])
		}
		assert.Equal(t, slog.InfoLevel.String(), mps[4][slog.LevelKey])
		assert.Equal(t, "finish streaming", mps[4]["msg"])
		assert.Equal(t, 2_000_000_000.0, mps[4]["cost"])
		assert.Equal(t, "OK", mps[4]["code"])
	})
	t.Run("default_err", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: ctx,
			recvFn: func(m any) error {
				sp, ok := m.(*string)
				if !ok {
					return errors.Internal("resv", "msg type mismatch")
				}
				*sp = "hello"
				return nil
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err := interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return errors.Unimplemented("test", "test").
					WithCause(pkgerrors.New("test"))
			},
		)
		assert.NotNil(t, err)
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
		assert.Len(t, mps, 2)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "start streaming", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "error streaming", mps[1]["msg"])
		assert.Equal(t, codes.Unimplemented.String(), mps[1]["code"])
		assert.Equal(t, "test", mps[1]["reason"])
		assert.NotEmpty(t, mps[1]["err"])
	})
	t.Run("default_recv_err", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: ctx,
			recvFn: func(m any) error {
				return pkgerrors.New("test")
			},
			sendFn: func(m any) error {
				return nil
			},
		}
		err := interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return stream.RecvMsg(nil)
			},
		)
		assert.NotNil(t, err)
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
		assert.Len(t, mps, 3)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "start streaming", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "recv msg fail", mps[1]["msg"])
		assert.Empty(t, mps[1]["code"])
		assert.Empty(t, mps[1]["reason"])
		assert.NotEmpty(t, mps[1]["err"])
		assert.Equal(t, slog.InfoLevel.String(), mps[2][slog.LevelKey])
		assert.Equal(t, "error streaming", mps[2]["msg"])
		assert.NotEmpty(t, mps[2]["code"])
		assert.NotEmpty(t, mps[2]["err"])
	})
	t.Run("default_send_err", func(t *testing.T) {
		logBuf := &bytes.Buffer{}
		ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
		interceptor := StreamInterceptor()
		ss := &mockStream{
			ctx: ctx,
			recvFn: func(m any) error {
				return nil
			},
			sendFn: func(m any) error {
				return pkgerrors.New("test")
			},
		}
		err := interceptor(
			nil,
			ss,
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				for i := 0; i < 2; i++ {
					if err := stream.SendMsg("goodbye"); err != nil {
						return err
					}
				}
				return nil
			},
		)
		assert.NotNil(t, err)
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
		assert.Len(t, mps, 3)
		assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
		assert.Equal(t, "start streaming", mps[0]["msg"])
		assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
		assert.Equal(t, "send msg fail", mps[1]["msg"])
		assert.Empty(t, mps[1]["code"])
		assert.Empty(t, mps[1]["reason"])
		assert.NotEmpty(t, mps[1]["err"])
		assert.Equal(t, slog.InfoLevel.String(), mps[2][slog.LevelKey])
		assert.Equal(t, "error streaming", mps[2]["msg"])
		assert.NotEmpty(t, mps[2]["code"])
		assert.NotEmpty(t, mps[2]["err"])
	})
	tests := []struct {
		method       string
		serverStream bool
		clientStream bool
	}{
		{
			method:       "stream",
			serverStream: false,
			clientStream: false,
		},
		{
			method:       "serverStream",
			serverStream: true,
			clientStream: false,
		},
		{
			method:       "clientStream",
			serverStream: false,
			clientStream: true,
		},
		{
			method:       "bidirectionalStream",
			serverStream: true,
			clientStream: true,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("default_%s", test.method), func(t *testing.T) {
			logBuf := &bytes.Buffer{}
			ctx := slog.NewContext(context.Background(), slog.New(slog.NewJSONHandler(logBuf)))
			interceptor := StreamInterceptor()
			ss := &mockStream{
				ctx: ctx,
				recvFn: func(m any) error {
					return nil
				},
				sendFn: func(m any) error {
					return nil
				},
			}
			info := &grpc.StreamServerInfo{
				FullMethod:     "testMethod",
				IsClientStream: test.clientStream,
				IsServerStream: test.serverStream,
			}
			err := interceptor(
				nil,
				ss,
				info,
				func(srv any, stream grpc.ServerStream) error {
					return nil
				},
			)
			assert.Nil(t, err)
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
			assert.Len(t, mps, 2)
			assert.Equal(t, slog.InfoLevel.String(), mps[0][slog.LevelKey])
			assert.Equal(t, "start streaming", mps[0]["msg"])
			assert.Equal(t, slog.InfoLevel.String(), mps[1][slog.LevelKey])
			assert.Equal(t, "finish streaming", mps[1]["msg"])
		})
	}
}
