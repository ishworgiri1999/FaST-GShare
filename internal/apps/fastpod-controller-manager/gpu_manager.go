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
	"context"
	"fmt"
	"strconv"
	"sync"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
	"github.com/KontonGu/FaST-GShare/pkg/grpcclient"

	"github.com/KontonGu/FaST-GShare/pkg/libs/bitmap"
	"github.com/KontonGu/FaST-GShare/pkg/proto/seti/v1"
	"github.com/KontonGu/FaST-GShare/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/klog/v2"
)

type FastPodReq struct {
	Key           string
	QtRequest     float64
	QtLimit       float64
	SMPartition   int //0-100
	Memory        int64
	GPUClientPort int
}

type MPSPodReq struct {
	Key    string
	Memory int64
}

type ExclusivePodReq struct {
	Key string
}

type GPUDevInfo struct {
	virtual        bool //can this be deleted
	smCount        int  // number of SMs not to be confused with SMPartition
	allocationType types.AllocationType
	GPUType        string // GPU type, eg. V100-PCIE-16GB
	UUID           string
	Mem            int64
	Name           string // could be different than GPUType or same
	ParentUUID     string // physical gpu uuid (different for mig gpu, same for physical gpu)
	SMPercentage   int    // 0-100 // 100 for physical GPU . for mig gpu, it is the percentage of SMs.

	// Usage of GPU resource, SM * QtRequest only for FastPod
	Usage float64
	// Usage of GPU Memory
	UsageMem     int64
	FastPodList  *list.List //FastPodReq
	MPSPodList   *list.List //MPSPodReq
	ExclusivePod *ExclusivePodReq
}

type Node struct {
	// The available GPU device number
	GPUNum int32
	// The IP to the node Daemon which contains the the Configurator and FaST-Manager Schedulers.
	DaemonIP string
	// The mapping from Physical GPU device UUID to FaST-Manager's Scheduler port, 1 physical GPU -> 1 FaST-Manager Scheudler
	UUID2SchedPort map[string]string
	// The mapping of GPU UUID to the GPU Type, eg. GPU-4a87a50d-337f-a293-6c9e-xxx -> V100-PCIE-16GB
	UUID2GPUType map[string]string
	// The mapping from vGPU ID (DummyPod GPU) to physical GPU device, eg. dummyPod GPU ->  physical GPU UUID (1-to-1 mapping)
	vGPUID2GPU map[string]*GPUDevInfo
	// The port allocator for FaST-Manager gpu clients and the configurator
	DaemonPortAlloc *bitmap.Bitmap

	vgpus      []*seti.VirtualGPU
	grpcClient *grpcclient.GrpcClient
	hostName   string
}

var (
	// record all fastpods and gpu information (allocation/available)
	nodes        map[string]*Node = make(map[string]*Node)
	nodesInfoMtx sync.Mutex
	// mapping from fastpod name to its corresponding pod list;
	fstp2Pods    map[string]*list.List = make(map[string]*list.List)
	fstp2PodsMtx sync.Mutex

	fastPodToPhysicalGPUs map[string](map[string]bool) = make(map[string](map[string]bool))
)

var Quantity1 = resource.MustParse("1")

const PortRange int = 1024

func (ctr *Controller) deleteDummyPod(nodeName, uuid, vGPUID string) {
	dummypodName := fmt.Sprintf("%s-%s-%s", fastpodv1.FaSTGShareDummyPodName, nodeName, vGPUID)
	klog.Infof("To delete the dummyPod=%s.", dummypodName)
	ctr.kubeClient.CoreV1().Pods("kube-system").Delete(context.TODO(), dummypodName, metav1.DeleteOptions{})
	ctr.updatePodsGPUConfig(nodeName, uuid, nil)
}

type Allocation struct {
	UUID string
	node *Node

	MPSConfig *MPSConfig
}

func (a *Allocation) Undo() {
	//todo:
	//try undo allocation
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()
	_, ok := a.node.vGPUID2GPU[a.UUID]
	if !ok {
		klog.Errorf("Error: The gpuInfo is not found, uuid = %s", a.UUID)
		return
	}

}

func (ctr *Controller) RequestGPUAndUpdateConfig(selectionResult *SelectionResult, request *ResourceRequest, podKey string) (*Allocation, int) {
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	node, ok := nodes[selectionResult.NodeName]
	if !ok {
		msg := fmt.Sprintf("Error The node = %s is not initialized", selectionResult.NodeName)
		klog.Errorf(msg)
		return nil, 2
	}

	var mpsConfig *MPSConfig
	var fastPodMPSConfig *FastPodMPSConfig

	gpuInfo, ok := node.vGPUID2GPU[selectionResult.VGPUUUID]

	if !ok {
		klog.Errorf("Error: The gpuInfo is not found, uuid = %s", selectionResult.VGPUUUID)
		return nil, 3
	}

	if gpuInfo.allocationType == types.AllocationTypeNone {
		//set gpu type
		gpuInfo.allocationType = request.AllocationType
	}

	if gpuInfo.allocationType == types.AllocationTypeMPS || gpuInfo.allocationType == types.AllocationTypeFastPod {
		//try to enable mps
		res, err := node.grpcClient.EnableMPS(context.TODO(), gpuInfo.UUID)
		if err != nil {
			klog.Errorf("Error when enabling MPS for vGPU %s, err = %s", gpuInfo.UUID, err)
		}
		if res.Success {
			klog.Infof("Successfully enabled MPS for vGPU %s,.", gpuInfo.UUID)
		}

		mpsConfig = &MPSConfig{
			LogDirectory:  res.MpsConfig.LogPath,
			PipeDirectory: res.MpsConfig.TmpPath,
		}
	}

	port := 0

	if request.AllocationType == types.AllocationTypeMPS {

		if _, isFound := FindInQueue(podKey, gpuInfo.MPSPodList); !isFound {
			newMemoryUsage := gpuInfo.UsageMem + request.Memory
			if newMemoryUsage > gpuInfo.Mem {
				klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough memory to pod %s, MemUsed=%d, MemReq=%d, MemTotal=%d.", gpuInfo.UUID, selectionResult.VGPUUUID, podKey, gpuInfo.UsageMem, request.Memory, gpuInfo.Mem)
				return nil, 4
			}

			gpuInfo.UsageMem = newMemoryUsage

			gpuInfo.MPSPodList.PushBack(&MPSPodReq{
				Key:    podKey,
				Memory: request.Memory,
			})

		}

		return &Allocation{
			node:      node,
			UUID:      gpuInfo.UUID,
			MPSConfig: mpsConfig,
		}, 0
	} else if request.AllocationType == types.AllocationTypeExclusive {
		gpuInfo.ExclusivePod = &ExclusivePodReq{
			Key: podKey,
		}

		return &Allocation{
			node: node,
			UUID: gpuInfo.UUID,
		}, 0
	}

	//handle fastpod case
	if podreq, isFound := FindInQueue(podKey, gpuInfo.FastPodList); !isFound {

		requestedSM := float64(request.FastPodRequirements.SMPartition) / 100.0
		requestedQuota := request.FastPodRequirements.QuotaReq

		newSMUsage := gpuInfo.Usage + requestedQuota*requestedSM

		//Without TIME Quota
		//newSMUsage := gpuInfo.Usage + (float64(selectionResult.FinalSM) / 100.0)

		if newSMUsage > 1.0 {
			klog.Infof("Resource exceed! The gpu = %s with 	can not allocate enough compute resource to pod %s, GPUAllocated=%f, GPUReq=%f.", gpuInfo.UUID, podKey, gpuInfo.Usage, requestedQuota*requestedSM)
			for k := gpuInfo.FastPodList.Front(); k != nil; k = k.Next() {
				klog.Infof("Pod = %s, Usage=%f, MemUsage=%d", k.Value.(*FastPodReq).Key, k.Value.(*FastPodReq).QtRequest*(float64(k.Value.(*FastPodReq).SMPartition)/100.0), k.Value.(*FastPodReq).Memory)
			}
			return nil, 3
		}
		newMemoryUsage := gpuInfo.UsageMem + request.Memory
		if newMemoryUsage > gpuInfo.Mem {
			klog.Infof("Resource exceed! The gpu = %s with can not allocate enough memory to pod %s, MemUsed=%d, MemReq=%d, MemTotal=%d.", gpuInfo.UUID, podKey, gpuInfo.UsageMem, request.Memory, gpuInfo.Mem)
			return nil, 4
		}

		newPort := node.DaemonPortAlloc.FindFirstUnsetAndSet()
		if newPort != -1 {
			port = newPort + GPUClientPortStart
		} else {
			klog.Errorf("Error the ports for gpu clients are full. node=%s.", selectionResult.NodeName)
			return nil, 5
		}

		gpuInfo.Usage = newSMUsage
		gpuInfo.UsageMem = newMemoryUsage

		gpuInfo.FastPodList.PushBack(&FastPodReq{
			Key:           podKey,
			QtRequest:     request.FastPodRequirements.QuotaReq,
			QtLimit:       request.FastPodRequirements.QuotaLimit,
			SMPartition:   request.FastPodRequirements.SMPartition,
			Memory:        request.Memory,
			GPUClientPort: port,
		})

	} else {
		port = podreq.GPUClientPort
	}
	if request.AllocationType == types.AllocationTypeFastPod {
		ctr.updatePodsGPUConfig(selectionResult.NodeName, gpuInfo.UUID, gpuInfo.FastPodList)
	}

	fastPodMPSConfig = &FastPodMPSConfig{
		SchedulerIP:            node.DaemonIP,
		GpuClientPort:          port,
		ActiveThreadPercentage: request.FastPodRequirements.SMPartition,
	}

	mpsConfig.FastPodMPSConfig = fastPodMPSConfig
	return &Allocation{
		node:      node,
		UUID:      gpuInfo.UUID,
		MPSConfig: mpsConfig,
	}, 0

}

// remove pod information in the nodesInfo and update the pods configuration file with the function updatePodsGPUConfig
func (ctr *Controller) removePodFromList(fastpod *fastpodv1.FaSTPod, pod *corev1.Pod) {
	nodeName := pod.Spec.NodeName
	vGPUID := pod.Annotations[fastpodv1.FaSTGShareVGPUID]
	allocationType := types.GetAllocationType(pod.Annotations[fastpodv1.FastGshareAllocationType])
	key := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	if node, has := nodes[nodeName]; has {
		if gpuInfo, ghas := node.vGPUID2GPU[vGPUID]; ghas {
			var podlist *list.List
			if allocationType == types.AllocationTypeFastPod {

				podlist = gpuInfo.FastPodList
				for pod := podlist.Front(); pod != nil; pod = pod.Next() {
					podreq := pod.Value.(*FastPodReq)
					if podreq.Key == key {
						podlist.Remove(pod)
						klog.Infof("[fastpod]Removing Pod=%s from the fastpod=%s, The vGPU=%s still has pods number=%d.", key, fastpod.Name, vGPUID, podlist.Len())
						uuid := gpuInfo.UUID

						gpuInfo.UsageMem -= podreq.Memory
						gpuInfo.Usage -= podreq.QtRequest * (float64(podreq.SMPartition) / 100.0)
						ctr.updatePodsGPUConfig(nodeName, uuid, podlist)

						node.DaemonPortAlloc.Clear(podreq.GPUClientPort - GPUClientPortStart)
						break

					}
				}
			}
			if allocationType == types.AllocationTypeMPS {
				podlist = gpuInfo.MPSPodList
				for pod := podlist.Front(); pod != nil; pod = pod.Next() {
					podreq := pod.Value.(*MPSPodReq)
					if podreq.Key == key {
						podlist.Remove(pod)
						klog.Infof("[mps]Removing Pod=%s from the fastpod=%s, The vGPU=%s still has pods number=%d.", key, fastpod.Name, vGPUID, podlist.Len())

						gpuInfo.UsageMem -= podreq.Memory

						break
					}
				}
			}
			if allocationType == types.AllocationTypeExclusive {
				gpuInfo.ExclusivePod = nil
				gpuInfo.Usage = 0
				gpuInfo.UsageMem = 0
				gpuInfo.Mem = 0
				klog.Infof("[exclusive]Removing Pod=%s from the fastpod=%s, The vGPU=%s still has pods number=%d.", key, fastpod.Name, vGPUID, 0)

			}
			klog.Infof("All pods removed from fastpod=%s, vGPU=%s.", fastpod.Name, vGPUID)

			if (podlist.Len() == 0 && allocationType != types.AllocationTypeExclusive) || allocationType == types.AllocationTypeExclusive {

				//mps
				if allocationType == types.AllocationTypeFastPod || allocationType == types.AllocationTypeMPS {

					//try to disable mps
					resp, err := node.grpcClient.DisableMPS(context.TODO(), gpuInfo.UUID)
					if err != nil {
						klog.Errorf("Error when disabling MPS for vGPU %s, err = %s", gpuInfo.UUID, err)
					}
					if resp.Success {
						klog.Infof("Successfully disabled MPS for vGPU %s,.", gpuInfo.UUID)

					}
				}
				podKey := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
				// destroy the gpu if possible

				if gpuInfo.virtual {
					//removal of gpu
					resp, err := node.grpcClient.ReleaseVirtualGPU(context.TODO(), &seti.ReleaseVirtualGPURequest{
						Uuid: gpuInfo.UUID,
					})

					if err != nil {
						klog.Errorf("Error when releasing the vGPU %s, err = %s", gpuInfo.UUID, err)
					}

					if len(resp.AvailableVirtualGpus) > 0 {
						klog.Infof("Release vGPU %s successfully.", gpuInfo.UUID)
						node.vgpus = resp.AvailableVirtualGpus

					}
				}

				delete(fastPodToPhysicalGPUs[podKey], gpuInfo.ParentUUID)
				delete(node.vGPUID2GPU, vGPUID)

			}

		}
	}

	klog.Infof("Pod %s removed from fastpod %s.", pod.Name, fastpod.Name)
}

// Remove the FaSTPod instance, update fastpod podlist in nodesInfo and
// delete the pods of the fastpod.
func (ctr *Controller) removeFaSTPodFromList(fastpod *fastpodv1.FaSTPod) {
	selector, err := metav1.LabelSelectorAsSelector(fastpod.Spec.Selector)
	if err != nil {
		utilruntime.HandleError(err)
	}
	namespace := fastpod.Namespace
	pods, err := ctr.podsLister.Pods(namespace).List(selector)
	if err != nil {
		utilruntime.HandleError(err)
	}

	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	for _, pod := range pods {
		klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", pod.Name, fastpod.Name)
		nodeName := pod.Spec.NodeName
		vgpuID := pod.Annotations[fastpodv1.FaSTGShareVGPUID]
		key := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		allocationType := types.GetAllocationType(pod.Annotations[fastpodv1.FastGshareAllocationType])

		if node, nodeOk := nodes[nodeName]; nodeOk {
			if gpuInfo, gpuOk := node.vGPUID2GPU[vgpuID]; gpuOk {
				var podlist *list.List

				//check allocation type
				if allocationType == types.AllocationTypeFastPod {
					podlist = gpuInfo.FastPodList
					listLen := podlist.Len()
					klog.Infof("The list length of the podlist = %s of the FaSTPod = %s is %d", key, fastpod.Name, listLen)
					// delete pod information in the nodesInfo
					for podreq := podlist.Front(); podreq != nil; podreq = podreq.Next() {
						podreqValue := podreq.Value.(*FastPodReq)
						klog.Infof("The podreqValue.Key = %s, key = %s", podreqValue.Key, key)

						if podreqValue.Key == key {
							klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
							podlist.Remove(podreq)
							uuid := gpuInfo.UUID

							// gpu.Usage -= podreqValue.QtRequest * (float64(podreqValue.SMPartition) / 100.0)
							gpuInfo.Usage -= (float64(podreqValue.SMPartition) / 100.0)

							gpuInfo.UsageMem -= podreqValue.Memory
							ctr.updatePodsGPUConfig(nodeName, uuid, podlist)
							node.DaemonPortAlloc.Clear(podreqValue.GPUClientPort - GPUClientPortStart)

						}
						break
					}

				}

				if allocationType == types.AllocationTypeMPS {
					podlist = gpuInfo.MPSPodList

					// delete pod information in the nodesInfo
					for podreq := podlist.Front(); podreq != nil; podreq = podreq.Next() {
						podreqValue := podreq.Value.(*MPSPodReq)
						if podreqValue.Key == key {
							klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
							podlist.Remove(podreq)
							gpuInfo.UsageMem -= podreqValue.Memory
						}
					}
				}

				if allocationType == types.AllocationTypeExclusive {
					gpuInfo.ExclusivePod = nil
					gpuInfo.Usage = 0
					gpuInfo.UsageMem = 0
					gpuInfo.Mem = 0
					klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
				}

				if (podlist != nil && podlist.Len() == 0 && allocationType != types.AllocationTypeExclusive) || allocationType == types.AllocationTypeExclusive {
					// destroy the gpu if possible
					//disable mps

					if allocationType == types.AllocationTypeFastPod || allocationType == types.AllocationTypeMPS {

						//try to disable mps
						resp, err := node.grpcClient.DisableMPS(context.TODO(), gpuInfo.UUID)
						if err != nil {
							klog.Errorf("Error when disabling MPS for vGPU %s, err = %s", gpuInfo.UUID, err)
						} else if resp.Success {
							klog.Infof("Successfully disabled MPS for vGPU %s,.", gpuInfo.UUID)

						}
					}

					if gpuInfo.virtual {
						//remove fron node list, let release be handled by fastfunc
						delete(node.vGPUID2GPU, vgpuID)

					}

					delete(node.vGPUID2GPU, vgpuID)
				}

				// delete the pod in the kube system
				err := ctr.kubeClient.CoreV1().Pods(namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					klog.Errorf("Error when Removing the pod = %s of the FaSTPod = %s", key, fastpod.Name)
					klog.Errorf("Error = %s", err)
				} else {
					klog.Infof("Finish removing the pod = %s of the FaSTPod = %s (kube delete).", key, fastpod.Name)
				}
			}
		} else {
			klog.Errorf("Node %s not found for pod when removing the FaSTPod = %s", nodeName, fastpod.Name)
		}
	}

}

// check the validity of resource configuration values
func (ctr *Controller) resourceValidityCheck(pod *corev1.Pod) bool {
	var tmp_err error
	quota_limit, tmp_err := strconv.ParseFloat(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit], 64)
	if tmp_err != nil || quota_limit > 1.0 || quota_limit < 0.0 {
		return false
	}
	quota_req, tmp_err := strconv.ParseFloat(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest], 64)
	if tmp_err != nil || quota_limit > 1.0 || quota_limit < 0.0 || quota_limit < quota_req {
		return false
	}

	sm_partition, tmp_err := strconv.ParseInt(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUSMPartition], 10, 64)
	if tmp_err != nil || sm_partition < 0 || sm_partition > 100 {
		sm_partition = int64(100)
		return false
	}

	gpu_mem, tmp_err := strconv.ParseInt(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory], 10, 64)
	if tmp_err != nil || gpu_mem < 0 {
		return false
	}

	return true
}
