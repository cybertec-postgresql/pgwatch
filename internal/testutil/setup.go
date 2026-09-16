package testutil

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v7/api/pb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/etcd"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func SetupPostgresContainer() (*postgres.PostgresContainer, func(), error) {
	pgContainer, err := postgres.Run(TestContext,
		PostgresImage,
		postgres.WithDatabase(MockDatabase),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)

	tearDown := func() {
		_ = pgContainer.Terminate(TestContext)
	}

	return pgContainer, tearDown, err
}

// Creates a PostgreSQL container with CSV logging enabled.
// This is useful for testing log parsing functionality with server_log_event_counts metric.
func SetupPostgresContainerWithConfig(configPath string) (*postgres.PostgresContainer, func(), error) {
	pgContainer, err := postgres.Run(TestContext,
		PostgresImage,
		postgres.WithDatabase(MockDatabase),
		postgres.WithConfigFile(configPath),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(5*time.Second)),
	)

	tearDown := func() {
		_ = pgContainer.Terminate(TestContext)
	}

	return pgContainer, tearDown, err
}

func SetupPostgresContainerWithInitScripts(scripts ...string) (*postgres.PostgresContainer, func(), error) {
	pgContainer, err := postgres.Run(TestContext,
		PostgresImage,
		postgres.WithDatabase(MockDatabase),
		postgres.WithInitScripts(scripts...),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)

	tearDown := func() {
		_ = pgContainer.Terminate(TestContext)
	}

	return pgContainer, tearDown, err
}

func SetupEtcdContainer() (*etcd.EtcdContainer, func(), error) {
	etcdContainer, err := etcd.Run(TestContext, EtcdImage,
		testcontainers.
			WithWaitStrategy(wait.ForLog("ready to serve client requests").
				WithStartupTimeout(15*time.Second)))

	tearDown := func() {
		_ = etcdContainer.Terminate(TestContext)
	}

	return etcdContainer, tearDown, err
}

//-----------Setup gRPC test servers-----------------

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

// SetupRPCServers starts the plain and TLS gRPC receivers used by the sink
// tests and publishes their addresses in PlainServerAddress/PlainConnStr and
// TLSServerAddress/TLSConnStr. The ports are ephemeral on purpose: a fixed one
// collides when two test binaries run at once, and on Windows it cannot even
// be rebound while an earlier binary's connections sit in TIME_WAIT, which Go
// does not paper over with SO_REUSEADDR there.
func SetupRPCServers() (func(), error) {
	var servers []*grpc.Server
	teardown := func() {
		for _, s := range servers {
			s.Stop()
		}
		_ = os.Remove(CAFile)
	}

	if err := os.WriteFile(CAFile, []byte(CA), 0644); err != nil {
		return teardown, err
	}

	for _, withTLS := range [2]bool{false, true} {
		lis, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			return teardown, err
		}
		// The server certificate has localhost as its CN, so the address must
		// be spelled that way and not as the resolved 127.0.0.1.
		address := fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)

		var creds credentials.TransportCredentials
		if withTLS {
			if creds, err = LoadServerTLSCredentials(); err != nil {
				return teardown, err
			}
			TLSServerAddress = address
			TLSConnStr = fmt.Sprintf("grpc://%s?sslrootca=%s", address, CAFile)
		} else {
			PlainServerAddress = address
			PlainConnStr = "grpc://" + address
		}

		server := grpc.NewServer(
			grpc.UnaryInterceptor(AuthInterceptor),
			grpc.Creds(creds),
		)
		servers = append(servers, server)

		recv := new(Receiver)
		pb.RegisterReceiverServer(server, recv)

		go func() {
			if err := server.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				panic(err)
			}
		}()
	}
	return teardown, nil
}
