/*
Copyright 2024 FaST-GShare Authors, KontonGu (Jianfeng Gu), et. al.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fastpodcontrollermanager

import (
	"container/list"
	"fmt"
	"math"

	"github.com/KontonGu/FaST-GShare/pkg/types"
	"github.com/KontonGu/FaST-GShare/proto/seti/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

// FindBestNode finds the best node for scheduling a FaSTPod
// doesn't create resources

type FastPodRequirements struct {
	QuotaReq    float64
	QuotaLimit  float64
	SMPartition int //0-100
}

type ResourceRequest struct {
	podKey         string
	AllocationType types.AllocationType

	RequestedNode    *string
	RequestGPUUUID   *string
	RequestedGPUType *string
	//For mig
	SMRequest    *int
	SMPercentage *int //0-100
	Memory       int64

	//for mps
	FastPodRequirements *FastPodRequirements
}

// Generalized canFit function
func canFit(req *ResourceRequest, info *GPUDevInfo) bool {
	switch req.AllocationType {
	case types.AllocationTypeExclusive:
		return canFitExclusive(req, info)
	case types.AllocationTypeMPS:
		return canFitMPSPod(req, info)
	case types.AllocationTypeFastPod:
		return req.FastPodRequirements != nil && canFitFastPod(req, info)
	default:
		return false
	}
}

var tFlopsMap = map[string]float64{
	"NVIDIA A10G": 100,
	"NVIDIA H100": 815,
}

// TransformedSM stub: for now, just return the requested SM percentage
func TransformedSM(req *ResourceRequest, vgpu *seti.VirtualGPU) (int, error) {
	tflopSource, ok1 := tFlopsMap[vgpu.ProvisionedGpu.Name]
	tflopTarget, ok2 := tFlopsMap[*req.RequestedGPUType]

	if !ok1 || !ok2 {
		return 0, fmt.Errorf("no tflop info for the gpu %s", vgpu.ProvisionedGpu.Name)
	}

	return int(math.Ceil(float64(*req.SMPercentage) * tflopSource / tflopTarget)), nil
}

type SelectionResult struct {
	Node    *Node
	VGPU    *seti.VirtualGPU
	FinalSM int
}

func (ctr *Controller) FindBestNode(req *ResourceRequest) (*SelectionResult, error) {
	nodeList, err := ctr.nodesLister.List(labels.Set{"gpu": "present"}.AsSelector())
	if err != nil {
		errInfo := fmt.Errorf("error Cannot find gpu node with the lable \"gpu:present\"")
		utilruntime.HandleError(errInfo)
		return nil, errInfo
	}

	klog.Infof("Nodelist count is: %d", len(nodeList))

	var bestNode *Node
	var bestVGPU *seti.VirtualGPU
	bestScore := 1e9 // Initialize to a large number
	var finalSM float64

	for _, n := range nodeList {
		node, ok := nodes[n.Name]
		if !ok {
			continue
		}
		allVGPU := node.vgpus
		usageMap := node.vGPUID2GPU

		for _, vgpu := range allVGPU {
			var devInfo *GPUDevInfo
			var memBytes int64
			var uuid string

			if vgpu.IsProvisioned && vgpu.ProvisionedGpu != nil {
				uuid = vgpu.ProvisionedGpu.Uuid
				memBytes = int64(vgpu.ProvisionedGpu.MemoryBytes)
				devInfo, ok = usageMap[uuid]
				if !ok {
					devInfo = &GPUDevInfo{
						smCount:        int(vgpu.ProvisionedGpu.MultiprocessorCount),
						SMPercentage:   int(vgpu.SmPercentage),
						UUID:           uuid,
						Mem:            memBytes,
						Usage:          0,
						UsageMem:       0,
						FastPodList:    list.New(),
						MPSPodList:     list.New(),
						allocationType: types.AllocationTypeNone,
					}
				}
			} else {
				memBytes = int64(vgpu.MemoryBytes)
				uuid = vgpu.Id
				devInfo = &GPUDevInfo{
					smCount:        int(vgpu.MultiprocessorCount),
					SMPercentage:   int(vgpu.SmPercentage),
					UUID:           uuid,
					Mem:            memBytes,
					Usage:          0,
					UsageMem:       0,
					FastPodList:    list.New(),
					MPSPodList:     list.New(),
					allocationType: types.AllocationTypeNone,
				}
			}

			if memBytes == 0 {
				continue
			}

			// Step 10: if not CanFit(G, R) then continue
			if !canFit(req, devInfo) {
				continue
			}

			// Step 13: if R.gpu type != G.gpu type, adjust SMs
			var adjSM int
			var smRatio float64
			if req.AllocationType == types.AllocationTypeMPS {
				adjSM = 0
				smRatio = 0.0
			} else {
				if req.RequestedGPUType != nil && vgpu.ProvisionedGpu != nil && *req.RequestedGPUType != vgpu.ProvisionedGpu.Name {
					adjSM, err = TransformedSM(req, vgpu)
					if err != nil {
						klog.Errorf("error TransformedSM: %s", err)
						continue
					}
				} else if req.SMPercentage != nil {
					adjSM = *req.SMPercentage
				} else {
					adjSM = 0
				}
				if devInfo.smCount > 0 {
					smRatio = float64(adjSM) / float64(devInfo.smCount)
				} else {
					smRatio = 0.0
				}
			}

			// Step 19: mem ratio = R.mem req / G.total memory
			var memRatio float64
			if devInfo.Mem > 0 {
				memRatio = float64(req.Memory) / float64(devInfo.Mem)
			} else {
				memRatio = 0.0
			}

			// Affinity priority
			affinity_priority := 0.0
			gpuSet, ok := fastPodToPhysicalGPUs[req.podKey]
			if ok && gpuSet[vgpu.ProvisionedGpu.ParentUuid] {
				affinity_priority = 10.0
			}

			// Mode priority
			mode_priority := 0.0
			if req.AllocationType == devInfo.allocationType && req.AllocationType != types.AllocationTypeExclusive {
				mode_priority = 3.0
			}

			// GPU priority
			gpu_priority := 0.0
			if req.RequestedGPUType != nil && vgpu.ProvisionedGpu != nil && *req.RequestedGPUType == vgpu.ProvisionedGpu.Name {
				gpu_priority = 1.0
			}

			// Step 22-27: scoring
			var score float64
			if req.AllocationType == types.AllocationTypeMPS || smRatio <= memRatio {
				// Mem-heavy: balance SMs (or always for MPS)
				score = (devInfo.Usage - float64(adjSM)) / 100
			} else {
				// SM-heavy: balance memory
				score = (float64((devInfo.Mem - devInfo.UsageMem) - req.Memory)) / float64(devInfo.Mem)
			}
			score = score - affinity_priority - mode_priority - gpu_priority

			if score < bestScore {
				bestScore = score
				bestNode = node
				bestVGPU = vgpu
				finalSM = float64(adjSM)
			}
		}
	}

	if bestNode != nil && bestVGPU != nil {
		// Allocation logic would go here (not implemented)
		klog.Infof("Selected GPU: %s on node %s with score %f and finalSM %f", bestVGPU.Id, bestNode.hostName, bestScore, finalSM)
		return &SelectionResult{Node: bestNode, VGPU: bestVGPU, FinalSM: int(finalSM)}, nil
	}
	return nil, fmt.Errorf("no suitable candidates found")
}

// canFitExclusive returns true if this GPU is completely free,
// has at least the requested memory, and (if specified) has enough SMs.
func canFitExclusive(req *ResourceRequest, info *GPUDevInfo) bool {

	klog.Infof("request smpercentage: %d", *req.SMPercentage)
	klog.Infof("info smpercentage: %d", info.SMPercentage)
	if req.AllocationType != types.AllocationTypeExclusive {
		return false
	}
	// Exclusive must start on a fresh GPU
	if info.allocationType != types.AllocationTypeNone {
		return false
	}
	// Memory requirement
	if info.Mem < req.Memory {
		return false
	}
	// SM requirement (if any) — SMRequest is an absolute count
	if req.SMRequest != nil && info.smCount < *req.SMRequest {
		return false
	}

	// SM percentage requirement (if any) — SMPercentage is a percentage
	if req.SMPercentage != nil && info.SMPercentage < *req.SMPercentage {
		return false
	}

	klog.Infof("GPU %s is free and has enough memory and SMs", info.UUID)

	return true
}

// canFitMPSPod returns true if this GPU is either already MPS
// or unused, and has enough free memory.
func canFitMPSPod(req *ResourceRequest, info *GPUDevInfo) bool {
	if req.AllocationType != types.AllocationTypeMPS {
		klog.Info("AllocationType is not MPS")
		return false
	}
	allocOK := info.allocationType == types.AllocationTypeMPS ||
		info.allocationType == types.AllocationTypeNone
	if !allocOK {
		klog.Infof("GPU AllocationType is not MPS or None")
		return false
	}
	return (info.Mem - info.UsageMem) >= req.Memory
}

// canFitFastPod returns true if this GPU is either already a FastPod host
// or unused, has enough free memory, and enough SM headroom
// (QuotaReq is fractional, e.g. 0.2 → 20% of SMs).
func canFitFastPod(req *ResourceRequest, info *GPUDevInfo) bool {
	if req.AllocationType != types.AllocationTypeFastPod {
		return false
	}

	if req.FastPodRequirements == nil {
		return false
	}

	allocOK := info.allocationType == types.AllocationTypeFastPod ||
		info.allocationType == types.AllocationTypeNone
	if !allocOK {
		return false
	}
	if (info.Mem - info.UsageMem) < req.Memory {
		return false
	}

	// quota := req.FastPodRequirements.QuotaReq
	sm_partition := req.FastPodRequirements.SMPartition

	return info.Usage+float64(sm_partition)/100.0 <= 1.0
}
