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
	"github.com/KontonGu/FaST-GShare/pkg/libs/bitmap"
	"github.com/KontonGu/FaST-GShare/pkg/types"
	"github.com/KontonGu/FaST-GShare/proto/seti/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	allocationType types.AllocationType
	GPUType        string
	UUID           string
	Mem            int64
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
	grpcClient *GrpcClient
	hostName   string
}

var (
	gpusUseCases map[string]string //mps, exclusive, fastpod
	// record all fastpods and gpu information (allocation/available)
	nodes        map[string]*Node = make(map[string]*Node)
	nodesInfoMtx sync.Mutex
	// mapping from fastpod name to its corresponding pod list;
	fstp2Pods    map[string]*list.List = make(map[string]*list.List)
	fstp2PodsMtx sync.Mutex
)

var Quantity1 = resource.MustParse("1")

const PortRange int = 1024

func (ctr *Controller) gpuNodeInit() error {

	return nil
	var nodess []*corev1.Node
	var err error

	if nodess, err = ctr.nodesLister.List(labels.Set{"gpu": "present"}.AsSelector()); err != nil {
		tmperr := fmt.Errorf("Error when listing nodes: #{err}")
		klog.Error(tmperr)
		return tmperr
	}
	klog.Infof("gpuNodeInit found %d nodes with gpu", len(nodes))
	if len(nodess) > 0 {
		klog.Infof("First node name:%s", nodess[0].Name)
	}

	dummySelector := labels.Set{fastpodv1.FaSTGShareRole: "dummyPod"}.AsSelector()
	var existedDummyPods []*corev1.Pod
	if existedDummyPods, err = ctr.podsLister.Pods("kube-system").List(dummySelector); err != nil {
		tmperr := fmt.Errorf("Error when listing dummyPods %s", err)
		klog.Error(tmperr)
		return tmperr
	}

	type dummyPodInfo struct {
		nodename string
		vgpuID   string
		gpuUuid  string
		gpuType  string
	}

	var dummyPodInfoList []dummyPodInfo
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	// dummyPod already existed, then the gpu_manager needs to recover its setting/information in nodesInfo
	// The case happens when fast-controller-manager crashed due to some reasons
	for _, dpod := range existedDummyPods {
		vgpuID, _ := dpod.ObjectMeta.Labels[fastpodv1.FaSTGShareVGPUID]
		gpuUuid, _ := dpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareDummyPodUUID]
		gpuType, _ := dpod.ObjectMeta.Annotations[fastpodv1.FaSTGShareVGPUType]
		if node, has := nodes[dpod.Spec.NodeName]; !has {
			pBm := bitmap.NewBitmap(PortRange)
			pBm.Set(0)
			node = &Node{
				vGPUID2GPU:      make(map[string]*GPUDevInfo),
				DaemonPortAlloc: pBm,
			}
			node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
				GPUType:     gpuType,
				UUID:        gpuUuid,
				Mem:         0,
				Usage:       0.0,
				UsageMem:    0,
				FastPodList: list.New(),
			}
			nodes[dpod.Spec.NodeName] = node
		} else {
			// The node already has information in nodesInfo, meaning at least one GPU's dummyPod have been initialized;
			// considering the scenario of multiple gpus in a node, initialize this dummyPod's physical gpu in the nodesInfo.
			node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
				GPUType:     gpuType,
				UUID:        gpuUuid,
				Mem:         0,
				Usage:       0.0,
				UsageMem:    0,
				FastPodList: list.New(),
			}
		}
	}

	// Recover the fastpods' information in nodesInfo if the controller-manager crashed;
	// TODO: Recover fastpods
	var fastpods []*fastpodv1.FaSTPod
	if fastpods, err = ctr.fastpodsLister.List(labels.Everything()); err != nil {
		tmperr := fmt.Errorf("Error when listing fastpods %s", err)
		klog.Error(tmperr)
		return tmperr
	}
	for _, fastpod := range fastpods {

		klog.Infof("Recover the fastpod = %s.", fastpod.ObjectMeta.Name)

		// list the pods of the fastpod
		selector, err := metav1.LabelSelectorAsSelector(fastpod.Spec.Selector)
		if err != nil {
			klog.Errorf("Cannot get the selector of the FaSTPod: %s.", fastpod.ObjectMeta.Name)
			return err
		}
		var pods []*corev1.Pod
		if pods, err = ctr.podsLister.Pods(fastpod.Namespace).List(selector); err != nil {
			klog.Errorf("Cannot list the pod of the FaSTPod: %s.", fastpod.ObjectMeta.Name)
			return err
		}

		for _, pod := range pods {

			klog.Infof("Recover the pod = %s.", pod.ObjectMeta.Name)
			quota_req := 0.0
			quota_limit := 0.0
			sm_partition := int64(100)
			gpu_mem := int64(0)
			vgpu_id := ""
			// check the validity of resource configuration values
			var tmp_err error
			quota_limit, tmp_err = strconv.ParseFloat(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaLimit], 64)
			if tmp_err != nil || quota_limit > 1.0 || quota_limit < 0.0 {
				klog.Errorf("Error the quota limit is invalid, pod = %s.", pod.ObjectMeta.Name)
				continue
			}
			quota_req, tmp_err = strconv.ParseFloat(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUQuotaRequest], 64)
			if tmp_err != nil || quota_limit > 1.0 || quota_limit < 0.0 || quota_limit < quota_req {
				klog.Errorf("Error the quota request is invalid, pod = %s.", pod.ObjectMeta.Name)
				continue
			}

			sm_partition, tmp_err = strconv.ParseInt(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUSMPartition], 10, 64)
			if tmp_err != nil || sm_partition < 0 || sm_partition > 100 {
				klog.Errorf("Error the sm partition is invalid, pod = %s.", pod.ObjectMeta.Name)
				sm_partition = int64(100)
				continue
			}

			gpu_mem, tmp_err = strconv.ParseInt(pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUMemory], 10, 64)
			if tmp_err != nil || gpu_mem < 0 {
				klog.Errorf("Error the gpu memory is invalid, pod = %s.", pod.ObjectMeta.Name)
				continue
			}

			node_name := pod.Spec.NodeName
			if node_name == "" {
				klog.Errorf("Error the node name is empty, pod = %s.", pod.ObjectMeta.Name)
				continue
			}

			if vgpu_id_tmp, has := pod.ObjectMeta.Annotations[fastpodv1.FaSTGShareVGPUID]; !has {
				klog.Errorf("Error the vgpu id is not set, pod = %s.", pod.ObjectMeta.Name)
				continue
			} else {
				vgpu_id = vgpu_id_tmp
			}

			// check if node information is initialized or not.
			node, has := nodes[node_name]
			if !has {
				klog.Errorf("Error the node does not have any dummyPod for the fastpod. node = %s, fastpod = %s.", node_name, fastpod.ObjectMeta.Name)
				continue
			}

			// check if the dummyPod for the physical GPU of the fastpod is created or not.
			gpu_info, has := node.vGPUID2GPU[vgpu_id]
			if !has {
				klog.Errorf("Error the dummyPod for the physical GPU is not created, node = %s, fastpod = %s.", node_name, fastpod.ObjectMeta.Name)
				continue
			}
			gpu_info.Usage += quota_req * (float64(sm_partition) / 100.0)
			gpu_info.Mem += gpu_mem
			podPort := (*fastpod.Status.GPUClientPort)[pod.ObjectMeta.Name]
			gpu_info.FastPodList.PushBack(&FastPodReq{
				Key:           fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name),
				QtRequest:     quota_req,
				QtLimit:       quota_limit,
				SMPartition:   int(sm_partition),
				Memory:        gpu_mem,
				GPUClientPort: podPort,
			})
			node.DaemonPortAlloc.Set(podPort - GPUClientPortStart)
		}

	}

	// for _, node := range nodes {
	// 	infoItem := dummyPodInfo{
	// 		nodename: node.Name,
	// 		vgpuID:   fastpodv1.GenerateGPUID(8),
	// 		gpuUuid:  "",
	// 		gpuType:  "",
	// 	}
	// 	dummyPodInfoList = append(dummyPodInfoList, infoItem)
	// 	// if the node already has been initialized, skip it; otherwise intialize the node information in nodesInfo,
	// 	// and create a basic dummyPod for the node;
	// 	if nodeItem, ok := nodesInfo[node.Name]; !ok {
	// 		pBm := bitmap.NewBitmap(PortRange)
	// 		pBm.Set(0)
	// 		nodeItem = &NodeStatusInfo{
	// 			vGPUID2GPU:      make(map[string]*GPUDevInfo),
	// 			DaemonPortAlloc: pBm,
	// 		}
	// 		nodeItem.vGPUID2GPU[infoItem.vgpuID] = &GPUDevInfo{
	// 			GPUType: "",
	// 			UUID:    "",
	// 			Mem:     0,
	// 			Usage:   0.0,
	// 			PodList: list.New(),
	// 		}
	// 		nodesInfo[node.Name] = nodeItem
	// 	}
	// }

	for _, item := range dummyPodInfoList {
		go ctr.createDummyPod(item.nodename, item.vgpuID, item.gpuType, item.gpuUuid)
	}
	return nil
}

func (ctr *Controller) createDummyPod(nodeName, vgpuID, gpuType, gpuUuid string) error {
	dummypodName := fmt.Sprintf("%s-%s-%s", fastpodv1.FaSTGShareDummyPodName, nodeName, vgpuID)

	// function to create the dummy pod
	createFunc := func() error {
		dummypod, err := ctr.kubeClient.CoreV1().Pods("kube-system").Create(context.TODO(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dummypodName,
				Namespace: "kube-system",
				Labels: map[string]string{
					fastpodv1.FaSTGShareRole:     "dummyPod",
					fastpodv1.FaSTGShareNodeName: nodeName,
					fastpodv1.FaSTGShareVGPUID:   vgpuID,
				},
				Annotations: map[string]string{
					fastpodv1.FaSTGShareDummyPodUUID: gpuUuid,
					fastpodv1.FaSTGShareVGPUType:     gpuType,
				},
			},
			Spec: corev1.PodSpec{
				NodeName: nodeName,
				Containers: []corev1.Container{
					corev1.Container{
						Name:  "dummy-gpu-acquire",
						Image: "kontonpuku666/dummycontainer:release",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{fastpodv1.OriginalNvidiaResourceName: Quantity1},
							Limits:   corev1.ResourceList{fastpodv1.OriginalNvidiaResourceName: Quantity1},
						},
					},
				},
				TerminationGracePeriodSeconds: new(int64),
				RestartPolicy:                 corev1.RestartPolicyNever,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			_, ok := ctr.kubeClient.CoreV1().Pods("kube-system").Get(context.TODO(), dummypodName, metav1.GetOptions{})
			if ok != nil {
				klog.Errorf("Create DummyPod failed: dummyPodName = %s\n, podSpec = \n %-v, \n err = '%s'.", dummypodName, dummypod, err)
				return err
			}
		}
		return nil
	}
	klog.Infof("Starting to create DummyPod = %s.", dummypodName)

	if existedpod, err := ctr.kubeClient.CoreV1().Pods("kube-system").Get(context.TODO(), dummypodName, metav1.GetOptions{}); err != nil {
		// The dummypod is not existed currently
		if errors.IsNotFound(err) {
			tocreate := true
			nodesInfoMtx.Lock()
			_, tocreate = nodes[nodeName].vGPUID2GPU[vgpuID]
			nodesInfoMtx.Unlock()
			if tocreate {
				createFunc()
				klog.Infof("The DummyPod = %s is successfully created.", dummypodName)
			}
		} else {
			// other reason except the NotFound
			klog.Errorf("Error when trying to get the DummyPod = %s, not the NotFound Error.\n", dummypodName)
			return err
		}
	} else {
		if existedpod.ObjectMeta.DeletionTimestamp != nil {
			// TODO: If Dummy Pod had been deleted, re-create it later
			klog.Warningf("Unhandled: Dummy Pod %s is deleting! re-create it later!", dummypodName)
		}
		if existedpod.Status.Phase == corev1.PodRunning || existedpod.Status.Phase == corev1.PodFailed {
			// TODO logic if dummy Pod is running or PodFailed
		}
	}
	return nil

}

func (ctr *Controller) deleteDummyPod(nodeName, uuid, vGPUID string) {
	dummypodName := fmt.Sprintf("%s-%s-%s", fastpodv1.FaSTGShareDummyPodName, nodeName, vGPUID)
	klog.Infof("To delete the dummyPod=%s.", dummypodName)
	ctr.kubeClient.CoreV1().Pods("kube-system").Delete(context.TODO(), dummypodName, metav1.DeleteOptions{})
	ctr.updatePodsGPUConfig(nodeName, uuid, nil)
}

/*
The function will retive the current nodes and gpu information and return the uuid of the vGPU which it corresponds to
Meanwhile, the funciton will update tht pod gpu information for nodes configurator via function updatePodsGPUConfig()
if uuid is successfully retrived based on vGPUID;
errCode 0: no error
errCode 1: node with nodeName is not initialized
errCode 2: vGPUID is not initialized or no DummyPod created;
errCode 3: resource exceed;
errCode 4: GPU is out of memory
errCode 5: No enough gpu client ports
*/

// Deprecated: use RequestGPUAndUpdateConfig instead
func (ctr *Controller) getGPUDevUUIDAndUpdateConfig(nodeName, vGPUID string, quotaReq, quotaLimit float64, smPartition, gpuMem int64, key string, port *int) (uuid string, errCode int) {
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	node, ok := nodes[nodeName]
	if !ok {
		msg := fmt.Sprintf("Error The node = %s is not initialized", nodeName)
		klog.Errorf(msg)
		return "", 1
	}
	gpuInfo, ok := node.vGPUID2GPU[vGPUID]
	if !ok {
		msg := fmt.Sprintf("Error The vGPU = %s is not initialized", vGPUID)
		klog.Errorf(msg)
		return "", 2
	}

	if gpuInfo.UUID == "" {
		return "", 2
	}

	if podreq, isFound := FindInQueue(key, gpuInfo.FastPodList); !isFound {
		// newSMUsage := gpuInfo.Usage + quotaReq*(float64(smPartition)/100.0)
		//Without TIME Quota
		newSMUsage := gpuInfo.Usage + (float64(smPartition) / 100.0)

		if newSMUsage > 1.0 {
			klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough compute resource to pod %s, GPUAllocated=%f, GPUReq=%f.", gpuInfo.UUID, vGPUID, key, gpuInfo.Usage, quotaReq*(float64(smPartition)/100.0))
			for k := gpuInfo.FastPodList.Front(); k != nil; k = k.Next() {
				klog.Infof("Pod = %s, Usage=%f, MemUsage=%d", k.Value.(*FastPodReq).Key, k.Value.(*FastPodReq).QtRequest*(float64(k.Value.(*FastPodReq).SMPartition)/100.0), k.Value.(*FastPodReq).Memory)
			}
			return "", 3
		}
		newMemoryUsage := gpuInfo.UsageMem + gpuMem
		if newMemoryUsage > gpuInfo.Mem {
			klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough memory to pod %s, MemUsed=%d, MemReq=%d, MemTotal=%d.", gpuInfo.UUID, vGPUID, key, gpuInfo.UsageMem, gpuMem, gpuInfo.Mem)
			return "", 4
		}

		newPort := node.DaemonPortAlloc.FindFirstUnsetAndSet()
		if newPort != -1 {
			*port = newPort + GPUClientPortStart
		} else {
			klog.Errorf("Error the ports for gpu clients are full. node=%s.", nodeName)
			return "", 5
		}

		gpuInfo.Usage = newSMUsage
		gpuInfo.UsageMem = newMemoryUsage

		gpuInfo.FastPodList.PushBack(&FastPodReq{
			Key:           key,
			QtRequest:     quotaReq,
			QtLimit:       quotaLimit,
			SMPartition:   int(smPartition),
			Memory:        gpuMem,
			GPUClientPort: *port,
		})

	} else {
		*port = podreq.GPUClientPort
	}

	ctr.updatePodsGPUConfig(nodeName, gpuInfo.UUID, gpuInfo.FastPodList)
	return gpuInfo.UUID, 0

}

type Allocation struct {
	UUID string
	node *Node

	MPSConfig *MPSConfig
}

func (ctr *Controller) RequestGPUAndUpdateConfig(nodeName string, gpu *seti.VirtualGPU, request *ResourceRequest, podKey string) (*Allocation, int) {
	nodesInfoMtx.Lock()
	defer nodesInfoMtx.Unlock()

	node, ok := nodes[nodeName]
	if !ok {
		msg := fmt.Sprintf("Error The node = %s is not initialized", nodeName)
		klog.Errorf(msg)
		return nil, 2
	}

	//get gpu
	physicalGPU := gpu.ProvisionedGpu

	var mpsConfig *MPSConfig
	var fastPodMPSConfig *FastPodMPSConfig

	if physicalGPU == nil {
		klog.Info("Error: The physical GPU is nil, CREATING NEW VGPU")
		resp, err := node.grpcClient.RequestVirtualGPU(context.TODO(), &seti.RequestVirtualGPURequest{
			Profileid: gpu.Profileid,
			UseMps:    request.AllocationType == types.AllocationTypeFastPod || request.AllocationType == types.AllocationTypeMPS,
		})

		if err != nil {
			klog.Errorf("Error: The grpc client failed to request vGPU, err = %s", err)
			return nil, 1
		}
		physicalGPU = resp.ProvisionedGpu

	} else if !physicalGPU.MpsEnabled &&
		(request.AllocationType == types.AllocationTypeFastPod ||
			request.AllocationType == types.AllocationTypeMPS) {

		resp, err := node.grpcClient.EnableMPS(context.TODO(), physicalGPU.Uuid)
		if err != nil || !resp.Success {
			klog.Errorf("Error: The grpc client failed to enable mps, err = %s", err)
			return nil, 1
		}
	}

	gpuInfo, ok := node.vGPUID2GPU[physicalGPU.Uuid]

	if !ok {
		node.vGPUID2GPU[physicalGPU.Uuid] = &GPUDevInfo{
			allocationType: request.AllocationType,
			UUID:           physicalGPU.Uuid,
			GPUType:        physicalGPU.Name,
			Mem:            int64(physicalGPU.MemoryBytes),
			Usage:          0.0,
			UsageMem:       0,
			FastPodList:    list.New(),
		}
	}
	//rare case
	if gpuInfo.allocationType != request.AllocationType {
		klog.Errorf("Error: The allocation type is not the same, %s != %s", gpuInfo.allocationType, request.AllocationType)
		return nil, 1
	}
	if physicalGPU.MpsConfig != nil {
		mpsConfig = &MPSConfig{
			LogDirectory:  physicalGPU.MpsConfig.LogPath,
			PipeDirectory: physicalGPU.MpsConfig.TmpPath,
		}
	}

	port := 0

	if request.AllocationType == types.AllocationTypeMPS {

		if _, isFound := FindInQueue(podKey, gpuInfo.MPSPodList); !isFound {
			newMemoryUsage := gpuInfo.UsageMem + request.Memory
			if newMemoryUsage > gpuInfo.Mem {
				klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough memory to pod %s, MemUsed=%d, MemReq=%d, MemTotal=%d.", gpuInfo.UUID, gpu.Id, podKey, gpuInfo.UsageMem, request.Memory, gpuInfo.Mem)
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
	} else if request.AllocationType == types.AllocationTypeMIG {
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

		// newSMUsage := gpuInfo.Usage + quotaReq*(float64(smPartition)/100.0)
		//Without TIME Quota
		newSMUsage := gpuInfo.Usage + (float64(*&request.FastPodRequirements.SMPartition) / 100.0)

		if newSMUsage > 1.0 {
			klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough compute resource to pod %s, GPUAllocated=%f, GPUReq=%f.", gpuInfo.UUID, gpu.Id, podKey, gpuInfo.Usage, (float64(*request.SMRequest) / 100.0))
			for k := gpuInfo.FastPodList.Front(); k != nil; k = k.Next() {
				klog.Infof("Pod = %s, Usage=%f, MemUsage=%d", k.Value.(*FastPodReq).Key, k.Value.(*FastPodReq).QtRequest*(float64(k.Value.(*FastPodReq).SMPartition)/100.0), k.Value.(*FastPodReq).Memory)
			}
			return nil, 3
		}
		newMemoryUsage := gpuInfo.UsageMem + request.Memory
		if newMemoryUsage > gpuInfo.Mem {
			klog.Infof("Resource exceed! The gpu = %s with vgpu = %s can not allocate enough memory to pod %s, MemUsed=%d, MemReq=%d, MemTotal=%d.", gpuInfo.UUID, gpu.Id, podKey, gpuInfo.UsageMem, request.Memory, gpuInfo.Mem)
			return nil, 4
		}

		newPort := node.DaemonPortAlloc.FindFirstUnsetAndSet()
		if newPort != -1 {
			port = newPort + GPUClientPortStart
		} else {
			klog.Errorf("Error the ports for gpu clients are full. node=%s.", nodeName)
			return nil, 5
		}

		gpuInfo.Usage = newSMUsage
		gpuInfo.UsageMem = newMemoryUsage

		gpuInfo.FastPodList.PushBack(&FastPodReq{
			Key:           podreq.Key,
			QtRequest:     request.FastPodRequirements.QuotaLimit,
			QtLimit:       request.FastPodRequirements.QuotaLimit,
			SMPartition:   request.FastPodRequirements.SMPartition,
			Memory:        request.Memory,
			GPUClientPort: port,
		})

	} else {
		port = podreq.GPUClientPort
	}
	if request.AllocationType == types.AllocationTypeFastPod {
		ctr.updatePodsGPUConfig(nodeName, gpuInfo.UUID, gpuInfo.FastPodList)
	}

	fastPodMPSConfig = &FastPodMPSConfig{
		SchedulerIP:            node.DaemonIP,
		GpuClientPort:          port,
		ActiveThreadPercentage: *&request.FastPodRequirements.SMPartition,
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
			if allocationType == types.AllocationTypeMIG {
				gpuInfo.ExclusivePod = nil
				gpuInfo.Usage = 0
				gpuInfo.UsageMem = 0
				gpuInfo.Mem = 0
				klog.Infof("[exclusive]Removing Pod=%s from the fastpod=%s, The vGPU=%s still has pods number=%d.", key, fastpod.Name, vGPUID, 0)
			}

			if podlist.Len() == 0 && allocationType != types.AllocationTypeMIG {
				// destroy the gpu if possible
				klog.Infof("All pods removed from fastpod=%s, vGPU=%s.", fastpod.Name, vGPUID)
				node.grpcClient.ReleaseVirtualGPU(context.TODO(), &seti.ReleaseVirtualGPURequest{
					Uuid: gpuInfo.UUID,
				})
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
		nodeName := pod.Spec.NodeName
		vgpuID := pod.Annotations[fastpodv1.FaSTGShareVGPUID]
		key := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		allocationType := types.GetAllocationType(pod.Annotations[fastpodv1.FastGshareAllocationType])

		if node, nodeOk := nodes[nodeName]; nodeOk {
			if gpu, gpuOk := node.vGPUID2GPU[vgpuID]; gpuOk {
				var podlist *list.List

				//check allocation type
				if allocationType == types.AllocationTypeFastPod {
					podlist = gpu.FastPodList

					// delete pod information in the nodesInfo
					for podreq := podlist.Front(); podreq != nil; podreq = podreq.Next() {
						podreqValue := podreq.Value.(*FastPodReq)
						if podreqValue.Key == key {
							klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
							podlist.Remove(podreq)
							uuid := gpu.UUID

							// gpu.Usage -= podreqValue.QtRequest * (float64(podreqValue.SMPartition) / 100.0)
							gpu.Usage -= (float64(podreqValue.SMPartition) / 100.0)

							gpu.UsageMem -= podreqValue.Memory
							ctr.updatePodsGPUConfig(nodeName, uuid, podlist)
							node.DaemonPortAlloc.Clear(podreqValue.GPUClientPort - GPUClientPortStart)

						}
						break
					}

				}

				if allocationType == types.AllocationTypeMPS {
					podlist = gpu.MPSPodList

					// delete pod information in the nodesInfo
					for podreq := podlist.Front(); podreq != nil; podreq = podreq.Next() {
						podreqValue := podreq.Value.(*MPSPodReq)
						if podreqValue.Key == key {
							klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
							podlist.Remove(podreq)
							gpu.UsageMem -= podreqValue.Memory
						}
					}
				}

				if allocationType == types.AllocationTypeMIG {
					gpu.ExclusivePod = nil
					gpu.Usage = 0
					gpu.UsageMem = 0
					gpu.Mem = 0
					klog.Infof("Removing the pod = %s of the FaSTPod = %s ....", key, fastpod.Name)
				}

				if podlist.Len() == 0 { //check if destroying is possible
					// destroy the gpu if possible
					klog.Infof("All pods removed from fastpod=%s, vGPU=%s.", fastpod.Name, vgpuID)
					node.grpcClient.ReleaseVirtualGPU(context.TODO(), &seti.ReleaseVirtualGPURequest{
						Uuid: gpu.UUID,
					})
				}

				// delete the pod in the kube system
				err := ctr.kubeClient.CoreV1().Pods(namespace).Delete(context.Background(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					klog.Errorf("Error when Removing the pod = %s of the FaSTPod = %s", key, fastpod.Name)
				} else {
					klog.Infof("Finish removing the pod = %s of the FaSTPod = %s (kube delete).", key, fastpod.Name)
				}
			}
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
