package fastconfigurator

import (
	"context"
	"log"

	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
)

// GetAvailableGPUs returns a list of available GPUs
func (s *ConfiguratorServer) GetAvailableGPUs(ctx context.Context, in *seti.GetAvailableGPUsRequest) (*seti.GetAvailableGPUsResponse, error) {
	log.Printf("Received request for available GPUs: %v", in)
	// TODO: Implement actual GPU detection logic

	//sample gpus

	virtuals := s.manager.VirtualGPUs

	gpus := []*seti.VirtualGPU{}

	for _, vgpu := range virtuals {
		var provisionedGPU *seti.GPU
		if vgpu.ProvisionedGPU != nil {
			provisionedGPU = &seti.GPU{
				Uuid:                vgpu.ProvisionedGPU.UUID,
				Name:                vgpu.ProvisionedGPU.Name,
				MemoryBytes:         vgpu.ProvisionedGPU.MemoryBytes,
				MultiprocessorCount: int32(vgpu.ProvisionedGPU.MultiProcessorCount),
				ParentDeviceIndex:   int32(vgpu.ProvisionedGPU.ParentDeviceIndex),
				ParentUuid:          vgpu.ProvisionedGPU.ParentUUID,
			}
		}

		gpu := &seti.VirtualGPU{
			Id:                  vgpu.ID,
			MemoryBytes:         vgpu.MemoryBytes,
			DeviceIndex:         int32(vgpu.DeviceIndex),
			MultiprocessorCount: int32(vgpu.MultiProcessorCount),
			IsProvisioned:       vgpu.IsProvisioned,
			InUse:               vgpu.InUse,
			ProvisionedGpu:      provisionedGPU,
		}
		gpus = append(gpus, gpu)
	}
	return &seti.GetAvailableGPUsResponse{
		Gpus: gpus,
	}, nil
}

func (s *ConfiguratorServer) GetHealth(ctx context.Context, in *seti.GetHealthRequest) (*seti.GetHealthResponse, error) {
	return &seti.GetHealthResponse{
		Healthy: true,
	}, nil
}
