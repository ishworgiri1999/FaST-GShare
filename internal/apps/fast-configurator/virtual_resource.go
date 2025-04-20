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
	Name                string
	ID                  string
	DeviceIndex         int // Index of the physical GPU that this virtual GPU can be created on
	MemoryBytes         uint64
	MultiProcessorCount int
	IsProvisioned       bool
	InUse               bool
	Mig                 *MIGProperties
	GPUInstance         *mig.GpuInstance
	ComputeInstance     *mig.ComputeInstance
	ProvisionedGPU      *GPU
	mutex               sync.Mutex
}

type MIGProperties struct {
	GPUInstance     *nvml.GpuInstance
	ComputeInstance *nvml.ComputeInstance
	profile         nvml.GpuInstanceProfileInfo
}

type GPU struct {
	UUID                string
	Name                string
	MemoryBytes         uint64
	MultiProcessorCount int
	ParentDeviceIndex   int
	ParentUUID          string
	ProvisionedDevice   nvml.Device
	mpsServer           MPSServer
}

// ResourceManager manages virtual GPU resources
type ResourceManager struct {
	mig  mig.Interface
	nvml nvml.Interface

	PhysicalGPUs []mig.Device
	VirtualGPUs  []*VirtualGPU //available ones
	mutex        sync.Mutex

	provisionedGPUs map[string]*VirtualGPU
}

// NewResourceManager initializes NVML and discovers available GPUs
func NewResourceManager() (*ResourceManager, error) {
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", ret)
	}

	rm := &ResourceManager{
		mig:             mig.New(),
		nvml:            nvml.New(),
		provisionedGPUs: make(map[string]*VirtualGPU),
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

		name, ret := device.GetName()
		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get name for GPU %d: %v", i, ret)
			continue
		}

		//get device uuid, memory , multiprocessor count
		uuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get UUID for GPU %d: %v", i, ret)
			continue
		}

		memoo, ret := device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get memory info for GPU %d: %v", i, ret)
			continue
		}

		// Check MIG capability
		_, currentMode, ret := device.GetMigMode()

		if ret == nvml.ERROR_NOT_SUPPORTED {
			log.Printf("MIG not supported for GPU %d (%s). Creating a non-MIG virtual GPU.", i, name)

			smCount, err := GetSMCount(name)
			if err != nil {
				log.Printf("Warning: Could not get SM count for GPU %s: %v. Using default of 0.", name, err)
				smCount = 0 // Default to a reasonable value for modern GPUs
			}

			// Safely check if the GPU is already provisioned
			var isUsed bool
			if vGPU, exists := rm.provisionedGPUs[uuid]; exists && vGPU != nil {
				isUsed = vGPU.InUse
			}

			vGPU := &VirtualGPU{
				ID:                  fmt.Sprintf("vgpu-%d", i),
				DeviceIndex:         i,
				MemoryBytes:         memoo.Total,
				MultiProcessorCount: int(smCount),
				Mig:                 nil,
				IsProvisioned:       true,
				Name:                name,
				InUse:               isUsed,
				ProvisionedGPU: &GPU{
					UUID:                uuid,
					Name:                name,
					MemoryBytes:         memoo.Total,
					MultiProcessorCount: int(smCount),
					ParentDeviceIndex:   i,
					ParentUUID:          uuid,
					mpsServer: MPSServer{
						UUID:      uuid,
						Name:      name,
						isEnabled: false,
					},
					ProvisionedDevice: device.Device,
				},
			}

			rm.provisionedGPUs[uuid] = vGPU
			gpus = append(gpus, vGPU)
			continue
		}

		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get MIG mode for GPU %d: %v", i, ret)
			continue
		}

		if currentMode != nvml.DEVICE_MIG_ENABLE {
			log.Printf("Warning: MIG not enabled for GPU %d, skipping", i)
			continue
		}

		migCount, ret := device.GetMaxMigDeviceCount()
		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get MIG device count for GPU %d: %v", i, ret)
			continue
		}

		for j := 0; j < migCount; j++ {
			migDevice, ret := device.GetMigDeviceHandleByIndex(j)
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get MIG device handle for GPU %d, index %d: %v", i, j, ret)
				continue
			}
			migDeviceName, ret := migDevice.GetName()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get MIG device name for GPU %d, index %d: %v", i, j, ret)
				continue
			}
			migDeviceUUID, ret := migDevice.GetUUID()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get MIG device UUID for GPU %d, index %d: %v", i, j, ret)
				continue
			}
			migDeviceMemory, ret := migDevice.GetMemoryInfo()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get MIG device memory info for GPU %d, index %d: %v", i, j, ret)
				continue
			}

			atters, ret := migDevice.GetAttributes()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get attributes for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			//get compute instance

			computeInstanceID, ret := migDevice.GetComputeInstanceId()

			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get compute instance ID for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			gpuInstanceID, ret := migDevice.GetGpuInstanceId()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get GPU instance ID for MIG device %d, index %d: %v", i, j, ret)
				continue
			}
			gpuInstance, ret := device.GetGpuInstanceById(gpuInstanceID)

			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get compute instance for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			info, ret := gpuInstance.GetInfo()
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get GPU instance info for MIG device %d, index %d: %v", i, j, ret)
				continue
			}
			profile, ret := device.GetGpuInstanceProfileInfo(int(info.ProfileId))
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get GPU instance profile info for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			computeInstance, ret := gpuInstance.GetComputeInstanceById(computeInstanceID)
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get compute instance for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			old, ok := rm.provisionedGPUs[migDeviceUUID]

			vgpu := &VirtualGPU{
				Name:                migDeviceName,
				ID:                  fmt.Sprintf("vgpu-%d-mig-%d", i, j),
				MultiProcessorCount: int(atters.MultiprocessorCount),
				MemoryBytes:         migDeviceMemory.Total,
				Mig: &MIGProperties{
					GPUInstance:     &gpuInstance,
					profile:         profile,
					ComputeInstance: &computeInstance,
				},
				IsProvisioned: true,
				DeviceIndex:   i,
				ProvisionedGPU: &GPU{
					UUID:                migDeviceUUID,
					Name:                migDeviceName,
					MemoryBytes:         migDeviceMemory.Total,
					MultiProcessorCount: int(atters.MultiprocessorCount),
					ParentDeviceIndex:   i,
					ParentUUID:          uuid,
					ProvisionedDevice:   migDevice,
					mpsServer: MPSServer{
						UUID:      migDeviceUUID,
						Name:      migDeviceName,
						isEnabled: ok && old.ProvisionedGPU.mpsServer.isEnabled,
					},
				},
			}
			gpus = append(gpus, vgpu)

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
				log.Printf("Error getting GPU instance remaining capacity for profile %d on device %d: %v",
					profile.Id, i, ret.String())
				continue
			}

			for j := 0; j < count; j++ {
				vGPU := &VirtualGPU{
					Name:                fmt.Sprintf("%s-mig-profile-%d", name, profile.Id),
					ID:                  fmt.Sprintf("vgpu-%d-profile%d-%d", i, profile.Id, j),
					DeviceIndex:         i,
					MemoryBytes:         profile.MemorySizeMB * 1024 * 1024,
					MultiProcessorCount: int(profile.MultiprocessorCount),
					Mig: &MIGProperties{
						profile: *profile,
					},
					IsProvisioned: false,
				}
				gpus = append(gpus, vGPU)
			}

		}
	}
	return gpus
}

func (rm *ResourceManager) GetVirtualGPU(deviceUUID string) (*VirtualGPU, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	vGPU, ok := rm.provisionedGPUs[deviceUUID]
	if ok {
		return vGPU, nil
	}

	return nil, fmt.Errorf("virtual GPU with uuid %s not found", deviceUUID)
}

func (rm *ResourceManager) FindVirtualGPU(profileID string) (*VirtualGPU, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, vGPU := range rm.VirtualGPUs {
		// Skip if already provisioned
		if vGPU.IsProvisioned {
			continue
		}
		// Check if this virtual GPU meets requirements
		if vGPU.ID == profileID {
			return vGPU, nil
		}
	}

	return nil, fmt.Errorf("virtual GPU with prolfile id %s not found", profileID)
}

// Access provisions the GPU instance and compute instance if not already done
func (rm *ResourceManager) Access(v *VirtualGPU) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if v.IsProvisioned {
		return nil
	}

	// Get physical device handle
	device, ret := nvml.DeviceGetHandleByIndex(v.DeviceIndex)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to get device handle for GPU %d: %v", v.DeviceIndex, ret)
	}
	// Create GPU instance
	gpuInstance, ret := device.CreateGpuInstance(&v.Mig.profile)
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to create GPU instance: %v", ret.String())
	}
	instance := rm.mig.GpuInstance(gpuInstance)
	v.GPUInstance = &instance

	// Select profile that uses all SMs in the GPU instance
	ciProfile, err := instance.GetFullUsageComputeInstanceProfile(&v.Mig.profile)

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

	ciInfo, ret := computeInstance.GetInfo()
	if ret != nvml.SUCCESS {
		gpuInstance.Destroy()
		computeInstance.Destroy()
		return fmt.Errorf("failed to get compute instance info: %v", ret)
	}

	ciDevice := ciInfo.Device

	//name
	newDeviceName, ret := ciDevice.GetName()
	if ret != nvml.SUCCESS {
		gpuInstance.Destroy()
		computeInstance.Destroy()
		return fmt.Errorf("failed to get compute instance name: %v", ret)
	}

	//uuid
	newDeviceUUID, ret := ciDevice.GetUUID()
	if ret != nvml.SUCCESS {
		gpuInstance.Destroy()
		computeInstance.Destroy()
		return fmt.Errorf("failed to get compute instance uuid: %v", ret)
	}

	parentUUID, ret := device.GetUUID()
	if ret != nvml.SUCCESS {
		gpuInstance.Destroy()
		computeInstance.Destroy()
		return fmt.Errorf("failed to get parent device uuid: %v", ret)
	}
	//memory
	memoo, ret := ciDevice.GetMemoryInfo()
	if ret != nvml.SUCCESS {
		gpuInstance.Destroy()
		computeInstance.Destroy()
		return fmt.Errorf("failed to get compute instance memory info: %v", ret)
	}

	v.ProvisionedGPU = &GPU{
		UUID:                newDeviceUUID,
		Name:                newDeviceName,
		MemoryBytes:         memoo.Total,
		MultiProcessorCount: v.MultiProcessorCount,
		ParentDeviceIndex:   v.DeviceIndex,
		ParentUUID:          parentUUID,
		ProvisionedDevice:   ciDevice,
		mpsServer: MPSServer{
			UUID: newDeviceUUID,
			Name: newDeviceName,
		},
	}

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
	v.ProvisionedGPU = nil

	v.InUse = false
	return nil
}
