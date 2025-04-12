package fastconfigurator

import (
	"context"
	"log"

	"github.com/KontonGu/FaST-GShare/proto/seti"
)

// GetAvailableGPUs returns a list of available GPUs
func (s *ConfiguratorServer) GetAvailableGPUs(ctx context.Context, in *seti.GetGPUsRequest) (*seti.GetGPUsResponse, error) {
	log.Printf("Received request for available GPUs: %v", in)
	// TODO: Implement actual GPU detection logic

	//sample gpus

	uuid, _ := s.manager.PhysicalGPUs[0].GetUUID()

	gpus := []*seti.VirtualGPU{
		&seti.VirtualGPU{
			Id:                  uuid,
			MemoryBytes:         1024,
			MultiprocessorCount: 4,
		},
		&seti.VirtualGPU{
			Id:                  "gpu-2",
			MemoryBytes:         2048,
			MultiprocessorCount: 8,
		},
	}
	return &seti.GetGPUsResponse{
		Gpus: gpus,
	}, nil
}
