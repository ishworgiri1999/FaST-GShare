package fastconfigurator

import (
	"fmt"
	"log"
	"sync"

	"github.com/KontonGu/FaST-GShare/pkg/mig"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"k8s.io/klog/v2"
)

// VirtualGPU represents a virtual GPU resource that exists logically
// but is only physically created when accessed
type VirtualGPU struct {
	Name                string
	ID                  string //not unique
	DeviceIndex         int    // Index of the physical GPU that this virtual GPU can be created on
	MemoryBytes         uint64
	MultiProcessorCount int
	IsProvisioned       bool
	Mig                 *MIGProperties
	ProvisionedGPU      *GPU
	Physical            bool
	PhysicalGPUType     string // Name of the physical GPU
	SMPercentage        int    //100 for physical
	mutex               sync.Mutex
}

func (v *VirtualGPU) String() string {
	return fmt.Sprintf(
		"VirtualGPU{\n"+
			"  Name: %s,\n"+
			"  ID: %s,\n"+
			"  DeviceIndex: %d,\n"+
			"  MemoryBytes: %d,\n"+
			"  MultiProcessorCount: %d,\n"+
			"  IsProvisioned: %t,\n"+
			"  Physical: %t,\n"+
			"  PhysicalGPUType: %s,\n"+
			"  SMPercentage: %d,\n"+
			"  Mig: %v,\n"+
			"  ProvisionedGPU: %v,\n"+
			"}",
		v.Name, v.ID, v.DeviceIndex, v.MemoryBytes, v.MultiProcessorCount, v.IsProvisioned, v.Physical, v.PhysicalGPUType, v.SMPercentage, v.Mig, v.ProvisionedGPU)
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
	klog.Infof("Found %d virtual GPUs", len(gpus))
	klog.Infof("Found %d physical GPUs", len(rm.PhysicalGPUs))
	klog.Infof("Found %d provisioned GPUs", len(rm.provisionedGPUs))
	return rm, nil
}

func (rm *ResourceManager) CleanUp() {
	//clear all mps servers

	for _, vGPU := range rm.provisionedGPUs {
		if vGPU.ProvisionedGPU.mpsServer.isEnabled {
			err := vGPU.ProvisionedGPU.mpsServer.StopMPSDaemon()
			if err != nil {
				log.Printf("cleanup:Warning: Failed to stop MPS daemon: %v", err)
			}
		}
	}

	//destroy all virtual instances
	for _, vGPU := range rm.provisionedGPUs {
		if vGPU.IsProvisioned {
			err := rm.Release(vGPU)
			if err != nil {
				log.Printf("cleanup:Warning: Failed to release virtual GPU: %v", err)
			}
			klog.Info("cleanup: Released virtual GPU: ", vGPU.ID)
		}
	}
}

// discoverVirtualResources creates virtual GPU resources based on physical GPU capabilities
func (rm *ResourceManager) getAvailableVirtualResources() []*VirtualGPU {
	var gpus []*VirtualGPU
	for i, device := range rm.PhysicalGPUs {

		physicalGPUName, ret := device.GetName()
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
		currentMode, _, ret := device.GetMigMode()

		if ret == nvml.ERROR_NOT_SUPPORTED || (ret == nvml.SUCCESS && currentMode != nvml.DEVICE_MIG_ENABLE) {
			if ret == nvml.ERROR_NOT_SUPPORTED {
				log.Printf("MIG not supported for GPU %d (%s). Creating a non-MIG virtual GPU.", i, physicalGPUName)
			} else {
				log.Printf("MIG not enabled for GPU %d (%s). Creating a non-MIG virtual GPU.", i, physicalGPUName)
			}

			smCount, err := GetSMCount(physicalGPUName)
			if err != nil {
				log.Printf("Warning: Could not get SM count for GPU %s: %v. Using default of 0.", physicalGPUName, err)
				smCount = 0 // Default to a reasonable value for modern GPUs
			}

			// Safely check if the GPU is already provisioned
			// if vGPU, exists := rm.provisionedGPUs[uuid]; exists && vGPU != nil {
			// }

			vGPU := &VirtualGPU{
				ID:                  fmt.Sprintf("vgpu-physical-%s", uuid),
				DeviceIndex:         i,
				MemoryBytes:         memoo.Total,
				MultiProcessorCount: int(smCount),
				Mig:                 nil,
				IsProvisioned:       true,
				Name:                physicalGPUName,
				Physical:            true,
				SMPercentage:        100,
				PhysicalGPUType:     physicalGPUName,
				ProvisionedGPU: &GPU{
					UUID:                uuid,
					Name:                physicalGPUName,
					MemoryBytes:         memoo.Total,
					MultiProcessorCount: int(smCount),
					ParentDeviceIndex:   i,
					ParentUUID:          uuid,
					mpsServer: MPSServer{
						UUID:      uuid,
						Name:      physicalGPUName,
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

		// if currentMode != nvml.DEVICE_MIG_ENABLE {
		// 	log.Printf("Warning: MIG not enabled for GPU %d, skipping", i)
		// 	continue
		// }

		migCount, ret := device.GetMaxMigDeviceCount()
		if ret != nvml.SUCCESS {
			log.Printf("Warning: Could not get MIG device count for GPU %d: %v", i, ret)
			continue
		}

		klog.Infof("MIG device count for GPU %d: %d", i, migCount)

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
			var profile *nvml.GpuInstanceProfileInfo

			profiles, err := device.GetPossiblGPUInstanceeProfiles()
			if err != nil {
				log.Printf("Warning: Could not get GPU instance profiles for MIG device %d, index %d: %v", i, j, err)
				continue
			}

			for _, p := range profiles {
				if p.Id == info.ProfileId {
					profile = p
					break
				}

			}

			if profile == nil {
				log.Printf("Warning: Could not find GPU instance profile for MIG device %d, index %d", i, j)
				continue
			}

			computeInstance, ret := gpuInstance.GetComputeInstanceById(computeInstanceID)
			if ret != nvml.SUCCESS {
				log.Printf("Warning: Could not get compute instance for MIG device %d, index %d: %v", i, j, ret)
				continue
			}

			old, ok := rm.provisionedGPUs[migDeviceUUID]
			//todo check if need to set values as they are already there
			vgpu := &VirtualGPU{
				Name:                migDeviceName,
				ID:                  fmt.Sprintf("vgpu-mig-%s", migDeviceUUID),
				MultiProcessorCount: int(atters.MultiprocessorCount),
				MemoryBytes:         migDeviceMemory.Total,
				Mig: &MIGProperties{
					GPUInstance:     &gpuInstance,
					profile:         *profile,
					ComputeInstance: &computeInstance,
				},
				IsProvisioned:   true,
				DeviceIndex:     i,
				PhysicalGPUType: physicalGPUName,
				SMPercentage:    int(atters.ComputeInstanceSliceCount) * 100 / int(migCount),
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

			if !ok {
				rm.provisionedGPUs[migDeviceUUID] = vgpu
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
			klog.Info("profile id: ", profile.Id)
			klog.Info("profile: ", profile)
			count, ret := device.GetGpuInstanceRemainingCapacity(profile)
			if ret != nvml.SUCCESS {
				log.Printf("Error getting GPU instance remaining capacity for profile %d on device %d: %v",
					profile.Id, i, ret.String())
				continue
			}

			for j := 0; j < count; j++ {
				vGPU := &VirtualGPU{
					PhysicalGPUType:     physicalGPUName,
					SMPercentage:        int(profile.InstanceCount) * 100 / int(migCount),
					Name:                fmt.Sprintf("%s-mig-profile-%d", physicalGPUName, profile.Id),
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

func (rm *ResourceManager) FindVirtualGPU(profileID uint32) (*VirtualGPU, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, vGPU := range rm.VirtualGPUs {
		// Skip if already provisioned
		if vGPU.IsProvisioned {
			continue
		}
		// Check if this virtual GPU meets requirements
		if vGPU.Mig != nil && vGPU.Mig.profile.Id == profileID {
			return vGPU, nil
		}
	}

	return nil, fmt.Errorf("virtual GPU with profile id %d not found", profileID)
}

func (rm *ResourceManager) FindVirtualGPUUsingUUID(uuid string) (*VirtualGPU, error) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	// Check if this virtual GPU meets requirements
	vgpu, ok := rm.provisionedGPUs[uuid]
	if ok {
		return vgpu, nil
	}

	return nil, fmt.Errorf("virtual GPU with uuid %s not found", uuid)
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
	v.Mig.GPUInstance = &gpuInstance

	giMig := rm.mig.GpuInstance(gpuInstance)

	// Select profile that uses all SMs in the GPU instance
	ciProfile, err := giMig.GetFullUsageComputeInstanceProfile(&v.Mig.profile)

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

	v.Mig.ComputeInstance = &computeInstance
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
func (rm *ResourceManager) Release(v *VirtualGPU) error {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	if !v.IsProvisioned || v.Physical {
		return nil // Nothing to do
	}

	if v.ProvisionedGPU == nil {
		klog.Info("provisioned gpu is nil for vgpu: ", v.ID)
		return nil
	}
	// if mps remove

	err := v.ProvisionedGPU.mpsServer.StopMPSDaemon()
	if err != nil {
		log.Printf("Warning: Failed to stop MPS daemon: %v", err)
	}

	// Destroy compute instance first
	if v.Mig.ComputeInstance != nil {
		if ret := (*v.Mig.ComputeInstance).Destroy(); ret != nvml.SUCCESS {
			return fmt.Errorf("failed to destroy compute instance: %v", ret)
		}
		v.Mig.ComputeInstance = nil

	} else {
		klog.Info("compute instance is nil for vgpu: ", v.ID)
	}

	// Then destroy GPU instance
	if v.Mig.GPUInstance != nil {
		if ret := (*v.Mig.GPUInstance).Destroy(); ret != nvml.SUCCESS {
			return fmt.Errorf("failed to destroy GPU instance: %v", ret)
		}
		v.Mig.GPUInstance = nil
	} else {
		klog.Info("gpu instance is nil for vgpu: ", v.ID)
	}
	rm.provisionedGPUs[v.ProvisionedGPU.UUID] = nil

	v.IsProvisioned = false
	v.ProvisionedGPU = nil

	return nil
}
