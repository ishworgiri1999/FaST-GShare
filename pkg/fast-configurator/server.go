package fastconfigurator

import (
	"fmt"
	"log"
	"net"

	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
	"google.golang.org/grpc"
)

type ConfigurationServerParams struct{}

// ConfiguratorServer implements the GPU configurator service
type ConfiguratorServer struct {
	seti.UnimplementedGPUConfiguratorServiceServer
	manager *ResourceManager
}

// Server represents the gRPC server instance
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
}

func NewServer(port string) (*Server, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	grpcServer := grpc.NewServer()

	server := &Server{
		grpcServer: grpcServer,
		listener:   listener,
	}

	configServer, err := NewConfigurationServer(ConfigurationServerParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration server: %w", err)
	}
	// Register the ConfiguratorServer with the gRPC server
	seti.RegisterGPUConfiguratorServiceServer(grpcServer, configServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("gRPC server error: %v", err)
		}
	}()

	return server, nil
}

// NewServer creates and starts a new gRPC server
func NewConfigurationServer(params ConfigurationServerParams) (*ConfiguratorServer, error) {

	rm, err := NewResourceManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create resource manager: %w", err)
	}
	if rm == nil {
		return nil, fmt.Errorf("failed to create resource manager")
	}

	server := &ConfiguratorServer{
		manager: rm,
	}

	return server, nil
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() {
	if s.grpcServer != nil {
		log.Println("Stopping gRPC server...")
		s.grpcServer.GracefulStop()
		log.Println("gRPC server stopped")
	}
}
