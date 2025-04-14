package fastpodcontrollermanager

import (
	"context"
	"fmt"

	// Replace with your package name
	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcClient struct {
	conn   *grpc.ClientConn
	client seti.GPUConfiguratorServiceClient
}

func (g *GrpcClient) Connect(nodeIP string, nodePort int) error {

	credentials := grpc.WithTransportCredentials(insecure.NewCredentials())

	// Dial to the gRPC server using insecure connection (for simplicity)
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", nodeIP, nodePort), credentials) // Use secure connection in production
	if err != nil {
		return fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	// Set the connection and create the GPUConfigurator client
	g.conn = conn
	g.client = seti.NewGPUConfiguratorServiceClient(conn)
	return nil
}

func (g *GrpcClient) Health(ctx context.Context) (*seti.GetHealthResponse, error) {
	// Create a request and call the gRPC service
	req := &seti.GetHealthRequest{}
	res, err := g.client.GetHealth(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *GrpcClient) GetAvailableGPUs(ctx context.Context) (*seti.GetAvailableGPUsResponse, error) {
	// Create a request and call the gRPC service
	req := &seti.GetAvailableGPUsRequest{}
	res, err := g.client.GetAvailableGPUs(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *GrpcClient) EnableMPS(ctx context.Context, deviceUUID string) (*seti.EnableMPSResponse, error) {
	// Create a request and call the gRPC service

	req := &seti.EnableMPSRequest{
		DeviceUuid: deviceUUID,
	}
	res, err := g.client.EnableMPS(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *GrpcClient) DisableMPS(ctx context.Context, deviceUUID string) (*seti.DisableMPSResponse, error) {
	// Create a request and call the gRPC service
	req := &seti.DisableMPSRequest{
		DeviceUuid: deviceUUID,
	}
	res, err := g.client.DisableMPS(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *GrpcClient) RequestVirtualGPU(ctx context.Context, req *seti.RequestVirtualGPURequest) (*seti.RequestVirtualGPUResponse, error) {
	// Call the gRPC service
	res, err := g.client.RequestVirtualGPU(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (g *GrpcClient) ReleaseVirtualGPU(ctx context.Context, req *seti.ReleaseVirtualGPURequest) (*seti.ReleaseVirtualGPUResponse, error) {
	// Call the gRPC service
	res, err := g.client.ReleaseVirtualGPU(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}
func (g *GrpcClient) UpdateMPSConfigs(ctx context.Context, req *seti.UpdateMPSConfigsRequest) (*seti.UpdateMPSConfigsResponse, error) {

	res, err := g.client.UpdateMPSConfigs(ctx, req)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (g *GrpcClient) Close() {
	// Close the connection when done
	if g.conn != nil {
		g.conn.Close()
	}
}

func NewGrpcClient() *GrpcClient {

	return &GrpcClient{}
}
