package fastconfigurator

import (
	"context"
	"fmt"
	"log"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
)

// GetAvailableGPUs returns a list of available GPUs
func (s *ConfiguratorServer) GetAvailableGPUs(ctx context.Context, in *seti.GetAvailableGPUsRequest) (*seti.GetAvailableGPUsResponse, error) {
	log.Printf("Received request for available GPUs: %v", in)
	// TODO: Implement actual GPU detection logic

	//sample gpus

	virtuals := s.manager.getAvailableVirtualResources()

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

// RequestVirtualGPU allocates a virtual GPU with the specified requirements
func (s *ConfiguratorServer) RequestVirtualGPU(ctx context.Context, in *seti.RequestVirtualGPURequest) (*seti.RequestVirtualGPUResponse, error) {
	log.Printf("Received request for virtual GPU: %v", in)

	var vgpu *VirtualGPU
	var err error

	if in.Profileid != nil {
		// Find a virtual GPU with the specified profile ID
		vgpu, err = s.manager.FindVirtualGPU(*in.Profileid)
	} else if in.DeviceUuid != nil {
		// Get a specific virtual GPU by UUID
		vgpu, err = s.manager.GetVirtualGPU(*in.DeviceUuid)
	} else {
		return nil, fmt.Errorf("no profile ID or device UUID provided")

	}

	if vgpu == nil {
		err = fmt.Errorf("no available virtual GPUs")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to find suitable virtual GPU: %w", err)
	}

	if vgpu == nil {
		return nil, fmt.Errorf("no suitable virtual GPU available")
	}

	// Mark as in use
	vgpu.InUse = true

	// If MIG is requested, provision it
	if in.Profileid != nil && !vgpu.IsProvisioned && vgpu.Mig != nil {
		if err := s.manager.Access(vgpu); err != nil {
			return nil, fmt.Errorf("failed to provision virtual GPU: %w", err)
		}
	}

	// If MPS is requested and supported on this GPU
	if in.UseMps && vgpu.ProvisionedGPU != nil {
		// Try to start MPS daemon
		mpsServer := vgpu.ProvisionedGPU.mpsServer

		err := mpsServer.StartMPSDaemon()
		if err != nil {
			log.Printf("Warning: Failed to start MPS daemon: %v", err)
		}

	}

	// Build the response
	response := &seti.RequestVirtualGPUResponse{
		Id:                  vgpu.ID,
		DeviceIndex:         int32(vgpu.DeviceIndex),
		MemoryBytes:         vgpu.MemoryBytes,
		MultiprocessorCount: int32(vgpu.MultiProcessorCount),
		IsProvisioned:       vgpu.IsProvisioned,
		InUse:               vgpu.InUse,
	}

	// Add provisioned GPU info if available
	if vgpu.ProvisionedGPU != nil {
		response.ProvisionedGpu = &seti.GPU{
			Uuid:                vgpu.ProvisionedGPU.UUID,
			Name:                vgpu.ProvisionedGPU.Name,
			MemoryBytes:         vgpu.ProvisionedGPU.MemoryBytes,
			MultiprocessorCount: int32(vgpu.ProvisionedGPU.MultiProcessorCount),
			ParentDeviceIndex:   int32(vgpu.ProvisionedGPU.ParentDeviceIndex),
			ParentUuid:          vgpu.ProvisionedGPU.ParentUUID,
			MpsEnabled:          vgpu.ProvisionedGPU.mpsServer.isEnabled,
		}
	}

	// Get the current available GPUs and add to response
	availableGPUs := s.manager.getAvailableVirtualResources()

	for _, vg := range availableGPUs {
		if !vg.InUse {
			var provisionedGPU *seti.GPU
			if vg.ProvisionedGPU != nil {
				provisionedGPU = &seti.GPU{
					Uuid:                vg.ProvisionedGPU.UUID,
					Name:                vg.ProvisionedGPU.Name,
					MemoryBytes:         vg.ProvisionedGPU.MemoryBytes,
					MultiprocessorCount: int32(vg.ProvisionedGPU.MultiProcessorCount),
					ParentDeviceIndex:   int32(vg.ProvisionedGPU.ParentDeviceIndex),
					ParentUuid:          vg.ProvisionedGPU.ParentUUID,
					MpsEnabled:          vg.ProvisionedGPU.mpsServer.isEnabled,
				}
			}

			vGPU := &seti.VirtualGPU{
				Id:                  vg.ID,
				DeviceIndex:         int32(vg.DeviceIndex),
				MemoryBytes:         vg.MemoryBytes,
				MultiprocessorCount: int32(vg.MultiProcessorCount),
				IsProvisioned:       vg.IsProvisioned,

				ProvisionedGpu: provisionedGPU,
			}
			response.AvailableVirtualGpus = append(response.AvailableVirtualGpus, vGPU)
		}
	}

	return response, nil
}

// ReleaseVirtualGPU releases a previously allocated virtual GPU
func (s *ConfiguratorServer) ReleaseVirtualGPU(ctx context.Context, in *seti.ReleaseVirtualGPURequest) (*seti.ReleaseVirtualGPUResponse, error) {
	log.Printf("Received request to release virtual GPU: %v", in)

	// Find the virtual GPU by ID

	s.manager.FindVirtualGPU(in.Id)
	for _, vgpu := range s.manager.VirtualGPUs {
		if vgpu.ID == in.Id {

			// If provisioned using MIG, release resources
			if vgpu.IsProvisioned && !vgpu.IsProvisioned && vgpu.Mig != nil {
				if err := vgpu.Release(); err != nil {
					return &seti.ReleaseVirtualGPUResponse{
						Success: false,
						Message: fmt.Sprintf("Failed to release virtual GPU: %v", err),
					}, nil
				}
			}

			return &seti.ReleaseVirtualGPUResponse{
				Success: true,
				Message: fmt.Sprintf("Successfully released virtual GPU %s", in.Id),
			}, nil
		}
	}

	return &seti.ReleaseVirtualGPUResponse{
		Success: false,
		Message: fmt.Sprintf("Virtual GPU with ID %s not found", in.Id),
	}, nil
}

// EnableMPS enables MPS for a specific GPU device
func (s *ConfiguratorServer) EnableMPS(ctx context.Context, in *seti.EnableMPSRequest) (*seti.EnableMPSResponse, error) {
	log.Printf("Received request to enable MPS for device: %s", in.DeviceUuid)

	// Find the GPU by UUID
	for _, vgpu := range s.manager.VirtualGPUs {
		if vgpu.ProvisionedGPU != nil && vgpu.ProvisionedGPU.UUID == in.DeviceUuid {
			// Create MPS server if not already created
			if vgpu.ProvisionedGPU.mpsServer.UUID == "" {
				mpsServer, err := NewMPSServer(vgpu.ProvisionedGPU.Name, vgpu.ProvisionedGPU.UUID)
				if err != nil {
					return &seti.EnableMPSResponse{
						Success: false,
						Message: fmt.Sprintf("Failed to create MPS server: %v", err),
					}, nil
				}
				vgpu.ProvisionedGPU.mpsServer = *mpsServer
			}

			// Start MPS daemon if not already running
			if !vgpu.ProvisionedGPU.mpsServer.isEnabled {
				err := vgpu.ProvisionedGPU.mpsServer.StartMPSDaemon()
				if err != nil {
					return &seti.EnableMPSResponse{
						Success: false,
						Message: fmt.Sprintf("Failed to start MPS daemon: %v", err),
					}, nil
				}
				vgpu.ProvisionedGPU.mpsServer.isEnabled = true
			}

			return &seti.EnableMPSResponse{
				Success: true,
				Message: fmt.Sprintf("Successfully enabled MPS for device %s", in.DeviceUuid),
			}, nil
		}
	}

	return &seti.EnableMPSResponse{
		Success: false,
		Message: fmt.Sprintf("Device with UUID %s not found", in.DeviceUuid),
	}, nil
}

// DisableMPS disables MPS for a specific GPU device
func (s *ConfiguratorServer) DisableMPS(ctx context.Context, in *seti.DisableMPSRequest) (*seti.DisableMPSResponse, error) {
	log.Printf("Received request to disable MPS for device: %s", in.DeviceUuid)

	// Find the GPU by UUID
	for _, vgpu := range s.manager.VirtualGPUs {
		if vgpu.ProvisionedGPU != nil && vgpu.ProvisionedGPU.UUID == in.DeviceUuid {
			// Stop MPS daemon if running
			if vgpu.ProvisionedGPU.mpsServer.isEnabled {
				err := vgpu.ProvisionedGPU.mpsServer.StopMPSDaemon()
				if err != nil {
					return &seti.DisableMPSResponse{
						Success: false,
						Message: fmt.Sprintf("Failed to stop MPS daemon: %v", err),
					}, nil
				}
				vgpu.ProvisionedGPU.mpsServer.isEnabled = false
			}

			return &seti.DisableMPSResponse{
				Success: true,
				Message: fmt.Sprintf("Successfully disabled MPS for device %s", in.DeviceUuid),
			}, nil
		}
	}

	return &seti.DisableMPSResponse{
		Success: false,
		Message: fmt.Sprintf("Device with UUID %s not found", in.DeviceUuid),
	}, nil
}

// UpdateMPSConfigs updates MPS configurations for a device with FastPod GPU configurations
func (s *ConfiguratorServer) UpdateMPSConfigs(ctx context.Context, in *seti.UpdateMPSConfigsRequest) (*seti.UpdateMPSConfigsResponse, error) {
	log.Printf("Received request to update MPS configs for device: %s with %d FastPod configs",
		in.DeviceUuid, len(in.FastpodGpuConfigs))

	_, ok := s.manager.provisionedGPUs[in.DeviceUuid]
	if !ok {
		return nil, fmt.Errorf("device with UUID %s not found", in.DeviceUuid)
	}

	// Convert the incoming FastPod GPU configurations to PodGPURequest format
	podGPURequests := make([]types.PodGPURequest, 0, len(in.FastpodGpuConfigs))
	for _, config := range in.FastpodGpuConfigs {
		podGPURequests = append(podGPURequests, types.PodGPURequest{

			Key:           config.Key,
			QtRequest:     config.QtRequest,
			QtLimit:       config.QtLimit,
			SMPartition:   config.SmPartition,
			Memory:        config.Memory,
			GPUClientPort: int(config.GpuClientPort),
		})
	}

	// Create and process the update message
	updateMsg := types.UpdatePodsGPUConfigMessage{
		GpuUUID:        in.DeviceUuid,
		PodGPURequests: podGPURequests,
	}

	// Call the handler that writes configs to the appropriate files
	handleMsg(updateMsg)

	// Always return success since update is best-effort
	return &seti.UpdateMPSConfigsResponse{}, nil
}
