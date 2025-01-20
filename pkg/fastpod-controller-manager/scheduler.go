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
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

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

	for _, node := range nodeList {

		prefereredGPU := fastpod.ObjectMeta.Annotations[fastpodv1.FaSTGSharePrefferedGPUType]

		schedNode = node.Name
		klog.Infof("current node name: %s.", schedNode)
		//curently tmmporary use
		klog.Infof("Prefered GPU: %s", prefereredGPU)
		nodesInfoMtx.Lock()
		defer nodesInfoMtx.Unlock()
		node := nodesInfo[schedNode]
		for key, g := range node.vGPUID2GPU {
			log.Printf("List GPU: %v", g)
			vgpuID = key
			if prefereredGPU != "" && strings.Contains(g.GPUType, prefereredGPU) {
				log.Printf("Selecting Preferred GPU %s", prefereredGPU)
				break
			}
		}

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
