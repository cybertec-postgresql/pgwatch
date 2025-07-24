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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

var ctx = context.Background()

// the CN in server test cert is set to `localhost`
var CAFile = "ca.crt"
var TLSConnStr = fmt.Sprintf("grpc://localhost:5050?sslrootca=%s", CAFile) 
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
	cert, err := tls.X509KeyPair(Cert, PrivateKey)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	return credentials.NewTLS(tlsConfig), nil
}

func AuthInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)

	clientUsername := md.Get("username")[0]
	clientPassword := md.Get("password")[0]

	if clientUsername != "" && clientUsername != "pgwatch" && clientPassword != "pgwatch" {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	return handler(ctx, req)
}

func TestMain(m *testing.M) {
	err := os.WriteFile(CAFile, []byte(CA), 0644)
	if err != nil {
		panic(err)
	}

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
			grpc.UnaryInterceptor(AuthInterceptor),
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

	exitCode := m.Run()
	_ = os.Remove(CAFile)

	os.Exit(exitCode)
}

// Tests begin from here ---------------------------------------------------------

func TestCACertParamValidation(t *testing.T) {
	a := assert.New(t)
	_, err := sinks.NewRPCWriter(ctx, TLSConnStr)
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

func TestAuthCredsSending(t *testing.T) {
	a := assert.New(t)

	unauthenticatedConnStr := "grpc://notpgwatch:notpgwatch@localhost:6060"
	rw, err := sinks.NewRPCWriter(ctx, unauthenticatedConnStr)
	a.NoError(err)

	err = rw.Write(metrics.MeasurementEnvelope{})
	a.Equal(err, status.Error(codes.Unauthenticated, "unauthenticated"))
}

// End of tests ----------------------------------

var CA = `-----BEGIN CERTIFICATE-----
MIIDPzCCAiegAwIBAgIUeENQlQFVH5h7HszFJLLWo+KCQwQwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHcGd3YXRjaDAeFw0yNTA2MTEwNTI1MDJaFw0zNTA2MDkw
NTI1MDJaMBIxEDAOBgNVBAMMB3Bnd2F0Y2gwggEiMA0GCSqGSIb3DQEBAQUAA4IB
DwAwggEKAoIBAQDtTW+kyvb3Y5OaYlriKp8HkHt95lfOgxQNZQfiREfEyLWU59bx
0ZIvFmejK38Qc0dlca9d+5tEkxotsbggJflLljfmnzhxsuZpr8SjmDd1m8XSo0IA
oDlVbKO6SZMlsyq3QrAOYAjG1LTPlATqvGAOs9NfFonjwoPXCjIwSfa+wexe5dRD
gJ114AKXw3ck5ZQ4Pw+w5ylgNSfVl548WY9DSOA+6HlZ17MYA1qmMOTwKae5fmsc
xlRPoIV3EJrKas7VlbebDOXOSXDV+9aMW6ox1xanUUDUgabzkzntfmuOttaIX1g1
nSMuHa7EEQF7lxgdg9OU8i/jygdlGcgBUYjtAgMBAAGjgYwwgYkwDAYDVR0TBAUw
AwEB/zAdBgNVHQ4EFgQUKGJs7OXINd2WL6X2meH90eINoJQwTQYDVR0jBEYwRIAU
KGJs7OXINd2WL6X2meH90eINoJShFqQUMBIxEDAOBgNVBAMMB3Bnd2F0Y2iCFHhD
UJUBVR+Yex7MxSSy1qPigkMEMAsGA1UdDwQEAwIBBjANBgkqhkiG9w0BAQsFAAOC
AQEABdY/4rsgMu+sCqEdacNzHqAz9X1ew37y1UONngm/7LPqbQrzzg/fBvOOJLcd
IzMJPtpdwokPYOW29jw/hY4R1RWr8012zc8Z0GsuDR7I/Z2Hww7tzYhf1H5mjy1d
eQDhHNpsSb5pHLoPft5O0sT/0WqAlKWPb2KmSoAio8jE2BSUTK3ZgE0yJIikONon
HCWOlNCWx+RsyPoRnQqbpVa+SmGBqpiyHchpZ8sFPe+pgPu+8u921lJ0PRvmfp7L
4YZIaM8LQAV8FWk2VLXmsqYUJYYLAXCG6Unkx1oIOtq1AyAoXHCl/3hKbCeXIrgA
Cs5qN+ZUHRdKff5gFpraKtHKkw==
-----END CERTIFICATE-----`

var Cert = []byte(`-----BEGIN CERTIFICATE-----
MIIDZTCCAk2gAwIBAgIQeTQ+4M7xwydf7MvrDnDdsTANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQDDAdwZ3dhdGNoMB4XDTI1MDYxMTA1MjUxOFoXDTI3MDkxNDA1MjUx
OFowFDESMBAGA1UEAwwJbG9jYWxob3N0MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA69Ja9K4ZHoIdBK+34YE1+0a/sB7YKZJb3+gYFahvszS37Oa6h/0+
d9TCY58bMpPQSQQdvhG/s1H6Yc4BWTfH/ssRmDhKciMmdfMj/lr8TytglIUPPSzb
PKy2t9idpk30PwiV1UlijjlFZxoxcO3Aia8mmrDJzkoHsQL96PhDT25YRinnflg8
vVthasVqGIHNJIXORyz5lgkBW3NeeZPEUSxbvmo63AB1lFJZMz4xlpdN/LVsEOwg
FrrYzb4mpGkAcTenkOfU4W7m7bxpsusW2JDm2O8bsx+v3cazOqJpCLMHP3Vqnzcl
loYmeQauBep6H0wspid0YzMVza75pDx7rQIDAQABo4G0MIGxMAkGA1UdEwQCMAAw
HQYDVR0OBBYEFFcDjPaZMelQIueauIRfo4L4wMehME0GA1UdIwRGMESAFChibOzl
yDXdli+l9pnh/dHiDaCUoRakFDASMRAwDgYDVQQDDAdwZ3dhdGNoghR4Q1CVAVUf
mHsezMUkstaj4oJDBDATBgNVHSUEDDAKBggrBgEFBQcDATALBgNVHQ8EBAMCBaAw
FAYDVR0RBA0wC4IJbG9jYWxob3N0MA0GCSqGSIb3DQEBCwUAA4IBAQCM6tYNxoP2
Gbp3aAPjoA3+U1gWHPHXOOgyhaQw4jJ7xK1MUlrFgSG6cJgO7IRSCIZp7GDZmIjo
+PqWRgMNK2pFCUCqjrAV6NwMjApLzDdSza9xKb3nWXMKnV6j3tNUFUCS68CHAM7Q
E1iuepjIy2VReFfjJoPuhp9OQBWobTo3H9F74Sj+Guu0lDcHWbwn5Y92pnKk0vOh
v1AJ6vwdMpd6DAPlwmY3OcZI2FGYyoPP2CnzHIGP5RoVFp1zkJzoFvnOHnsRMByz
HpGKqYFQVJSAOMCtL2OMiP8MxtiCsdz6j/e3/VOUQuYoM6fXFhZO64xekZdlh/ZR
glsaMXQPWvHX
-----END CERTIFICATE-----`)

var PrivateKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDr0lr0rhkegh0E
r7fhgTX7Rr+wHtgpklvf6BgVqG+zNLfs5rqH/T531MJjnxsyk9BJBB2+Eb+zUfph
zgFZN8f+yxGYOEpyIyZ18yP+WvxPK2CUhQ89LNs8rLa32J2mTfQ/CJXVSWKOOUVn
GjFw7cCJryaasMnOSgexAv3o+ENPblhGKed+WDy9W2FqxWoYgc0khc5HLPmWCQFb
c155k8RRLFu+ajrcAHWUUlkzPjGWl038tWwQ7CAWutjNviakaQBxN6eQ59Thbubt
vGmy6xbYkObY7xuzH6/dxrM6omkIswc/dWqfNyWWhiZ5Bq4F6nofTCymJ3RjMxXN
rvmkPHutAgMBAAECggEAAc8djQJ35VzEqbhKXhO+bQTMLCb0bA84HrXaV3IxFywY
nBviAvCNpeAvNJHwJLlvD9xU+RQMRy0iEVWB+6P6qAj5Q9Rst8buwNliZY1foaDY
zxLdPNAnB2ZgyXTDMtcmwEQJ2DbFp4cnceTIy8+7GiNKlcW06pz1RaWa+opLA+U2
STIxvTEAvqsyE/0KHbeEltwZxeZ83BsX8vhpyrCVvniFJnIMvyYG7iTzLWuTK98Z
R3Baqim8CdWbh2W0OOfphAVlTjG6c0r6FqJIqsds9wf2FfuhgUcNQUXke+uuWbPQ
36RsytymUgqye3DkxrC4dEi27S3cjRh5wK53gqET+QKBgQD8zM5wiindvDRSx+66
ppbl6RJQcL7uv1os9TeNvfqwnhC75y2k4+2s8kiG2ik7aJiRg4dkj3rVJ4S6wQ67
sjRVo5z68J1twP6PqvpJyx/G5Fmy8HPJUmy9FM1AdnYCWGX3bh3qNTkxEVn78yFZ
zfn9CczDAmGErAXPRRDNQQy8ZQKBgQDuzofs5EXTpqw25XveCJKHT3fAis7pkNGu
jwopiR9peKuJ9nNarHH2wWpHRn6zgHmhmAu7oEzo4OEmk4Elo5ffqqRPZZaU8aEo
Ow7cRvedoP/EjJaR8m2uQnh9bWXuEVibfKmkrPswYYUCWmwoALFmzd6Gl0dIdyGk
JXeA/jFZqQKBgQDhFbn5mgsM0rYDvuBgcFOLAaq81KYsDVRNE0kTe0PqXdKoe324
gvjsNA0/hJ+Rtd+iMGosr1O+1iDn510m4dSXK8Zp6DNDtcLySFnxulngzRDQsidl
6W3ILO1TqCYKkIq5c+JO1nTFq51jJ2dafntHQaJ/P290oXXKxsPe/TxJwQKBgD6g
dS8f8lv+Mt22sxRYhSztH0ekX30LWKIBqzWXW2CKn9nvgvL9lGmU8a09hI7Im51Q
RYtwD5tnFkTKnCzlyTeEBdE4oBPxhkUJr+z+w4NYLJs8D2S5AiCYGAc0wG19qRIl
0Et6femDOaGTWxfmjp+aT8hWNgCAFZd5p+xxPTn5AoGBAIeUbCRTe0KcVaKw/Lcj
KHjTih9x9d3f2EbnYbziBZz6fZfWdDIBfA6CbIHil21hNvjDm9Yci5b6FgtUzScb
j3vPa99sMGc2xie07Cd7LTvZeWIVXeW1Dxzex89CqoJzmONrco1ZKQ+xXbPoZCBj
VwAI/SWm7NlxgF6Sr5CIo2KR
-----END PRIVATE KEY-----`)