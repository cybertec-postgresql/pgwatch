package sinks_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/metrics"
	"github.com/cybertec-postgresql/pgwatch/v3/internal/sinks"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

var ctx = context.Background()

const CA = "./rpc_tests_certs/ca.crt"
const ServerCert = "./rpc_tests_certs/server.crt"
const ServerKey = "./rpc_tests_certs/server.key"

// the CN in server test cert is set to `localhost`
var TLSConnStr = fmt.Sprintf("grpc://localhost:5050?sslrootca=%s", CA) 
const TLSServerAddress = "localhost:5050"

const PlainServerAddress = "localhost:6060"
const PlainConnStr = "grpc://localhost:6060"

type Receiver struct {
	pb.UnimplementedReceiverServer
}

func (receiver *Receiver) UpdateMeasurements(_ context.Context, msg *pb.MeasurementEnvelope) (*pb.Reply, error) {
	if len(msg.GetData()) == 0 {
		return nil, errors.New("empty message")
	}
	if msg.GetDBName() != "Db" {
		return nil, errors.New("invalid message")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) SyncMetric(_ context.Context, syncReq *pb.SyncReq) (*pb.Reply, error) {
	if syncReq == nil {
		return nil, errors.New("nil sync request")
	}
	if syncReq.GetOperation() == pb.SyncOp_InvalidOp {
		return nil, errors.New("invalid sync request")
	}
	return &pb.Reply{}, nil
}

func (receiver *Receiver) DefineMetrics(_ context.Context, metricsStruct *structpb.Struct) (*pb.Reply, error) {
	if metricsStruct == nil {
		return nil, errors.New("nil metrics struct")
	}
	if metricsStruct.GetFields() == nil {
		return nil, errors.New("empty metrics struct")
	}
	return &pb.Reply{Logmsg: "metrics defined successfully"}, nil
}

func LoadServerTLSCredentials() (credentials.TransportCredentials, error) {
	cert, err := tls.LoadX509KeyPair(ServerCert, ServerKey)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return credentials.NewTLS(tlsConfig), nil
}

func init() {
	addresses := [2]string{PlainServerAddress, TLSServerAddress}
	for _, address := range addresses {
		lis, err := net.Listen("tcp", address)
		if err != nil {
			panic(err)
		}

		var creds credentials.TransportCredentials
		if address == TLSServerAddress {
			creds, err = LoadServerTLSCredentials()
			if err != nil {
				panic(err)
			}
		}

		server := grpc.NewServer(
			grpc.Creds(creds),
		)

		recv := new(Receiver)
		pb.RegisterReceiverServer(server, recv)

		go func() {
			if err := server.Serve(lis); err != nil {
				panic(err)
			}
		}()
	}
	// wait a little for servers start
	time.Sleep(time.Second)
}

// Tests begin from here ---------------------------------------------------------

func TestCACertParamValidation(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)

	_, _ = os.Create("badca.crt")
	defer func ()  {  _ = os.Remove("badca.crt") }()

	BadRPCParams := map[string]string{
		"?sslrootca=file.txt": "error loading CA file: open file.txt: no such file or directory", 
		"?sslrootca=": "error loading CA file: open : no such file or directory", 
		"?sslrootca=badca.crt": "invalid CA file",
	}

	for param, errMsg := range BadRPCParams {
		_, err = sinks.NewRPCWriter(ctx, fmt.Sprintf("grpc://%s%s", TLSServerAddress, param))
		a.EqualError(err, errMsg)
	}
}

func TestRPCTLSWriter(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, TLSConnStr)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)
}

func TestRPCWrite(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)

	// no error for valid messages
	msgs := metrics.MeasurementEnvelope{
		DBName: "Db",
		Data:   metrics.Measurements{{"test": 1}},
	}
	err = rw.Write(msgs)
	a.NoError(err)

	// error for invalid messages
	msgs.DBName = "invalid"
	err = rw.Write(msgs)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid message"))

	// error for empty messages
	err = rw.Write(metrics.MeasurementEnvelope{})
	a.ErrorIs(err, status.Error(codes.Unknown, "empty message"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)
	cancel()
	err = rw.Write(msgs)
	a.Error(err)
}

func TestRPCSyncMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)

	// no error for valid Sync requests
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.NoError(err)

	// error for invalid Sync requests
	err = rw.SyncMetric("", "", sinks.InvalidOp)
	a.ErrorIs(err, status.Error(codes.Unknown, "invalid sync request"))

	// error for cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)
	cancel()
	err = rw.SyncMetric("Test-DB", "DB-Metric", sinks.AddOp)
	a.Error(err)
}

func TestRPCDefineMetric(t *testing.T) {
	a := assert.New(t)

	rw, err := sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)

	// Test that RPCWriter implements MetricsDefiner interface
	var writer sinks.Writer = rw
	definer, ok := writer.(sinks.MetricsDefiner)
	a.True(ok, "RPCWriter should implement MetricsDefiner interface")

	// Test with valid metrics
	testMetrics := &metrics.Metrics{
		MetricDefs: metrics.MetricDefs{
			"test_metric": metrics.Metric{
				SQLs: metrics.SQLs{
					11: "SELECT 1 as test_column",
					12: "SELECT 2 as test_column",
				},
				Description: "Test metric",
				Gauges:      []string{"test_column"},
			},
		},
		PresetDefs: metrics.PresetDefs{
			"test_preset": metrics.Preset{
				Description: "Test preset",
				Metrics:     map[string]float64{"test_metric": 30.0},
			},
		},
	}

	err = definer.DefineMetrics(testMetrics)
	a.NoError(err)

	// Test with empty metrics (should still work)
	emptyMetrics := &metrics.Metrics{
		MetricDefs: make(metrics.MetricDefs),
		PresetDefs: make(metrics.PresetDefs),
	}

	err = definer.DefineMetrics(emptyMetrics)
	a.NoError(err)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(ctx)
	rw, err = sinks.NewRPCWriter(ctx, PlainConnStr)
	a.NoError(err)
	cancel()

	writer = rw
	definer = writer.(sinks.MetricsDefiner)
	err = definer.DefineMetrics(testMetrics)
	a.Error(err)
}
