package metrics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yimi-go/metrics"
	"google.golang.org/grpc"
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

type mockCounter struct {
	labels map[string]string
	count  float64
}

var counters = map[string]*mockCounter{}

func (m *mockCounter) With(labels map[string]string) metrics.Counter {
	kb := &strings.Builder{}
	mp := make(map[string]string, len(m.labels)+len(labels))
	for k, v := range m.labels {
		mp[k] = v
	}
	for k, v := range labels {
		mp[k] = v
	}
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kb.WriteString(k)
		kb.WriteRune(':')
		kb.WriteString(mp[k])
		kb.WriteRune(';')
	}
	counter, ok := counters[kb.String()]
	if ok {
		return counter
	}
	counter = &mockCounter{
		labels: mp,
		count:  0,
	}
	counters[kb.String()] = counter
	return counter
}

func (m *mockCounter) Inc() {
	m.count += 1
}

func (m *mockCounter) Add(delta float64) {
	m.count += delta
}

type mockObserver struct {
	labels map[string]string
	values []float64
}

var observers = map[string]*mockObserver{}

func (m *mockObserver) With(labels map[string]string) metrics.Observer {
	kb := &strings.Builder{}
	mp := make(map[string]string, len(m.labels)+len(labels))
	for k, v := range m.labels {
		mp[k] = v
	}
	for k, v := range labels {
		mp[k] = v
	}
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kb.WriteString(k)
		kb.WriteRune(':')
		kb.WriteString(mp[k])
		kb.WriteRune(';')
	}
	observer, ok := observers[kb.String()]
	if ok {
		return observer
	}
	observer = &mockObserver{
		labels: mp,
	}
	observers[kb.String()] = observer
	return observer
}

func (m *mockObserver) Observe(f float64) {
	m.values = append(m.values, f)
}

type mockGauge struct {
	labels map[string]string
	value  float64
}

var gauges = map[string]*mockGauge{}

func (m *mockGauge) With(labels map[string]string) metrics.Gauge {
	kb := &strings.Builder{}
	mp := make(map[string]string, len(m.labels)+len(labels))
	for k, v := range m.labels {
		mp[k] = v
	}
	for k, v := range labels {
		mp[k] = v
	}
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		kb.WriteString(k)
		kb.WriteRune(':')
		kb.WriteString(mp[k])
		kb.WriteRune(';')
	}
	gauge, ok := gauges[kb.String()]
	if ok {
		return gauge
	}
	gauge = &mockGauge{
		labels: mp,
		value:  0,
	}
	gauges[kb.String()] = gauge
	return gauge
}

func (m *mockGauge) Set(value float64) {
	m.value = value
}

func (m *mockGauge) Add(delta float64) {
	m.value += delta
}

func (m *mockGauge) Sub(delta float64) {
	m.value -= delta
}

func TestWithRequestCounter(t *testing.T) {
	mw := &metricsWrapper{}
	m := &mockCounter{}
	WithRequestCounter(m)(mw)
	assert.Equal(t, m, mw.requestCounter)
}

func TestWithDurationObserver(t *testing.T) {
	mw := &metricsWrapper{}
	m := &mockObserver{}
	WithDurationObserver(m)(mw)
	assert.Equal(t, m, mw.requestDuration)
}

func TestWithInflightGauge(t *testing.T) {
	mw := &metricsWrapper{}
	m := &mockGauge{}
	WithInflightGauge(m)(mw)
	assert.Equal(t, m, mw.inflightRequests)
}

func TestUnaryInterceptor(t *testing.T) {
	t.Run("no_metrics_ok", func(t *testing.T) {
		interceptor := UnaryInterceptor()
		_, err := interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			return "mockResp", nil
		})
		assert.Nil(t, err)
	})
	t.Run("no_metrics_err", func(t *testing.T) {
		interceptor := UnaryInterceptor()
		_, err := interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			return nil, fmt.Errorf("test")
		})
		assert.NotNil(t, err)
	})
	t.Run("with_metrics_ok", func(t *testing.T) {
		now := time.Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }
		defer func() {
			Now = time.Now
			Since = time.Since
			counters = map[string]*mockCounter{}
			gauges = map[string]*mockGauge{}
			observers = map[string]*mockObserver{}
		}()
		interceptor := UnaryInterceptor(
			WithRequestCounter(&mockCounter{}),
			WithDurationObserver(&mockObserver{}),
			WithInflightGauge(&mockGauge{}),
		)
		_, err := interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			now = now.Add(time.Second)
			gauge := gauges["api:testMethod;method:unary;"]
			assert.NotNil(t, gauge)
			assert.Equal(t, 1.0, gauge.value)
			return "mockResp", nil
		})
		assert.Nil(t, err)
		gauge := gauges["api:testMethod;method:unary;"]
		assert.NotNil(t, gauge)
		assert.Equal(t, 0.0, gauge.value)
		counter := counters["api:testMethod;code:OK;method:unary;reason:;"]
		assert.NotNil(t, counter)
		assert.Equal(t, 1.0, counter.count)
		observer := observers["api:testMethod;code:OK;method:unary;reason:;"]
		assert.NotNil(t, observer)
		assert.Equal(t, []float64{1000.0}, observer.values)
	})
	t.Run("with_metrics_err", func(t *testing.T) {
		now := time.Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }
		defer func() {
			Now = time.Now
			Since = time.Since
			counters = map[string]*mockCounter{}
			gauges = map[string]*mockGauge{}
			observers = map[string]*mockObserver{}
		}()
		interceptor := UnaryInterceptor(
			WithRequestCounter(&mockCounter{}),
			WithDurationObserver(&mockObserver{}),
			WithInflightGauge(&mockGauge{}),
		)
		_, err := interceptor(context.Background(), "mockReq", &grpc.UnaryServerInfo{
			Server:     nil,
			FullMethod: "testMethod",
		}, func(ctx context.Context, req any) (any, error) {
			now = now.Add(time.Second)
			gauge := gauges["api:testMethod;method:unary;"]
			assert.NotNil(t, gauge)
			assert.Equal(t, 1.0, gauge.value)
			return nil, fmt.Errorf("test")
		})
		assert.NotNil(t, err)
		gauge := gauges["api:testMethod;method:unary;"]
		assert.NotNil(t, gauge)
		assert.Equal(t, 0.0, gauge.value)
		counter := counters["api:testMethod;code:Unknown;method:unary;reason:UNKNOWN;"]
		assert.NotNil(t, counter)
		assert.Equal(t, 1.0, counter.count)
		observer := observers["api:testMethod;code:Unknown;method:unary;reason:UNKNOWN;"]
		assert.NotNil(t, observer)
		assert.Equal(t, []float64{1000.0}, observer.values)
	})
}

func TestStreamInterceptor(t *testing.T) {
	t.Run("no_metrics_ok", func(t *testing.T) {
		interceptor := StreamInterceptor()
		err := interceptor(
			nil,
			&mockStream{},
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return nil
			})
		assert.Nil(t, err)
	})
	t.Run("no_metrics_err", func(t *testing.T) {
		interceptor := StreamInterceptor()
		err := interceptor(
			nil,
			&mockStream{},
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				return fmt.Errorf("test")
			})
		assert.NotNil(t, err)
	})
	t.Run("with_metrics_ok", func(t *testing.T) {
		now := time.Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }
		defer func() {
			Now = time.Now
			Since = time.Since
			counters = map[string]*mockCounter{}
			gauges = map[string]*mockGauge{}
			observers = map[string]*mockObserver{}
		}()
		interceptor := StreamInterceptor(
			WithRequestCounter(&mockCounter{}),
			WithDurationObserver(&mockObserver{}),
			WithInflightGauge(&mockGauge{}),
		)
		err := interceptor(
			nil,
			&mockStream{},
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				now = now.Add(time.Second)
				gauge := gauges["api:testMethod;method:stream;"]
				assert.NotNil(t, gauge)
				assert.Equal(t, 1.0, gauge.value)
				return nil
			})
		assert.Nil(t, err)
		gauge := gauges["api:testMethod;method:stream;"]
		assert.NotNil(t, gauge)
		assert.Equal(t, 0.0, gauge.value)
		counter := counters["api:testMethod;code:OK;method:stream;reason:;"]
		assert.NotNil(t, counter)
		assert.Equal(t, 1.0, counter.count)
		observer := observers["api:testMethod;code:OK;method:stream;reason:;"]
		assert.NotNil(t, observer)
		assert.Equal(t, []float64{1000.0}, observer.values)
	})
	t.Run("with_metrics_err", func(t *testing.T) {
		now := time.Now()
		Now = func() time.Time { return now }
		Since = func(t time.Time) time.Duration { return now.Sub(t) }
		defer func() {
			Now = time.Now
			Since = time.Since
			counters = map[string]*mockCounter{}
			gauges = map[string]*mockGauge{}
			observers = map[string]*mockObserver{}
		}()
		interceptor := StreamInterceptor(
			WithRequestCounter(&mockCounter{}),
			WithDurationObserver(&mockObserver{}),
			WithInflightGauge(&mockGauge{}),
		)
		err := interceptor(
			nil,
			&mockStream{},
			&grpc.StreamServerInfo{FullMethod: "testMethod"},
			func(srv any, stream grpc.ServerStream) error {
				now = now.Add(time.Second)
				gauge := gauges["api:testMethod;method:stream;"]
				assert.NotNil(t, gauge)
				assert.Equal(t, 1.0, gauge.value)
				return fmt.Errorf("test")
			})
		assert.NotNil(t, err)
		gauge := gauges["api:testMethod;method:stream;"]
		assert.NotNil(t, gauge)
		assert.Equal(t, 0.0, gauge.value)
		counter := counters["api:testMethod;code:Unknown;method:stream;reason:UNKNOWN;"]
		assert.NotNil(t, counter)
		assert.Equal(t, 1.0, counter.count)
		observer := observers["api:testMethod;code:Unknown;method:stream;reason:UNKNOWN;"]
		assert.NotNil(t, observer)
		assert.Equal(t, []float64{1000.0}, observer.values)
	})
}
