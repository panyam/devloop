// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             (unknown)
// source: protos/devloop/v1/devloop_gateway.proto

package v1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	DevloopGatewayService_Communicate_FullMethodName = "/devloop_gateway.v1.DevloopGatewayService/Communicate"
)

// DevloopGatewayServiceClient is the client API for DevloopGatewayService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// DevloopGatewayService defines the gRPC service for communication between devloop and the gateway.
type DevloopGatewayServiceClient interface {
	// Communicate handles all bidirectional communication between devloop and the gateway.
	Communicate(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[DevloopMessage, DevloopMessage], error)
}

type devloopGatewayServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewDevloopGatewayServiceClient(cc grpc.ClientConnInterface) DevloopGatewayServiceClient {
	return &devloopGatewayServiceClient{cc}
}

func (c *devloopGatewayServiceClient) Communicate(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[DevloopMessage, DevloopMessage], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &DevloopGatewayService_ServiceDesc.Streams[0], DevloopGatewayService_Communicate_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[DevloopMessage, DevloopMessage]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type DevloopGatewayService_CommunicateClient = grpc.BidiStreamingClient[DevloopMessage, DevloopMessage]

// DevloopGatewayServiceServer is the server API for DevloopGatewayService service.
// All implementations should embed UnimplementedDevloopGatewayServiceServer
// for forward compatibility.
//
// DevloopGatewayService defines the gRPC service for communication between devloop and the gateway.
type DevloopGatewayServiceServer interface {
	// Communicate handles all bidirectional communication between devloop and the gateway.
	Communicate(grpc.BidiStreamingServer[DevloopMessage, DevloopMessage]) error
}

// UnimplementedDevloopGatewayServiceServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedDevloopGatewayServiceServer struct{}

func (UnimplementedDevloopGatewayServiceServer) Communicate(grpc.BidiStreamingServer[DevloopMessage, DevloopMessage]) error {
	return status.Errorf(codes.Unimplemented, "method Communicate not implemented")
}
func (UnimplementedDevloopGatewayServiceServer) testEmbeddedByValue() {}

// UnsafeDevloopGatewayServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to DevloopGatewayServiceServer will
// result in compilation errors.
type UnsafeDevloopGatewayServiceServer interface {
	mustEmbedUnimplementedDevloopGatewayServiceServer()
}

func RegisterDevloopGatewayServiceServer(s grpc.ServiceRegistrar, srv DevloopGatewayServiceServer) {
	// If the following call pancis, it indicates UnimplementedDevloopGatewayServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&DevloopGatewayService_ServiceDesc, srv)
}

func _DevloopGatewayService_Communicate_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(DevloopGatewayServiceServer).Communicate(&grpc.GenericServerStream[DevloopMessage, DevloopMessage]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type DevloopGatewayService_CommunicateServer = grpc.BidiStreamingServer[DevloopMessage, DevloopMessage]

// DevloopGatewayService_ServiceDesc is the grpc.ServiceDesc for DevloopGatewayService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var DevloopGatewayService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "devloop_gateway.v1.DevloopGatewayService",
	HandlerType: (*DevloopGatewayServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Communicate",
			Handler:       _DevloopGatewayService_Communicate_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "protos/devloop/v1/devloop_gateway.proto",
}

const (
	GatewayClientService_ListProjects_FullMethodName            = "/devloop_gateway.v1.GatewayClientService/ListProjects"
	GatewayClientService_GetConfig_FullMethodName               = "/devloop_gateway.v1.GatewayClientService/GetConfig"
	GatewayClientService_GetRuleStatus_FullMethodName           = "/devloop_gateway.v1.GatewayClientService/GetRuleStatus"
	GatewayClientService_TriggerRuleClient_FullMethodName       = "/devloop_gateway.v1.GatewayClientService/TriggerRuleClient"
	GatewayClientService_ListWatchedPaths_FullMethodName        = "/devloop_gateway.v1.GatewayClientService/ListWatchedPaths"
	GatewayClientService_ReadFileContent_FullMethodName         = "/devloop_gateway.v1.GatewayClientService/ReadFileContent"
	GatewayClientService_StreamLogsClient_FullMethodName        = "/devloop_gateway.v1.GatewayClientService/StreamLogsClient"
	GatewayClientService_GetHistoricalLogsClient_FullMethodName = "/devloop_gateway.v1.GatewayClientService/GetHistoricalLogsClient"
)

// GatewayClientServiceClient is the client API for GatewayClientService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// GatewayClientService defines the gRPC service for MCP clients to interact with the gateway.
type GatewayClientServiceClient interface {
	// List all registered devloop projects.
	ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error)
	// Get the configuration for a specific devloop project.
	GetConfig(ctx context.Context, in *GetConfigRequest, opts ...grpc.CallOption) (*GetConfigResponse, error)
	// Get the detailed status of a specific rule within a project.
	GetRuleStatus(ctx context.Context, in *GetRuleStatusRequest, opts ...grpc.CallOption) (*GetRuleStatusResponse, error)
	// Manually trigger a specific rule in a devloop project.
	TriggerRuleClient(ctx context.Context, in *TriggerRuleClientRequest, opts ...grpc.CallOption) (*TriggerRuleClientResponse, error)
	// List all glob patterns being watched by a specific devloop project.
	ListWatchedPaths(ctx context.Context, in *ListWatchedPathsRequest, opts ...grpc.CallOption) (*ListWatchedPathsResponse, error)
	// Read and return the content of a specific file within a devloop project.
	ReadFileContent(ctx context.Context, in *ReadFileContentRequest, opts ...grpc.CallOption) (*ReadFileContentResponse, error)
	// Stream real-time logs for a specific rule in a project.
	StreamLogsClient(ctx context.Context, in *StreamLogsClientRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogLine], error)
	// Retrieve historical logs for a specific rule, with optional time filtering.
	GetHistoricalLogsClient(ctx context.Context, in *GetHistoricalLogsClientRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogLine], error)
}

type gatewayClientServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewGatewayClientServiceClient(cc grpc.ClientConnInterface) GatewayClientServiceClient {
	return &gatewayClientServiceClient{cc}
}

func (c *gatewayClientServiceClient) ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListProjectsResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_ListProjects_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) GetConfig(ctx context.Context, in *GetConfigRequest, opts ...grpc.CallOption) (*GetConfigResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetConfigResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_GetConfig_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) GetRuleStatus(ctx context.Context, in *GetRuleStatusRequest, opts ...grpc.CallOption) (*GetRuleStatusResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRuleStatusResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_GetRuleStatus_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) TriggerRuleClient(ctx context.Context, in *TriggerRuleClientRequest, opts ...grpc.CallOption) (*TriggerRuleClientResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(TriggerRuleClientResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_TriggerRuleClient_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) ListWatchedPaths(ctx context.Context, in *ListWatchedPathsRequest, opts ...grpc.CallOption) (*ListWatchedPathsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListWatchedPathsResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_ListWatchedPaths_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) ReadFileContent(ctx context.Context, in *ReadFileContentRequest, opts ...grpc.CallOption) (*ReadFileContentResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ReadFileContentResponse)
	err := c.cc.Invoke(ctx, GatewayClientService_ReadFileContent_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClientServiceClient) StreamLogsClient(ctx context.Context, in *StreamLogsClientRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogLine], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &GatewayClientService_ServiceDesc.Streams[0], GatewayClientService_StreamLogsClient_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[StreamLogsClientRequest, LogLine]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type GatewayClientService_StreamLogsClientClient = grpc.ServerStreamingClient[LogLine]

func (c *gatewayClientServiceClient) GetHistoricalLogsClient(ctx context.Context, in *GetHistoricalLogsClientRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[LogLine], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &GatewayClientService_ServiceDesc.Streams[1], GatewayClientService_GetHistoricalLogsClient_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[GetHistoricalLogsClientRequest, LogLine]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type GatewayClientService_GetHistoricalLogsClientClient = grpc.ServerStreamingClient[LogLine]

// GatewayClientServiceServer is the server API for GatewayClientService service.
// All implementations should embed UnimplementedGatewayClientServiceServer
// for forward compatibility.
//
// GatewayClientService defines the gRPC service for MCP clients to interact with the gateway.
type GatewayClientServiceServer interface {
	// List all registered devloop projects.
	ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error)
	// Get the configuration for a specific devloop project.
	GetConfig(context.Context, *GetConfigRequest) (*GetConfigResponse, error)
	// Get the detailed status of a specific rule within a project.
	GetRuleStatus(context.Context, *GetRuleStatusRequest) (*GetRuleStatusResponse, error)
	// Manually trigger a specific rule in a devloop project.
	TriggerRuleClient(context.Context, *TriggerRuleClientRequest) (*TriggerRuleClientResponse, error)
	// List all glob patterns being watched by a specific devloop project.
	ListWatchedPaths(context.Context, *ListWatchedPathsRequest) (*ListWatchedPathsResponse, error)
	// Read and return the content of a specific file within a devloop project.
	ReadFileContent(context.Context, *ReadFileContentRequest) (*ReadFileContentResponse, error)
	// Stream real-time logs for a specific rule in a project.
	StreamLogsClient(*StreamLogsClientRequest, grpc.ServerStreamingServer[LogLine]) error
	// Retrieve historical logs for a specific rule, with optional time filtering.
	GetHistoricalLogsClient(*GetHistoricalLogsClientRequest, grpc.ServerStreamingServer[LogLine]) error
}

// UnimplementedGatewayClientServiceServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedGatewayClientServiceServer struct{}

func (UnimplementedGatewayClientServiceServer) ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProjects not implemented")
}
func (UnimplementedGatewayClientServiceServer) GetConfig(context.Context, *GetConfigRequest) (*GetConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetConfig not implemented")
}
func (UnimplementedGatewayClientServiceServer) GetRuleStatus(context.Context, *GetRuleStatusRequest) (*GetRuleStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRuleStatus not implemented")
}
func (UnimplementedGatewayClientServiceServer) TriggerRuleClient(context.Context, *TriggerRuleClientRequest) (*TriggerRuleClientResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method TriggerRuleClient not implemented")
}
func (UnimplementedGatewayClientServiceServer) ListWatchedPaths(context.Context, *ListWatchedPathsRequest) (*ListWatchedPathsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListWatchedPaths not implemented")
}
func (UnimplementedGatewayClientServiceServer) ReadFileContent(context.Context, *ReadFileContentRequest) (*ReadFileContentResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReadFileContent not implemented")
}
func (UnimplementedGatewayClientServiceServer) StreamLogsClient(*StreamLogsClientRequest, grpc.ServerStreamingServer[LogLine]) error {
	return status.Errorf(codes.Unimplemented, "method StreamLogsClient not implemented")
}
func (UnimplementedGatewayClientServiceServer) GetHistoricalLogsClient(*GetHistoricalLogsClientRequest, grpc.ServerStreamingServer[LogLine]) error {
	return status.Errorf(codes.Unimplemented, "method GetHistoricalLogsClient not implemented")
}
func (UnimplementedGatewayClientServiceServer) testEmbeddedByValue() {}

// UnsafeGatewayClientServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GatewayClientServiceServer will
// result in compilation errors.
type UnsafeGatewayClientServiceServer interface {
	mustEmbedUnimplementedGatewayClientServiceServer()
}

func RegisterGatewayClientServiceServer(s grpc.ServiceRegistrar, srv GatewayClientServiceServer) {
	// If the following call pancis, it indicates UnimplementedGatewayClientServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&GatewayClientService_ServiceDesc, srv)
}

func _GatewayClientService_ListProjects_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListProjectsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).ListProjects(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_ListProjects_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).ListProjects(ctx, req.(*ListProjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_GetConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).GetConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_GetConfig_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).GetConfig(ctx, req.(*GetConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_GetRuleStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRuleStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).GetRuleStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_GetRuleStatus_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).GetRuleStatus(ctx, req.(*GetRuleStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_TriggerRuleClient_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TriggerRuleClientRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).TriggerRuleClient(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_TriggerRuleClient_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).TriggerRuleClient(ctx, req.(*TriggerRuleClientRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_ListWatchedPaths_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListWatchedPathsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).ListWatchedPaths(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_ListWatchedPaths_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).ListWatchedPaths(ctx, req.(*ListWatchedPathsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_ReadFileContent_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReadFileContentRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayClientServiceServer).ReadFileContent(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: GatewayClientService_ReadFileContent_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayClientServiceServer).ReadFileContent(ctx, req.(*ReadFileContentRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _GatewayClientService_StreamLogsClient_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(StreamLogsClientRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(GatewayClientServiceServer).StreamLogsClient(m, &grpc.GenericServerStream[StreamLogsClientRequest, LogLine]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type GatewayClientService_StreamLogsClientServer = grpc.ServerStreamingServer[LogLine]

func _GatewayClientService_GetHistoricalLogsClient_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetHistoricalLogsClientRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(GatewayClientServiceServer).GetHistoricalLogsClient(m, &grpc.GenericServerStream[GetHistoricalLogsClientRequest, LogLine]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type GatewayClientService_GetHistoricalLogsClientServer = grpc.ServerStreamingServer[LogLine]

// GatewayClientService_ServiceDesc is the grpc.ServiceDesc for GatewayClientService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var GatewayClientService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "devloop_gateway.v1.GatewayClientService",
	HandlerType: (*GatewayClientServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ListProjects",
			Handler:    _GatewayClientService_ListProjects_Handler,
		},
		{
			MethodName: "GetConfig",
			Handler:    _GatewayClientService_GetConfig_Handler,
		},
		{
			MethodName: "GetRuleStatus",
			Handler:    _GatewayClientService_GetRuleStatus_Handler,
		},
		{
			MethodName: "TriggerRuleClient",
			Handler:    _GatewayClientService_TriggerRuleClient_Handler,
		},
		{
			MethodName: "ListWatchedPaths",
			Handler:    _GatewayClientService_ListWatchedPaths_Handler,
		},
		{
			MethodName: "ReadFileContent",
			Handler:    _GatewayClientService_ReadFileContent_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamLogsClient",
			Handler:       _GatewayClientService_StreamLogsClient_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "GetHistoricalLogsClient",
			Handler:       _GatewayClientService_GetHistoricalLogsClient_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "protos/devloop/v1/devloop_gateway.proto",
}
