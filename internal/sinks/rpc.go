package sinks

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/log"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	jsoniter "github.com/json-iterator/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// RPCWriter sends metric measurements to a remote server using gRPC.
// Remote servers should make use the .proto file under api/pb/ to integrate with it.
// It's up to the implementer to define the behavior of the server.
// It can be a simple logger, external storage, alerting system, or an analytics system.
type RPCWriter struct {
	ctx    context.Context
	conn   *grpc.ClientConn
	client pb.ReceiverClient
}

// convertSyncOp converts sinks.SyncOp to pb.SyncOp
func convertSyncOp(op SyncOp) pb.SyncOp {
	switch op {
	case AddOp:
		return pb.SyncOp_AddOp
	case DeleteOp:
		return pb.SyncOp_DeleteOp
	case DefineOp:
		return pb.SyncOp_DefineOp
	default:
		return pb.SyncOp_InvalidOp
	}
}


func NewRPCWriter(ctx context.Context, connStr string) (*RPCWriter, error) {
	uri, err := url.Parse(connStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing gRPC URI: %s", err)
	}

	l := log.GetLogger(ctx).WithField("sink", "grpc").WithField("server", fmt.Sprintf("\"%s\"", uri.Host))
	ctx = log.WithLogger(ctx, l)

	params, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("error parsing gRPC URI parameters: %s", err)
	}

	creds := insecure.NewCredentials()

	CAFile, ok := params["sslrootca"]
	if ok {
		creds, err = LoadTLSCredentials(CAFile[0])
		if err != nil {
			return nil, err
		}
		log.GetLogger(ctx).Infof("Valid CA File %s loaded - enabling TLS", CAFile)
	}

	conn, err := grpc.NewClient(uri.Host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	
	password, _ := uri.User.Password()
	md := metadata.Pairs(  
		"username", uri.User.Username(),  
		"password", password,
	)  
	newCtx := metadata.NewOutgoingContext(ctx, md)
	
	client := pb.NewReceiverClient(conn)
	rw := &RPCWriter{
		ctx:    newCtx,
		conn:   conn,
		client: client,
	}

	if err = rw.Ping(); err != nil {
		return nil, err
	}

	go rw.watchCtx()
	return rw, nil
}

func (rw *RPCWriter) Ping() error {
	err := rw.SyncMetric("", "", InvalidOp)
	st, ok := status.FromError(err)
	if ok && st.Code() == codes.Unavailable {
		return err
	}
	return nil
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
		if err != nil {
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
		DBName:     msg.DBName,
		MetricName: msg.MetricName,
		CustomTags: msg.CustomTags,
		Data:       measurements,
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

// SyncMetric synchronizes a metric and monitored source with the remote server
func (rw *RPCWriter) SyncMetric(sourceName, metricName string, op SyncOp) error {
	syncReq := &pb.SyncReq{
		DBName:     sourceName,
		MetricName: metricName,
		Operation:  convertSyncOp(op),
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

// DefineMetrics sends metric definitions to the remote server
func (rw *RPCWriter) DefineMetrics(metrics *metrics.Metrics) error {
	var json = jsoniter.ConfigFastest

	// Convert metrics to JSON first, then to structpb.Struct
	// to automatically handle all the type conversions
	jsonData, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	var metricMap map[string]any
	if err := json.Unmarshal(jsonData, &metricMap); err != nil {
		return err
	}

	metricStruct, err := structpb.NewStruct(metricMap)
	if err != nil {
		return err
	}

	t1 := time.Now()
	reply, err := rw.client.DefineMetrics(rw.ctx, metricStruct)
	if err != nil {
		return err
	}

	diff := time.Since(t1)
	log.GetLogger(rw.ctx).WithField("elapsed", diff).Info("metric definitions written")
	if reply.GetLogmsg() != "" {
		log.GetLogger(rw.ctx).Info(reply.GetLogmsg())
	}
	return nil
}

func (rw *RPCWriter) watchCtx() {
	<-rw.ctx.Done()
	rw.conn.Close()
}

func LoadTLSCredentials(CAFile string) (credentials.TransportCredentials, error) {
	ca, err := os.ReadFile(CAFile)
	if err != nil {
		return nil, fmt.Errorf("error loading CA file: %v", err)
	}

	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(ca)
	if !ok {
		return nil, errors.New("invalid CA file")
	}

	tlsClientConfig := &tls.Config{
		RootCAs: certPool,
	}
	return credentials.NewTLS(tlsClientConfig), nil
}