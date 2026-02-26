package pb

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type SyncOp int32

const (
	SyncOp_InvalidOp SyncOp = 0
	SyncOp_AddOp     SyncOp = 1
	SyncOp_DeleteOp  SyncOp = 2
	SyncOp_DefineOp  SyncOp = 3
)

type Reply struct {
	Logmsg string
}

func (m *Reply) GetLogmsg() string {
	if m != nil {
		return m.Logmsg
	}
	return ""
}

type MeasurementEnvelope struct {
	DBName     string
	MetricName string
	CustomTags map[string]string
	Data       []*structpb.Struct
}

func (m *MeasurementEnvelope) GetDBName() string {
	if m != nil {
		return m.DBName
	}
	return ""
}

func (m *MeasurementEnvelope) GetMetricName() string {
	if m != nil {
		return m.MetricName
	}
	return ""
}

func (m *MeasurementEnvelope) GetData() []*structpb.Struct {
	if m != nil {
		return m.Data
	}
	return nil
}

type SyncReq struct {
	DBName    string
	MetricName string
	Operation  SyncOp
}

func (m *SyncReq) GetDBName() string {
	if m != nil {
		return m.DBName
	}
	return ""
}

func (m *SyncReq) GetOperation() SyncOp {
	if m != nil {
		return m.Operation
	}
	return SyncOp_InvalidOp
}

type ReceiverClient interface {
	UpdateMeasurements(ctx context.Context, in *MeasurementEnvelope, opts ...grpc.CallOption) (*Reply, error)
	SyncMetric(ctx context.Context, in *SyncReq, opts ...grpc.CallOption) (*Reply, error)
	DefineMetrics(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*Reply, error)
}

type receiverClient struct {
	cc grpc.ClientConnInterface
}

func NewReceiverClient(cc grpc.ClientConnInterface) ReceiverClient {
	return &receiverClient{cc}
}

func (c *receiverClient) UpdateMeasurements(ctx context.Context, in *MeasurementEnvelope, opts ...grpc.CallOption) (*Reply, error) {
	out := new(Reply)
	err := c.cc.Invoke(ctx, "/Receiver/UpdateMeasurements", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *receiverClient) SyncMetric(ctx context.Context, in *SyncReq, opts ...grpc.CallOption) (*Reply, error) {
	out := new(Reply)
	err := c.cc.Invoke(ctx, "/Receiver/SyncMetric", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *receiverClient) DefineMetrics(ctx context.Context, in *structpb.Struct, opts ...grpc.CallOption) (*Reply, error) {
	out := new(Reply)
	err := c.cc.Invoke(ctx, "/Receiver/DefineMetrics", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

type ReceiverServer interface {
	UpdateMeasurements(context.Context, *MeasurementEnvelope) (*Reply, error)
	SyncMetric(context.Context, *SyncReq) (*Reply, error)
	DefineMetrics(context.Context, *structpb.Struct) (*Reply, error)
}

func RegisterReceiverServer(s grpc.ServiceRegistrar, srv ReceiverServer) {
	s.RegisterService(&_Receiver_serviceDesc, srv)
}

var _Receiver_serviceDesc = grpc.ServiceDesc{
	ServiceName: "Receiver",
	HandlerType: (*ReceiverServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "UpdateMeasurements",
			Handler:    _Receiver_UpdateMeasurements_Handler,
		},
		{
			MethodName: "SyncMetric",
			Handler:    _Receiver_SyncMetric_Handler,
		},
		{
			MethodName: "DefineMetrics",
			Handler:    _Receiver_DefineMetrics_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pgwatch.proto",
}

func _Receiver_UpdateMeasurements_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MeasurementEnvelope)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ReceiverServer).UpdateMeasurements(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Receiver/UpdateMeasurements",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ReceiverServer).UpdateMeasurements(ctx, req.(*MeasurementEnvelope))
	}
	return interceptor(ctx, in, info, handler)
}

func _Receiver_SyncMetric_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SyncReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ReceiverServer).SyncMetric(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Receiver/SyncMetric",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ReceiverServer).SyncMetric(ctx, req.(*SyncReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _Receiver_DefineMetrics_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(structpb.Struct)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ReceiverServer).DefineMetrics(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/Receiver/DefineMetrics",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ReceiverServer).DefineMetrics(ctx, req.(*structpb.Struct))
	}
	return interceptor(ctx, in, info, handler)
}

type UnimplementedReceiverServer struct{}

func (UnimplementedReceiverServer) UpdateMeasurements(context.Context, *MeasurementEnvelope) (*Reply, error) {
	return nil, nil
}
func (UnimplementedReceiverServer) SyncMetric(context.Context, *SyncReq) (*Reply, error) {
	return nil, nil
}
func (UnimplementedReceiverServer) DefineMetrics(context.Context, *structpb.Struct) (*Reply, error) {
	return nil, nil
}
