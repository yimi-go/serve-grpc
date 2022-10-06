// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             (unknown)
// source: internal/hello/v1/hello.proto

package hello

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// GreeterServiceClient is the client API for GreeterService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type GreeterServiceClient interface {
	// Sends a greeting
	SayHello(ctx context.Context, in *SayHelloRequest, opts ...grpc.CallOption) (*SayHelloResponse, error)
	// Response greetings
	ServerGreets(ctx context.Context, in *ServerGreetsRequest, opts ...grpc.CallOption) (GreeterService_ServerGreetsClient, error)
}

type greeterServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewGreeterServiceClient(cc grpc.ClientConnInterface) GreeterServiceClient {
	return &greeterServiceClient{cc}
}

func (c *greeterServiceClient) SayHello(ctx context.Context, in *SayHelloRequest, opts ...grpc.CallOption) (*SayHelloResponse, error) {
	out := new(SayHelloResponse)
	err := c.cc.Invoke(ctx, "/internal.hello.v1.GreeterService/SayHello", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *greeterServiceClient) ServerGreets(ctx context.Context, in *ServerGreetsRequest, opts ...grpc.CallOption) (GreeterService_ServerGreetsClient, error) {
	stream, err := c.cc.NewStream(ctx, &GreeterService_ServiceDesc.Streams[0], "/internal.hello.v1.GreeterService/ServerGreets", opts...)
	if err != nil {
		return nil, err
	}
	x := &greeterServiceServerGreetsClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type GreeterService_ServerGreetsClient interface {
	Recv() (*ServerGreetsResponse, error)
	grpc.ClientStream
}

type greeterServiceServerGreetsClient struct {
	grpc.ClientStream
}

func (x *greeterServiceServerGreetsClient) Recv() (*ServerGreetsResponse, error) {
	m := new(ServerGreetsResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// GreeterServiceServer is the server API for GreeterService service.
// All implementations must embed UnimplementedGreeterServiceServer
// for forward compatibility
type GreeterServiceServer interface {
	// Sends a greeting
	SayHello(context.Context, *SayHelloRequest) (*SayHelloResponse, error)
	// Response greetings
	ServerGreets(*ServerGreetsRequest, GreeterService_ServerGreetsServer) error
	mustEmbedUnimplementedGreeterServiceServer()
}

// UnimplementedGreeterServiceServer must be embedded to have forward compatible implementations.
type UnimplementedGreeterServiceServer struct {
}

func (UnimplementedGreeterServiceServer) SayHello(context.Context, *SayHelloRequest) (*SayHelloResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SayHello not implemented")
}
func (UnimplementedGreeterServiceServer) ServerGreets(*ServerGreetsRequest, GreeterService_ServerGreetsServer) error {
	return status.Errorf(codes.Unimplemented, "method ServerGreets not implemented")
}
func (UnimplementedGreeterServiceServer) mustEmbedUnimplementedGreeterServiceServer() {}

// UnsafeGreeterServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GreeterServiceServer will
// result in compilation errors.
type UnsafeGreeterServiceServer interface {
	mustEmbedUnimplementedGreeterServiceServer()
}

func RegisterGreeterServiceServer(s grpc.ServiceRegistrar, srv GreeterServiceServer) {
	s.RegisterService(&GreeterService_ServiceDesc, srv)
}

func _GreeterService_SayHello_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SayHelloRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GreeterServiceServer).SayHello(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/internal.hello.v1.GreeterService/SayHello",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GreeterServiceServer).SayHello(ctx, req.(*SayHelloRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GreeterService_ServerGreets_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ServerGreetsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(GreeterServiceServer).ServerGreets(m, &greeterServiceServerGreetsServer{stream})
}

type GreeterService_ServerGreetsServer interface {
	Send(*ServerGreetsResponse) error
	grpc.ServerStream
}

type greeterServiceServerGreetsServer struct {
	grpc.ServerStream
}

func (x *greeterServiceServerGreetsServer) Send(m *ServerGreetsResponse) error {
	return x.ServerStream.SendMsg(m)
}

// GreeterService_ServiceDesc is the grpc.ServiceDesc for GreeterService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var GreeterService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "internal.hello.v1.GreeterService",
	HandlerType: (*GreeterServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SayHello",
			Handler:    _GreeterService_SayHello_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ServerGreets",
			Handler:       _GreeterService_ServerGreets_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "internal/hello/v1/hello.proto",
}