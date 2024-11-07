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
	"bufio"
	"bytes"
	"container/list"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KontonGu/FaST-GShare/pkg/libs/bitmap"
	"github.com/KontonGu/FaST-GShare/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	fastpodv1 "github.com/KontonGu/FaST-GShare/pkg/apis/fastgshare.caps.in.tum/v1"
)

type NodeStatus string

const (
	NodeReady    NodeStatus = "Ready"
	NodeNotReady NodeStatus = "NotReady"
)

type NodeLiveness struct {
	ConfigConn    net.Conn
	Status        NodeStatus
	LastHeartbeat time.Time
}

var (
	nodesLiveness    map[string]*NodeLiveness
	nodesLivenessMtx sync.Mutex
	configNetAddr    string = "0.0.0.0:10086"
	checkTickerItv   int    = 240
	kubeClient       kubernetes.Interface
)

func init() {
	nodesLiveness = make(map[string]*NodeLiveness)
}

// listen and initialize the gpu information received from each node and periodically check liveness of node;
// send pods' resource configuration to the node's configurator
func (ctr *Controller) startConfigManager(stopCh <-chan struct{}, kube_client kubernetes.Interface) error {
	klog.Infof("Starting the configuration manager of the controller manager .... ")
	// listenr of the socket connection from the configurator of each node
	connListen, err := net.Listen("tcp", configNetAddr)
	if err != nil {
		klog.Errorf("Error while listening to the tcp socket %s from configurator, %s", configNetAddr, err.Error())
		return err
	}
	defer connListen.Close()

	// check liveness of the gpu work node periodically
	checkTicker := time.NewTicker(time.Second * time.Duration(checkTickerItv))
	go ctr.checkNodeLiveness(checkTicker.C)

	kubeClient = kube_client
	klog.Infof("Listening to the nodes' connection .... ")
	// accept each node connection
	for {
		conn, err := connListen.Accept()
		if err != nil {
			klog.Errorf("Error while accepting the tcp socket: %s", err.Error())
			continue
		}
		klog.Infof("Received the connection from a node with IP:Port = %s", conn.RemoteAddr().String())
		go ctr.initNodeInfo(conn)
	}

}

// check liveness of the gpu work nodes every `tick` seconds
func (ctr *Controller) checkNodeLiveness(tick <-chan time.Time) {
	for {
		<-tick
		nodesLivenessMtx.Lock()
		curTime := time.Now()
		// traverse all nodes for status and liveness check
		for _, val := range nodesLiveness {
			itv := curTime.Sub(val.LastHeartbeat).Seconds()
			if itv > float64(checkTickerItv) {
				val.Status = NodeNotReady
			}
		}
		nodesLivenessMtx.Unlock()
	}
}

// intialize GPU device information in nodesInfo and send resource configuration of podList within a GPU
// device to nodes' configurator
func (ctr *Controller) initNodeInfo(conn net.Conn) {
	nodeIP := strings.Split(conn.RemoteAddr().String(), ":")[0]
	reader := bufio.NewReader(conn)

	var err error
	// get hostname of the node, hostname here is the daemonset's pod name.
	res, err := reader.ReadBytes('\n')
	if err != nil {
		klog.Errorf("Error while reading hostname from node information.")
		klog.Errorf("Error: %v", err)
		return
	}
	klog.Infof("%v", res)
	var helloMessage types.ConfiguratorNodeHelloMessage
	err = types.DecodeFromByte(res, &helloMessage)
	if err != nil {
		klog.Errorf("Error while parsing hostname from node information.")
		klog.Errorf("Error: %v", err)
		return
	}
	klog.Infof("Received hostname from node information: %s", helloMessage.Hostname)
	daemonPodName := helloMessage.Hostname
	daemonPod, err := kubeClient.CoreV1().Pods("kube-system").Get(context.TODO(), daemonPodName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error cannot find the node daemonset.")
		return
	}
	nodeName := daemonPod.Spec.NodeName

	// nodeName := helloMessage.Hostname

	var gpuInfoMsg types.GPURegisterMessage

	// get gpu device information
	d, err := reader.ReadBytes('\n')
	if err != nil {
		klog.Error("Error while reading gpu device information.")
		return
	}
	err = types.DecodeFromByte(d, &gpuInfoMsg)
	if err != nil {
		klog.Error("Error while parsing gpu device information.")
		return
	}
	klog.Infof("Received gpu device information")
	for _, gpu := range gpuInfoMsg.GPU {
		klog.Infof("GPU: %v", gpu)
	}

	devsNum := len(gpuInfoMsg.GPU)
	klog.Infof("GPU Device number is %d.", devsNum)
	// scheduler port for each node starts from 52001
	schedPort := GPUSchedPortStart
	uuid2port := make(map[string]string, devsNum)
	uuid2mem := make(map[string]uint64, devsNum)
	uuid2type := make(map[string]string, devsNum)
	var uuidList []string
	for i := 0; i < devsNum; i++ {

		// infoSplit := strings.Split(devsInfo[i], ":")
		// uuidType, mem := infoSplit[0], infoSplit[1]
		// uuidTypeSplit := strings.Split(uuidType, "_")
		// uuid, devType := uuidTypeSplit[0], uuidTypeSplit[1]
		uuid := gpuInfoMsg.GPU[i].UUID
		uuid2port[uuid] = strconv.Itoa(schedPort)
		schedPort += 1
		uuid2mem[uuid] = gpuInfoMsg.GPU[i].Memory
		uuid2type[uuid] = gpuInfoMsg.GPU[i].TypeName
		uuidList = append(uuidList, uuid)
		klog.Infof("Device Info: uuid = %s, type = %s, mem = %d.", uuid, uuid2type[uuid], uuid2mem[uuid])
	}

	// update nodesInfo and create dummyPod
	nodesInfoMtx.Lock()

	if nodesInfo[nodeName] != nil {
		klog.Info(nodesInfo[nodeName].vGPUID2GPU)
	}

	if node, has := nodesInfo[nodeName]; !has {
		pBm := bitmap.NewBitmap(PortRange)
		pBm.Set(0)
		node = &NodeStatusInfo{
			vGPUID2GPU:      make(map[string]*GPUDevInfo),
			UUID2SchedPort:  uuid2port,
			UUID2GPUType:    uuid2type,
			DaemonIP:        nodeIP,
			DaemonPortAlloc: pBm,
		}
		for _, uuid := range uuidList {
			vgpuID := fastpodv1.GenerateGPUID(8)
			mem := uuid2mem[uuid]
			node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
				GPUType: uuid2type[uuid],
				UUID:    uuid,
				//TODO: change Mem to also uint64
				Mem:      int64(mem),
				Usage:    0.0,
				UsageMem: 0,
				PodList:  list.New(),
			}
			nodesInfo[nodeName] = node
			go ctr.createDummyPod(nodeName, vgpuID, uuid2type[uuid], uuid)
		}
	} else {
		// For the case of controller-manager crashed and recovery;
		node.UUID2SchedPort = uuid2port
		node.DaemonIP = nodeIP
		node.UUID2GPUType = uuid2type
		usedUuid := make(map[string]string)
		for key, gpuinfo := range nodesInfo[nodeName].vGPUID2GPU {
			usedUuid[gpuinfo.UUID] = key
		}
		for _, uuid := range uuidList {
			if key, hastmp := usedUuid[uuid]; !hastmp {
				vgpuID := fastpodv1.GenerateGPUID(8)
				mem, _ := uuid2mem[uuid]
				node.vGPUID2GPU[vgpuID] = &GPUDevInfo{
					GPUType: uuid2type[uuid],
					UUID:    uuid,
					//TODO: change Mem to also uint64
					Mem:      int64(mem),
					Usage:    0.0,
					UsageMem: 0,
					PodList:  list.New(),
				}
				go ctr.createDummyPod(nodeName, vgpuID, uuid2type[uuid], uuid)
			} else {
				//update memory
				node.vGPUID2GPU[key].Mem = int64(uuid2mem[uuid])

			}
		}
	}
	nodesInfoMtx.Unlock()

	// Initialize node liveness
	nodesLivenessMtx.Lock()
	if _, has := nodesLiveness[nodeName]; !has {
		nodesLiveness[nodeName] = &NodeLiveness{
			ConfigConn:    conn,
			LastHeartbeat: time.Time{},
			Status:        NodeReady,
		}
	} else {
		nodesLiveness[nodeName].ConfigConn = conn
		nodesLiveness[nodeName].LastHeartbeat = time.Time{}
		nodesLiveness[nodeName].Status = NodeReady
	}
	nodesLivenessMtx.Unlock()

	uuidTomMemStringified := make(map[string]string)
	for uuid, mem := range uuid2mem {
		uuidTomMemStringified[uuid] = strconv.FormatUint(mem, 10)
	}
	// update node annotation to include gpu device information
	ctr.updateNodeGPUAnnotation(nodeName, &uuid2port, &uuidTomMemStringified, &uuid2type)
	klog.Infof("updateNodeGPUAnnotation finished. nodeName=%s", nodeName)

	// update pods' gpu resource configuration to configurator
	nodesInfoMtx.Lock()
	for vgpuID, gpuDevInfo := range nodesInfo[nodeName].vGPUID2GPU {
		klog.Infof("updatePodsGPUConfig started. vgpu_id = %s.", vgpuID)
		ctr.updatePodsGPUConfig(nodeName, gpuDevInfo.UUID, gpuDevInfo.PodList)
	}
	nodesInfoMtx.Unlock()

	// check the hearbeats and update the ready status of the node
	for {
		heartbeatMsg, err := reader.ReadBytes('\n')
		klog.Infof(("heartbeatMsg: %v"), heartbeatMsg)
		hasError := false

		if err != nil {
			klog.Errorf("Error while reading heartbeat message from node %s.", nodeName)
			klog.Errorf("Error: %v", err)
			hasError = true
		}
		var heartBeat types.ConfiguratorHeartbeatMessage
		err = types.DecodeFromByte(heartbeatMsg, &heartBeat)

		if err != nil {
			klog.Errorf("Error while decoding heartbeat message from node %s.", nodeName)
			klog.Errorf("Error decoding: %v", err)
			hasError = true

		}

		if !heartBeat.Alive {
			klog.Errorf("Node %s is not alive.", nodeName)
			hasError = true
		}

		nodesLivenessMtx.Lock()
		if hasError {
			nodesLiveness[nodeName].Status = NodeNotReady
			nodesLivenessMtx.Unlock()
			return
		} else {
			nodesLiveness[nodeName].Status = NodeReady
			nodesLiveness[nodeName].LastHeartbeat = time.Now()
			klog.Infof("Received heartbeat from the node %s.", nodeName)
			nodesLivenessMtx.Unlock()
		}
	}
}

// update the node annotation to include gpu uuid, type, mem and scheduler port information
// for each GPU devices in the node
func (ctr *Controller) updateNodeGPUAnnotation(nodeName string, uuid2port, uuid2mem, uuid2type *map[string]string) {
	node, err := kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error when update node annotation of gpu information, nodeName=%s.", nodeName)
		return
	}
	dcNode := node.DeepCopy()
	if dcNode.ObjectMeta.Annotations == nil {
		dcNode.ObjectMeta.Annotations = make(map[string]string)
	}
	var buf bytes.Buffer
	klog.Infof("updateNodeGPUAnnotation start buf write.")
	for key, val := range *uuid2mem {
		buf.WriteString(key + "_" + (*uuid2type)[key])
		buf.WriteString(":")
		buf.WriteString(val + "_" + (*uuid2port)[key])
		buf.WriteString(",")
	}
	klog.Infof("updateNodeGPUAnnotation end buf write.")
	devsInfoMsg := buf.String()
	dcNode.ObjectMeta.Annotations[fastpodv1.FaSTGShareGPUsINfo] = devsInfoMsg
	klog.Infof("Update the node = %s 's gpu information annoation to be: %s.", nodeName, devsInfoMsg)
	_, err = kubeClient.CoreV1().Nodes().Update(context.TODO(), dcNode, metav1.UpdateOptions{})
	if err != nil {
		klog.Errorf("Error while Updating the node = %s 's gpu information annoation to be: %s.", nodeName, devsInfoMsg)
		return
	}

}

// update pods' gpu resource configuration to configurator
func (ctr *Controller) updatePodsGPUConfig(nodeName, uuid string, podlist *list.List) error {
	nodesLivenessMtx.Lock()
	nodeLive, has := nodesLiveness[nodeName]
	nodesLivenessMtx.Unlock()
	if !has {
		errMsg := fmt.Sprintf("The node = %s is not initialized.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	conn := nodeLive.ConfigConn
	if nodeLive.Status != NodeReady {
		errMsg := fmt.Sprintf("The node = %s is not ready.", nodeName)
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	// Write and Sned podlist's GPU resource configuration to the configurator

	podGPUConfig := types.UpdatePodsGPUConfigMessage{
		GpuUUID:        uuid,
		PodGPURequests: make([]types.PodGPURequest, podlist.Len()),
	}

	var buf bytes.Buffer
	buf.WriteString(uuid)
	buf.WriteString(":")
	// write resource configuration
	if podlist != nil {
		for pod := podlist.Front(); pod != nil; pod = pod.Next() {
			podRequest := pod.Value.(*PodReq)
			podGPUConfig.PodGPURequests = append(podGPUConfig.PodGPURequests, types.PodGPURequest{
				Key:           podRequest.Key,
				QtRequest:     podRequest.QtRequest,
				QtLimit:       podRequest.QtLimit,
				SMPartition:   podRequest.SMPartition,
				Memory:        podRequest.Memory,
				GPUClientPort: podRequest.GPUClientPort,
			})
			//TODO: remove string message
			buf.WriteString(pod.Value.(*PodReq).Key) // pod's namespace + name
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%f", pod.Value.(*PodReq).QtRequest))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%f", pod.Value.(*PodReq).QtLimit))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%d", pod.Value.(*PodReq).SMPartition))
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%d", pod.Value.(*PodReq).Memory))
			buf.WriteString(",")

		}
	}
	buf.WriteString(":")
	// write gpu clients' ports
	if podlist != nil {
		for pod := podlist.Front(); pod != nil; pod = pod.Next() {
			buf.WriteString(pod.Value.(*PodReq).Key) // pod's namespace + name
			buf.WriteString(" ")
			buf.WriteString(fmt.Sprintf("%d", pod.Value.(*PodReq).GPUClientPort))
			buf.WriteString(",")
		}
	}
	buf.WriteString("\n")

	getByte, err := types.EncodeToByte(podGPUConfig)
	if err != nil {
		klog.Errorf("Error failed to encode UpdatePodsGPUConfigMessage: %v", err)
		return err
	}

	klog.Infof("Update the gpu device = %s in the node = %s resource configuration with: %s", uuid, nodeName, buf.String())
	if _, err := conn.Write(getByte); err != nil {
		errMsg := fmt.Sprintf("Failed to write msg to node = %s with the GPU = %s reosurce configuration of pods, write msg = %s.", nodeName, uuid, buf.String())
		klog.Errorf(errMsg)
		return fmt.Errorf(errMsg)
	}
	return nil
}
