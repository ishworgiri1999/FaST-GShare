package fastconfigurator

import (
	"log"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type ComputeCapability struct {
	Major int
	Minor int
}

type GPU struct {
	Name              string
	UUID              string
	ComputeCapability ComputeCapability
	Memory            uint64
	SM                int
	MIGEnabled        bool
	Parent            *GPU
}

func (g *GPU) isMigInstance() bool {
	return g.Parent != nil
}

func (g *GPU) isUsable() bool {
	return !g.MIGEnabled
}
func getGPUs() []*GPU {

	var gpus []*GPU

	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		log.Fatalf("Unable to initialize NVML: %v", nvml.ErrorString(ret))
	}
	defer func() {
		ret := nvml.Shutdown()
		if ret != nvml.SUCCESS {
			log.Fatalf("Unable to shutdown NVML: %v", nvml.ErrorString(ret))
		}
	}()

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		log.Fatalf("Unable to get device count: %v", nvml.ErrorString(ret))
	}

	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)

		if ret != nvml.SUCCESS {
			log.Fatalf("Unable to get device at index %d: %v", i, nvml.ErrorString(ret))
		}

		name, ret := device.GetName()
		if ret != nvml.SUCCESS {
			log.Fatalf("Unable to get name of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		memory, ret := device.GetMemoryInfo()
		if ret != nvml.SUCCESS {
			log.Fatalf("Unable to get memory info of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		// multiProcessorCount := device.GetMultiProcessorCount

		// log.Printf("memory", memory)

		uuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			log.Fatalf("Unable to get uuid of device at index %d: %v", i, nvml.ErrorString(ret))
		}
		gpu := &GPU{
			Name:   name,
			UUID:   uuid,
			Memory: memory.Total,
		}

		gpus = append(gpus, gpu)
		// Check if MIG mode is enabled
		currentMode, _, ret := device.GetMigMode()

		if ret == nvml.ERROR_NOT_SUPPORTED {
			continue
		}

		if ret != nvml.SUCCESS {
			log.Printf("Unable to get MIG mode for device %s: %v", name, nvml.ErrorString(ret))
			continue
		}

		// If MIG is enabled, get MIG devices
		if currentMode == 1 {
			gpu.MIGEnabled = true
			// Get MIG device count
			migCount, ret := device.GetMaxMigDeviceCount()
			if ret != nvml.SUCCESS {
				log.Printf("Unable to get MIG device count for device %s: %v", name, nvml.ErrorString(ret))
				continue
			}

			for j := 0; j < migCount; j++ {
				migDevice, ret := device.GetMigDeviceHandleByIndex(j)
				if ret == nvml.SUCCESS {
					// Get MIG device properties
					migName, ret := migDevice.GetName()
					if ret != nvml.SUCCESS {
						log.Printf("Unable to get name of MIG device at index %d: %v", j, nvml.ErrorString(ret))
						continue
					}
					uuid, ret := migDevice.GetUUID()

					if ret != nvml.SUCCESS {
						log.Printf("Unable to get uuid of MIG device at index %d: %v", j, nvml.ErrorString(ret))
						continue
					}

					// Get GPU Instance ID
					// giid, ret := migDevice.GetGpuInstanceId()

					// Get Compute Instance ID
					// ciid, ret := migDevice.GetComputeInstanceId()

					major, minor, ret := device.GetCudaComputeCapability()
					if ret != nvml.SUCCESS {
						log.Fatalf("Unable to get cuda compute capability of device at index %d: %v", i, nvml.ErrorString(ret))
					}

					// Get MIG device memory info
					memory, ret := migDevice.GetMemoryInfo()

					if ret != nvml.SUCCESS {
						log.Printf("Unable to get memory info of MIG device at index %d: %v", j, nvml.ErrorString(ret))
						continue
					}

					migGpu := &GPU{
						UUID:       uuid,
						Name:       migName,
						Memory:     memory.Total,
						MIGEnabled: true,
						Parent:     gpu,
					}
					migGpu.ComputeCapability = ComputeCapability{
						Major: major,
						Minor: minor,
					}

					gpus = append(gpus, migGpu)

				}
			}
		}

		// Print original device information
		major, minor, ret := device.GetCudaComputeCapability()
		if ret != nvml.SUCCESS {

			log.Fatalf("Unable to get cuda compute capability of device at index %d: %v", i, nvml.ErrorString(ret))
		}

		gpu.ComputeCapability = ComputeCapability{
			Major: major,
			Minor: minor,
		}

	}

	return gpus
}

// func main() {
// 	gpus := getGPUs()

// 	for _, gpu := range gpus {
// 		if gpu.isMigInstance() {
// 			fmt.Printf("MIG Device: %s, Parent: %s\n", gpu.Name, gpu.Parent.Name)
// 			fmt.Printf("UUID: %s\n", gpu.UUID)
// 		} else {
// 			fmt.Printf("Device: %s\n", gpu.Name)
// 			fmt.Printf("UUID: %s\n", gpu.UUID)

// 		}
// 	}
// }
