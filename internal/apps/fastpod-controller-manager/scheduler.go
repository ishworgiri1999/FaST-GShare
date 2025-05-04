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
	"log"
	"strings"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
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

type ScoredGPU struct {
	VGPU  *seti.VirtualGPU
	Node  *Node
	Score float64
}

func (ctr *Controller) FindBestNode(fastpod *fastpodv1.FaSTPod, req *ResourceRequest) (*Node, *seti.VirtualGPU, error) {
	nodeList, err := ctr.nodesLister.List(labels.Set{"gpu": "present"}.AsSelector())
	if err != nil {
		errInfo := fmt.Errorf("error Cannot find gpu node with the lable \"gpu:present\"")
		utilruntime.HandleError(errInfo)
		return nil, nil, errInfo
	}

	klog.Infof("Nodelist count is: %d", len(nodeList))

	var candidates []ScoredGPU

	for _, n := range nodeList {
		node, ok := nodes[n.Name]
		if !ok {
			continue
		}
		allVGPU := node.vgpus

		klog.Infof("scheduler: node %s has %d GPUs", n.Name, len(allVGPU))
		usageMap := node.vGPUID2GPU

		for _, vgpu := range allVGPU {
			var devInfo *GPUDevInfo
			var memBytes int64
			var uuid string

			// If provisioned, use usage data
			if vgpu.IsProvisioned && vgpu.ProvisionedGpu != nil {
				uuid = vgpu.ProvisionedGpu.Uuid
				memBytes = int64(vgpu.ProvisionedGpu.MemoryBytes)
				devInfo, ok = usageMap[uuid]
				if !ok {
					devInfo = &GPUDevInfo{
						smCount:        int(vgpu.ProvisionedGpu.MultiprocessorCount),
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
				// Unprovisioned GPU — treat as empty GPU
				memBytes = int64(vgpu.MemoryBytes)
				uuid = vgpu.Id // fallback ID

				//fake device info, not to be inserted into the map
				devInfo = &GPUDevInfo{
					smCount:        int(vgpu.MultiprocessorCount),
					UUID:           uuid,
					Mem:            memBytes,
					Usage:          0,
					UsageMem:       0,
					FastPodList:    list.New(),
					MPSPodList:     list.New(),
					allocationType: types.AllocationTypeNone,
				}
			}

			// If we couldn't get memory info, skip this GPU
			if memBytes == 0 {
				continue
			}

			// Apply optional filters (e.g., RequestedGPUType, RequestedGPUUUID)
			// if req.RequestedGPUType != nil &&
			// 	vgpu.ProvisionedGpu != nil &&
			// 	*req.RequestedGPUType != vgpu.ProvisionedGpu.Name {
			// 	continue
			// }

			match := false
			var score float64

			switch req.AllocationType {
			case types.AllocationTypeExclusive:
				if canFitExclusive(req, devInfo) {
					match = true
					score = scoreExclusive(req, devInfo)
				}

			case types.AllocationTypeMPS:
				if canFitMPSPod(req, devInfo) {
					match = true
					score = scoreMPSPod(req, devInfo)
				}

			case types.AllocationTypeFastPod:
				if req.FastPodRequirements != nil &&
					canFitFastPod(req, devInfo) {
					match = true
					score = scoreFastPod(req, devInfo)
				}
			}

			if match {
				candidates = append(candidates, ScoredGPU{VGPU: vgpu, Node: node, Score: score})
			}
		}
	}

	return ctr.selectBestCandidate(candidates)
}

func (ctr *Controller) selectBestCandidate(candidates []ScoredGPU) (*Node, *seti.VirtualGPU, error) {

	if len(candidates) == 0 {
		return nil, nil, fmt.Errorf("no suitable candidates found")
	}
	bestCandidate := candidates[0]
	for _, candidate := range candidates {
		if candidate.Score > bestCandidate.Score {
			bestCandidate = candidate
		}
	}
	return bestCandidate.Node, bestCandidate.VGPU, nil
}

// canFitExclusive returns true if this GPU is completely free,
// has at least the requested memory, and (if specified) has enough SMs.
func canFitExclusive(req *ResourceRequest, info *GPUDevInfo) bool {
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
	allocOK := info.allocationType == types.AllocationTypeFastPod ||
		info.allocationType == types.AllocationTypeNone
	if !allocOK {
		return false
	}
	if (info.Mem - info.UsageMem) < req.Memory {
		return false
	}
	quota := req.FastPodRequirements.QuotaReq
	return (1.0 - info.Usage) >= (quota / 100)
}

// scoreExclusive prefers GPUs whose SM‐count is just large enough
// and whose total RAM is minimal (to reduce waste).
func scoreExclusive(req *ResourceRequest, info *GPUDevInfo) float64 {

	if req.SMPercentage != nil {

		percentageDiff := float64(info.SMPercentage - *req.SMPercentage)
		// smaller difference → higher score
		percentageScore := -percentageDiff
		// smaller total RAM → higher score
		memScore := -float64(info.Mem)
		// weight SM more heavily
		return percentageScore*1e6 + memScore

	}

	// Distance between GPU.SMCount and requested SM
	smDiff := float64(info.smCount - *req.SMRequest)
	// smaller difference → higher score
	smScore := -smDiff
	// smaller total RAM → higher score
	memScore := -float64(info.Mem)
	// weight SM more heavily
	return smScore*1e6 + memScore
}

// scoreMPSPod gives a bonus for reuse, otherwise ranks
// unprovisioned GPUs by “SMs per byte of RAM” (higher is better).
func scoreMPSPod(req *ResourceRequest, info *GPUDevInfo) float64 {
	if info.allocationType == types.AllocationTypeMPS {
		// reuse bonus + pack more memory‐utilization
		return 2.0 + float64(info.UsageMem)/float64(info.Mem)
	}
	// unprovisioned: prefer GPUs with many SMs and small RAM
	return float64(info.smCount) / float64(info.Mem)
}

// scoreFastPod gives a big bonus for FastPod reuse and then
// favors high SM and mem utilization.
func scoreFastPod(req *ResourceRequest, info *GPUDevInfo) float64 {
	reuseBonus := 0.0
	if info.allocationType == types.AllocationTypeFastPod {
		reuseBonus = 3.0
	}
	memUtil := float64(info.UsageMem) / float64(info.Mem)
	smUtil := info.Usage
	return reuseBonus + memUtil + smUtil
}

// deprecated: use FindBestNode instead
func (ctr *Controller) schedule(fastpod *fastpodv1.FaSTPod, quotaReq float64, quotaLimit float64, smPartition int64, gpuMem int64, isValid bool, key string) (string, string) {
	nodeList, err := ctr.nodesLister.List(labels.Set{"gpu": "present"}.AsSelector())
	if err != nil {
		errInfo := fmt.Errorf("Error Cannot find gpu node with the lable \"gpu:present\"")
		utilruntime.HandleError(errInfo)
	}
	// fastpod.Annotations
	// schedNode := "kgpu1"
	schedNode := ""
	var vgpuID string
	prefereredGPU := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGSharePrefferedGPUType]
	foundPreferred := false
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	for _, node := range nodeList {
		schedNode = node.Name
		klog.Infof("current node name: %s.", schedNode)
		//curently tmmporary use
		klog.Infof("Prefered GPU: %s", prefereredGPU)

		node := nodes[schedNode]

		log.Printf("node %s has %d GPUs", schedNode, len(node.vGPUID2GPU))
		for key, g := range node.vGPUID2GPU {
			log.Printf("List GPU: %v", g)
			if prefereredGPU != "" && strings.Contains(g.GPUType, prefereredGPU) {
				vgpuID = key
				foundPreferred = true
				log.Printf("Selecting Preferred GPU %s", prefereredGPU)
				break
			} else {
				vgpuID = key
			}
		}
		if foundPreferred {
			break
		}

	}

	if prefereredGPU != "" && !foundPreferred {
		log.Printf("Preferred GPU %s not found, cant schedule", prefereredGPU)
		vgpuID = ""
	}

	log.Printf("Selected GPU id: %s", vgpuID)

	if schedNode == "" {
		klog.Infof("No enough resources for Pod of a FaSTPod=%s/%s", fastpod.ObjectMeta.Namespace, fastpod.ObjectMeta.Name)
		ctr.pendingListMux.Lock()
		ctr.pendingList.PushBack(key)
		ctr.pendingListMux.Unlock()
		return "", ""
	}
	return schedNode, vgpuID
}
