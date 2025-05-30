package clientinterceptors

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	ztrace "github.com/gomsr/atom-otel/trace"
	"github.com/gomsr/atom-otel/trace/tracetest"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	tcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestOpenTracingInterceptor(t *testing.T) {
	ztrace.StartAgent(ztrace.Config{
		Name:     "go-zero-test",
		Endpoint: "http://localhost:14268/api/traces",
		Batcher:  "jaeger",
		Sampler:  1.0,
	})
	defer ztrace.StopAgent()

	cc := new(grpc.ClientConn)
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.MD{})
	err := UnaryTracingInterceptor(ctx, "/ListUser", nil, nil, cc,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
			opts ...grpc.CallOption) error {
			return nil
		})
	assert.Nil(t, err)
}

func TestUnaryTracingInterceptor(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		var run int32
		cc := new(grpc.ClientConn)
		me := tracetest.NewInMemoryExporter(t)
		err := UnaryTracingInterceptor(context.Background(), "/proto.Hello/Echo",
			nil, nil, cc,
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
				opts ...grpc.CallOption) error {
				atomic.AddInt32(&run, 1)
				return nil
			})
		assert.Nil(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&run))

		assert.Equal(t, 1, len(me.GetSpans()))
		span := me.GetSpans()[0].Snapshot()
		assert.Equal(t, 2, len(span.Events()))
		assert.ElementsMatch(t, []attribute.KeyValue{
			ztrace.RPCSystemGRPC,
			semconv.RPCServiceKey.String("proto.Hello"),
			semconv.RPCMethodKey.String("Echo"),
			ztrace.StatusCodeAttr(codes.OK),
		}, span.Attributes())
	})

	t.Run("grpc error status", func(t *testing.T) {
		me := tracetest.NewInMemoryExporter(t)
		cc := new(grpc.ClientConn)
		err := UnaryTracingInterceptor(context.Background(), "/proto.Hello/Echo",
			nil, nil, cc,
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
				opts ...grpc.CallOption) error {
				return status.Error(codes.Unknown, "test")
			})
		assert.Error(t, err)
		assert.Equal(t, 1, len(me.GetSpans()))
		span := me.GetSpans()[0].Snapshot()
		assert.Equal(t, trace.Status{
			Code:        tcodes.Error,
			Description: "test",
		}, span.Status())
		assert.Equal(t, 2, len(span.Events()))
		assert.ElementsMatch(t, []attribute.KeyValue{
			ztrace.RPCSystemGRPC,
			semconv.RPCServiceKey.String("proto.Hello"),
			semconv.RPCMethodKey.String("Echo"),
			ztrace.StatusCodeAttr(codes.Unknown),
		}, span.Attributes())
	})

	t.Run("non grpc status error", func(t *testing.T) {
		me := tracetest.NewInMemoryExporter(t)
		cc := new(grpc.ClientConn)
		err := UnaryTracingInterceptor(context.Background(), "/proto.Hello/Echo",
			nil, nil, cc,
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
				opts ...grpc.CallOption) error {
				return errors.New("test")
			})
		assert.Error(t, err)
		assert.Equal(t, 1, len(me.GetSpans()))
		span := me.GetSpans()[0].Snapshot()
		assert.Equal(t, trace.Status{
			Code:        tcodes.Error,
			Description: "test",
		}, span.Status())
		assert.Equal(t, 2, len(span.Events()))
		assert.ElementsMatch(t, []attribute.KeyValue{
			ztrace.RPCSystemGRPC,
			semconv.RPCServiceKey.String("proto.Hello"),
			semconv.RPCMethodKey.String("Echo"),
		}, span.Attributes())
	})
}

func TestUnaryTracingInterceptor_WithError(t *testing.T) {
	var run int32
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	err := UnaryTracingInterceptor(context.Background(), "/foo", nil, nil, cc,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
			opts ...grpc.CallOption) error {
			defer wg.Done()
			atomic.AddInt32(&run, 1)
			return errors.New("dummy")
		})
	wg.Wait()
	assert.NotNil(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&run))
}

func TestUnaryTracingInterceptor_WithStatusError(t *testing.T) {
	var run int32
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	err := UnaryTracingInterceptor(context.Background(), "/foo", nil, nil, cc,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
			opts ...grpc.CallOption) error {
			defer wg.Done()
			atomic.AddInt32(&run, 1)
			return status.Error(codes.DataLoss, "dummy")
		})
	wg.Wait()
	assert.NotNil(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&run))
}

func TestStreamTracingInterceptor(t *testing.T) {
	var run int32
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	_, err := StreamTracingInterceptor(context.Background(), nil, cc, "/foo",
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
			opts ...grpc.CallOption) (grpc.ClientStream, error) {
			defer wg.Done()
			atomic.AddInt32(&run, 1)
			return nil, nil
		})
	wg.Wait()
	assert.Nil(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&run))
}

func TestStreamTracingInterceptor_FinishWithNormalError(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := StreamTracingInterceptor(ctx, nil, cc, "/foo",
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
			opts ...grpc.CallOption) (grpc.ClientStream, error) {
			defer wg.Done()
			return nil, nil
		})
	wg.Wait()
	assert.Nil(t, err)

	cancel()
	cs := stream.(*clientStream)
	<-cs.eventsDone
}

func TestStreamTracingInterceptor_FinishWithGrpcError(t *testing.T) {
	tests := []struct {
		name  string
		event streamEventType
		err   error
	}{
		{
			name:  "receive event",
			event: receiveEndEvent,
			err:   status.Error(codes.DataLoss, "dummy"),
		},
		{
			name:  "error event",
			event: errorEvent,
			err:   status.Error(codes.DataLoss, "dummy"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var wg sync.WaitGroup
			wg.Add(1)
			cc := new(grpc.ClientConn)
			stream, err := StreamTracingInterceptor(context.Background(), nil, cc, "/foo",
				func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
					opts ...grpc.CallOption) (grpc.ClientStream, error) {
					defer wg.Done()
					return &mockedClientStream{
						err: errors.New("dummy"),
					}, nil
				})
			wg.Wait()
			assert.Nil(t, err)

			cs := stream.(*clientStream)
			cs.sendStreamEvent(test.event, status.Error(codes.DataLoss, "dummy"))
			<-cs.eventsDone
			cs.sendStreamEvent(test.event, test.err)
			assert.NotNil(t, cs.CloseSend())
		})
	}
}

func TestStreamTracingInterceptor_WithError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "normal error",
			err:  errors.New("dummy"),
		},
		{
			name: "grpc error",
			err:  status.Error(codes.DataLoss, "dummy"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var run int32
			var wg sync.WaitGroup
			wg.Add(1)
			cc := new(grpc.ClientConn)
			_, err := StreamTracingInterceptor(context.Background(), nil, cc, "/foo",
				func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
					opts ...grpc.CallOption) (grpc.ClientStream, error) {
					defer wg.Done()
					atomic.AddInt32(&run, 1)
					return new(mockedClientStream), test.err
				})
			wg.Wait()
			assert.NotNil(t, err)
			assert.Equal(t, int32(1), atomic.LoadInt32(&run))
		})
	}
}

func TestUnaryTracingInterceptor_GrpcFormat(t *testing.T) {
	var run int32
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	err := UnaryTracingInterceptor(context.Background(), "/foo", nil, nil, cc,
		func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
			opts ...grpc.CallOption) error {
			defer wg.Done()
			atomic.AddInt32(&run, 1)
			return nil
		})
	wg.Wait()
	assert.Nil(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&run))
}

func TestStreamTracingInterceptor_GrpcFormat(t *testing.T) {
	var run int32
	var wg sync.WaitGroup
	wg.Add(1)
	cc := new(grpc.ClientConn)
	_, err := StreamTracingInterceptor(context.Background(), nil, cc, "/foo",
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
			opts ...grpc.CallOption) (grpc.ClientStream, error) {
			defer wg.Done()
			atomic.AddInt32(&run, 1)
			return nil, nil
		})
	wg.Wait()
	assert.Nil(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&run))
}

func TestClientStream_RecvMsg(t *testing.T) {
	tests := []struct {
		name          string
		serverStreams bool
		err           error
	}{
		{
			name: "nil error",
		},
		{
			name: "EOF",
			err:  io.EOF,
		},
		{
			name: "dummy error",
			err:  errors.New("dummy"),
		},
		{
			name:          "server streams",
			serverStreams: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			desc := new(grpc.StreamDesc)
			desc.ServerStreams = test.serverStreams
			stream := wrapClientStream(context.Background(), &mockedClientStream{
				md:  nil,
				err: test.err,
			}, desc)
			assert.Equal(t, test.err, stream.RecvMsg(nil))
		})
	}
}

func TestClientStream_Header(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "nil error",
		},
		{
			name: "with error",
			err:  errors.New("dummy"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			desc := new(grpc.StreamDesc)
			stream := wrapClientStream(context.Background(), &mockedClientStream{
				md:  metadata.MD{},
				err: test.err,
			}, desc)
			_, err := stream.Header()
			assert.Equal(t, test.err, err)
		})
	}
}

func TestClientStream_SendMsg(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "nil error",
		},
		{
			name: "with error",
			err:  errors.New("dummy"),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			desc := new(grpc.StreamDesc)
			stream := wrapClientStream(context.Background(), &mockedClientStream{
				md:  metadata.MD{},
				err: test.err,
			}, desc)
			assert.Equal(t, test.err, stream.SendMsg(nil))
		})
	}
}

type mockedClientStream struct {
	md  metadata.MD
	err error
}

func (m *mockedClientStream) Header() (metadata.MD, error) {
	return m.md, m.err
}

func (m *mockedClientStream) Trailer() metadata.MD {
	panic("implement me")
}

func (m *mockedClientStream) CloseSend() error {
	return m.err
}

func (m *mockedClientStream) Context() context.Context {
	return context.Background()
}

func (m *mockedClientStream) SendMsg(v any) error {
	return m.err
}

func (m *mockedClientStream) RecvMsg(v any) error {
	return m.err
}
