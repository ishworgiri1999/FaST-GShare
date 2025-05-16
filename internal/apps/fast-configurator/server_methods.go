package fastconfigurator

import (
	"context"
	"fmt"
	"log"

	"github.com/KontonGu/FaST-GShare/pkg/proto/seti/v1"
	"github.com/KontonGu/FaST-GShare/pkg/types"
)

// Mapper function: normal GPU to seti.GPU
func mapToSetiGPU(gpu *GPU) *seti.GPU {
	if gpu == nil {
		return nil
	}
	return &seti.GPU{
		Uuid:                gpu.UUID,
		Name:                gpu.Name,
		MemoryBytes:         gpu.MemoryBytes,
		MultiprocessorCount: int32(gpu.MultiProcessorCount),
		ParentDeviceIndex:   int32(gpu.ParentDeviceIndex),
		ParentUuid:          gpu.ParentUUID,
		MpsEnabled:          gpu.mpsServer.isEnabled,
		MpsConfig: &seti.MPSConfig{
			DeviceUuid: gpu.UUID,
			LogPath:    gpu.mpsServer.GetLogDir(),
			TmpPath:    gpu.mpsServer.GetPipeDir(),
		},
	}
}

// Mapper function: VirtualGPU to seti.VirtualGPU
func mapToSetiVirtualGPU(vgpu *VirtualGPU) *seti.VirtualGPU {
	if vgpu == nil {
		return nil
	}
	var provisionedGPU *seti.GPU
	if vgpu.ProvisionedGPU != nil {
		provisionedGPU = mapToSetiGPU(vgpu.ProvisionedGPU)
	}

	gpu := seti.VirtualGPU{
		Id:                  vgpu.ID,
		MemoryBytes:         vgpu.MemoryBytes,
		DeviceIndex:         int32(vgpu.DeviceIndex),
		MultiprocessorCount: int32(vgpu.MultiProcessorCount),
		IsProvisioned:       vgpu.IsProvisioned,
		PhysicalGpuType:     vgpu.PhysicalGPUType,
		SmPercentage:        int32(vgpu.SMPercentage),
		ProvisionedGpu:      provisionedGPU,
	}

	if vgpu.Mig != nil {
		gpu.Profileid = &vgpu.Mig.profile.Id
	}

	return &gpu
}

// GetAvailableGPUs returns a list of available GPUs
func (s *ConfiguratorServer) GetAvailableGPUs(ctx context.Context, in *seti.GetAvailableGPUsRequest) (*seti.GetAvailableGPUsResponse, error) {
	log.Printf("Received request for available GPUs: %v", in)
	// TODO: Implement actual GPU detection logic

	//sample gpus

	virtuals := s.manager.getAvailableVirtualResources()

	gpus := []*seti.VirtualGPU{}

	for _, vgpu := range virtuals {

		gpu := mapToSetiVirtualGPU(vgpu)
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

		err := mpsServer.SetupMPSEnvironment()
		if err != nil {
			log.Printf("Warning: Failed to setup MPS environment: %v", err)
		}

	}

	// Build the response
	response := &seti.RequestVirtualGPUResponse{
		Id:                  vgpu.ID,
		DeviceIndex:         int32(vgpu.DeviceIndex),
		MemoryBytes:         vgpu.MemoryBytes,
		MultiprocessorCount: int32(vgpu.MultiProcessorCount),
	}

	// Add provisioned GPU info if available
	if vgpu.ProvisionedGPU != nil {
		response.ProvisionedGpu = mapToSetiGPU(vgpu.ProvisionedGPU)
	}

	// Get the current available GPUs and add to response
	availableGPUs := s.manager.getAvailableVirtualResources()

	for _, vg := range availableGPUs {
		var provisionedGPU *seti.GPU
		if vg.ProvisionedGPU != nil {
			provisionedGPU = mapToSetiGPU(vg.ProvisionedGPU)
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

	return response, nil
}

// ReleaseVirtualGPU releases a previously allocated virtual GPU
func (s *ConfiguratorServer) ReleaseVirtualGPU(ctx context.Context, in *seti.ReleaseVirtualGPURequest) (*seti.ReleaseVirtualGPUResponse, error) {
	log.Printf("Received request to release virtual GPU: %v", in)

	// Find the virtual GPU by ID

	vgpu, err := s.manager.FindVirtualGPUUsingUUID(in.Uuid)
	if err != nil {
		return &seti.ReleaseVirtualGPUResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to find virtual GPU: %v", err),
		}, nil

	}
	// If provisioned using MIG, release resources
	if err := s.manager.Release(vgpu); err != nil {
		return &seti.ReleaseVirtualGPUResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to release virtual GPU: %v", err),
		}, nil
	}

	availabe := s.manager.getAvailableVirtualResources()

	var availableSetiVGpus []*seti.VirtualGPU
	for _, vgpu := range availabe {

		availableSetiVGpus = append(availableSetiVGpus, mapToSetiVirtualGPU(vgpu))
	}

	return &seti.ReleaseVirtualGPUResponse{
		Success:              true,
		Message:              fmt.Sprintf("Successfully released virtual GPU %s", in.Uuid),
		AvailableVirtualGpus: availableSetiVGpus,
	}, nil
}

// EnableMPS enables MPS for a specific GPU device
func (s *ConfiguratorServer) EnableMPS(ctx context.Context, in *seti.EnableMPSRequest) (*seti.EnableMPSResponse, error) {
	log.Printf("Received request to enable MPS for device: %s", in.DeviceUuid)

	// Find the GPU by UUID
	vgpu := s.manager.provisionedGPUs[in.DeviceUuid]
	if vgpu == nil {
		return &seti.EnableMPSResponse{
			Success: false,
			Message: fmt.Sprintf("Device with UUID %s not found", in.DeviceUuid),
		}, nil
	}

	// Start MPS daemon if not already running
	if !vgpu.ProvisionedGPU.mpsServer.isEnabled {
		err := vgpu.ProvisionedGPU.mpsServer.SetupMPSEnvironment()
		if err != nil {
			return &seti.EnableMPSResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to start MPS daemon: %v", err),
			}, err
		}
		vgpu.ProvisionedGPU.mpsServer.isEnabled = true
	}

	return &seti.EnableMPSResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully enabled MPS for device %s", in.DeviceUuid),
		MpsConfig: &seti.MPSConfig{
			DeviceUuid: in.DeviceUuid,
			LogPath:    vgpu.ProvisionedGPU.mpsServer.GetLogDir(),
			TmpPath:    vgpu.ProvisionedGPU.mpsServer.GetPipeDir(),
		},
	}, nil

}

// DisableMPS disables MPS for a specific GPU device
func (s *ConfiguratorServer) DisableMPS(ctx context.Context, in *seti.DisableMPSRequest) (*seti.DisableMPSResponse, error) {
	log.Printf("Received request to disable MPS for device: %s", in.DeviceUuid)

	vgpu := s.manager.provisionedGPUs[in.DeviceUuid]
	if vgpu == nil {
		return &seti.DisableMPSResponse{
			Success: false,
			Message: fmt.Sprintf("Device with UUID %s not found", in.DeviceUuid),
		}, nil
	}

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

	if err := vgpu.ProvisionedGPU.mpsServer.CleanupDirectories(); err != nil {
		log.Printf("Warning: failed to clean up MPS directories: %v", err)
	}

	return &seti.DisableMPSResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully disabled MPS for device %s", in.DeviceUuid),
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

func (s *ConfiguratorServer) GetGPU(ctx context.Context, in *seti.GetGPURequest) (*seti.GetGPUResponse, error) {

	gpu, err := s.manager.GetVirtualGPU(in.Uuid)
	if err != nil {
		return nil, fmt.Errorf("failed to get GPU: %w", err)
	}

	return &seti.GetGPUResponse{ProvisionedGpu: mapToSetiVirtualGPU(gpu)}, nil
}
