package sinks

import (
	"context"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

// RPCWriter sends metric measurements to a remote server using gRPC.
// Remote servers should make use the .proto file under api/pb/ to integrate with it.
// It's up to the implementer to define the behavior of the server. 
// It can be a simple logger, external storage, alerting system, or an analytics system.
type RPCWriter struct {
	ctx     context.Context
	conn    *grpc.ClientConn
	client  pb.ReceiverClient
}

func NewRPCWriter(ctx context.Context, host string) (*RPCWriter, error) {
	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	client := pb.NewReceiverClient(conn)
	rw := &RPCWriter{
		ctx: ctx, 
		conn: conn, 
		client: client,
	}
	go rw.watchCtx()

	return rw, nil
}

// Sends Measurement Message to RPC Sink
func (rw *RPCWriter) Write(msg metrics.MeasurementEnvelope) error {
	if rw.ctx.Err() != nil {
		return rw.ctx.Err()
	}

	dataLength := len(msg.Data)
	failCnt := 0
	measurements := make([]*structpb.Struct, 0, dataLength)
	for _, item := range msg.Data {
		st, err := structpb.NewStruct(item) 
		if err == nil {
			failCnt++
			continue
		}
		measurements = append(measurements, st)
	}
	if failCnt > 0 {
		log.GetLogger(rw.ctx).WithField("database", msg.DBName).WithField("metric", 
			msg.MetricName).Warningf("gRPC sink failed to encode %d rows", failCnt)
	}

	envelope := &pb.MeasurementEnvelope{
		DBName: msg.DBName,
		MetricName: msg.MetricName,
		CustomTags: msg.CustomTags, 
		Data: measurements,
	}

	t1 := time.Now()
	reply, err := rw.client.UpdateMeasurements(rw.ctx, envelope)
	if err != nil {
		return err
	}

	diff := time.Since(t1)
	log.GetLogger(rw.ctx).WithField("rows", dataLength).WithField("elapsed", diff).Info("measurements written")
	if reply.GetLogmsg() != "" {
		log.GetLogger(rw.ctx).Info(reply.GetLogmsg())
	}
	return nil
}

func (rw *RPCWriter) SyncMetric(dbUnique, metricName string, op pb.SyncOp) error {
	syncReq := &pb.SyncReq{
		DBName: dbUnique,	
		MetricName: metricName,
		Operation: op,
	}

	reply, err := rw.client.SyncMetric(rw.ctx, syncReq)
	if err != nil {
		return err
	}
	
	if reply.GetLogmsg() != "" {
		log.GetLogger(rw.ctx).Info(reply.GetLogmsg())
	}
	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.conn.Close()
}