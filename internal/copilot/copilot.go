package copilot

import (
	"context"
	"fmt"

	"github.com/cybertec-postgresql/pgwatch/v5/api/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
)

type Client struct {
	conn       *grpc.ClientConn
	grpcClient pb.ReceiverClient
	opts       CmdOpts
}

// New creates a new AI Copilot client. If mode is "mock", it returns a client that
// provides static simulated responses without connecting to a real gRPC server.
func New(ctx context.Context, opts CmdOpts) (*Client, error) {
	if opts.Mode == "mock" {
		return &Client{opts: opts}, nil
	}

	// Connect to the Python AI service using insecure credentials for local communication
	conn, err := grpc.DialContext(ctx, opts.CopilotAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to copilot server at %s: %w", opts.CopilotAddr, err)
	}

	return &Client{
		conn:       conn,
		grpcClient: pb.NewReceiverClient(conn),
		opts:       opts,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Analyze sends metric data to the AI Copilot for analysis.
func (c *Client) Analyze(ctx context.Context, dbname, metric string, data []map[string]any) (string, error) {
	if c.opts.Mode == "mock" {
		return fmt.Sprintf("[MOCK] AI analysis of %s for %s: Metrics are within expected range.", metric, dbname), nil
	}

	// Convert Go maps to protobuf Structs
	pbData := make([]*structpb.Struct, len(data))
	for i, row := range data {
		s, err := structpb.NewStruct(row)
		if err != nil {
			return "", fmt.Errorf("failed to convert data row to structpb: %w", err)
		}
		pbData[i] = s
	}

	// Call the gRPC service
	resp, err := c.grpcClient.UpdateMeasurements(ctx, &pb.MeasurementEnvelope{
		DBName:     dbname,
		MetricName: metric,
		Data:       pbData,
	})
	if err != nil {
		return "", fmt.Errorf("copilot gRPC call failed: %w", err)
	}

	return resp.GetLogmsg(), nil
}
