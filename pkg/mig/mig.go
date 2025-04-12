/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mig

import (
	"fmt"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Interface struct {
	nvml nvml.Interface
}

type Device struct {
	nvml.Device
}

type GpuInstance struct {
	nvml.GpuInstance
}
type ComputeInstance struct {
	nvml.ComputeInstance
}

func New() Interface {
	return Interface{nvml.New()}
}

func NewMock(nvml nvml.Interface) Interface {
	return Interface{nvml}
}

func (i Interface) Device(d nvml.Device) Device {
	return Device{d}
}

func (i Interface) GpuInstance(gi nvml.GpuInstance) GpuInstance {
	return GpuInstance{gi}
}

func (i Interface) ComputeInstance(ci nvml.ComputeInstance) ComputeInstance {
	return ComputeInstance{ci}
}

func (device Device) AssertMigEnabled() error {
	mode, _, ret := device.GetMigMode()
	if ret == nvml.ERROR_NOT_SUPPORTED {
		return fmt.Errorf("MIG not supported")
	}
	if ret != nvml.SUCCESS {
		return fmt.Errorf("error getting MIG mode: %v", ret)
	}
	if mode != nvml.DEVICE_MIG_ENABLE {
		return fmt.Errorf("MIG mode disabled")
	}
	return nil
}

func (device Device) WalkGpuInstances(f func(nvml.GpuInstance, int, nvml.GpuInstanceProfileInfo) error) error {
	for i := 0; i < nvml.GPU_INSTANCE_PROFILE_COUNT; i++ {
		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(i)
		if ret == nvml.ERROR_NOT_SUPPORTED {
			continue
		}
		if ret == nvml.ERROR_INVALID_ARGUMENT {
			continue
		}
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instance profile info for '%v': %v", i, ret)
		}

		gis, ret := device.GetGpuInstances(&giProfileInfo)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting GPU instances for profile '%v': %v", i, ret)
		}

		for _, gi := range gis {
			err := f(gi, i, giProfileInfo)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (device Device) GetPossiblGPUInstanceeProfiles() ([]*nvml.GpuInstanceProfileInfo, error) {
	var profiles []*nvml.GpuInstanceProfileInfo
	for i := 0; i < nvml.GPU_INSTANCE_PROFILE_COUNT; i++ {
		giProfileInfo, ret := device.GetGpuInstanceProfileInfo(i)

		if ret == nvml.ERROR_NOT_SUPPORTED {
			continue
		}
		if ret == nvml.ERROR_INVALID_ARGUMENT {
			continue
		}
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting GPU instance profile info for '%v': %v", i, ret)
		}

		profiles = append(profiles, &giProfileInfo)

		// count, ret := device.GetGpuInstanceRemainingCapacity(&giProfileInfo)
		// if ret != nvml.SUCCESS {
		// 	return nil, fmt.Errorf("error getting GPU instance remaining capacity for profile '%v': %v", i, ret)
		// }

	}
	return profiles, nil
}

func (gi GpuInstance) GetPossibleComputeInstanceProfiles(profile *nvml.GpuInstanceProfileInfo) ([]*nvml.ComputeInstanceProfileInfo, error) {

	var profiles []*nvml.ComputeInstanceProfileInfo
	for j := 0; j < nvml.COMPUTE_INSTANCE_PROFILE_COUNT; j++ {
		for k := 0; k < nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_COUNT; k++ {
			ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(j, k)
			if ret == nvml.ERROR_NOT_SUPPORTED {
				continue
			}
			if ret == nvml.ERROR_INVALID_ARGUMENT {
				continue
			}
			if ret != nvml.SUCCESS {
				return nil, fmt.Errorf("error getting Compute instance profile info for '(%v, %v)': %v", j, k, ret)
			}

			profiles = append(profiles, &ciProfileInfo)
		}

	}
	return profiles, nil

}

func (gi GpuInstance) GetFullUsageComputeInstanceProfile(profile *nvml.GpuInstanceProfileInfo) (*nvml.ComputeInstanceProfileInfo, error) {

	profiles, err := gi.GetPossibleComputeInstanceProfiles(profile)

	if err != nil {
		return nil, err
	}

	for _, prof := range profiles {
		// full profile
		if prof.InstanceCount == 1 {
			return prof, nil

		}
	}
	return nil, nil
}

func (gi GpuInstance) WalkComputeInstances(f func(ci nvml.ComputeInstance, ciProfileId int, ciEngProfileId int, ciProfileInfo nvml.ComputeInstanceProfileInfo) error) error {
	for j := 0; j < nvml.COMPUTE_INSTANCE_PROFILE_COUNT; j++ {
		for k := 0; k < nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_COUNT; k++ {
			ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(j, k)
			if ret == nvml.ERROR_NOT_SUPPORTED {
				continue
			}
			if ret == nvml.ERROR_INVALID_ARGUMENT {
				continue
			}
			if ret != nvml.SUCCESS {
				return fmt.Errorf("error getting Compute instance profile info for '(%v, %v)': %v", j, k, ret)
			}

			cis, ret := gi.GetComputeInstances(&ciProfileInfo)
			if ret != nvml.SUCCESS {
				return fmt.Errorf("error getting Compute instances for profile '(%v, %v)': %v", j, k, ret)
			}

			for _, ci := range cis {
				err := f(ci, j, k, ciProfileInfo)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}
