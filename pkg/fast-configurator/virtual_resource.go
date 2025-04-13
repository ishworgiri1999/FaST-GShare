package fastconfigurator

import (
	"fmt"
	"log"
	"sync"

	"github.com/KontonGu/FaST-GShare/pkg/mig"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// VirtualGPU represents a virtual GPU resource that exists logically
// but is only physically created when accessed
type VirtualGPU struct {
	ID                  string
	DeviceIndex         int // Index of the physical GPU that this virtual GPU is created on
	MemoryBytes         uint64
	MultiProcessorCount int
	IsProvisioned       bool
	InUse               bool
	profile             *nvml.GpuInstanceProfileInfo
	GPUInstance         *mig.GpuInstance
	ComputeInstance     *mig.ComputeInstance
	ProvisionedDevice   *nvml.Device

	ProvisionedGPU *GPU
	mutex          sync.Mutex
}

type GPU struct {
	UUID                string
	Name                string
	MemoryBytes         uint64
	MultiProcessorCount int
	ParentDeviceIndex   int
	ParentUUID          string
}

// ResourceManager manages virtual GPU resources
type ResourceManager struct {
	mig  mig.Interface
	nvml nvml.Interface

	PhysicalGPUs []mig.Device
	VirtualGPUs  []*VirtualGPU
	mutex        sync.Mutex
}

// NewResourceManager initializes NVML and discovers available GPUs
func NewResourceManager() (*ResourceManager, error) {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", ret)
	}

	rm := &ResourceManager{
		mig:  mig.New(),
		nvml: nvml.New(),
	}

	// Discover physical GPUs
	count, ret := rm.nvml.DeviceGetCount()

	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to get device count: %v", ret)
	}

	rm.PhysicalGPUs = make([]mig.Device, count)
	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("failed to get device handle for GPU %d: %v", i, ret)
		}
		rm.PhysicalGPUs[i] = rm.mig.Device(device)
	}

	// Initialize virtual resources
	gpus := rm.getAvailableVirtualResources()
	rm.VirtualGPUs = gpus

	return rm, nil
}

// discoverVirtualResources creates virtual GPU resources based on physical GPU capabilities
func (rm *ResourceManager) getAvailableVirtualResources() []*VirtualGPU {
	var gpus []*VirtualGPU
	for i, device := range rm.PhysicalGPUs {
		// Check MIG capability
		_, currentMode, ret := device.GetMigMode()

		if ret == nvml.ERROR_NOT_SUPPORTED {
			log.Printf("Warning: MIG not supported for GPU %d, skipping", i)

			//get device uuid, memory , multiprocessor count
			uuid, ret := device.GetUUID()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get UUID for GPU %d: %v", i, ret)
				continue
			}
			name, ret := device.GetName()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get name for GPU %d: %v", i, ret)
				continue
			}
			memoo, ret := device.GetMemoryInfo()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get memory info for GPU %d: %v", i, ret)
				continue
			}
			// Get SM count using the pci info
			// pciInfo, ret := device.GetPciInfo()
			// if ret != nvml.SUCCESS {
			// 	log.Printf("Failed to get PCI info for device %d: %v", i, nvml.ErrorString(ret))
			// 	continue
			// }

			// // Get CUDA compute capability
			// major, minor, ret := device.GetCudaComputeCapability()
			// if ret != nvml.SUCCESS {
			// 	log.Printf("Failed to get compute capability for device %d: %v", i, nvml.ErrorString(ret))
			// 	continue
			// }

			// fmt.Printf("Device %d: %s (PCI: %s)\n", i, name, pciInfo.BusId)
			// fmt.Printf("  Compute Capability: %d.%d\n", major, minor)

			// // Get the raw value that contains SM count
			// value, ret := device.GetFieldValue(nvml.NVML_FI_DEV_CUDA_COMPUTE_CAPABILITY)
			// if ret != nvml.SUCCESS {
			// 	log.Printf("Failed to get field value for device %d: %v", i, nvml.ErrorString(ret))
			// 	continue
			// }

			// // The SM count is stored in bits 16-31 of this value
			// smCount := (value >> 16) & 0xFFFF
			// fmt.Printf("  SM Count: %d\n", smCount)

			//get multiprocessor count
			// mpc, ret := device.M()
			// Create a virtual GPU for the physical GPU

			vGPU := &VirtualGPU{
				ID:                  fmt.Sprintf("vgpu-%d", i),
				DeviceIndex:         i,
				MemoryBytes:         memoo.Total,
				MultiProcessorCount: 0,
				profile:             nil,
				IsProvisioned:       true,
				ProvisionedDevice:   &device.Device,
				ProvisionedGPU:      &GPU{UUID: uuid, Name: name, MemoryBytes: memoo.Total, MultiProcessorCount: 0},
			}
			gpus = append(gpus, vGPU)

		}

		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get MIG mode for GPU %d: %v", i, ret)
			continue
		}

		if currentMode != nvml.DEVICE_MIG_ENABLE {
			log.Printf("Warning: MIG not enabled for GPU %d, skipping", i)
			continue
		}

		// Get GPU profiles supported by this device
		profiles, err := device.GetPossiblGPUInstanceeProfiles()
		if err != nil {
			log.Printf("Error getting GPU profiles for device %d: %v", i, err)
			continue
		}

		// Create virtual GPUs for each profile
		for _, profile := range profiles {
			count, ret := device.GetGpuInstanceRemainingCapacity(profile)
			if ret != nvml.SUCCESS {
				log.Printf("Error getting GPU instance remaining capacity for device %d: %v", i, err)
				continue
			}

			for j := 0; j < count; j++ {
				vGPU := &VirtualGPU{
					ID:                  fmt.Sprintf("vgpu-%d-profile%d-%d", i, profile.Id, j),
					DeviceIndex:         i,
					MemoryBytes:         profile.MemorySizeMB * 1024 * 1024,
					MultiProcessorCount: int(profile.MultiprocessorCount),
					profile:             profile,
					IsProvisioned:       false,
				}
				gpus = append(gpus, vGPU)
			}
			//get gpu instance if available
			if ret != nvml.SUCCESS {
				log.Printf("Error getting GPU instance for device %d: %v", i, err)
				continue
			}

		}
	}
	return gpus
}

// GetVirtualGPU returns a virtual GPU that meets the requirements
func (rm *ResourceManager) GetVirtualGPU(requiredMemoryBytes uint64, requiredMultiProcessorCount int) (*VirtualGPU, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, vGPU := range rm.VirtualGPUs {
		// Skip if already provisioned
		if vGPU.IsProvisioned {
			continue
		}

		// Check if this virtual GPU meets requirements
		if vGPU.MemoryBytes >= requiredMemoryBytes && vGPU.MultiProcessorCount >= requiredMultiProcessorCount {
			return vGPU, nil
		}
	}

	return nil, fmt.Errorf("no suitable virtual GPU available")
}

// Access provisions the GPU instance and compute instance if not already done
func (rm *ResourceManager) Access(v *VirtualGPU) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.IsProvisioned {
		return nil // Already provisioned
	}

	// Get physical device handle
	device, ret := nvml.DeviceGetHandleByIndex(v.DeviceIndex)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device handle for GPU %d: %v", v.DeviceIndex, ret)
	}
	// Create GPU instance
	gpuInstance, ret := device.CreateGpuInstance(v.profile)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to create GPU instance: %v", ret.String())
	}
	instance := rm.mig.GpuInstance(gpuInstance)
	v.GPUInstance = &instance

	// Select profile that uses all SMs in the GPU instance
	ciProfile, err := instance.GetFullUsageComputeInstanceProfile(v.profile)

	if err != nil {
		// Cleanup on failure
		ret := gpuInstance.Destroy()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("failed to destroy GPU instance: on failure to get compute instance profiles: %v", err)
		}
		return fmt.Errorf("failed to get compute instance profiles: %v", err)
	}

	// Create compute instance
	computeInstance, ret := gpuInstance.CreateComputeInstance(ciProfile)

	if ret != nvml.SUCCESS {
		// Cleanup on failure
		gpuInstance.Destroy()
		return fmt.Errorf("failed to create compute instance: %v", err)
	}

	ci := rm.mig.ComputeInstance(computeInstance)

	v.ComputeInstance = &ci
	v.IsProvisioned = true

	return nil
}

// Release destroys the GPU instance and compute instance
func (v *VirtualGPU) Release() error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if !v.IsProvisioned {
		return nil // Nothing to do
	}

	// Destroy compute instance first
	if v.ComputeInstance != nil {
		if ret := v.ComputeInstance.Destroy(); ret != nvml.SUCCESS {
			return fmt.Errorf("failed to destroy compute instance: %v", ret)
		}
		v.ComputeInstance = nil
	}

	// Then destroy GPU instance
	if v.GPUInstance != nil {
		if ret := v.GPUInstance.Destroy(); ret != nvml.SUCCESS {
			return fmt.Errorf("failed to destroy GPU instance: %v", ret)
		}
		v.GPUInstance = nil
	}

	v.IsProvisioned = false
	return nil
}
