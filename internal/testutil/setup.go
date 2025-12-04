package testutil

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"time"

	"github.com/cybertec-postgresql/pgwatch/v3/api/pb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func SetupPostgresContainer() (*postgres.PostgresContainer, func(), error) {
	pgContainer, err := postgres.Run(ctx,
		pgImageName,
		postgres.WithDatabase("mydatabase"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)

	tearDown := func() {
		_ = pgContainer.Terminate(ctx)
	}

	return pgContainer, tearDown, err
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

func SetupRPCServers() (func(), error) {
	err := os.WriteFile(CAFile, []byte(CA), 0644)
	teardown := func() { _ = os.Remove(CAFile) }
	if err != nil {
		return teardown, err
	}

	addresses := [2]string{PlainServerAddress, TLSServerAddress}
	for _, address := range addresses {
		lis, err := net.Listen("tcp", address)
		if err != nil {
			return teardown, err
		}

		var creds credentials.TransportCredentials
		if address == TLSServerAddress {
			creds, err = LoadServerTLSCredentials()
			if err != nil {
				return nil, err
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
	// wait a little for servers to start
	time.Sleep(time.Second)
	return teardown, nil
}
