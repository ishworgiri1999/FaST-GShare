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
	SMRequest *int
	Memory    int64

	//for mps
	FastPodRequirements *FastPodRequirements
}

func (ctr *Controller) FindBestNode(fastpod *fastpodv1.FaSTPod, req *ResourceRequest) (*Node, *seti.VirtualGPU, error) {

	return nil, nil, nil
}

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
